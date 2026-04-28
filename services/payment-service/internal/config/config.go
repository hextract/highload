package config

import (
	"os"
	"strings"
)

type Config struct {
	PGDSN        string
	HTTPAddr     string
	KafkaBrokers []string
}

func Load() Config {
	brokers := strings.Split(getenv("KAFKA_BROKERS", "localhost:9092"), ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}
	return Config{
		PGDSN:        getenv("PG_DSN", "postgres://food:food@localhost:5432/fooddelivery?sslmode=disable"),
		HTTPAddr:     getenv("HTTP_ADDR", ":8083"),
		KafkaBrokers: brokers,
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
