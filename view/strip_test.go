package view

import (
	"testing"

	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/ast"
	goldmark_renderer "github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/renderer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bytes"
)

func renderWithTransformer(t *testing.T, input string, transformers ...DocumentTransformer) string {
	t.Helper()

	source := []byte(input)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(source))

	for _, tr := range transformers {
		tr(doc, source)
	}

	var buf bytes.Buffer
	r := renderer.New()
	gmr := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	err := gmr.Render(&buf, source, doc)
	require.NoError(t, err)
	return buf.String()
}

func TestStripDataURIs(t *testing.T) {
	t.Run("removes img with data URI", func(t *testing.T) {
		input := "before <img src=\"data:image/png;base64,iVBORw0KGgo=\"> after\n"
		output := renderWithTransformer(t, input, StripDataURIs)

		assert.Contains(t, output, "before")
		assert.Contains(t, output, "after")
		assert.NotContains(t, output, "data:image")
		assert.NotContains(t, output, "iVBOR")
	})

	t.Run("preserves img with normal URL", func(t *testing.T) {
		input := "text <img src=\"https://example.com/img.png\"> more\n"
		output := renderWithTransformer(t, input, StripDataURIs)

		assert.Contains(t, output, "https://example.com/img.png")
	})

	t.Run("removes self-closing img with data URI", func(t *testing.T) {
		input := "hello <img src=\"data:image/gif;base64,R0lGODlh\"/> world\n"
		output := renderWithTransformer(t, input, StripDataURIs)

		assert.Contains(t, output, "hello")
		assert.Contains(t, output, "world")
		assert.NotContains(t, output, "data:image")
	})

	t.Run("removes non-img tag with data URI", func(t *testing.T) {
		input := "text <source src=\"data:audio/mp3;base64,AAAA\"> end\n"
		output := renderWithTransformer(t, input, StripDataURIs)

		assert.Contains(t, output, "text")
		assert.Contains(t, output, "end")
		assert.NotContains(t, output, "data:audio")
	})

	t.Run("no data URIs passes through", func(t *testing.T) {
		input := "plain **markdown** text\n"
		output := renderWithTransformer(t, input, StripDataURIs)

		assert.Contains(t, output, "plain")
		assert.Contains(t, output, "markdown")
		assert.Contains(t, output, "text")
	})

	t.Run("multiple data URI tags removed", func(t *testing.T) {
		input := "a <img src=\"data:image/png;base64,AAA\"> b <img src=\"data:image/png;base64,BBB\"> c\n"
		output := renderWithTransformer(t, input, StripDataURIs)

		assert.Contains(t, output, "a")
		assert.Contains(t, output, "b")
		assert.Contains(t, output, "c")
		assert.NotContains(t, output, "data:image")
	})

	t.Run("preserves surrounding markdown", func(t *testing.T) {
		input := "# Heading\n\nSome text <img src=\"data:image/png;base64,AAA\"> more text.\n\n- list item\n"
		output := renderWithTransformer(t, input, StripDataURIs)

		assert.Contains(t, output, "Heading")
		assert.Contains(t, output, "Some text")
		assert.Contains(t, output, "more text")
		assert.Contains(t, output, "list item")
		assert.NotContains(t, output, "data:image")
	})

	t.Run("no transform without StripDataURIs", func(t *testing.T) {
		input := "text <img src=\"data:image/png;base64,iVBOR\"> end\n"

		// Render without transformer — RawHTML should be present.
		source := []byte(input)
		parser := goldmark.DefaultParser()
		doc := parser.Parse(text.NewReader(source))

		// Verify the RawHTML node exists.
		var found bool
		_ = ast.Walk(doc, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
			if enter {
				if _, ok := n.(*ast.RawHTML); ok {
					found = true
				}
			}
			return ast.WalkContinue, nil
		})
		assert.True(t, found, "RawHTML node should exist without transformer")
	})
}
