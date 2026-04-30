package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"highload/payment/internal/config"
	"highload/payment/internal/events"
	"highload/payment/internal/store"
	httpx "highload/payment/internal/transport/http"
	"highload/payment/internal/worker"
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

	st := store.New(pool)
	wResults := events.NewPaymentResultWriter(cfg.KafkaBrokers)
	defer func() { _ = wResults.Close() }()
	wDLQ := events.NewRequestsDeadLetterWriter(cfg.KafkaBrokers)
	defer func() { _ = wDLQ.Close() }()

	go worker.RunPaymentRequests(ctx, cfg.KafkaBrokers, &worker.Bus{
		Store:    st,
		Writer:   wResults,
		DLQ:      wDLQ,
		TopicSrc: events.TopicPaymentRequests,
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      httpx.NewMux(st),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	slog.Info("payment", "addr", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
