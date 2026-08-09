package postgrestest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const operationTimeout = 10 * time.Second

// Open creates a pool whose search path points at a unique temporary schema.
// The schema and everything in it are removed automatically when the test ends.
func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL is required in CI")
		}
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}

	adminConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("TEST_DATABASE_URL is not a valid PostgreSQL connection string")
	}
	adminConfig.MaxConns = 2
	adminConfig.ConnConfig.ConnectTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("create integration-test admin pool: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatalf("connect to integration-test PostgreSQL: %v", err)
	}

	schema := "portfolio_test_" + randomSuffix(t)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		adminPool.Close()
		t.Fatalf("create isolated test schema: %v", err)
	}

	testConfig := adminConfig.Copy()
	testConfig.ConnConfig.RuntimeParams = cloneRuntimeParams(adminConfig.ConnConfig.RuntimeParams)
	testConfig.ConnConfig.RuntimeParams["search_path"] = schema

	testPool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		dropSchema(t, adminPool, identifier)
		adminPool.Close()
		t.Fatalf("create isolated test pool: %v", err)
	}
	if err := testPool.Ping(ctx); err != nil {
		testPool.Close()
		dropSchema(t, adminPool, identifier)
		adminPool.Close()
		t.Fatalf("connect to isolated test schema: %v", err)
	}

	t.Cleanup(func() {
		testPool.Close()
		dropSchema(t, adminPool, identifier)
		adminPool.Close()
	})

	return testPool
}

func randomSuffix(t *testing.T) string {
	t.Helper()

	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatalf("generate test schema name: %v", err)
	}

	return hex.EncodeToString(bytes)
}

func cloneRuntimeParams(source map[string]string) map[string]string {
	result := make(map[string]string, len(source)+1)
	for key, value := range source {
		result[key] = value
	}

	return result
}

func dropSchema(t *testing.T, pool *pgxpool.Pool, identifier string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	if _, err := pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", identifier)); err != nil {
		t.Errorf("drop isolated test schema: %v", err)
	}
}
