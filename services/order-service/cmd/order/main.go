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
	poolCfg, err := pgxpool.ParseConfig(cfg.PGDSN)
	if err != nil {
		slog.Error("pg parse", "err", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = int32(cfg.PgPoolMax)
	poolCfg.MinConns = int32(cfg.PgPoolMin)
	if poolCfg.MinConns > poolCfg.MaxConns {
		poolCfg.MinConns = poolCfg.MaxConns
	}
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		slog.Error("pg", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	st := store.NewOrderStore(pool)
	kw := events.NewPaymentRequestWriter(cfg.KafkaBrokers)
	defer func() { _ = kw.Close() }()
	dlqResults := events.NewPaymentResultsDeadLetterWriter(cfg.KafkaBrokers)
	defer func() { _ = dlqResults.Close() }()

	go events.RunPaymentResultConsumer(ctx, cfg.KafkaBrokers, st, dlqResults)

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
