package http

import (
	"github.com/go-chi/chi/v5"

	"highload/catalog/internal/cache"
	"highload/catalog/internal/store"
)

func NewRouter(c *store.Catalog, menuRedis *cache.MenuRedis) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/livez", Livez)
	r.Get("/readyz", Readyz(c))

	h := &RestaurantHandler{Catalog: c, MenuRedis: menuRedis}
	r.Get("/api/v1/restaurants", h.List)
	r.Get("/api/v1/restaurants/{restaurantID}/menu", h.Menu)
	return r
}
