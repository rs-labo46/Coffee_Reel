package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterIPExtractorUsesRightmostUntrustedXFFAddress(t *testing.T) {
	e := newTestRouter(
		&controllerStub{},
		&userUsecaseStub{},
		&tokenServiceStub{},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "10.233.24.157:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.200, 198.51.100.10")

	if got := e.IPExtractor(req); got != "198.51.100.10" {
		t.Fatalf("IPExtractor() = %q, want %q", got, "198.51.100.10")
	}
}

func TestRouterIPExtractorSkipsTrustedPrivateProxyHop(t *testing.T) {
	e := newTestRouter(
		&controllerStub{},
		&userUsecaseStub{},
		&tokenServiceStub{},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "10.233.24.157:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.10, 10.233.27.227")

	if got := e.IPExtractor(req); got != "198.51.100.10" {
		t.Fatalf("IPExtractor() = %q, want %q", got, "198.51.100.10")
	}
}
