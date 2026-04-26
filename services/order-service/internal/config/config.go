package config

import (
	"os"
	"strings"
)

type Config struct {
	PGDSN        string
	HTTPAddr     string
	CatalogURL   string
	KafkaBrokers []string
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
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
