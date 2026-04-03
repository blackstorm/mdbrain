package handlers

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/domain/store"
	storageinfra "mdbrain.dev/internal/infra/storage"
)

type failOnNthPutStore struct {
	*storageinfra.LocalStore
	failOnPut int
	putCalls  int
}

func (s *failOnNthPutStore) PutObject(vaultID, objectKey string, content []byte, contentType string) error {
	s.putCalls++
	if s.putCalls == s.failOnPut {
		return errors.New("forced put failure")
	}
	return s.LocalStore.PutObject(vaultID, objectKey, content, contentType)
}

func TestUploadVaultLogoValidationAndPermissions(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleLogoHandler(repo, objectStore)

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

	req := httptest.NewRequest(http.MethodPost, "/console/vaults/not-found/logo", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant1)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "not-found"}})
	if err := handler.UploadVaultLogo(c); err != nil {
		t.Fatalf("upload logo: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/console/vaults/"+vaultID+"/logo", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant2)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UploadVaultLogo(c); err != nil {
		t.Fatalf("upload logo: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/console/vaults/"+vaultID+"/logo", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant1)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UploadVaultLogo(c); err != nil {
		t.Fatalf("upload logo: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = newLogoUploadRequest(t, http.MethodPost, "/console/vaults/"+vaultID+"/logo", "logo", "logo.txt", []byte("not image"), "text/plain")
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant1)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UploadVaultLogo(c); err != nil {
		t.Fatalf("upload logo: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = newLogoUploadRequest(t, http.MethodPost, "/console/vaults/"+vaultID+"/logo", "logo", "small.png", samplePNG(t, 64, 64), "image/png")
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant1)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UploadVaultLogo(c); err != nil {
		t.Fatalf("upload logo: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServeFaviconFallbackAndNoLogo(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleLogoHandler(repo, objectStore)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Tenant1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "blog.example.com", "sync"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/console/vaults/"+vaultID+"/favicon", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/favicon")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.ServeVaultFavicon(c); err != nil {
		t.Fatalf("serve favicon: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	logoKey := "site/logo/hash123.png"
	logoBytes := []byte("logo-data")
	if err := repo.UpdateVaultLogo(context.Background(), vaultID, &logoKey); err != nil {
		t.Fatal(err)
	}
	if err := objectStore.PutObject(vaultID, logoKey, logoBytes, "image/png"); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/console/vaults/"+vaultID+"/favicon", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/favicon")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.ServeVaultFavicon(c); err != nil {
		t.Fatalf("serve favicon: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(logoBytes) {
		t.Fatalf("expected fallback logo bytes, got: %s", rec.Body.String())
	}
}

func TestDeleteVaultLogoScenarios(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleLogoHandler(repo, objectStore)

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

	req := httptest.NewRequest(http.MethodDelete, "/console/vaults/"+vaultID+"/logo", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant2)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.DeleteVaultLogo(c); err != nil {
		t.Fatalf("delete logo: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/console/vaults/"+vaultID+"/logo", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant1)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.DeleteVaultLogo(c); err != nil {
		t.Fatalf("delete logo: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != true {
		t.Fatalf("expected success=true, got: %#v", payload)
	}

	logoKey := "site/logo/hash-delete.png"
	faviconKey := store.FaviconObjectKey(logoKey)
	if err := repo.UpdateVaultLogo(context.Background(), vaultID, &logoKey); err != nil {
		t.Fatal(err)
	}
	if err := objectStore.PutObject(vaultID, logoKey, []byte("logo"), "image/png"); err != nil {
		t.Fatal(err)
	}
	if err := objectStore.PutObject(vaultID, faviconKey, []byte("favicon"), "image/png"); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodDelete, "/console/vaults/"+vaultID+"/logo", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant1)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.DeleteVaultLogo(c); err != nil {
		t.Fatalf("delete logo: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	vault, err := repo.GetVaultByID(context.Background(), vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.LogoObjectKey != nil {
		t.Fatalf("expected logo key cleared, got: %#v", vault.LogoObjectKey)
	}
	obj, err := objectStore.GetObject(vaultID, logoKey)
	if err != nil {
		t.Fatal(err)
	}
	if obj != nil {
		t.Fatalf("expected logo object deleted")
	}
	obj, err = objectStore.GetObject(vaultID, faviconKey)
	if err != nil {
		t.Fatal(err)
	}
	if obj != nil {
		t.Fatalf("expected favicon object deleted")
	}
}

func TestUploadVaultLogoRollsBackWhenFaviconStoreFails(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	flakyStore := &failOnNthPutStore{LocalStore: objectStore, failOnPut: 2}
	handler := NewConsoleLogoHandler(repo, flakyStore)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "blog.example.com", "sync"); err != nil {
		t.Fatal(err)
	}

	content := samplePNG(t, 256, 256)
	req := newLogoUploadRequest(t, http.MethodPost, "/console/vaults/"+vaultID+"/logo", "logo", "logo.png", content, "image/png")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UploadVaultLogo(c); err != nil {
		t.Fatalf("upload logo with failing favicon store: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got %#v", payload)
	}

	logoKey := store.LogoObjectKey(sha256Hex(content, 16), "png")
	faviconKey := store.FaviconObjectKey(logoKey)
	obj, err := objectStore.GetObject(vaultID, logoKey)
	if err != nil {
		t.Fatal(err)
	}
	if obj != nil {
		t.Fatalf("expected uploaded logo to be rolled back")
	}
	obj, err = objectStore.GetObject(vaultID, faviconKey)
	if err != nil {
		t.Fatal(err)
	}
	if obj != nil {
		t.Fatalf("expected favicon object to be absent after rollback")
	}

	vault, err := repo.GetVaultByID(context.Background(), vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.LogoObjectKey != nil {
		t.Fatalf("expected vault logo key to remain unset, got %#v", vault.LogoObjectKey)
	}
}

func newLogoUploadRequest(t *testing.T, method, target, fieldName, fileName string, content []byte, contentType string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(method, target, &body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("X-Test-Content-Type", contentType)
	}
	return req
}
