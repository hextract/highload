package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"highload/catalog/internal/cache"
	"highload/catalog/internal/config"
	"highload/catalog/internal/store"
	httpx "highload/catalog/internal/transport/http"
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
	if d, err := time.ParseDuration(cfg.PgPoolMaxConnLifetime); err == nil && d > 0 {
		poolCfg.MaxConnLifetime = d
	} else {
		poolCfg.MaxConnLifetime = 10 * time.Minute
	}
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		slog.Error("pg connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	var menuRedis *cache.MenuRedis
	if cfg.RedisAddr != "" {
		mr, err := cache.NewMenuRedis(cfg.RedisAddr)
		if err != nil {
			slog.Error("redis", "addr", cfg.RedisAddr, "err", err)
			os.Exit(1)
		}
		defer func() { _ = mr.Close() }()
		menuRedis = mr
		slog.Info("redis menu cache enabled", "addr", cfg.RedisAddr)
	}

	cat := store.NewCatalog(pool)
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      httpx.NewRouter(cat, menuRedis),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	slog.Info("catalog", "addr", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
