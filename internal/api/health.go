package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter exposes only unauthenticated health checks until admin auth is implemented.
func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"service": "telegram-backup-v2",
			"stage": "foundation",
		})
	})
	return r
}
