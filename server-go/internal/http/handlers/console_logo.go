package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strconv"

	xdraw "golang.org/x/image/draw"

	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/domain/store"
	"mdbrain.dev/internal/infra/repository"
	"mdbrain.dev/internal/infra/storage"
)

type ConsoleLogoHandler struct {
	repo  *repository.Repository
	store store.ObjectStore
}

func NewConsoleLogoHandler(repo *repository.Repository, objectStore store.ObjectStore) *ConsoleLogoHandler {
	return &ConsoleLogoHandler{repo: repo, store: objectStore}
}

func (h *ConsoleLogoHandler) UploadVaultLogo(c *echo.Context) error {
	vault, tenantID, err := authorizedVaultForSession(c, h.repo)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusForbidden, map[string]any{"success": false, "error": "Permission denied"})
	}
	file, err := c.FormFile("logo")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": "No file uploaded"})
	}
	if file.Size == 0 || file.Size > 2*1024*1024 {
		message := "File too large. Maximum size is 2MB."
		if file.Size == 0 {
			message = "File is empty. Please upload a valid image file."
		}
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": message})
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	content, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	contentType := http.DetectContentType(content)
	if contentType == "application/octet-stream" {
		contentType = firstNonEmpty(file.Header.Get("Content-Type"), contentType)
	}
	if contentType != "image/png" && contentType != "image/jpeg" {
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": "Invalid file type. Allowed: PNG, JPEG. Got: " + contentType})
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || (format != "png" && format != "jpeg") {
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": "Invalid image file"})
	}
	if min(cfg.Width, cfg.Height) < 128 {
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": "Image too small. Minimum size is 128x128."})
	}

	extension := "jpg"
	if contentType == "image/png" {
		extension = "png"
	}
	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:16])
	logoKey := store.LogoObjectKey(contentHash, extension)
	faviconBytes, err := generateFavicon(content, contentType)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"success": false, "error": "Failed to generate favicon"})
	}
	faviconKey := store.FaviconObjectKey(logoKey)
	if err := h.store.PutObject(vault.ID, logoKey, content, contentType); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"success": false, "error": "Failed to store logo"})
	}
	if err := h.store.PutObject(vault.ID, faviconKey, faviconBytes, contentType); err != nil {
		_ = h.store.DeleteObject(vault.ID, logoKey)
		return c.JSON(http.StatusInternalServerError, map[string]any{"success": false, "error": "Failed to store favicon"})
	}
	if err := h.repo.UpdateVaultLogo(c.Request().Context(), vault.ID, &logoKey); err != nil {
		_ = h.store.DeleteObject(vault.ID, logoKey)
		_ = h.store.DeleteObject(vault.ID, faviconKey)
		return c.JSON(http.StatusInternalServerError, map[string]any{"success": false, "error": "Failed to update vault logo"})
	}
	if vault.LogoObjectKey != nil && *vault.LogoObjectKey != "" && *vault.LogoObjectKey != logoKey {
		_ = h.store.DeleteObject(vault.ID, store.FaviconObjectKey(*vault.LogoObjectKey))
		_ = h.store.DeleteObject(vault.ID, *vault.LogoObjectKey)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"success":  true,
		"message":  "Logo uploaded successfully",
		"logo-url": ConsoleAssetURL(vault.ID, logoKey),
	})
}

func (h *ConsoleLogoHandler) DeleteVaultLogo(c *echo.Context) error {
	vault, tenantID, err := authorizedVaultForSession(c, h.repo)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusForbidden, map[string]any{"success": false, "error": "Permission denied"})
	}
	if vault.LogoObjectKey == nil || *vault.LogoObjectKey == "" {
		return c.JSON(http.StatusOK, map[string]any{"success": true, "message": "No logo to delete"})
	}
	_ = h.store.DeleteObject(vault.ID, store.FaviconObjectKey(*vault.LogoObjectKey))
	_ = h.store.DeleteObject(vault.ID, *vault.LogoObjectKey)
	if err := h.repo.UpdateVaultLogo(c.Request().Context(), vault.ID, nil); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"success": true, "message": "Logo deleted successfully"})
}

func (h *ConsoleLogoHandler) ServeVaultLogo(c *echo.Context) error {
	return h.serveVaultObject(c, false)
}

func (h *ConsoleLogoHandler) ServeVaultFavicon(c *echo.Context) error {
	return h.serveVaultObject(c, true)
}

func (h *ConsoleLogoHandler) serveVaultObject(c *echo.Context, favicon bool) error {
	vault, tenantID, err := authorizedVaultForSession(c, h.repo)
	if err != nil {
		return err
	}
	if vault == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"success": false, "error": "Vault not found"})
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusForbidden, map[string]any{"success": false, "error": "Permission denied"})
	}
	if vault.LogoObjectKey == nil || *vault.LogoObjectKey == "" {
		return c.JSON(http.StatusNotFound, map[string]any{"success": false, "error": "Logo not found"})
	}
	key := *vault.LogoObjectKey
	if favicon {
		if meta, err := h.store.HeadObject(vault.ID, store.FaviconObjectKey(key)); err == nil && meta != nil {
			key = store.FaviconObjectKey(key)
		}
	}
	object, err := h.store.GetObject(vault.ID, key)
	if err != nil {
		return err
	}
	if object == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"success": false, "error": "Logo not found"})
	}
	body, err := storage.ReadAll(object)
	if err != nil {
		return err
	}
	contentType := firstNonEmpty(object.ContentType, "application/octet-stream")
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("Content-Length", strconv.Itoa(len(body)))
	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	return c.Blob(http.StatusOK, contentType, body)
}

func generateFavicon(content []byte, contentType string) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	size := min(bounds.Dx(), bounds.Dy())
	startX := bounds.Min.X + (bounds.Dx()-size)/2
	startY := bounds.Min.Y + (bounds.Dy()-size)/2
	cropped := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.Draw(cropped, cropped.Bounds(), src, image.Point{X: startX, Y: startY}, xdraw.Src)
	dst := image.NewRGBA(image.Rect(0, 0, 32, 32))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), xdraw.Over, nil)

	var buf bytes.Buffer
	switch contentType {
	case "image/png":
		err = png.Encode(&buf, dst)
	default:
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 90})
	}
	return buf.Bytes(), err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
