package migrate

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/testutil/postgrestest"
)

func TestIntegrationEmptyDatabaseMigratesDeterministically(t *testing.T) {
	pool := postgrestest.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	migrationFS := os.DirFS(filepath.Join("..", "..", "..", "migrations"))
	if err := Up(ctx, pool, migrationFS, discardLogger()); err != nil {
		t.Fatalf("first Up() error = %v", err)
	}
	if err := Up(ctx, pool, migrationFS, discardLogger()); err != nil {
		t.Fatalf("second Up() error = %v", err)
	}

	var count int
	var version int64
	var checksumLength int
	err := pool.QueryRow(ctx, `
		SELECT count(*), max(version), max(octet_length(checksum))
		FROM schema_migrations
	`).Scan(&count, &version, &checksumLength)
	if err != nil {
		t.Fatalf("read migration state: %v", err)
	}
	if count != 4 || version != 4 || checksumLength != 32 {
		t.Fatalf("migration state = count %d, version %d, checksum bytes %d, want 4 migrations through version 4", count, version, checksumLength)
	}
}

func TestIntegrationMigrationFailureRollsBackEntireRun(t *testing.T) {
	pool := postgrestest.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	migrationFS := fstest.MapFS{
		"000001_probe.sql":  &fstest.MapFile{Data: []byte("CREATE TABLE migration_probe (id bigint PRIMARY KEY);")},
		"000002_broken.sql": &fstest.MapFile{Data: []byte("THIS IS NOT VALID SQL;")},
	}
	if err := Up(ctx, pool, migrationFS, discardLogger()); err == nil {
		t.Fatal("Up() error = nil, want migration failure")
	}

	var migrationTable *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('schema_migrations')`).Scan(&migrationTable); err != nil {
		t.Fatalf("check migration table: %v", err)
	}
	if migrationTable != nil {
		t.Fatal("schema_migrations persisted after failed atomic run")
	}

	var probeTable *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('migration_probe')`).Scan(&probeTable); err != nil {
		t.Fatalf("check probe table: %v", err)
	}
	if probeTable != nil {
		t.Fatal("migration DDL persisted after failed atomic run")
	}
}

func TestIntegrationAppliedMigrationCannotBeChanged(t *testing.T) {
	pool := postgrestest.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	original := fstest.MapFS{
		"000001_probe.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	if err := Up(ctx, pool, original, discardLogger()); err != nil {
		t.Fatalf("initial Up() error = %v", err)
	}

	changed := fstest.MapFS{
		"000001_probe.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
	}
	err := Up(ctx, pool, changed, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "has been modified") {
		t.Fatalf("changed Up() error = %v, want checksum error", err)
	}
}

func TestIntegrationConcurrentMigrationRunsSerialize(t *testing.T) {
	pool := postgrestest.Open(t)
	migrationFS := fstest.MapFS{
		"000001_probe.sql": &fstest.MapFile{Data: []byte("CREATE TABLE migration_probe (id bigint PRIMARY KEY);")},
	}

	start := make(chan struct{})
	errors := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			errors <- Up(ctx, pool, migrationFS, discardLogger())
		}()
	}
	ready.Wait()
	close(start)

	for range 2 {
		if err := <-errors; err != nil {
			t.Fatalf("concurrent Up() error = %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migration state: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration record count = %d, want 1", count)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
