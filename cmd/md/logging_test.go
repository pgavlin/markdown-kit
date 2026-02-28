package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	if !strings.HasSuffix(dir, "md") {
		t.Errorf("expected dir to end with 'md', got %q", dir)
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
