package middlewarehelpers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostOriginGuard_AllowsValidHostAndOrigin(t *testing.T) {
	guard := HostOriginGuard(
		[]string{"localhost", "pay.bog-best-online-games.ru"},
		[]string{"https://pay.bog-best-online-games.ru"},
	)

	handler := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "pay.bog-best-online-games.ru"
	req.Header.Set("Origin", "https://pay.bog-best-online-games.ru")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHostOriginGuard_BlocksUnknownOriginAndHost(t *testing.T) {
	guard := HostOriginGuard(
		[]string{"localhost"},
		[]string{"https://pay.bog-best-online-games.ru"},
	)

	handler := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "malicious.example"
	req.Header.Set("Origin", "https://evil.example")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
