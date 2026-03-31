package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/domain/model"
	"mdbrain.dev/internal/domain/store"
	"mdbrain.dev/internal/infra/repository"
	"mdbrain.dev/internal/templatex"
)

type ConsoleVaultHandler struct {
	repo     *repository.Repository
	store    store.ObjectStore
	renderer *templatex.Renderer
}

func NewConsoleVaultHandler(repo *repository.Repository, objectStore store.ObjectStore, renderer *templatex.Renderer) *ConsoleVaultHandler {
	return &ConsoleVaultHandler{repo: repo, store: objectStore, renderer: renderer}
}

func (h *ConsoleVaultHandler) ConsoleHome(c *echo.Context) error {
	tenantID := strings.TrimSpace(anyString(c.Get("session.tenant_id")))
	tenant, err := h.repo.GetTenant(c.Request().Context(), tenantID)
	if err != nil {
		return err
	}
	vaults, err := h.repo.ListVaultsByTenant(c.Request().Context(), tenantID)
	if err != nil {
		return err
	}
	body, err := h.renderer.Render("templates/console/vaults.html", map[string]any{
		"tenant":     map[string]any{"id": tenant.ID, "name": tenant.Name},
		"vaults":     h.enrichVaults(c, vaults),
		"csrf-token": anyString(c.Get("csrf_token")),
	})
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, body)
}

func (h *ConsoleVaultHandler) ListVaults(c *echo.Context) error {
	tenantID := strings.TrimSpace(anyString(c.Get("session.tenant_id")))
	vaults, err := h.repo.ListVaultsByTenant(c.Request().Context(), tenantID)
	if err != nil {
		return err
	}
	body, err := h.renderer.Render("templates/console/vault-list.html", map[string]any{
		"vaults": h.enrichVaults(c, vaults),
	})
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, body)
}

func (h *ConsoleVaultHandler) CreateVault(c *echo.Context) error {
	tenantID := strings.TrimSpace(anyString(c.Get("session.tenant_id")))
	name := strings.TrimSpace(firstNonEmpty(c.FormValue("name"), valueFromJSON(c, "name")))
	domain := strings.TrimSpace(firstNonEmpty(c.FormValue("domain"), valueFromJSON(c, "domain")))
	switch {
	case name == "" || domain == "":
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Missing required fields"})
	}
	existing, err := h.repo.GetVaultByDomain(c.Request().Context(), domain)
	if err == nil && existing != nil {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Domain already in use"})
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	vaultID := uuid.NewString()
	syncKey := uuid.NewString()
	if err := h.repo.CreateVault(c.Request().Context(), vaultID, tenantID, name, domain, syncKey); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"vault": map[string]any{
			"id":       vaultID,
			"name":     name,
			"domain":   domain,
			"sync-key": syncKey,
		},
	})
}

func (h *ConsoleVaultHandler) DeleteVault(c *echo.Context) error {
	vault, tenantID, err := h.authorizedVault(c)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Permission denied"})
	}
	if err := h.store.DeleteVaultObjects(vault.ID); err != nil {
		return err
	}
	if err := h.repo.DeleteVault(c.Request().Context(), vault.ID); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"success": true, "message": "Vault deleted"})
}

func (h *ConsoleVaultHandler) UpdateVault(c *echo.Context) error {
	vault, tenantID, err := h.authorizedVault(c)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Permission denied"})
	}

	name := strings.TrimSpace(firstNonEmpty(c.FormValue("name"), valueFromJSON(c, "name")))
	domain := strings.TrimSpace(firstNonEmpty(c.FormValue("domain"), valueFromJSON(c, "domain")))
	switch {
	case name == "":
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Vault name is required"})
	case domain == "":
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Domain is required"})
	}
	existing, err := h.repo.GetVaultByDomain(c.Request().Context(), domain)
	if err == nil && existing != nil && existing.ID != vault.ID {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Domain already in use"})
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if err := h.repo.UpdateVault(c.Request().Context(), vault.ID, name, domain); err != nil {
		if repository.IsUniqueConstraint(err, "vaults", "domain") {
			return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Domain already in use"})
		}
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"vault": map[string]any{
			"id":     vault.ID,
			"name":   name,
			"domain": domain,
		},
	})
}

func (h *ConsoleVaultHandler) SearchVaultNotes(c *echo.Context) error {
	vault, tenantID, err := h.authorizedVault(c)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Permission denied"})
	}
	query := strings.TrimSpace(c.QueryParam("q"))
	notes, err := h.repo.SearchNotesByVault(c.Request().Context(), vault.ID, query)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"success": true, "notes": noteListData(notes)})
}

func (h *ConsoleVaultHandler) UpdateVaultRootNote(c *echo.Context) error {
	vault, tenantID, err := h.authorizedVault(c)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Permission denied"})
	}
	rootNoteID := strings.TrimSpace(firstNonEmpty(c.FormValue("rootNoteId"), valueFromJSON(c, "rootNoteId")))
	if rootNoteID == "" {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Missing rootNoteId"})
	}
	if err := h.repo.UpdateVaultRootNote(c.Request().Context(), vault.ID, &rootNoteID); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"success": true, "message": "Root note updated", "root-note-id": rootNoteID})
}

func (h *ConsoleVaultHandler) GetRootNoteSelector(c *echo.Context) error {
	vault, tenantID, err := h.authorizedVault(c)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.HTML(http.StatusNotFound, `<div class="alert alert-error"><span>Vault not found</span></div>`)
	}
	if vault.TenantID != tenantID {
		return c.HTML(http.StatusForbidden, `<div class="alert alert-error"><span>Permission denied</span></div>`)
	}
	notes, err := h.repo.SearchNotesByVault(c.Request().Context(), vault.ID, "")
	if err != nil {
		return err
	}
	body, err := h.renderer.Render("templates/console/root-note-selector.html", map[string]any{
		"notes":        noteListData(notes),
		"vault-id":     vault.ID,
		"root-note-id": stringValue(vault.RootNoteID),
	})
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, body)
}

func (h *ConsoleVaultHandler) RenewVaultSyncKey(c *echo.Context) error {
	vault, tenantID, err := h.authorizedVault(c)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Permission denied"})
	}
	syncKey := uuid.NewString()
	if err := h.repo.UpdateVaultSyncKey(c.Request().Context(), vault.ID, syncKey); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"success": true, "message": "Publish key renewed", "sync-key": syncKey})
}

func (h *ConsoleVaultHandler) UpdateCustomHeadHTML(c *echo.Context) error {
	vault, tenantID, err := h.authorizedVault(c)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Permission denied"})
	}
	custom := firstNonEmpty(c.FormValue("customHeadHtml"), valueFromJSON(c, "customHeadHtml"))
	if len(custom) > 65536 {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Custom HTML exceeds maximum size of 64KB"})
	}
	if err := h.repo.UpdateVaultCustomHeadHTML(c.Request().Context(), vault.ID, &custom); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"success": true, "message": "Custom HTML updated"})
}

func (h *ConsoleVaultHandler) authorizedVault(c *echo.Context) (*model.Vault, string, error) {
	tenantID := strings.TrimSpace(anyString(c.Get("session.tenant_id")))
	vaultID := strings.TrimSpace(c.Param("id"))
	vault, err := h.repo.GetVaultByID(c.Request().Context(), vaultID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tenantID, nil
	}
	return vault, tenantID, err
}

func (h *ConsoleVaultHandler) enrichVaults(c *echo.Context, vaults []model.Vault) []any {
	out := make([]any, 0, len(vaults))
	for _, vault := range vaults {
		notes, _ := h.repo.SearchNotesByVault(c.Request().Context(), vault.ID, "")
		storageSize, _ := h.repo.GetVaultStorageSize(c.Request().Context(), vault.ID)
		publishStatus := vault.LastPublishStatus
		if publishStatus == "" {
			publishStatus = "never"
		}
		item := map[string]any{
			"id":                         vault.ID,
			"name":                       vault.Name,
			"domain":                     stringValue(vault.Domain),
			"sync_key":                   vault.SyncKey,
			"masked_key":                 maskKey(vault.SyncKey),
			"root_note_id":               stringValue(vault.RootNoteID),
			"custom_head_html":           stringValue(vault.CustomHeadHTML),
			"last_publish_at":            vault.LastPublishAt,
			"last_publish_status":        publishStatus,
			"last_publish_error_message": stringValue(vault.LastPublishErrorMessage),
			"publish_ok":                 publishStatus == "ok",
			"publish_error":              publishStatus == "error",
			"publish_never":              publishStatus != "ok" && publishStatus != "error",
			"notes":                      noteListData(notes),
			"storage_size":               formatStorageSize(storageSize),
		}
		if vault.LogoObjectKey != nil && *vault.LogoObjectKey != "" {
			item["logo_url"] = ConsoleAssetURL(vault.ID, *vault.LogoObjectKey)
		}
		out = append(out, item)
	}
	return out
}
