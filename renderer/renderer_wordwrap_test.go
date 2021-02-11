package renderer

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testdataPath = filepath.Join("..", "internal", "testdata")

func TestWordWrap(t *testing.T) {
	input, err := ioutil.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	source := input
	parser := goldmark.DefaultParser()
	document := parser.Parse(text.NewReader(source))

	cases := []int{80, 120}
	for _, wrap := range cases {
		filename := fmt.Sprintf("getting-started.wrapped.%d.md", wrap)
		t.Run(filename, func(t *testing.T) {
			expected, err := ioutil.ReadFile(filepath.Join(testdataPath, filename))
			require.NoError(t, err)

			var buf bytes.Buffer
			r := New(WithWordWrap(wrap))
			renderer := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(r, 100)))
			err = renderer.Render(&buf, source, document)
			require.NoError(t, err)

			assert.Equal(t, string(expected), buf.String())
		})
	}
}
