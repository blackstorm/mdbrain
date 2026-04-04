package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/domain/store"
	"mdbrain.dev/internal/infra/repository"
	"mdbrain.dev/internal/infra/storage"
)

type ConsoleCommonHandler struct {
	repo  *repository.Repository
	store store.ObjectStore
}

func NewConsoleCommonHandler(repo *repository.Repository, objectStore store.ObjectStore) *ConsoleCommonHandler {
	return &ConsoleCommonHandler{repo: repo, store: objectStore}
}

func ConsoleAssetURL(vaultID, objectKey string) string {
	return "/console/storage/" + vaultID + "/" + objectKey
}

func (h *ConsoleCommonHandler) ServeConsoleAsset(c *echo.Context) error {
	tenantID := strings.TrimSpace(anyString(c.Get("session.tenant_id")))
	vaultID := strings.TrimSpace(c.Param("id"))
	path := strings.TrimSpace(c.Param("*"))
	vault, err := h.repo.GetVaultByID(c.Request().Context(), vaultID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, map[string]any{"success": false, "error": "Vault not found"})
		}
		return err
	}
	if vault.TenantID != tenantID {
		return c.JSON(http.StatusForbidden, map[string]any{"success": false, "error": "Permission denied"})
	}
	if path == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": "Missing path"})
	}

	object, err := h.store.GetObject(vaultID, path)
	if err != nil {
		return err
	}
	if object == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"success": false, "error": "Not found"})
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
