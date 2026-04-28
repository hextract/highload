package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"

	"highload/payment/internal/config"
	"highload/payment/internal/events"
	"highload/payment/internal/store"
	httpx "highload/payment/internal/transport/http"
	"highload/payment/internal/worker"
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

	st := store.New(pool)
	wResults := &kafka.Writer{
		Addr:         kafka.TCP(cfg.KafkaBrokers...),
		Topic:        events.TopicPaymentResults,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
	defer func() { _ = wResults.Close() }()

	go worker.RunPaymentRequests(ctx, cfg.KafkaBrokers, &worker.Bus{Store: st, Writer: wResults})

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
