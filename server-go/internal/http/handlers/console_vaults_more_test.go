package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestCreateVaultValidationAndConflict(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleVaultHandler(repo, objectStore, nil)

	tenantID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/console/vaults", strings.NewReader("name=OnlyName"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	if err := handler.CreateVault(c); err != nil {
		t.Fatalf("create vault: %v", err)
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}

	if err := repo.CreateVault(context.Background(), uuid.NewString(), tenantID, "Old", "same.example.com", "sync"); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/console/vaults", strings.NewReader("name=New&domain=same.example.com"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	if err := handler.CreateVault(c); err != nil {
		t.Fatalf("create vault: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}
}

func TestDeleteVaultAuthorizationAndNotFound(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleVaultHandler(repo, objectStore, nil)

	tenant1 := uuid.NewString()
	tenant2 := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenant1, "Tenant1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTenant(context.Background(), tenant2, "Tenant2"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenant1, "Blog", "blog.example.com", "sync"); err != nil {
		t.Fatal(err)
	}
	if err := objectStore.PutObject(vaultID, "assets/file.png", []byte("asset"), "image/png"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/console/vaults/not-found", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant1)
	c.SetPath("/console/vaults/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "not-found"}})
	if err := handler.DeleteVault(c); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}

	req = httptest.NewRequest(http.MethodDelete, "/console/vaults/"+vaultID, nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant2)
	c.SetPath("/console/vaults/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.DeleteVault(c); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}

	req = httptest.NewRequest(http.MethodDelete, "/console/vaults/"+vaultID, nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant1)
	c.SetPath("/console/vaults/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.DeleteVault(c); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != true {
		t.Fatalf("expected success=true, got: %#v", payload)
	}
	_, err := repo.GetVaultByID(context.Background(), vaultID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after delete, got: %v", err)
	}
}

func TestRenewSyncKeyAndUpdateVault(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleVaultHandler(repo, objectStore, nil)

	tenantID := uuid.NewString()
	vault1 := uuid.NewString()
	vault2 := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vault1, tenantID, "Blog1", "blog1.example.com", "sync-old"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vault2, tenantID, "Blog2", "blog2.example.com", "sync-2"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/console/vaults/"+vault1+"/renew-sync-key", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/renew-sync-key")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vault1}})
	if err := handler.RenewVaultSyncKey(c); err != nil {
		t.Fatalf("renew sync key: %v", err)
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != true {
		t.Fatalf("expected success=true, got: %#v", payload)
	}
	afterRenew, err := repo.GetVaultByID(context.Background(), vault1)
	if err != nil {
		t.Fatal(err)
	}
	if afterRenew.SyncKey == "sync-old" {
		t.Fatalf("sync key should change")
	}

	req = httptest.NewRequest(http.MethodPut, "/console/vaults/"+vault1, strings.NewReader("name=&domain=blog3.example.com"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vault1}})
	if err := handler.UpdateVault(c); err != nil {
		t.Fatalf("update vault: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}

	req = httptest.NewRequest(http.MethodPut, "/console/vaults/"+vault1, strings.NewReader("name=Blog1New&domain=blog2.example.com"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vault1}})
	if err := handler.UpdateVault(c); err != nil {
		t.Fatalf("update vault: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}

	req = httptest.NewRequest(http.MethodPut, "/console/vaults/"+vault1, strings.NewReader("name=Blog1New&domain=blog1new.example.com"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vault1}})
	if err := handler.UpdateVault(c); err != nil {
		t.Fatalf("update vault: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != true {
		t.Fatalf("expected success=true, got: %#v", payload)
	}
	updated, err := repo.GetVaultByID(context.Background(), vault1)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Blog1New" || updated.Domain == nil || *updated.Domain != "blog1new.example.com" {
		t.Fatalf("unexpected updated vault: %#v", updated)
	}
}

func TestSearchRootNoteAndCustomHeadHTML(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleVaultHandler(repo, objectStore, nil)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "blog.example.com", "sync"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(context.Background(), uuid.NewString(), tenantID, vaultID, "Hello.md", "note-hello", strPtr("hello world"), strPtr("{}"), strPtr("h1"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(context.Background(), uuid.NewString(), tenantID, vaultID, "Other.md", "note-other", strPtr("other"), strPtr("{}"), strPtr("h2"), nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/console/vaults/"+vaultID+"/notes/search?q=hello", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/notes/search")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.SearchVaultNotes(c); err != nil {
		t.Fatalf("search notes: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Hello.md") {
		t.Fatalf("expected note in response: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/console/vaults/"+vaultID+"/root-note", strings.NewReader(""))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/root-note")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UpdateVaultRootNote(c); err != nil {
		t.Fatalf("update root note: %v", err)
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}

	req = httptest.NewRequest(http.MethodPut, "/console/vaults/"+vaultID+"/root-note", strings.NewReader("rootNoteId=note-hello"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/root-note")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UpdateVaultRootNote(c); err != nil {
		t.Fatalf("update root note: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != true {
		t.Fatalf("expected success=true, got: %#v", payload)
	}
	vault, err := repo.GetVaultByID(context.Background(), vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.RootNoteID == nil || *vault.RootNoteID != "note-hello" {
		t.Fatalf("unexpected root note id: %#v", vault.RootNoteID)
	}

	req = httptest.NewRequest(http.MethodPut, "/console/vaults/"+vaultID+"/custom-head-html", strings.NewReader("customHeadHtml="+strings.Repeat("x", 70000)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/custom-head-html")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UpdateCustomHeadHTML(c); err != nil {
		t.Fatalf("update custom head html: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}

	req = httptest.NewRequest(http.MethodPut, "/console/vaults/"+vaultID+"/custom-head-html", strings.NewReader("customHeadHtml=<script>ok</script>"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/custom-head-html")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UpdateCustomHeadHTML(c); err != nil {
		t.Fatalf("update custom head html: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != true {
		t.Fatalf("expected success=true, got: %#v", payload)
	}
	vault, err = repo.GetVaultByID(context.Background(), vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.CustomHeadHTML == nil || *vault.CustomHeadHTML != "<script>ok</script>" {
		t.Fatalf("unexpected custom head html: %#v", vault.CustomHeadHTML)
	}
}

func TestConsoleHomeListAndRootNoteSelectorRender(t *testing.T) {
	_, repo, objectStore, renderer := setupTestCoreWithRenderer(t)
	handler := NewConsoleVaultHandler(repo, objectStore, renderer)
	ctx := context.Background()

	tenantID := uuid.NewString()
	otherTenantID := uuid.NewString()
	vaultID := uuid.NewString()
	secondVaultID := uuid.NewString()
	if err := repo.CreateTenant(ctx, tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTenant(ctx, otherTenantID, "Other"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(ctx, vaultID, tenantID, "Garden", "garden.example.com", "sync-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(ctx, secondVaultID, tenantID, "Sandbox", "sandbox.example.com", "sync-2"); err != nil {
		t.Fatal(err)
	}

	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "Hello.md", "note-hello", strPtr("# Hello"), strPtr("{}"), strPtr("hash-hello"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "World.md", "note-world", strPtr("# World"), strPtr("{}"), strPtr("hash-world"), nil); err != nil {
		t.Fatal(err)
	}
	rootNoteID := "note-world"
	if err := repo.UpdateVaultRootNote(ctx, vaultID, &rootNoteID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-1", "img/logo.png", "assets/logo.png", 2048, "image/png", "md5-logo"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordVaultPublishSuccess(ctx, vaultID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/console", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.Set("csrf_token", "csrf-home")
	if err := handler.ConsoleHome(c); err != nil {
		t.Fatalf("console home: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected home status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Garden") ||
		!strings.Contains(rec.Body.String(), "Sandbox") ||
		!strings.Contains(rec.Body.String(), "OK") ||
		!strings.Contains(rec.Body.String(), "Never") ||
		!strings.Contains(rec.Body.String(), "2.00 KB") {
		t.Fatalf("unexpected home body: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/console/vaults", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	if err := handler.ListVaults(c); err != nil {
		t.Fatalf("list vaults: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "garden.example.com") || !strings.Contains(rec.Body.String(), "sandbox.example.com") {
		t.Fatalf("unexpected list vaults response: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/console/vaults/"+vaultID+"/root-note-selector", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/root-note-selector")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.GetRootNoteSelector(c); err != nil {
		t.Fatalf("root note selector: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "note-selector") || !strings.Contains(rec.Body.String(), "World.md") || !strings.Contains(rec.Body.String(), "selected") {
		t.Fatalf("unexpected root note selector response: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/console/vaults/"+vaultID+"/root-note-selector", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", otherTenantID)
	c.SetPath("/console/vaults/:id/root-note-selector")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.GetRootNoteSelector(c); err != nil {
		t.Fatalf("unauthorized root note selector: %v", err)
	}
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Permission denied") {
		t.Fatalf("unexpected unauthorized selector response: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/console/vaults/not-found/root-note-selector", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/root-note-selector")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "not-found"}})
	if err := handler.GetRootNoteSelector(c); err != nil {
		t.Fatalf("missing root note selector: %v", err)
	}
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Vault not found") {
		t.Fatalf("unexpected missing selector response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateVaultRootNoteRejectsMissingOrForeignNote(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleVaultHandler(repo, objectStore, nil)
	ctx := context.Background()

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	otherVaultID := uuid.NewString()
	if err := repo.CreateTenant(ctx, tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(ctx, vaultID, tenantID, "Garden", "garden.example.com", "sync-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(ctx, otherVaultID, tenantID, "Sandbox", "sandbox.example.com", "sync-2"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "Home.md", "note-home", strPtr("# Home"), strPtr("{}"), strPtr("hash-home"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, otherVaultID, "Foreign.md", "note-foreign", strPtr("# Foreign"), strPtr("{}"), strPtr("hash-foreign"), nil); err != nil {
		t.Fatal(err)
	}

	run := func(noteID string) map[string]any {
		req := httptest.NewRequest(http.MethodPut, "/console/vaults/"+vaultID+"/root-note", strings.NewReader("rootNoteId="+noteID))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("session.tenant_id", tenantID)
		c.SetPath("/console/vaults/:id/root-note")
		c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
		if err := handler.UpdateVaultRootNote(c); err != nil {
			t.Fatalf("update root note %q: %v", noteID, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status for note %q: %d body=%s", noteID, rec.Code, rec.Body.String())
		}
		return readJSONMap(t, rec.Body.Bytes())
	}

	if payload := run("missing-note"); payload["success"] != false || payload["error"] != "Root note not found" {
		t.Fatalf("unexpected missing-note payload: %#v", payload)
	}
	if payload := run("note-foreign"); payload["success"] != false || payload["error"] != "Root note not found" {
		t.Fatalf("unexpected foreign-note payload: %#v", payload)
	}

	vault, err := repo.GetVaultByID(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.RootNoteID != nil {
		t.Fatalf("expected root note to stay unset after invalid updates, got %#v", vault.RootNoteID)
	}

	if payload := run("note-home"); payload["success"] != true {
		t.Fatalf("unexpected valid root-note payload: %#v", payload)
	}
	vault, err = repo.GetVaultByID(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.RootNoteID == nil || *vault.RootNoteID != "note-home" {
		t.Fatalf("unexpected root note after valid update: %#v", vault.RootNoteID)
	}
}
