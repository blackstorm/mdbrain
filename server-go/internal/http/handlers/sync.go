package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/domain/linker"
	"mdbrain.dev/internal/domain/model"
	"mdbrain.dev/internal/domain/store"
	"mdbrain.dev/internal/http/response"
	"mdbrain.dev/internal/infra/repository"
)

type SyncHandler struct {
	repo  *repository.Repository
	store store.ObjectStore
}

func NewSyncHandler(repo *repository.Repository, objectStore store.ObjectStore) *SyncHandler {
	return &SyncHandler{repo: repo, store: objectStore}
}

type syncChangesRequest struct {
	Notes  []model.NoteHashEntry  `json:"notes"`
	Assets []model.AssetHashEntry `json:"assets"`
}

type syncNoteRequest struct {
	Path        string                  `json:"path"`
	Content     *string                 `json:"content"`
	Hash        string                  `json:"hash"`
	Metadata    map[string]any          `json:"metadata"`
	Assets      *[]model.AssetHashEntry `json:"assets"`
	LinkedNotes *[]model.NoteHashEntry  `json:"linked_notes"`
}

type syncAssetRequest struct {
	Path        string  `json:"path"`
	ContentType string  `json:"contentType"`
	Size        *int64  `json:"size"`
	Hash        string  `json:"hash"`
	Content     *string `json:"content"`
}

func (h *SyncHandler) SyncChanges(c *echo.Context) error {
	vault, err := h.requireSyncVault(c)
	if err != nil || vault == nil {
		return err
	}

	var req syncChangesRequest
	if err := c.Bind(&req); err != nil {
		respErr := response.BadRequest(c, "Invalid JSON body")
		_ = h.repo.RecordVaultPublishError(c.Request().Context(), vault.ID, "bad_request", "Invalid JSON body")
		return respErr
	}

	resp := h.syncChanges(c.Request().Context(), vault, req)
	return h.recordAndWrite(c, vault, resp)
}

func (h *SyncHandler) SyncNote(c *echo.Context) error {
	vault, err := h.requireSyncVault(c)
	if err != nil || vault == nil {
		return err
	}

	var req syncNoteRequest
	if err := c.Bind(&req); err != nil {
		respErr := handlerResult{Status: http.StatusBadRequest, Body: map[string]any{"success": false, "error": "Invalid JSON body"}}
		return h.recordAndWrite(c, vault, respErr)
	}
	resp := h.syncNote(c.Request().Context(), vault, c.Param("id"), req)
	return h.recordAndWrite(c, vault, resp)
}

func (h *SyncHandler) SyncAsset(c *echo.Context) error {
	vault, err := h.requireSyncVault(c)
	if err != nil || vault == nil {
		return err
	}

	var req syncAssetRequest
	if err := c.Bind(&req); err != nil {
		respErr := handlerResult{Status: http.StatusBadRequest, Body: map[string]any{"success": false, "error": "Invalid JSON body"}}
		return h.recordAndWrite(c, vault, respErr)
	}
	resp := h.syncAsset(c.Request().Context(), vault, c.Param("id"), req)
	return h.recordAndWrite(c, vault, resp)
}

func (h *SyncHandler) VaultInfo(c *echo.Context) error {
	vault, err := h.requireSyncVault(c)
	if err != nil || vault == nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"vault": map[string]any{
			"id":        vault.ID,
			"name":      vault.Name,
			"domain":    stringValue(vault.Domain),
			"createdAt": vault.CreatedAt,
		},
	})
}

type handlerResult struct {
	Status int
	Body   any
}

func (h *SyncHandler) syncChanges(ctx context.Context, vault *model.Vault, req syncChangesRequest) handlerResult {
	serverNotes, err := h.repo.ListNotesByVault(ctx, vault.ID)
	if err != nil {
		return internalErrorResult(err)
	}
	serverAssets, err := h.repo.ListAssetsByVault(ctx, vault.ID)
	if err != nil {
		return internalErrorResult(err)
	}

	clientNotes := make(map[string]string, len(req.Notes))
	for _, item := range req.Notes {
		clientNotes[item.ID] = item.Hash
	}
	clientAssets := make(map[string]string, len(req.Assets))
	for _, item := range req.Assets {
		clientAssets[item.ID] = item.Hash
	}

	var notesToDelete []model.NoteHashEntry
	for _, note := range serverNotes {
		hash := stringValue(note.Hash)
		if _, ok := clientNotes[note.ClientID]; !ok {
			notesToDelete = append(notesToDelete, model.NoteHashEntry{ID: note.ClientID, Hash: hash})
		}
	}

	var assetsToDelete []model.AssetHashEntry
	for _, asset := range serverAssets {
		if _, ok := clientAssets[asset.ClientID]; !ok {
			assetsToDelete = append(assetsToDelete, model.AssetHashEntry{ID: asset.ClientID, Hash: asset.MD5})
		}
	}

	var notesToUpsert []model.NoteHashEntry
	for id, hash := range clientNotes {
		serverHash := ""
		for _, note := range serverNotes {
			if note.ClientID == id {
				serverHash = stringValue(note.Hash)
				break
			}
		}
		if hash != serverHash {
			notesToUpsert = append(notesToUpsert, model.NoteHashEntry{ID: id, Hash: hash})
		}
	}

	var assetsToUpsert []model.AssetHashEntry
	for id, hash := range clientAssets {
		serverHash := ""
		for _, asset := range serverAssets {
			if asset.ClientID == id {
				serverHash = asset.MD5
				break
			}
		}
		if hash != serverHash {
			assetsToUpsert = append(assetsToUpsert, model.AssetHashEntry{ID: id, Hash: hash})
		}
	}

	for _, item := range notesToDelete {
		if err := h.deleteNote(ctx, vault.ID, item.ID); err != nil {
			return internalErrorResult(err)
		}
	}
	for _, item := range assetsToDelete {
		if err := h.deleteAsset(ctx, vault.ID, item.ID); err != nil {
			return internalErrorResult(err)
		}
	}

	sort.Slice(notesToUpsert, func(i, j int) bool { return notesToUpsert[i].ID < notesToUpsert[j].ID })
	sort.Slice(assetsToUpsert, func(i, j int) bool { return assetsToUpsert[i].ID < assetsToUpsert[j].ID })
	sort.Slice(notesToDelete, func(i, j int) bool { return notesToDelete[i].ID < notesToDelete[j].ID })
	sort.Slice(assetsToDelete, func(i, j int) bool { return assetsToDelete[i].ID < assetsToDelete[j].ID })

	return handlerResult{
		Status: http.StatusOK,
		Body: map[string]any{
			"need_upsert": map[string]any{
				"notes":  notesToUpsert,
				"assets": assetsToUpsert,
			},
			"deleted_on_server": map[string]any{
				"notes":  notesToDelete,
				"assets": assetsToDelete,
			},
		},
	}
}

func (h *SyncHandler) syncNote(ctx context.Context, vault *model.Vault, noteID string, req syncNoteRequest) handlerResult {
	notePath := strings.TrimSpace(req.Path)
	noteHash := strings.TrimSpace(req.Hash)
	switch {
	case strings.TrimSpace(noteID) == "":
		return badRequestResult("Missing note id")
	case notePath == "":
		return badRequestResult("Missing note path")
	case noteHash == "":
		return badRequestResult("Missing note hash")
	case req.Content == nil:
		return badRequestResult("Missing note content")
	case req.Assets == nil:
		return badRequestResult("Missing assets")
	case req.LinkedNotes == nil:
		return badRequestResult("Missing linked notes")
	}

	existing, err := h.repo.GetNoteByClientID(ctx, vault.ID, noteID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return internalErrorResult(err)
	}
	if existing != nil && stringValue(existing.Hash) == noteHash && existing.Path == notePath {
		return handlerResult{Status: http.StatusOK, Body: map[string]any{"status": "skipped", "noteId": noteID}}
	}

	var metadataJSON *string
	if req.Metadata != nil {
		payload, err := json.Marshal(req.Metadata)
		if err != nil {
			return badRequestResult("Invalid note metadata")
		}
		text := string(payload)
		metadataJSON = &text
	}

	if err := h.upsertNoteWithLinks(ctx, vault.TenantID, vault.ID, noteID, notePath, *req.Content, noteHash, metadataJSON); err != nil {
		return internalErrorResult(err)
	}

	assetEntries := *req.Assets
	assetIDs := make([]string, 0, len(assetEntries))
	for _, item := range assetEntries {
		assetIDs = append(assetIDs, item.ID)
	}
	if err := h.updateNoteAssetRefs(ctx, vault.ID, noteID, assetIDs); err != nil {
		return internalErrorResult(err)
	}

	var needUploadAssets []model.AssetHashEntry
	for _, entry := range assetEntries {
		existing, err := h.repo.GetAssetByClientID(ctx, vault.ID, entry.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return internalErrorResult(err)
		}
		if existing == nil || existing.MD5 != entry.Hash {
			needUploadAssets = append(needUploadAssets, entry)
		}
	}

	var needUploadNotes []model.NoteHashEntry
	for _, entry := range *req.LinkedNotes {
		existing, err := h.repo.GetNoteByClientID(ctx, vault.ID, entry.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return internalErrorResult(err)
		}
		if existing == nil || stringValue(existing.Hash) != entry.Hash {
			needUploadNotes = append(needUploadNotes, entry)
		}
	}

	return handlerResult{
		Status: http.StatusOK,
		Body: map[string]any{
			"status":             "stored",
			"noteId":             noteID,
			"need_upload_assets": needUploadAssets,
			"need_upload_notes":  needUploadNotes,
		},
	}
}

func (h *SyncHandler) syncAsset(ctx context.Context, vault *model.Vault, assetID string, req syncAssetRequest) handlerResult {
	assetPath := strings.TrimSpace(req.Path)
	assetHash := strings.TrimSpace(req.Hash)
	contentType := strings.TrimSpace(req.ContentType)
	switch {
	case strings.TrimSpace(assetID) == "":
		return badRequestResult("Missing asset id")
	case assetPath == "":
		return badRequestResult("Missing asset path")
	case assetHash == "":
		return badRequestResult("Missing asset hash")
	case contentType == "":
		return badRequestResult("Missing asset contentType")
	}

	existing, err := h.repo.GetAssetByClientID(ctx, vault.ID, assetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return internalErrorResult(err)
	}
	if existing != nil && existing.MD5 == assetHash && existing.Path == assetPath {
		return handlerResult{Status: http.StatusOK, Body: map[string]any{"status": "skipped", "assetId": assetID}}
	}
	if req.Content == nil {
		return badRequestResult("Missing asset content")
	}
	bytes, err := base64.StdEncoding.DecodeString(*req.Content)
	if err != nil {
		return badRequestResult("Missing asset content")
	}

	objectKey := store.AssetObjectKey(assetID, store.ExtensionFromPath(assetPath))
	if err := h.store.PutObject(vault.ID, objectKey, bytes, contentType); err != nil {
		return internalErrorResult(err)
	}
	size := int64(len(bytes))
	if req.Size != nil {
		size = *req.Size
	}
	if err := h.repo.UpsertAsset(ctx, uuid.NewString(), vault.TenantID, vault.ID, assetID, assetPath, objectKey, size, contentType, assetHash); err != nil {
		return internalErrorResult(err)
	}
	return handlerResult{Status: http.StatusOK, Body: map[string]any{"status": "stored", "assetId": assetID}}
}

func (h *SyncHandler) requireSyncVault(c *echo.Context) (*model.Vault, error) {
	authHeader := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, response.Unauthorized(c, "Missing authorization header")
	}
	syncKey := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	vault, err := h.repo.GetVaultBySyncKey(c.Request().Context(), syncKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, response.Unauthorized(c, "Invalid publish key")
	}
	if err != nil {
		return nil, err
	}
	return vault, nil
}

func (h *SyncHandler) recordAndWrite(c *echo.Context, vault *model.Vault, resp handlerResult) error {
	if resp.Status < 400 {
		_ = h.repo.RecordVaultPublishSuccess(c.Request().Context(), vault.ID)
	} else {
		_ = h.repo.RecordVaultPublishError(c.Request().Context(), vault.ID, publishErrorCode(resp.Status), publishErrorMessage(resp.Body))
	}
	return c.JSON(resp.Status, resp.Body)
}

func (h *SyncHandler) upsertNoteWithLinks(ctx context.Context, tenantID, vaultID, noteID, path, content, hash string, metadata *string) error {
	if err := h.repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, path, noteID, &content, metadata, &hash, nil); err != nil {
		return err
	}
	allNotes, err := h.repo.GetNotesForLinkResolution(ctx, vaultID)
	if err != nil {
		return err
	}
	refs := make([]linker.NoteRef, 0, len(allNotes))
	for _, note := range allNotes {
		refs = append(refs, linker.NoteRef{ClientID: note.ClientID, Path: note.Path})
	}
	resolved := linker.ResolveLinks(linker.ExtractLinks(content), refs)
	filtered := make([]linker.ResolvedLink, 0, len(resolved))
	for _, item := range resolved {
		if item.TargetClientID != "" {
			filtered = append(filtered, item)
		}
	}
	deduped := linker.DeduplicateByTarget(filtered)

	existingLinks, err := h.repo.GetNoteLinks(ctx, vaultID, noteID)
	if err != nil {
		return err
	}
	existingMap := make(map[string]linker.ResolvedLink, len(existingLinks))
	for _, item := range existingLinks {
		existingMap[item.TargetClientID] = linker.ResolvedLink{
			TargetClientID: item.TargetClientID,
			TargetPath:     stringValue(item.TargetPath),
			LinkType:       item.LinkType,
			DisplayText:    stringValue(item.DisplayText),
			Original:       stringValue(item.Original),
		}
	}
	newMap := make(map[string]linker.ResolvedLink, len(deduped))
	for _, item := range deduped {
		newMap[item.TargetClientID] = item
	}

	for targetID, item := range existingMap {
		newItem, exists := newMap[targetID]
		if !exists || newItem != item {
			if err := h.repo.DeleteNoteLinkByTarget(ctx, vaultID, noteID, targetID); err != nil {
				return err
			}
		}
	}
	for targetID, item := range newMap {
		if existing, exists := existingMap[targetID]; !exists || existing != item {
			if err := h.repo.InsertNoteLink(ctx, vaultID, noteID, item.TargetClientID, item.TargetPath, item.LinkType, item.DisplayText, item.Original); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *SyncHandler) updateNoteAssetRefs(ctx context.Context, vaultID, noteID string, assetIDs []string) error {
	existingRefs, err := h.repo.GetAssetRefsByNote(ctx, vaultID, noteID)
	if err != nil {
		return err
	}
	existingIDs := make(map[string]struct{}, len(existingRefs))
	for _, item := range existingRefs {
		existingIDs[item.AssetClientID] = struct{}{}
	}
	newIDs := make(map[string]struct{}, len(assetIDs))
	orderedNewIDs := make([]string, 0, len(assetIDs))
	for _, id := range assetIDs {
		if _, ok := newIDs[id]; ok {
			continue
		}
		newIDs[id] = struct{}{}
		orderedNewIDs = append(orderedNewIDs, id)
	}
	if err := h.repo.UpdateNoteAssetRefs(ctx, vaultID, noteID, orderedNewIDs); err != nil {
		return err
	}
	for existingID := range existingIDs {
		if _, ok := newIDs[existingID]; ok {
			continue
		}
		count, err := h.repo.CountAssetRefs(ctx, vaultID, existingID)
		if err != nil {
			return err
		}
		if count == 0 {
			if err := h.deleteAsset(ctx, vaultID, existingID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *SyncHandler) deleteNote(ctx context.Context, vaultID, noteID string) error {
	if err := h.repo.DeleteNoteLinksBySource(ctx, vaultID, noteID); err != nil {
		return err
	}
	if err := h.repo.DeleteNoteAssetRefsByNote(ctx, vaultID, noteID); err != nil {
		return err
	}
	if err := h.repo.DeleteNoteByClientID(ctx, vaultID, noteID); err != nil {
		return err
	}
	return h.repo.DeleteOrphanLinks(ctx, vaultID)
}

func (h *SyncHandler) deleteAsset(ctx context.Context, vaultID, assetID string) error {
	asset, err := h.repo.GetAssetByClientID(ctx, vaultID, assetID)
	if errors.Is(err, sql.ErrNoRows) || asset == nil {
		return nil
	}
	if err != nil {
		return err
	}
	if err := h.store.DeleteObject(vaultID, asset.ObjectKey); err != nil {
		return err
	}
	if err := h.repo.DeleteNoteAssetRefsByAsset(ctx, vaultID, assetID); err != nil {
		return err
	}
	return h.repo.DeleteAssetByClientID(ctx, vaultID, assetID)
}

func internalErrorResult(_ error) handlerResult {
	return handlerResult{
		Status: http.StatusInternalServerError,
		Body:   map[string]any{"success": false, "error": "Internal server error"},
	}
}

func badRequestResult(message string) handlerResult {
	return handlerResult{
		Status: http.StatusBadRequest,
		Body:   map[string]any{"success": false, "error": message},
	}
}

func publishErrorCode(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "bad_request"
	default:
		return "error"
	}
}

func publishErrorMessage(body any) string {
	if m, ok := body.(map[string]any); ok {
		if value, ok := m["error"].(string); ok && strings.TrimSpace(value) != "" {
			return truncate(value, 400)
		}
	}
	return "Request failed"
}

func truncate(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[:n]
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
