package main

import (
	"path/filepath"
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
