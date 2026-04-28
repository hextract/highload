package http

import (
	"github.com/go-chi/chi/v5"

	"highload/catalog/internal/store"
)

func NewRouter(c *store.Catalog) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/livez", Livez)
	r.Get("/readyz", Readyz(c))

	h := &RestaurantHandler{Catalog: c}
	r.Get("/api/v1/restaurants", h.List)
	r.Get("/api/v1/restaurants/{restaurantID}/menu", h.Menu)
	return r
}
