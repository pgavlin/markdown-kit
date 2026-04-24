package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataDir_XDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	dir, err := dataDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/custom/data", "md"), dir,
		"XDG_DATA_HOME should win over platform defaults")
}

func TestDataDir_PlatformDefault(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir, err := dataDir()
	require.NoError(t, err)
	assert.NotEmpty(t, dir)

	switch runtime.GOOS {
	case "darwin":
		assert.Contains(t, dir, filepath.Join("Library", "Application Support", "md"),
			"darwin default should use Library/Application Support/md, got %q", dir)
	case "windows":
		assert.Contains(t, dir, filepath.Join("md", "data"),
			"windows default should use md/data, got %q", dir)
	default:
		assert.Contains(t, dir, filepath.Join(".local", "share", "md"),
			"linux default should use .local/share/md, got %q", dir)
	}
}

func TestDataDir_WindowsLOCALAPPDATA(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific test")
	}
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOCALAPPDATA", `C:\Users\test\AppData\Local`)
	dir, err := dataDir()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(dir, filepath.Join("md", "data")),
		"expected windows LOCALAPPDATA-based path, got %q", dir)
	assert.Contains(t, dir, `AppData\Local`)
}
