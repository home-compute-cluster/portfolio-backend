package migrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockKey int64 = 0x7061636b65746372

var migrationFilename = regexp.MustCompile(`^([0-9]{6})_([a-z0-9][a-z0-9_]*)\.sql$`)

type Migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum [sha256.Size]byte
}

type appliedMigration struct {
	name     string
	checksum []byte
}

func Up(
	ctx context.Context,
	pool *pgxpool.Pool,
	migrationFS fs.FS,
	logger *slog.Logger,
) error {
	migrations, err := Load(migrationFS)
	if err != nil {
		return err
	}

	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint PRIMARY KEY CHECK (version > 0),
			name text NOT NULL UNIQUE CHECK (name <> ''),
			checksum bytea NOT NULL CHECK (octet_length(checksum) = 32),
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	applied, err := loadApplied(ctx, tx)
	if err != nil {
		return err
	}

	known := make(map[int64]Migration, len(migrations))
	for _, migration := range migrations {
		known[migration.Version] = migration
	}

	for version, state := range applied {
		migration, exists := known[version]
		if !exists {
			return fmt.Errorf("applied migration version %06d is missing from the migration directory", version)
		}
		if state.name != migration.Name {
			return fmt.Errorf("applied migration version %06d has name %q, expected %q", version, state.name, migration.Name)
		}
		if !bytes.Equal(state.checksum, migration.Checksum[:]) {
			return fmt.Errorf("applied migration %s has been modified", migration.Name)
		}
	}

	appliedNow := make([]Migration, 0, len(migrations))
	for _, migration := range migrations {
		if _, exists := applied[migration.Version]; exists {
			continue
		}

		if _, err := tx.Exec(ctx, migration.SQL, pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.Name, err)
		}

		if _, err := tx.Exec(
			ctx,
			`INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
			migration.Version,
			migration.Name,
			migration.Checksum[:],
		); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.Name, err)
		}

		appliedNow = append(appliedNow, migration)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}

	for _, migration := range appliedNow {
		logger.Info("migration applied", "version", migration.Version, "name", migration.Name)
	}

	return nil
}

func Load(migrationFS fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}

	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		matches := migrationFilename.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("migration filename %q must match NNNNNN_name.sql", entry.Name())
		}

		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse migration version in %q: %w", entry.Name(), err)
		}

		contents, err := fs.ReadFile(migrationFS, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if len(bytes.TrimSpace(contents)) == 0 {
			return nil, fmt.Errorf("migration %s is empty", entry.Name())
		}

		migrations = append(migrations, Migration{
			Version:  version,
			Name:     entry.Name(),
			SQL:      string(contents),
			Checksum: sha256.Sum256(contents),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	if len(migrations) == 0 {
		return nil, fmt.Errorf("migration directory contains no versioned SQL files")
	}

	for index, migration := range migrations {
		expected := int64(index + 1)
		if migration.Version != expected {
			return nil, fmt.Errorf("migration sequence must contain version %06d, found %06d", expected, migration.Version)
		}
	}

	return migrations, nil
}

func loadApplied(ctx context.Context, tx pgx.Tx) (map[int64]appliedMigration, error) {
	rows, err := tx.Query(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read migration state: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]appliedMigration)
	for rows.Next() {
		var version int64
		var state appliedMigration
		if err := rows.Scan(&version, &state.name, &state.checksum); err != nil {
			return nil, fmt.Errorf("scan migration state: %w", err)
		}
		applied[version] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration state: %w", err)
	}

	return applied, nil
}
