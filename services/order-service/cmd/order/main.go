package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"highload/order/internal/catalogclient"
	"highload/order/internal/config"
	"highload/order/internal/events"
	"highload/order/internal/store"
	httpx "highload/order/internal/transport/http"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.PGDSN)
	if err != nil {
		slog.Error("pg", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	st := store.NewOrderStore(pool)
	kw := events.NewPaymentRequestWriter(cfg.KafkaBrokers)
	defer func() { _ = kw.Close() }()

	go events.RunPaymentResultConsumer(ctx, cfg.KafkaBrokers, st)

	cat := catalogclient.New(cfg.CatalogURL)
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      httpx.NewRouter(pool, st, cat, kw),
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	slog.Info("order", "addr", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
