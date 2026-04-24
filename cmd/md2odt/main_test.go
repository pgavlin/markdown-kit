package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_MissingArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(&stdout, &stderr, []string{"md2odt"})
	assert.Equal(t, -1, code, "exit code for missing arg should be -1")
	assert.Contains(t, stderr.String(), "usage:",
		"stderr should carry the usage line, got: %q", stderr.String())
	assert.Zero(t, stdout.Len(), "stdout should be empty on usage error")
}

func TestRun_TooManyArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(&stdout, &stderr, []string{"md2odt", "a.md", "b.md"})
	assert.Equal(t, -1, code)
	assert.Contains(t, stderr.String(), "usage:")
}

func TestRun_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(&stdout, &stderr, []string{"md2odt", "/nonexistent/does-not-exist.md"})
	assert.Equal(t, -1, code)
	assert.Contains(t, stderr.String(), "failed to read")
	assert.Zero(t, stdout.Len())
}

// Golden(ish) structural test: convert a small fixture and assert the
// output is a valid ODF ZIP with the required entries.
func TestRun_ProducesValidODFZIP(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# Title\n\nBody paragraph.\n"), 0o644))

	var stdout, stderr bytes.Buffer
	code := run(&stdout, &stderr, []string{"md2odt", path})
	require.Equal(t, 0, code, "expected success, stderr=%q", stderr.String())
	require.Zero(t, stderr.Len(), "stderr should be empty on success")
	require.Greater(t, stdout.Len(), 0)

	// The output must be a valid ZIP archive.
	zr, err := zip.NewReader(bytes.NewReader(stdout.Bytes()), int64(stdout.Len()))
	require.NoError(t, err, "output should be a valid ZIP")

	names := make(map[string]bool, len(zr.File))
	for _, f := range zr.File {
		names[f.Name] = true
	}
	// Mandatory ODF entries.
	for _, entry := range []string{"mimetype", "META-INF/manifest.xml", "content.xml"} {
		assert.True(t, names[entry], "ODF archive must contain %q; got %v", entry, keysOf(names))
	}

	// The mimetype entry must be first in the archive and uncompressed
	// (per the ODF spec), and contain the OpenDocument text MIME type.
	require.NotEmpty(t, zr.File, "archive is empty")
	mimetype := zr.File[0]
	assert.Equal(t, "mimetype", mimetype.Name, "mimetype must be first entry")
	assert.Equal(t, uint16(zip.Store), mimetype.Method, "mimetype must be stored uncompressed")

	rc, err := mimetype.Open()
	require.NoError(t, err)
	defer rc.Close()
	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(body), "application/vnd.oasis.opendocument.text"),
		"mimetype contents wrong: %q", body)
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
