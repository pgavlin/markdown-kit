package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogDir_XDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")

	dir, err := logDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/custom/state", "md")
	if dir != want {
		t.Errorf("logDir() = %q, want %q", dir, want)
	}
}

func TestLogDir_DefaultFallback(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	dir, err := logDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir == "" {
		t.Error("expected non-empty log dir")
	}
	sep := string(filepath.Separator)
	if !strings.Contains(sep+dir+sep, sep+"md"+sep) {
		t.Errorf("expected 'md' as a path component, got %q", dir)
	}
}

func TestLogDir_PlatformDefault(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	dir, err := logDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(dir, "Library/Logs/md") {
			t.Errorf("expected darwin log dir to contain Library/Logs/md, got %q", dir)
		}
	case "windows":
		if !strings.Contains(dir, filepath.Join("md", "logs")) {
			t.Errorf("expected windows log dir to contain md/logs, got %q", dir)
		}
	default:
		if !strings.Contains(dir, ".local/state/md") {
			t.Errorf("expected linux log dir to contain .local/state/md, got %q", dir)
		}
	}
}

// capturingHandler is a minimal slog.Handler that records the records it
// receives so tests can assert on log output without touching the real
// logger.
type capturingHandler struct {
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func TestWritePanicReport_LogsAndWritesStderr(t *testing.T) {
	h := &capturingHandler{}
	logger := slog.New(h)
	var stderr bytes.Buffer

	writePanicReport(logger, nil, &stderr, "boom", []byte("goroutine 1 [running]\n..."))

	// Logger received exactly one Error record for the panic.
	require.Len(t, h.records, 1)
	rec := h.records[0]
	assert.Equal(t, slog.LevelError, rec.Level)
	assert.Equal(t, "panic", rec.Message)

	attrs := make(map[string]any)
	rec.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	assert.Equal(t, "boom", attrs["value"])
	assert.Contains(t, attrs["stack"].(string), "goroutine 1")

	// Stderr includes the panic value and the GitHub issues URL.
	out := stderr.String()
	assert.Contains(t, out, "md crashed unexpectedly")
	assert.Contains(t, out, "boom")
	assert.Contains(t, out, "https://github.com/pgavlin/markdown-kit/issues")
	// Without a logFile, no "Details have been written to" line.
	assert.NotContains(t, out, "Details have been written to")
}

func TestWritePanicReport_WithLogFileIncludesPath(t *testing.T) {
	// Use a real temp file so filePath behavior matches production.
	f, err := os.CreateTemp(t.TempDir(), "panic-*.log")
	require.NoError(t, err)
	defer f.Close()

	h := &capturingHandler{}
	logger := slog.New(h)
	var stderr bytes.Buffer

	writePanicReport(logger, f, &stderr, "kaboom", []byte("stack"))

	out := stderr.String()
	assert.Contains(t, out, "Details have been written to:")
	assert.Contains(t, out, f.Name(),
		"stderr should include the log file path so the user can inspect it")
}

func TestWritePanicReport_NonStringPanicValue(t *testing.T) {
	// Panics frequently carry runtime.Error or *os.PathError, not strings.
	// %v must handle them; the report must still be produced.
	type customPanic struct{ Cause string }

	h := &capturingHandler{}
	logger := slog.New(h)
	var stderr bytes.Buffer
	writePanicReport(logger, nil, &stderr, &customPanic{Cause: "divide by zero"}, nil)

	out := stderr.String()
	assert.Contains(t, out, "divide by zero")
	require.Len(t, h.records, 1)
}
