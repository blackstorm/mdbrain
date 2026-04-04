package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/config"
	"mdbrain.dev/internal/domain/markdown"
	"mdbrain.dev/internal/domain/model"
	"mdbrain.dev/internal/domain/store"
	mdbmiddleware "mdbrain.dev/internal/http/middleware"
	"mdbrain.dev/internal/infra/repository"
	"mdbrain.dev/internal/infra/storage"
	"mdbrain.dev/internal/templatex"
)

type AppHandler struct {
	cfg      *config.Config
	repo     *repository.Repository
	store    store.ObjectStore
	renderer *templatex.Renderer
	md       *markdown.Renderer
}

func NewAppHandler(cfg *config.Config, repo *repository.Repository, objectStore store.ObjectStore, renderer *templatex.Renderer) *AppHandler {
	handler := &AppHandler{
		cfg:      cfg,
		repo:     repo,
		store:    objectStore,
		renderer: renderer,
	}
	handler.md = markdown.New(func(vaultID, path string) string {
		asset, err := repo.FindAsset(context.Background(), vaultID, path)
		if err == nil && asset != nil {
			if publicURL := objectStore.PublicAssetURL(vaultID, asset.ObjectKey); publicURL != "" {
				return publicURL
			}
			return "/storage/" + asset.ObjectKey
		}
		escaped := url.PathEscape(strings.TrimPrefix(path, "/"))
		escaped = strings.ReplaceAll(escaped, "%2F", "/")
		return "/storage/" + escaped
	})
	return handler
}

func (h *AppHandler) GetNote(c *echo.Context) error {
	path := strings.TrimPrefix(c.Param("*"), "/")
	return h.withVault(c, func(vault *model.Vault) error {
		vaultData := h.publicVault(vault)
		isHTMX := c.Request().Header.Get("HX-Request") != ""
		pathClientIDs := parsePathIDs(path)

		if len(pathClientIDs) == 0 {
			if vault.RootNoteID != nil && *vault.RootNoteID != "" {
				rootNote, err := h.repo.GetNoteByClientID(c.Request().Context(), vault.ID, *vault.RootNoteID)
				if err == nil && rootNote != nil {
					renderData, err := h.prepareNoteData(c, rootNote, vault.ID)
					if err != nil {
						return err
					}
					description := h.md.ExtractDescription(stringValue(rootNote.Content), 160)
					if isHTMX {
						body, err := h.renderer.Render("templates/app/note.html", renderData)
						if err != nil {
							return err
						}
						return c.HTML(http.StatusOK, body)
					}
					body, err := h.renderer.Render("templates/app/note-page.html", map[string]any{
						"notes":       []any{renderData},
						"vault":       vaultData,
						"description": description,
					})
					if err != nil {
						return err
					}
					return c.HTML(http.StatusOK, body)
				}
			}

			notes, err := h.repo.ListNotesByVault(c.Request().Context(), vault.ID)
			if err != nil {
				return err
			}
			body, err := h.renderer.Render("templates/app/home.html", map[string]any{
				"vault": vaultData,
				"notes": noteListData(notes),
			})
			if err != nil {
				return err
			}
			return c.HTML(http.StatusOK, body)
		}

		validNotes := make([]model.Note, 0, len(pathClientIDs))
		validIDs := make([]string, 0, len(pathClientIDs))
		for _, clientID := range pathClientIDs {
			note, err := h.repo.GetNoteByClientID(c.Request().Context(), vault.ID, clientID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				return err
			}
			validNotes = append(validNotes, *note)
			validIDs = append(validIDs, note.ClientID)
		}
		if len(validNotes) == 0 {
			return c.String(http.StatusNotFound, "Note not found")
		}
		needsCorrection := len(validIDs) != len(pathClientIDs)
		correctedPath := "/"
		if len(validIDs) > 0 {
			correctedPath = "/" + strings.Join(validIDs, "+")
		}

		if isHTMX {
			lastNote := validNotes[len(validNotes)-1]
			renderData, err := h.prepareNoteData(c, &lastNote, vault.ID)
			if err != nil {
				return err
			}
			pushURL := correctedPath
			if !needsCorrection {
				pushURL = buildPushURL(c.Request().Header.Get("HX-Current-Url"), c.Request().Header.Get("X-From-Note-Id"), lastNote.ClientID, stringValue(vault.RootNoteID))
			}
			body, err := h.renderer.Render("templates/app/note.html", renderData)
			if err != nil {
				return err
			}
			c.Response().Header().Set("HX-Push-Url", pushURL)
			return c.HTML(http.StatusOK, body)
		}

		notesData := make([]any, 0, len(validNotes))
		for idx := range validNotes {
			renderData, err := h.prepareNoteData(c, &validNotes[idx], vault.ID)
			if err != nil {
				return err
			}
			notesData = append(notesData, renderData)
		}
		description := h.md.ExtractDescription(stringValue(validNotes[0].Content), 160)
		body, err := h.renderer.Render("templates/app/note-page.html", map[string]any{
			"notes":       notesData,
			"vault":       vaultData,
			"description": description,
		})
		if err != nil {
			return err
		}
		if needsCorrection {
			c.Response().Header().Set("HX-Replace-Url", correctedPath)
		}
		return c.HTML(http.StatusOK, body)
	})
}

func (h *AppHandler) ServeAsset(c *echo.Context) error {
	path := strings.TrimPrefix(c.Param("*"), "/")
	if path == "" {
		return c.String(http.StatusBadRequest, "Bad request")
	}
	if h.cfg.StorageType == "s3" {
		return c.String(http.StatusNotFound, "Not found")
	}
	return h.withVault(c, func(vault *model.Vault) error {
		obj, err := h.store.GetObject(vault.ID, path)
		if err != nil {
			return err
		}
		if obj == nil {
			return c.String(http.StatusNotFound, "Not found")
		}
		body, err := storage.ReadAll(obj)
		if err != nil {
			return err
		}
		c.Response().Header().Set("Content-Type", obj.ContentType)
		c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return c.Blob(http.StatusOK, obj.ContentType, body)
	})
}

func (h *AppHandler) ServeFavicon(c *echo.Context) error {
	return h.withVault(c, func(vault *model.Vault) error {
		if vault.LogoObjectKey == nil || *vault.LogoObjectKey == "" {
			return c.String(http.StatusNotFound, "Not found")
		}
		logoKey := *vault.LogoObjectKey
		hash := extractLogoHash(logoKey)
		etag := `"` + hash + `"`
		cacheControl := "public, max-age=300"
		if c.QueryParam("v") != "" {
			cacheControl = "public, max-age=31536000, immutable"
		}
		faviconKey := store.FaviconObjectKey(logoKey)
		assetKey := logoKey
		meta, err := h.store.HeadObject(vault.ID, faviconKey)
		if err == nil && meta != nil {
			assetKey = faviconKey
		}
		c.Response().Header().Set("Cache-Control", cacheControl)
		c.Response().Header().Set("ETag", etag)
		return c.Redirect(http.StatusFound, h.store.PublicAssetURL(vault.ID, assetKey))
	})
}

func (h *AppHandler) withVault(c *echo.Context, fn func(vault *model.Vault) error) error {
	uri := c.Request().URL.Path
	if strings.HasPrefix(uri, "/console") || strings.HasPrefix(uri, "/obsidian") {
		return c.String(http.StatusNotFound, "Not found")
	}
	parsed := mdbmiddleware.ParseHostDomain(c.Request().Host)
	switch parsed.Error {
	case "missing", "invalid":
		return c.String(http.StatusBadRequest, "Bad request")
	}
	vault, err := h.repo.GetVaultByDomain(c.Request().Context(), parsed.Domain)
	if errors.Is(err, sql.ErrNoRows) || vault == nil {
		return c.String(http.StatusForbidden, "Forbidden")
	}
	if err != nil {
		return err
	}
	return fn(vault)
}

func (h *AppHandler) prepareNoteData(c *echo.Context, note *model.Note, vaultID string) (map[string]any, error) {
	links, err := h.repo.GetNoteLinks(c.Request().Context(), vaultID, note.ClientID)
	if err != nil {
		return nil, err
	}
	storedLinks := make([]markdown.StoredLink, 0, len(links))
	for _, link := range links {
		storedLinks = append(storedLinks, markdown.StoredLink{
			Original:       stringValue(link.Original),
			TargetClientID: link.TargetClientID,
			TargetPath:     stringValue(link.TargetPath),
			DisplayText:    stringValue(link.DisplayText),
			LinkType:       link.LinkType,
		})
	}
	title := h.md.ExtractTitle(stringValue(note.Content))
	if title == "" {
		title = strings.ReplaceAll(strings.TrimSuffix(note.Path, ".md"), "/", " / ")
	}
	backlinks, err := h.repo.GetBacklinksWithNotes(c.Request().Context(), vaultID, note.ClientID)
	if err != nil {
		return nil, err
	}
	backlinkData := make([]any, 0, len(backlinks))
	for _, backlink := range backlinks {
		backlinkTitle := h.md.ExtractTitle(stringValue(backlink.Content))
		if backlinkTitle == "" {
			backlinkTitle = strings.TrimSuffix(backlink.Path, ".md")
		}
		backlinkData = append(backlinkData, map[string]any{
			"client_id":   backlink.ClientID,
			"title":       backlinkTitle,
			"description": h.md.ExtractDescription(stringValue(backlink.Content), 100),
		})
	}

	return map[string]any{
		"note": map[string]any{
			"client_id":    note.ClientID,
			"title":        title,
			"html_content": h.md.RenderMarkdown(stringValue(note.Content), vaultID, storedLinks),
			"path":         note.Path,
			"updated_at":   note.UpdatedAt,
		},
		"backlinks": backlinkData,
	}, nil
}

func (h *AppHandler) publicVault(vault *model.Vault) map[string]any {
	data := map[string]any{
		"id":               vault.ID,
		"name":             vault.Name,
		"domain":           stringValue(vault.Domain),
		"root_note_id":     stringValue(vault.RootNoteID),
		"custom_head_html": stringValue(vault.CustomHeadHTML),
	}
	if vault.LogoObjectKey != nil && *vault.LogoObjectKey != "" {
		data["logo_url"] = h.store.PublicAssetURL(vault.ID, *vault.LogoObjectKey)
		data["favicon_version"] = url.QueryEscape(*vault.LogoObjectKey)
		data["logo_hash"] = extractLogoHash(*vault.LogoObjectKey)
	}
	return data
}

func parsePathIDs(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "+")
}

func buildPushURL(currentURL, fromNoteID, targetNoteID, rootNoteID string) string {
	currentPath := "/"
	if currentURL != "" {
		currentPath = currentURL
		if idx := strings.Index(currentPath, "://"); idx >= 0 {
			if slash := strings.Index(currentPath[idx+3:], "/"); slash >= 0 {
				currentPath = currentPath[idx+3+slash:]
			}
		}
		if idx := strings.Index(currentPath, "?"); idx >= 0 {
			currentPath = currentPath[:idx]
		}
	}
	if currentPath == "/" {
		if rootNoteID != "" {
			return "/" + rootNoteID + "+" + targetNoteID
		}
		return "/" + targetNoteID
	}
	parts := strings.Split(strings.TrimPrefix(currentPath, "/"), "+")
	if fromNoteID != "" {
		for idx, part := range parts {
			if part == fromNoteID {
				parts = parts[:idx+1]
				break
			}
		}
	}
	return "/" + strings.Join(append(parts, targetNoteID), "+")
}

func extractLogoHash(logoObjectKey string) string {
	filename := logoObjectKey[strings.LastIndex(logoObjectKey, "/")+1:]
	return strings.TrimSuffix(filename, "."+store.ExtensionFromPath(filename))
}

func noteListData(notes []model.Note) []any {
	out := make([]any, 0, len(notes))
	for _, note := range notes {
		out = append(out, map[string]any{
			"client_id": note.ClientID,
			"path":      note.Path,
			"mtime":     note.MTime,
		})
	}
	return out
}
