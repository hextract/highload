package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	PGDSN        string
	HTTPAddr     string
	CatalogURL   string
	KafkaBrokers []string
	PgPoolMax    int
	PgPoolMin    int
}

func Load() Config {
	brokers := strings.Split(getenv("KAFKA_BROKERS", "localhost:9092"), ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}
	return Config{
		PGDSN:        getenv("PG_DSN", "postgres://food:food@localhost:5432/fooddelivery?sslmode=disable"),
		HTTPAddr:     getenv("HTTP_ADDR", ":8082"),
		CatalogURL:   strings.TrimRight(getenv("CATALOG_BASE_URL", "http://localhost:8081"), "/"),
		KafkaBrokers: brokers,
		PgPoolMax:    getenvInt("PG_POOL_MAX_CONNS", 12),
		PgPoolMin:    getenvInt("PG_POOL_MIN_CONNS", 2),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}
