package http

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"

	"highload/order/internal/catalogclient"
	"highload/order/internal/store"
)

func NewRouter(pool *pgxpool.Pool, st *store.OrderStore, cat *catalogclient.Client, kw *kafka.Writer) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/livez", Livez)
	r.Get("/readyz", Readyz(pool))

	h := &OrderHandler{Store: st, Catalog: cat, Kafka: kw}
	r.Post("/api/v1/orders", h.Create)
	r.Post("/api/v1/orders/{orderID}/pay", h.Pay)
	r.Get("/api/v1/orders/{orderID}/tracking", h.Tracking)
	return r
}
