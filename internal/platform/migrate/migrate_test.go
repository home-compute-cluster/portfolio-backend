package migrate

import (
	"crypto/sha256"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadOrdersAndChecksumsMigrations(t *testing.T) {
	firstSQL := []byte("SELECT 1;\n")
	migrationFS := fstest.MapFS{
		"README.md":             &fstest.MapFile{Data: []byte("documentation")},
		"000002_second.sql":     &fstest.MapFile{Data: []byte("SELECT 2;\n")},
		"000001_baseline.sql":   &fstest.MapFile{Data: firstSQL},
		"notes/ignored.sql.txt": &fstest.MapFile{Data: []byte("ignored")},
	}

	migrations, err := Load(migrationFS)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("migration count = %d, want 2", len(migrations))
	}
	if migrations[0].Name != "000001_baseline.sql" || migrations[1].Name != "000002_second.sql" {
		t.Fatalf("unexpected migration order: %q, %q", migrations[0].Name, migrations[1].Name)
	}
	if migrations[0].Checksum != sha256.Sum256(firstSQL) {
		t.Fatal("migration checksum does not match file contents")
	}
}

func TestLoadRejectsInvalidMigrationFilename(t *testing.T) {
	_, err := Load(fstest.MapFS{
		"1_bad.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	})
	if err == nil || !strings.Contains(err.Error(), "NNNNNN_name.sql") {
		t.Fatalf("Load() error = %v, want filename error", err)
	}
}

func TestLoadRejectsMissingVersion(t *testing.T) {
	_, err := Load(fstest.MapFS{
		"000001_first.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
		"000003_third.sql": &fstest.MapFile{Data: []byte("SELECT 3;")},
	})
	if err == nil || !strings.Contains(err.Error(), "version 000002") {
		t.Fatalf("Load() error = %v, want missing version error", err)
	}
}

func TestLoadRejectsEmptyMigration(t *testing.T) {
	_, err := Load(fstest.MapFS{
		"000001_empty.sql": &fstest.MapFile{Data: []byte(" \n\t")},
	})
	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("Load() error = %v, want empty migration error", err)
	}
}

func TestLoadRejectsDirectoryWithoutMigrations(t *testing.T) {
	_, err := Load(fstest.MapFS{
		"README.md": &fstest.MapFile{Data: []byte("documentation")},
	})
	if err == nil || !strings.Contains(err.Error(), "no versioned SQL") {
		t.Fatalf("Load() error = %v, want no migrations error", err)
	}
}
