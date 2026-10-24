package bootstrap

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	GRPCAddr, CounterAuthMode, RedisNamespace                      string
	ViewTTL, FlushInterval, ReconcileInterval, WorkerBudget        time.Duration
	JWTSecret, SMTPAddr, MailFrom, PasswordResetURL                string
	AccessTTL, RefreshTTL, ResetTTL, AuthTimeout, SMTPTimeout      time.Duration
	HTTPAddr, PostgresDSN, RedisAddr                               string
	RequestTimeout, PostgresTimeout, RedisTimeout, ShutdownTimeout time.Duration
}

func NewConfig() (Config, error) {
	c := Config{HTTPAddr: env("HTTP_ADDR", ":8080"), PostgresDSN: os.Getenv("POSTGRES_DSN"), RedisAddr: env("REDIS_ADDR", "localhost:6379")}
	c.GRPCAddr = env("GRPC_ADDR", ":9090")
	c.CounterAuthMode = env("COUNTER_AUTH_MODE", "bearer")
	if c.CounterAuthMode != "test" && c.CounterAuthMode != "bearer" {
		return c, fmt.Errorf("COUNTER_AUTH_MODE must be test or bearer")
	}
	c.RedisNamespace = env("REDIS_NAMESPACE", "counters")
	if strings.ContainsAny(c.RedisNamespace, "{}") {
		return c, fmt.Errorf("REDIS_NAMESPACE must not contain braces")
	}
	c.JWTSecret = os.Getenv("JWT_SECRET")
	c.SMTPAddr = env("SMTP_ADDR", "localhost:1025")
	c.MailFrom = env("MAIL_FROM", "no-reply@counters.local")
	c.PasswordResetURL = env("PASSWORD_RESET_URL", "http://localhost:5173/reset-password")
	if len(c.JWTSecret) < 32 {
		return c, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	if _, _, err := net.SplitHostPort(c.SMTPAddr); err != nil {
		return c, fmt.Errorf("invalid SMTP_ADDR")
	}
	address, err := mail.ParseAddress(c.MailFrom)
	if err != nil || address.Address != c.MailFrom || strings.ContainsAny(c.MailFrom, "\r\n") {
		return c, fmt.Errorf("invalid MAIL_FROM")
	}
	resetURL, err := url.Parse(c.PasswordResetURL)
	if err != nil || (resetURL.Scheme != "http" && resetURL.Scheme != "https") || resetURL.Host == "" || resetURL.User != nil || resetURL.Fragment != "" {
		return c, fmt.Errorf("invalid PASSWORD_RESET_URL")
	}
	for _, item := range []struct {
		name, fallback string
		dest           *time.Duration
	}{
		{"VIEW_TTL", "1h", &c.ViewTTL},
		{"FLUSH_INTERVAL", "1s", &c.FlushInterval},
		{"RECONCILE_INTERVAL", "1m", &c.ReconcileInterval},
		{"WORKER_BUDGET", "5s", &c.WorkerBudget},
		{"ACCESS_TOKEN_TTL", "15m", &c.AccessTTL},
		{"REFRESH_TOKEN_TTL", "720h", &c.RefreshTTL},
		{"PASSWORD_RESET_TTL", "15m", &c.ResetTTL},
		{"AUTH_TIMEOUT", "5s", &c.AuthTimeout},
		{"SMTP_TIMEOUT", "2s", &c.SMTPTimeout},
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
