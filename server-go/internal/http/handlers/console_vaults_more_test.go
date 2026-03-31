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
