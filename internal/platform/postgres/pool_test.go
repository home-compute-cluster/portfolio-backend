package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/home-compute-cluster/portfolio-backend/internal/config"
)

func TestOpenDoesNotLeakMalformedConnectionString(t *testing.T) {
	const secret = "do-not-log-this-password"

	_, err := Open(context.Background(), config.DatabaseConfig{
		URL: "postgres://portfolio:" + secret + "@%invalid-host/portfolio",
	})
	if err == nil {
		t.Fatal("Open() error = nil, want malformed connection string error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("Open() error leaked a database password")
	}
}

func TestOpenAppliesPasswordOverride(t *testing.T) {
	const password = "configured-password"

	pool, err := Open(context.Background(), config.DatabaseConfig{
		URL:      "postgres://portfolio:placeholder@127.0.0.1:15432/portfolio?sslmode=require",
		Password: password,
		MaxConns: 5,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(pool.Close)

	if pool.Config().ConnConfig.Password != password {
		t.Fatal("pool connection password did not use DB_PASSWORD override")
	}
}

func TestOpenKeepsURLPasswordWithoutOverride(t *testing.T) {
	const password = "url-password"

	pool, err := Open(context.Background(), config.DatabaseConfig{
		URL:      "postgres://portfolio:" + password + "@127.0.0.1:15432/portfolio?sslmode=require",
		MaxConns: 5,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(pool.Close)

	if pool.Config().ConnConfig.Password != password {
		t.Fatal("pool connection password did not preserve DATABASE_URL password")
	}
}
