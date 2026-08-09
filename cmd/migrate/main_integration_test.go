package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestIntegrationMigrationFailureReturnsNonZero(t *testing.T) {
	testDatabaseURL := os.Getenv("TEST_DATABASE_URL")
	if testDatabaseURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL is required in CI")
		}
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}

	migrationDir := t.TempDir()
	brokenMigration := filepath.Join(migrationDir, "000001_broken.sql")
	if err := os.WriteFile(brokenMigration, []byte("THIS IS NOT VALID SQL;"), 0o600); err != nil {
		t.Fatalf("write broken migration: %v", err)
	}

	t.Setenv("DATABASE_URL", testDatabaseURL)
	t.Setenv("DB_PASSWORD", "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if code := execute([]string{"-dir", migrationDir}, logger); code == 0 {
		t.Fatal("execute() code = 0, want non-zero for failed migration")
	}
}
