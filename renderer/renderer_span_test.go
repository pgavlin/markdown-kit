package renderer

import (
	"bytes"
	"testing"

	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanTree(t *testing.T) {
	const input = `# Header

Here's some *text*.

# Header 2

Here's a [link](http://google.com).
`

	source := []byte(input)
	parser := goldmark.DefaultParser()
	document := parser.Parse(text.NewReader(source))

	var buf bytes.Buffer
	r := New()
	renderer := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	err := renderer.Render(&buf, source, document)
	require.NoError(t, err)

	require.Equal(t, input, buf.String())

	tree := r.SpanTree()

	assert.Equal(t, 0, tree.Start)
	assert.Equal(t, len(input), tree.End)

	assert.Len(t, tree.Children, 4)

	_, ok := tree.Children[0].Node.(*ast.Heading)
	assert.True(t, ok)
	assert.Equal(t, 0, tree.Children[0].Start)
	assert.Equal(t, 9, tree.Children[0].End)

	_, ok = tree.Children[1].Node.(*ast.Paragraph)
	assert.True(t, ok)
	assert.Equal(t, 9, tree.Children[1].Start)
	assert.Equal(t, 30, tree.Children[1].End)

	_, ok = tree.Children[2].Node.(*ast.Heading)
	assert.True(t, ok)
	assert.Equal(t, 30, tree.Children[2].Start)
	assert.Equal(t, 42, tree.Children[2].End)

	_, ok = tree.Children[3].Node.(*ast.Paragraph)
	assert.True(t, ok)
	assert.Equal(t, 42, tree.Children[3].Start)
	assert.Equal(t, len(input), tree.Children[3].End)

}
