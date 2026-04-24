package odt

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests validate the generated ODT files against the official ODF 1.3
// RELAX NG schema using xmllint. xmllint is not a Go dependency; if it isn't
// installed the tests skip with a clear message. On the project's CI (which
// uses ubuntu-latest / macos-latest / windows-latest) xmllint ships with
// libxml2 and is available everywhere except base Windows — on Windows the
// tests skip gracefully.

const (
	contentSchemaPath  = "testdata/schemas/OpenDocument-v1.3-schema.rng"
	manifestSchemaPath = "testdata/schemas/OpenDocument-v1.3-manifest-schema.rng"
)

// validateAgainstSchema runs xmllint against schemaPath with the given XML
// bytes. It returns the combined stderr/stdout and the command error. The
// caller is responsible for skipping if xmllint is unavailable.
func validateAgainstSchema(t *testing.T, schemaPath string, xml []byte) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("xmllint", "--noout", "--relaxng", schemaPath, "-")
	cmd.Stdin = bytes.NewReader(xml)
	return cmd.CombinedOutput()
}

// requireXMLLint skips the test if xmllint isn't on PATH.
func requireXMLLint(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Skipf("xmllint not available; skipping schema validation (%v)", err)
	}
}

// extractODFPart returns the contents of the named file inside the ODT ZIP.
func extractODFPart(t *testing.T, odt []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(odt), int64(len(odt)))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		defer rc.Close()
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		return data
	}
	t.Fatalf("ODT archive does not contain %q", name)
	return nil
}

func renderODT(t *testing.T, source string) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, FromMarkdown(&buf, []byte(source)))
	return buf.Bytes()
}

// Basic structure: a single paragraph. Smoke test that the bare minimum
// output is valid.
func TestSchemaValidates_MinimalDocument(t *testing.T) {
	requireXMLLint(t)
	odt := renderODT(t, "A single paragraph.\n")
	content := extractODFPart(t, odt, "content.xml")
	out, err := validateAgainstSchema(t, contentSchemaPath, content)
	require.NoError(t, err, "content.xml should validate against ODF 1.3 schema:\n%s", out)
}

// Exercise every markdown feature the renderer handles, so every branch
// of the emitter is checked against the schema in one shot.
func TestSchemaValidates_AllFeatures(t *testing.T) {
	requireXMLLint(t)
	source := `# Heading 1

## Heading 2

A paragraph with **bold**, *italic*, ` + "`code`" + `, and a [link](https://example.com).

> A blockquote.
>
> With a second paragraph.
>
> > And a nested blockquote.

1. Ordered
2. Items
   1. Nested
   2. Items
3. Back to outer

- Unordered
- Items
  - Nested
  - Items

---

` + "```go\nfmt.Println(\"hi\")\n```\n" + `

Inline ` + "`code`" + ` and <https://autolink.example>.

Email autolink: <user@example.com>.
`
	odt := renderODT(t, source)
	content := extractODFPart(t, odt, "content.xml")
	out, err := validateAgainstSchema(t, contentSchemaPath, content)
	require.NoError(t, err, "content.xml should validate:\n%s", out)
}

// Manifest must validate against its dedicated schema.
func TestSchemaValidates_Manifest(t *testing.T) {
	requireXMLLint(t)
	odt := renderODT(t, "# Title\n")
	manifest := extractODFPart(t, odt, "META-INF/manifest.xml")
	out, err := validateAgainstSchema(t, manifestSchemaPath, manifest)
	require.NoError(t, err, "manifest.xml should validate:\n%s", out)
}

// Cross-check: the committed golden fixture must also validate. Catches
// drift if someone regenerates the golden without running validation.
func TestSchemaValidates_GoldenFixture(t *testing.T) {
	requireXMLLint(t)
	path := filepath.Join(testdataPath, "getting-started.odt")
	odt, err := os.ReadFile(path)
	require.NoError(t, err)

	content := extractODFPart(t, odt, "content.xml")
	out, err := validateAgainstSchema(t, contentSchemaPath, content)
	require.NoError(t, err, "golden fixture content.xml should validate:\n%s", out)
}
