package main

import (
	"io"
	"log/slog"
	"testing"
)

func TestExecuteRequiresManifestPath(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if code := execute(nil, logger); code != 2 {
		t.Fatalf("execute() code = %d, want 2", code)
	}
}
