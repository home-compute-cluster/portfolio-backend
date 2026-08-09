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
