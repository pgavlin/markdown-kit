package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderFixture builds a renderOptions with a fixed theme and deterministic
// settings so output is stable across environments. themeAndLinks are
// included/excluded by the caller to produce multiple snapshots.
func renderFixture(t *testing.T, path string, opts renderOptions) []byte {
	t.Helper()
	source, err := os.ReadFile(path)
	require.NoError(t, err)

	opts.sourceDir = filepath.Dir(path)

	var buf bytes.Buffer
	require.NoError(t, render(&buf, source, opts))
	return buf.Bytes()
}

// compareOrWriteSnapshot enforces the golden-file contract: if the fixture
// is missing, produce it under UPDATE_SNAPSHOTS=1 and skip; otherwise diff.
func compareOrWriteSnapshot(t *testing.T, fixturePath string, got []byte) {
	t.Helper()
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
			require.NoError(t, os.WriteFile(fixturePath, got, 0o644))
			t.Skipf("wrote new snapshot: %s (rerun without UPDATE_SNAPSHOTS)", fixturePath)
		}
		t.Fatalf("missing fixture %s; run with UPDATE_SNAPSHOTS=1 to create", fixturePath)
	}

	want, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	if !bytes.Equal(want, got) {
		if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
			require.NoError(t, os.WriteFile(fixturePath, got, 0o644))
			t.Logf("updated %s", fixturePath)
			return
		}
		assert.Equal(t, string(want), string(got),
			"golden file %s differs; rerun with UPDATE_SNAPSHOTS=1 to update",
			fixturePath)
	}
}

// Basic rendering: styled output, hyperlinks on, width 80. Pins the
// end-to-end rendering pipeline the mdcat binary drives.
func TestRender_BasicStyled(t *testing.T) {
	got := renderFixture(t, "testdata/basic.md", renderOptions{
		width:      80,
		images:     false,
		hyperlinks: true,
		theme:      styles.Pulumi,
	})
	compareOrWriteSnapshot(t, "testdata/basic.styled.golden", got)
}

// No theme, no hyperlinks: deterministic unstyled output. If this drifts
// the change is almost certainly a rendering regression.
func TestRender_BasicPlain(t *testing.T) {
	got := renderFixture(t, "testdata/basic.md", renderOptions{
		width:      80,
		images:     false,
		hyperlinks: false,
		theme:      nil,
	})
	compareOrWriteSnapshot(t, "testdata/basic.plain.golden", got)
}

// Narrower viewport — exercises word wrap.
func TestRender_BasicWrapped40(t *testing.T) {
	got := renderFixture(t, "testdata/basic.md", renderOptions{
		width:      40,
		images:     false,
		hyperlinks: false,
		theme:      nil,
	})
	compareOrWriteSnapshot(t, "testdata/basic.wrapped40.golden", got)
}

// Hyperlinks off: raw link syntax should be visible.
func TestRender_BasicHyperlinksOff(t *testing.T) {
	got := renderFixture(t, "testdata/basic.md", renderOptions{
		width:      80,
		images:     false,
		hyperlinks: false,
		theme:      styles.Pulumi,
	})
	compareOrWriteSnapshot(t, "testdata/basic.nolinks.golden", got)
}
