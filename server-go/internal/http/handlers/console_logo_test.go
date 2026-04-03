package handlers

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestUploadAndServeVaultLogo(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleLogoHandler(repo, objectStore)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "blog.example.com", "sync-key"); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("logo", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(samplePNG(t, 256, 256)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/console/vaults/"+vaultID+"/logo", &body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.UploadVaultLogo(c); err != nil {
		t.Fatalf("upload logo: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected upload status: %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/console/vaults/"+vaultID+"/logo", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/vaults/:id/logo")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: vaultID}})
	if err := handler.ServeVaultLogo(c); err != nil {
		t.Fatalf("serve logo: %v", err)
	}
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("unexpected logo status: %d body=%s", rec.Code, rec.Body.String())
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
		t.Fatalf("unexpected favicon status: %d", rec.Code)
	}
}

func samplePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
