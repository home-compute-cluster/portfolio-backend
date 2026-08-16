package content

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestReadManifestRequiresExplicitValidFullSnapshot(t *testing.T) {
	t.Parallel()

	manifest := `{
		"schema_version": 1,
		"source": "portfolio-site",
		"revision": "` + testRevision + `",
		"items": [{
			"slug": "new-reading",
			"kind": "review",
			"status": "published",
			"comments_enabled": false
		}]
	}`
	snapshot, err := ReadManifest(strings.NewReader(manifest))
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].CommentsEnabled {
		t.Fatalf("ReadManifest() = %#v", snapshot)
	}

	tests := map[string]string{
		"unknown field": strings.Replace(manifest, `"source":`, `"unexpected": true, "source":`, 1),
		"missing comments policy": strings.Replace(manifest, `,
			"comments_enabled": false`, "", 1),
		"duplicate slug": strings.Replace(manifest, `}]`, `},{"slug":"new-reading","kind":"post","status":"draft","comments_enabled":true}]`, 1),
		"empty snapshot": strings.Replace(manifest, `[{`+"\n", `[`, 1),
		"trailing value": manifest + `{}`,
	}
	// Replace the empty case directly to keep it syntactically valid.
	tests["empty snapshot"] = `{"schema_version":1,"source":"portfolio-site","revision":"` + testRevision + `","items":[]}`
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ReadManifest(strings.NewReader(value)); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("ReadManifest() error = %v, want ErrInvalidManifest", err)
			}
		})
	}
}

func TestReadManifestRejectsOversizedInput(t *testing.T) {
	t.Parallel()

	_, err := ReadManifest(strings.NewReader(strings.Repeat("x", MaxManifestBytes+1)))
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("ReadManifest() error = %v, want ErrInvalidManifest", err)
	}
}

func TestSyncServiceValidatesBeforeCallingStore(t *testing.T) {
	t.Parallel()

	store := &recordingSyncStore{}
	service := NewSyncService(store)
	if _, err := service.Sync(context.Background(), Snapshot{}); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Sync() error = %v, want ErrInvalidManifest", err)
	}
	if store.called {
		t.Fatal("invalid snapshot reached store")
	}
}

type recordingSyncStore struct {
	called bool
}

func (store *recordingSyncStore) SyncSnapshot(context.Context, Snapshot) (SyncResult, error) {
	store.called = true
	return SyncResult{}, nil
}
