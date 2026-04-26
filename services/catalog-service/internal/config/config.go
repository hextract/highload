package config

import "os"

type Config struct {
	PGDSN    string
	HTTPAddr string
}

func Load() Config {
	return Config{
		PGDSN:    getenv("PG_DSN", "postgres://food:food@localhost:5432/fooddelivery?sslmode=disable"),
		HTTPAddr: getenv("HTTP_ADDR", ":8081"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
