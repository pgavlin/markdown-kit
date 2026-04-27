package view

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMetadataModel(t *testing.T, md string) *Model {
	t.Helper()
	m := NewModel(WithTheme(styles.Pulumi), WithGutter(true), WithWidth(80), WithHeight(25))
	m.SetText("doc.md", md)
	return &m
}

func TestSetText_PopulatesFrontmatter(t *testing.T) {
	m := newMetadataModel(t, "---\ntitle: Hello\ntags: [a, b]\n---\n\n# Body\n")
	fm := m.Frontmatter()
	require.NotNil(t, fm)
	assert.Equal(t, "Hello", fm["title"])
	assert.Equal(t, []any{"a", "b"}, fm["tags"])
}

func TestSetText_NoFrontmatterReturnsNil(t *testing.T) {
	m := newMetadataModel(t, "# Body only\n")
	assert.Nil(t, m.Frontmatter())
}

func TestSetText_MalformedFrontmatterReturnsNil(t *testing.T) {
	m := newMetadataModel(t, "---\ninvalid: [unclosed\n---\n\n# Body\n")
	assert.Nil(t, m.Frontmatter(),
		"malformed YAML should leave Frontmatter() at nil rather than panicking")
}

func TestSetSourcePath_RoundTrip(t *testing.T) {
	m := newMetadataModel(t, "# x\n")
	assert.Equal(t, "", m.SourcePath())
	m.SetSourcePath("/tmp/foo.md")
	assert.Equal(t, "/tmp/foo.md", m.SourcePath())
}

func TestMetadataOverlay_OpenAndClose(t *testing.T) {
	m := newMetadataModel(t, "---\ntitle: Hello\n---\n\n# Body\n")
	require.False(t, m.MetadataActive())

	pressKey(t, m, "m")
	assert.True(t, m.MetadataActive())

	pressKey(t, m, "esc")
	assert.False(t, m.MetadataActive())
}

func TestMetadataOverlay_ToggleKeyClosesOverlay(t *testing.T) {
	m := newMetadataModel(t, "---\ntitle: Hello\n---\n\n# Body\n")
	pressKey(t, m, "m")
	require.True(t, m.MetadataActive())
	// Pressing 'm' again should dismiss.
	pressKey(t, m, "m")
	assert.False(t, m.MetadataActive())
}

func TestMetadataOverlay_RendersFilePath(t *testing.T) {
	m := newMetadataModel(t, "# Body\n")
	m.SetSourcePath("/tmp/example.md")
	pressKey(t, m, "m")
	out := ansi.Strip(m.View())
	assert.Contains(t, out, "Path",
		"path label should appear in the metadata overlay")
	assert.Contains(t, out, "/tmp/example.md",
		"actual path string should appear in the overlay")
}

func TestMetadataOverlay_RendersFrontmatterKeys(t *testing.T) {
	m := newMetadataModel(t, "---\ntitle: Hello\nauthor: Pat\ntags: [a, b]\n---\n\n# Body\n")
	pressKey(t, m, "m")
	out := ansi.Strip(m.View())

	assert.Contains(t, out, "title")
	assert.Contains(t, out, "Hello")
	assert.Contains(t, out, "author")
	assert.Contains(t, out, "Pat")
	assert.Contains(t, out, "tags")
	// Tag list rendered as comma-separated.
	assert.Contains(t, out, "a, b")
}

func TestMetadataOverlay_NoFrontmatterStillShowsName(t *testing.T) {
	// A document without frontmatter should still render the overlay
	// with at least the document name (and source path if set).
	m := newMetadataModel(t, "# Body\n")
	pressKey(t, m, "m")
	out := ansi.Strip(m.View())
	assert.Contains(t, out, "doc.md",
		"document name should appear when no frontmatter present")
}

func TestMetadataOverlay_TerminalTooSmallShowsStatus(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithGutter(true), WithWidth(20), WithHeight(5))
	m.SetText("doc.md", "# x\n")
	mp := &m
	pressKey(t, mp, "m")
	assert.False(t, mp.MetadataActive())
	assert.Equal(t, "Terminal too small for metadata", mp.statusMessage)
}

func TestFormatMetadataValue_Types(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{42, "42"},
		{3.14, "3.14"},
		{[]any{"a", "b", "c"}, "a, b, c"},
		{map[string]any{"a": 1, "b": "two"}, "a=1, b=two"},
	}
	for _, c := range cases {
		got := formatMetadataValue(c.in)
		assert.Equal(t, c.want, got, "input %#v", c.in)
	}
}

func TestMetadataOverlay_KeysSortedDeterministically(t *testing.T) {
	// With many keys, the output order must be stable across opens.
	m := newMetadataModel(t, "---\nzeta: 1\nalpha: 2\nmiddle: 3\n---\n\n# Body\n")
	pressKey(t, m, "m")
	out := ansi.Strip(m.View())
	// alpha should appear before middle, which appears before zeta.
	a := strings.Index(out, "alpha")
	mid := strings.Index(out, "middle")
	z := strings.Index(out, "zeta")
	require.Greater(t, a, 0)
	require.Greater(t, mid, a, "middle should follow alpha")
	require.Greater(t, z, mid, "zeta should follow middle")
}

func TestMetadataOverlay_BodyParsesNormallyAfterFrontmatter(t *testing.T) {
	m := newMetadataModel(t, "---\ntitle: x\n---\n\n# Heading\n\nA paragraph.\n")
	out := ansi.Strip(m.View())
	// The body should still render.
	assert.Contains(t, out, "Heading")
	assert.Contains(t, out, "A paragraph.")
	// And frontmatter content should NOT bleed into the body output.
	assert.NotContains(t, out, "title:",
		"YAML body must not appear as document content")
}
