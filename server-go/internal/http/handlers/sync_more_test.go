package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/infra/storage"
)

func TestSyncAuthFailures(t *testing.T) {
	handler, _, _ := setupSyncHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/changes", strings.NewReader(`{"notes":[],"assets":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	vault, err := handler.requireSyncVault(c)
	if err != nil {
		t.Fatalf("require sync vault without auth: %v", err)
	}
	if vault != nil {
		t.Fatalf("expected nil vault for missing auth, got %#v", vault)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing auth, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/obsidian/sync/changes", strings.NewReader(`{"notes":[],"assets":[]}`))
	req.Header.Set("Authorization", "Bearer bad-key")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	vault, err = handler.requireSyncVault(c)
	if err != nil {
		t.Fatalf("require sync vault with invalid auth: %v", err)
	}
	if vault != nil {
		t.Fatalf("expected nil vault for invalid auth, got %#v", vault)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid publish key, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestVaultInfoReturnsVaultMetadata(t *testing.T) {
	handler, _, vaultID := setupSyncHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/obsidian/vault/info", nil)
	req.Header.Set("Authorization", "Bearer sync-key-1")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.VaultInfo(c); err != nil {
		t.Fatalf("vault info: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	vault, ok := payload["vault"].(map[string]any)
	if !ok || vault["id"] != vaultID || vault["domain"] != "sync.example.com" {
		t.Fatalf("unexpected vault info payload: %#v", payload)
	}
}

func TestSyncChangesBadJSONRecordsPublishError(t *testing.T) {
	handler, repo, vaultID := setupSyncHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/changes", strings.NewReader("{"))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.SyncChanges(c); err != nil {
		t.Fatalf("sync changes with invalid json: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d body=%s", rec.Code, rec.Body.String())
	}

	vault, err := repo.GetVaultByID(context.Background(), vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.LastPublishStatus != "error" || stringValue(vault.LastPublishErrorCode) != "bad_request" || stringValue(vault.LastPublishErrorMessage) != "Invalid JSON body" {
		t.Fatalf("unexpected publish status after invalid changes sync: %#v", vault)
	}
}

func TestSyncNoteValidationAndPublishStatus(t *testing.T) {
	handler, repo, vaultID := setupSyncHandler(t)

	invalidBody := `{"path":"Note A.md","hash":"hash-a","assets":[],"linked_notes":[]}`
	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/notes/note-a", strings.NewReader(invalidBody))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/notes/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "note-a"}})
	if err := handler.SyncNote(c); err != nil {
		t.Fatalf("sync note with invalid body: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing note content, got %d body=%s", rec.Code, rec.Body.String())
	}

	vault, err := repo.GetVaultByID(context.Background(), vaultID)
	if err != nil {
		t.Fatalf("load vault after invalid sync: %v", err)
	}
	if vault.LastPublishStatus != "error" || stringValue(vault.LastPublishErrorCode) != "bad_request" {
		t.Fatalf("expected publish error status after invalid sync, got %#v", vault)
	}

	validBody := `{"path":"Note A.md","content":"# A","hash":"hash-a","assets":[],"linked_notes":[]}`
	req = httptest.NewRequest(http.MethodPost, "/obsidian/sync/notes/note-a", strings.NewReader(validBody))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/notes/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "note-a"}})
	if err := handler.SyncNote(c); err != nil {
		t.Fatalf("sync note with valid body: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid sync note, got %d body=%s", rec.Code, rec.Body.String())
	}

	vault, err = repo.GetVaultByID(context.Background(), vaultID)
	if err != nil {
		t.Fatalf("load vault after valid sync: %v", err)
	}
	if vault.LastPublishStatus != "ok" || vault.LastPublishErrorCode != nil || vault.LastPublishErrorMessage != nil {
		t.Fatalf("expected publish success to clear error fields, got %#v", vault)
	}
}

func TestSyncChangesDeletesMissingNotesAndAssets(t *testing.T) {
	handler, repo, vaultID := setupSyncHandler(t)
	ctx := context.Background()
	tenantID := mustTenantID(t, repo, vaultID)
	localStore := handler.store.(*storage.LocalStore)

	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "a.md", "note-a", strPtr("A"), strPtr("{}"), strPtr("hash-a"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "b.md", "note-b", strPtr("B"), strPtr("{}"), strPtr("hash-b"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertNoteLink(ctx, vaultID, "note-a", "note-b", "b.md", "link", "B", "[[B]]"); err != nil {
		t.Fatal(err)
	}

	const objectKey = "assets/asset-a.png"
	if err := localStore.PutObject(vaultID, objectKey, []byte("asset-a"), "image/png"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-a", "img/a.png", objectKey, 7, "image/png", "md5-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateNoteAssetRefs(ctx, vaultID, "note-a", []string{"asset-a"}); err != nil {
		t.Fatal(err)
	}

	body := `{"notes":[{"id":"note-b","hash":"hash-b"}],"assets":[]}`
	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/changes", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.SyncChanges(c); err != nil {
		t.Fatalf("sync changes delete missing items: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if !strings.Contains(rec.Body.String(), `"id":"note-a"`) || !strings.Contains(rec.Body.String(), `"id":"asset-a"`) {
		t.Fatalf("expected deleted items in payload, got %#v", payload)
	}

	_, err := repo.GetNoteByClientID(ctx, vaultID, "note-a")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing note-a after sync changes delete, got %v", err)
	}
	_, err = repo.GetAssetByClientID(ctx, vaultID, "asset-a")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing asset-a after sync changes delete, got %v", err)
	}
	obj, err := localStore.GetObject(vaultID, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if obj != nil {
		t.Fatalf("expected asset object deleted from local store")
	}
	links, err := repo.GetNoteLinks(ctx, vaultID, "note-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("expected deleted note links to be removed, got %#v", links)
	}
}

func TestSyncNoteAndAssetSkippedWhenHashUnchanged(t *testing.T) {
	handler, repo, vaultID := setupSyncHandler(t)
	ctx := context.Background()
	tenantID := mustTenantID(t, repo, vaultID)

	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "a.md", "note-a", strPtr("A"), strPtr("{}"), strPtr("hash-a"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-a", "img/a.png", "assets/asset-a.png", 12, "image/png", "md5-a"); err != nil {
		t.Fatal(err)
	}

	noteBody := `{"path":"a.md","content":"A changed but same hash","hash":"hash-a","assets":[],"linked_notes":[]}`
	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/notes/note-a", strings.NewReader(noteBody))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/notes/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "note-a"}})
	if err := handler.SyncNote(c); err != nil {
		t.Fatalf("sync skipped note: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected note status: %d body=%s", rec.Code, rec.Body.String())
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if payload["status"] != "skipped" {
		t.Fatalf("expected note skipped status, got %#v", payload)
	}

	assetBody := `{"path":"img/a.png","contentType":"image/png","hash":"md5-a"}`
	req = httptest.NewRequest(http.MethodPost, "/obsidian/sync/assets/asset-a", strings.NewReader(assetBody))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/assets/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "asset-a"}})
	if err := handler.SyncAsset(c); err != nil {
		t.Fatalf("sync skipped asset: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected asset status: %d body=%s", rec.Code, rec.Body.String())
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["status"] != "skipped" {
		t.Fatalf("expected asset skipped status, got %#v", payload)
	}
}

func TestSyncNoteRemovesOrphanAssetAndKeepsSharedAsset(t *testing.T) {
	handler, repo, vaultID := setupSyncHandler(t)
	ctx := context.Background()
	tenantID := mustTenantID(t, repo, vaultID)

	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-shared", "img/shared.png", "assets/shared.png", 11, "image/png", "md5-shared"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-orphan", "img/orphan.png", "assets/orphan.png", 10, "image/png", "md5-orphan"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "n1.md", "note-1", strPtr("N1"), strPtr("{}"), strPtr("hash-n1"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "n2.md", "note-2", strPtr("N2"), strPtr("{}"), strPtr("hash-n2"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateNoteAssetRefs(ctx, vaultID, "note-1", []string{"asset-shared", "asset-orphan"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateNoteAssetRefs(ctx, vaultID, "note-2", []string{"asset-shared"}); err != nil {
		t.Fatal(err)
	}

	noteBody := `{"path":"n1.md","content":"N1 updated","hash":"hash-n1-new","assets":[{"id":"asset-shared","hash":"md5-shared"}],"linked_notes":[]}`
	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/notes/note-1", strings.NewReader(noteBody))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/notes/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "note-1"}})
	if err := handler.SyncNote(c); err != nil {
		t.Fatalf("sync note removing orphan ref: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	refs, err := repo.GetAssetRefsByNote(ctx, vaultID, "note-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].AssetClientID != "asset-shared" {
		t.Fatalf("expected only shared asset ref left on note-1, got %#v", refs)
	}

	_, err = repo.GetAssetByClientID(ctx, vaultID, "asset-orphan")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected orphan asset to be deleted, got err=%v", err)
	}

	shared, err := repo.GetAssetByClientID(ctx, vaultID, "asset-shared")
	if err != nil || shared == nil {
		t.Fatalf("expected shared asset to remain, err=%v asset=%#v", err, shared)
	}
}

func TestSyncNoteNeedUploadListsReflectClientDiff(t *testing.T) {
	handler, repo, vaultID := setupSyncHandler(t)
	ctx := context.Background()
	tenantID := mustTenantID(t, repo, vaultID)

	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-1", "img/a.png", "assets/a.png", 10, "image/png", "md5-old"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "B.md", "note-b", strPtr("# B"), strPtr("{}"), strPtr("hash-old"), nil); err != nil {
		t.Fatal(err)
	}

	body := `{"path":"A.md","content":"[[B]]","hash":"hash-a","assets":[{"id":"asset-1","hash":"md5-new"}],"linked_notes":[{"id":"note-b","hash":"hash-new"}]}`
	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/notes/note-a", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/notes/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "note-a"}})
	if err := handler.SyncNote(c); err != nil {
		t.Fatalf("sync note for need_upload lists: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	payload := readJSONMap(t, rec.Body.Bytes())
	needAssets, ok := payload["need_upload_assets"].([]any)
	if !ok || len(needAssets) != 1 {
		t.Fatalf("unexpected need_upload_assets: %#v", payload["need_upload_assets"])
	}
	assetItem, ok := needAssets[0].(map[string]any)
	if !ok || assetItem["id"] != "asset-1" {
		t.Fatalf("unexpected need_upload_assets item: %#v", needAssets[0])
	}
	needNotes, ok := payload["need_upload_notes"].([]any)
	if !ok || len(needNotes) != 1 {
		t.Fatalf("unexpected need_upload_notes: %#v", payload["need_upload_notes"])
	}
	noteItem, ok := needNotes[0].(map[string]any)
	if !ok || noteItem["id"] != "note-b" {
		t.Fatalf("unexpected need_upload_notes item: %#v", needNotes[0])
	}
}

func TestSyncAssetValidationBadBase64(t *testing.T) {
	handler, _, _ := setupSyncHandler(t)
	content := base64.StdEncoding.EncodeToString([]byte("ok"))

	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/assets/asset-a", strings.NewReader(`{"path":"img/a.png","contentType":"image/png","hash":"h1","content":"`+content+`"}`))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/assets/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "asset-a"}})
	if err := handler.SyncAsset(c); err != nil {
		t.Fatalf("sync asset with valid body: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected success for valid asset body, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/obsidian/sync/assets/asset-b", strings.NewReader(`{"path":"img/b.png","contentType":"image/png","hash":"h2","content":"***"}`))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/assets/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "asset-b"}})
	if err := handler.SyncAsset(c); err != nil {
		t.Fatalf("sync asset with invalid base64 body: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid base64 asset body, got %d body=%s", rec.Code, rec.Body.String())
	}
}
