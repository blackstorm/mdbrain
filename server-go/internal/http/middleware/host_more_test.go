package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseHostDomainRejectsTooManyColons(t *testing.T) {
	got := ParseHostDomain("example.com:8080:9090")
	if got.Error != "invalid" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestParseHostDomainRejectsInvalidPort(t *testing.T) {
	got := ParseHostDomain("example.com:not-a-port")
	if got.Error != "invalid" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestParseHostDomainAllowsUnderscore(t *testing.T) {
	got := ParseHostDomain("docs_internal.example.com:8080")
	if got.Domain != "docs_internal.example.com" || got.Error != "" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestParseHostDomainSupportsBracketedIPv6WithoutPort(t *testing.T) {
	got := ParseHostDomain("[::1]")
	if got.Domain != "::1" || got.Error != "" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestParseHostDomainRejectsEmptyBracketedIPv6(t *testing.T) {
	got := ParseHostDomain("[]:8080")
	if got.Error != "invalid" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestParseHostDomainRejectsInvalidCharacters(t *testing.T) {
	got := ParseHostDomain("exa%mple.com")
	if got.Error != "invalid" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestHostErrorResponseBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	HostErrorResponse(rec, ParsedHost{Error: "invalid"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if rec.Body.String() != "Bad request" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestHostErrorResponseForbiddenForUnbound(t *testing.T) {
	rec := httptest.NewRecorder()
	HostErrorResponse(rec, ParsedHost{Error: "unbound"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if rec.Body.String() != "Forbidden" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestValidPortBoundaries(t *testing.T) {
	if !validPort("1") || !validPort("65535") {
		t.Fatal("expected boundary ports to be valid")
	}
}

func TestValidPortRejectsOutOfRange(t *testing.T) {
	if validPort("0") || validPort("65536") {
		t.Fatal("expected out of range ports to be invalid")
	}
}

func TestParseHostDomainMissingHost(t *testing.T) {
	got := ParseHostDomain("")
	if got.Error != "missing" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestParseHostDomainRejectsSlash(t *testing.T) {
	got := ParseHostDomain("example.com/path")
	if got.Error != "invalid" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestParseHostDomainRejectsSpace(t *testing.T) {
	got := ParseHostDomain("example .com")
	if got.Error != "invalid" {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestHostErrorResponseSetsNoStoreHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	HostErrorResponse(rec, ParsedHost{Error: "invalid"})
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unexpected cache control header: %s", got)
	}
}
