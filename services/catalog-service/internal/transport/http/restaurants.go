package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"highload/catalog/internal/cache"
	"highload/catalog/internal/store"
)

type RestaurantHandler struct {
	Catalog   *store.Catalog
	MenuRedis *cache.MenuRedis
}

func (h *RestaurantHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	latStr, lonStr := q.Get("lat"), q.Get("lon")
	if latStr == "" || lonStr == "" {
		http.Error(w, `{"error":"lat and lon required"}`, http.StatusBadRequest)
		return
	}
	lat, err1 := strconv.ParseFloat(latStr, 64)
	lon, err2 := strconv.ParseFloat(lonStr, 64)
	if err1 != nil || err2 != nil {
		http.Error(w, `{"error":"invalid coordinates"}`, http.StatusBadRequest)
		return
	}
	p := store.SearchParams{Lat: lat, Lon: lon, Radius: 3000, Page: 1, Limit: 20, Sort: q.Get("sort")}
	if p.Sort == "" {
		p.Sort = "distance"
	}
	if v := q.Get("radius"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Radius = n
		}
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Page = n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			p.Limit = n
		}
	}
	p.Cuisine = q.Get("cuisine")
	p.Query = q.Get("query")

	list, err := h.Catalog.SearchRestaurants(r.Context(), p)
	if err != nil {
		slog.Error("query", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	total := len(list)
	offset := (p.Page - 1) * p.Limit
	if offset > total {
		offset = total
	}
	end := offset + p.Limit
	if end > total {
		end = total
	}
	pageRows := list[offset:end]

	type item struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Cuisine      string  `json:"cuisine"`
		Rating       float64 `json:"rating"`
		AvgPrice     int     `json:"avg_price"`
		DistanceM    float64 `json:"distance_m"`
		DeliveryTime int     `json:"delivery_time_min"`
		ImageURL     string  `json:"image_url"`
		IsOpen       bool    `json:"is_open"`
	}
	out := struct {
		Restaurants []item `json:"restaurants"`
		Total       int    `json:"total"`
		Page        int    `json:"page"`
		Limit       int    `json:"limit"`
	}{Page: p.Page, Limit: p.Limit, Total: total}
	for _, rr := range pageRows {
		out.Restaurants = append(out.Restaurants, item{
			ID: rr.ID.String(), Name: rr.Name, Cuisine: rr.CuisineType, Rating: rr.Rating,
			AvgPrice: rr.AvgPrice, DistanceM: rr.DistanceM, DeliveryTime: rr.DeliveryTimeMin,
			ImageURL: rr.ImageURL, IsOpen: rr.IsOpen,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func menuCacheKey(restaurantID uuid.UUID) string {
	return "menu:v1:" + restaurantID.String()
}

func (h *RestaurantHandler) Menu(w http.ResponseWriter, r *http.Request) {
	ridStr := chi.URLParam(r, "restaurantID")
	rid, err := uuid.Parse(ridStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if h.MenuRedis != nil {
		if body, ok := h.MenuRedis.Get(ctx, menuCacheKey(rid)); ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
	}
	rows, err := h.Catalog.RestaurantMenu(ctx, rid)
	if err != nil {
		if errors.Is(err, store.ErrRestaurantUnavailable) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		slog.Error("menu", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	type menuItem struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Price       int    `json:"price"`
		ImageURL    string `json:"image_url"`
		Available   bool   `json:"is_available"`
	}
	type cat struct {
		Name  string     `json:"name"`
		Items []menuItem `json:"items"`
	}
	catOrder := []string{}
	cats := map[string]*cat{}
	for _, row := range rows {
		if _, exists := cats[row.CategoryName]; !exists {
			cats[row.CategoryName] = &cat{Name: row.CategoryName}
			catOrder = append(catOrder, row.CategoryName)
		}
		cats[row.CategoryName].Items = append(cats[row.CategoryName].Items, menuItem{
			ID: row.ItemID.String(), Name: row.Name, Description: row.Description, Price: row.Price,
			ImageURL: row.ImageURL, Available: row.Available,
		})
	}
	outCats := make([]cat, 0, len(catOrder))
	for _, n := range catOrder {
		outCats = append(outCats, *cats[n])
	}
	out := struct {
		RestaurantID string    `json:"restaurant_id"`
		Categories   []cat     `json:"categories"`
		UpdatedAt    time.Time `json:"updated_at"`
	}{
		RestaurantID: rid.String(),
		Categories:   outCats,
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	body, err := json.Marshal(out)
	if err != nil {
		slog.Error("menu json", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if h.MenuRedis != nil {
		h.MenuRedis.Set(ctx, menuCacheKey(rid), body)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
