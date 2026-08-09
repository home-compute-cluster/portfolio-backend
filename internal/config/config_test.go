package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio:password@127.0.0.1:15432/portfolio?sslmode=require")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTP.Address != ":8080" {
		t.Fatalf("HTTP address = %q, want %q", cfg.HTTP.Address, ":8080")
	}
	if cfg.HTTP.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("read header timeout = %s, want %s", cfg.HTTP.ReadHeaderTimeout, 5*time.Second)
	}
	if cfg.Database.MaxConns != 5 || cfg.Database.MinConns != 0 {
		t.Fatalf("pool sizes = (%d, %d), want (5, 0)", cfg.Database.MaxConns, cfg.Database.MinConns)
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	clearEnvironment(t)

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Load() error = %v, want missing DATABASE_URL error", err)
	}
}

func TestLoadRejectsMalformedDuration(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio:password@127.0.0.1:15432/portfolio?sslmode=require")
	t.Setenv("HTTP_READ_TIMEOUT", "eventually")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "HTTP_READ_TIMEOUT") {
		t.Fatalf("Load() error = %v, want HTTP_READ_TIMEOUT error", err)
	}
}

func TestLoadRejectsInvalidPoolBounds(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://portfolio:password@127.0.0.1:15432/portfolio?sslmode=require")
	t.Setenv("DB_MIN_CONNS", "6")
	t.Setenv("DB_MAX_CONNS", "5")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "pool sizes") {
		t.Fatalf("Load() error = %v, want pool size error", err)
	}
}

func clearEnvironment(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"DATABASE_URL",
		"DB_MAX_CONNS",
		"DB_MIN_CONNS",
		"HTTP_ADDR",
		"HTTP_READ_HEADER_TIMEOUT",
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"HTTP_SHUTDOWN_TIMEOUT",
		"DB_CONNECT_TIMEOUT",
		"DB_READINESS_TIMEOUT",
	} {
		t.Setenv(name, "")
	}
}
