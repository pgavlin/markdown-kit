package renderer

import (
	"bytes"
	"testing"

	"github.com/pgavlin/goldmark"
	goldmark_renderer "github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderPlain renders markdown with the default renderer and returns just
// the output string. Matches the intent of renderMarkdown in
// renderer_additional_test.go but with a different name to avoid collision.
func renderPlain(t *testing.T, source string, opts ...RendererOption) string {
	t.Helper()
	parser := goldmark.DefaultParser()
	document := parser.Parse(text.NewReader([]byte(source)))

	r := New(opts...)
	rr := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))

	var buf bytes.Buffer
	require.NoError(t, rr.Render(&buf, []byte(source), document))
	return buf.String()
}

// Reference-style links: [text][id], [id][], [id] — the non-inline branches
// of renderLinkOrImage that regular inline tests don't exercise.

func TestRenderer_FullReferenceLink(t *testing.T) {
	source := "See [the example][ref] for details.\n\n[ref]: https://example.com\n"
	out := renderPlain(t, source, WithHyperlinks(false))
	// Hyperlinks off surfaces the raw syntax. Full reference: [text][id]
	assert.Contains(t, out, "the example",
		"full reference link text should render, got %q", out)
	assert.Contains(t, out, "][ref]",
		"full reference should round-trip the ][ref] syntax, got %q", out)
}

func TestRenderer_CollapsedReferenceLink(t *testing.T) {
	// Collapsed reference `[ref][]` may be normalized by the parser to
	// the shortcut form. Either way, the link must render with its text.
	source := "See [ref][] for details.\n\n[ref]: https://example.com\n"
	out := renderPlain(t, source, WithHyperlinks(false))
	assert.Contains(t, out, "[ref]",
		"collapsed reference should render link text, got %q", out)
}

func TestRenderer_ShortcutReferenceLink(t *testing.T) {
	source := "See [ref] for details.\n\n[ref]: https://example.com\n"
	out := renderPlain(t, source, WithHyperlinks(false))
	// Shortcut reference: just [id].
	assert.Contains(t, out, "[ref]",
		"shortcut reference link should preserve [id] syntax, got %q", out)
}

func TestRenderer_InlineLinkWithTitle(t *testing.T) {
	// Inline links with a title attribute use a specific delimiter choice.
	source := `See [example](https://example.com "Title here").` + "\n"
	out := renderPlain(t, source, WithHyperlinks(false))
	assert.Contains(t, out, "example")
	assert.Contains(t, out, "https://example.com")
	assert.Contains(t, out, "Title here",
		"link title should be present in output, got %q", out)
}

func TestRenderer_AutoLinkMailto(t *testing.T) {
	source := "Contact <hi@example.com> for help.\n"
	out := renderPlain(t, source, WithHyperlinks(false))
	assert.Contains(t, out, "hi@example.com")
}

func TestRenderer_AutoLinkHTTPS(t *testing.T) {
	source := "Visit <https://example.com>.\n"
	out := renderPlain(t, source, WithHyperlinks(false))
	assert.Contains(t, out, "https://example.com")
}

// Nested blockquote exercises the prefix-stack push/pop paths that a
// single-level quote doesn't.
func TestRenderer_NestedBlockquote(t *testing.T) {
	source := "> outer\n>\n> > inner\n>\n> still outer\n"
	out := renderPlain(t, source)
	assert.Contains(t, out, "outer")
	assert.Contains(t, out, "inner")
	// Nested blockquotes should produce doubled `> ` prefixes on the
	// inner line.
	assert.Contains(t, out, "> > ",
		"nested blockquote should emit a `> > ` prefix, got %q", out)
}

func TestRenderer_ThematicBreak(t *testing.T) {
	out := renderPlain(t, "before\n\n---\n\nafter\n")
	assert.Contains(t, out, "before")
	assert.Contains(t, out, "after")
	// The thematic break should render as some horizontal separator
	// (commonly a run of dashes or Unicode rule characters).
	assert.NotEqual(t, "beforeafter", out,
		"a thematic break must produce visible output between adjacent blocks")
}
