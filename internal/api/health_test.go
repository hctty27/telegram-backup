package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	w := httptest.NewRecorder()
	NewRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "telegram-backup") || strings.Contains(w.Body.String(), "stage") {
		t.Fatalf("unexpected health response: %d %s", w.Code, w.Body.String())
	}
}

func TestUnimplementedAPI(t *testing.T) {
	w := httptest.NewRecorder()
	NewRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/accounts", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
