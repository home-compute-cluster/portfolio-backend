package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	ManifestSchemaVersion = 1
	MaxManifestBytes      = 1 << 20
	MaxManifestItems      = 10_000
	MaxKindChars          = 50
	MaxSourceChars        = 50
	MaxRevisionChars      = 64
)

var ErrInvalidManifest = errors.New("invalid content manifest")

// Status is the publication state synchronized from the frontend repository.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

// Item is one frontend-owned content identity and its dynamic-feature policy.
type Item struct {
	Slug            string `json:"slug"`
	Kind            string `json:"kind"`
	Status          Status `json:"status"`
	CommentsEnabled bool   `json:"comments_enabled"`
}

// Snapshot is a complete view of content owned by one frontend source. Items
// omitted from a valid snapshot are archived by the synchronizer.
type Snapshot struct {
	SchemaVersion int    `json:"schema_version"`
	Source        string `json:"source"`
	Revision      string `json:"revision"`
	Items         []Item `json:"items"`
}

// SyncResult summarizes database rows changed by an authoritative snapshot.
type SyncResult struct {
	Changed  int64
	Archived int64
}

// SyncStore persists a complete snapshot as one correctness boundary.
type SyncStore interface {
	SyncSnapshot(ctx context.Context, snapshot Snapshot) (SyncResult, error)
}

// SyncService validates trusted deployment input before changing the registry.
type SyncService struct {
	store SyncStore
}

func NewSyncService(store SyncStore) *SyncService {
	return &SyncService{store: store}
}

// Sync validates and atomically applies one complete source snapshot.
func (service *SyncService) Sync(ctx context.Context, snapshot Snapshot) (SyncResult, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return SyncResult{}, err
	}

	return service.store.SyncSnapshot(ctx, snapshot)
}

// ReadManifest strictly decodes a bounded JSON manifest and validates its full
// snapshot contract. comments_enabled is required so omission cannot silently
// switch a collection's policy.
func ReadManifest(reader io.Reader) (Snapshot, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, MaxManifestBytes+1))
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: read JSON: %v", ErrInvalidManifest, err)
	}
	if len(payload) > MaxManifestBytes {
		return Snapshot{}, fmt.Errorf("%w: exceeds %d bytes", ErrInvalidManifest, MaxManifestBytes)
	}

	type wireItem struct {
		Slug            string `json:"slug"`
		Kind            string `json:"kind"`
		Status          Status `json:"status"`
		CommentsEnabled *bool  `json:"comments_enabled"`
	}
	var wire struct {
		SchemaVersion int        `json:"schema_version"`
		Source        string     `json:"source"`
		Revision      string     `json:"revision"`
		Items         []wireItem `json:"items"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Snapshot{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidManifest, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return Snapshot{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidManifest, err)
	}

	snapshot := Snapshot{
		SchemaVersion: wire.SchemaVersion,
		Source:        wire.Source,
		Revision:      wire.Revision,
		Items:         make([]Item, len(wire.Items)),
	}
	for index, item := range wire.Items {
		if item.CommentsEnabled == nil {
			return Snapshot{}, fmt.Errorf("%w: item %d omits comments_enabled", ErrInvalidManifest, index)
		}
		snapshot.Items[index] = Item{
			Slug:            item.Slug,
			Kind:            item.Kind,
			Status:          item.Status,
			CommentsEnabled: *item.CommentsEnabled,
		}
	}

	if err := ValidateSnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// ValidateSnapshot enforces the same identity shapes as PostgreSQL before a
// transaction starts. An empty snapshot is rejected as likely build damage.
func ValidateSnapshot(snapshot Snapshot) error {
	if snapshot.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("%w: schema_version must be %d", ErrInvalidManifest, ManifestSchemaVersion)
	}
	if !validIdentifier(snapshot.Source, MaxSourceChars, true) {
		return fmt.Errorf("%w: invalid source", ErrInvalidManifest)
	}
	if !validRevision(snapshot.Revision) {
		return fmt.Errorf("%w: revision must be a 7 to 64 character lowercase hexadecimal commit ID", ErrInvalidManifest)
	}
	if len(snapshot.Items) == 0 || len(snapshot.Items) > MaxManifestItems {
		return fmt.Errorf("%w: items must contain between 1 and %d entries", ErrInvalidManifest, MaxManifestItems)
	}

	seen := make(map[string]struct{}, len(snapshot.Items))
	for index, item := range snapshot.Items {
		if !ValidSlug(item.Slug) {
			return fmt.Errorf("%w: item %d has invalid slug", ErrInvalidManifest, index)
		}
		if !ValidKind(item.Kind) {
			return fmt.Errorf("%w: item %d has invalid kind", ErrInvalidManifest, index)
		}
		if item.Status != StatusDraft && item.Status != StatusPublished && item.Status != StatusArchived {
			return fmt.Errorf("%w: item %d has invalid status", ErrInvalidManifest, index)
		}
		if _, duplicate := seen[item.Slug]; duplicate {
			return fmt.Errorf("%w: duplicate slug %q", ErrInvalidManifest, item.Slug)
		}
		seen[item.Slug] = struct{}{}
	}

	return nil
}

// ValidKind reports whether kind is a bounded lower-kebab identifier.
func ValidKind(kind string) bool {
	return validIdentifier(kind, MaxKindChars, true)
}

func validIdentifier(value string, maximum int, firstMustBeLetter bool) bool {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value {
		return false
	}

	previousHyphen := false
	for index, character := range value {
		isLetter := character >= 'a' && character <= 'z'
		isDigit := character >= '0' && character <= '9'
		if isLetter || (isDigit && (!firstMustBeLetter || index > 0)) {
			previousHyphen = false
			continue
		}
		if character != '-' || index == 0 || previousHyphen {
			return false
		}
		previousHyphen = true
	}

	return !previousHyphen
}

func validRevision(revision string) bool {
	if len(revision) < 7 || len(revision) > MaxRevisionChars {
		return false
	}
	for _, character := range revision {
		if !((character >= 'a' && character <= 'f') || (character >= '0' && character <= '9')) {
			return false
		}
	}
	return true
}
