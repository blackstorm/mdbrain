package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestConsoleVaultCreateAndList(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleVaultHandler(repo, objectStore, nil)

	tenantID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/console/vaults", strings.NewReader("name=Blog&domain=blog.example.com"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	if err := handler.CreateVault(c); err != nil {
		t.Fatalf("create vault: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("unexpected create response: %d %s", rec.Code, rec.Body.String())
	}

	vault, err := repo.GetVaultByDomain(context.Background(), "blog.example.com")
	if err != nil {
		t.Fatalf("get created vault: %v", err)
	}
	if vault == nil || vault.Name != "Blog" {
		t.Fatalf("unexpected vault: %#v", vault)
	}
}
