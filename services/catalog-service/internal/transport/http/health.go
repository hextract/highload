package http

import (
	"net/http"

	"highload/catalog/internal/store"
)

func Livez(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func Readyz(c *store.Catalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := c.Ping(r.Context()); err != nil {
			http.Error(w, "no db", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}
}
