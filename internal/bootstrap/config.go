package bootstrap

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPAddr, PostgresDSN, RedisAddr                               string
	RequestTimeout, PostgresTimeout, RedisTimeout, ShutdownTimeout time.Duration
}

func NewConfig() (Config, error) {
	c := Config{HTTPAddr: env("HTTP_ADDR", ":8080"), PostgresDSN: os.Getenv("POSTGRES_DSN"), RedisAddr: env("REDIS_ADDR", "localhost:6379")}
	for _, item := range []struct {
		name, fallback string
		dest           *time.Duration
	}{
		{"REQUEST_TIMEOUT", "1s", &c.RequestTimeout},
		{"POSTGRES_TIMEOUT", "500ms", &c.PostgresTimeout},
		{"REDIS_TIMEOUT", "100ms", &c.RedisTimeout},
		{"SHUTDOWN_TIMEOUT", "10s", &c.ShutdownTimeout},
	} {
		d, err := time.ParseDuration(env(item.name, item.fallback))
		if err != nil || d <= 0 {
			return c, fmt.Errorf("%s must be a positive duration", item.name)
		}
		*item.dest = d
	}
	if c.PostgresDSN == "" {
		return c, fmt.Errorf("POSTGRES_DSN is required")
	}
	return c, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
