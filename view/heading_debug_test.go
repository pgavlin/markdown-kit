package view

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadingNavigation(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("getting-started.md", string(source))
	m.SetSize(80, 24)

	require.NotNil(t, m.spanTree)

	// Collect all heading spans.
	var headingStarts []int
	for s := m.spanTree; s != nil; s = s.Next {
		if s.Node.Kind() == ast.KindHeading {
			headingStarts = append(headingStarts, s.Start)
		}
	}
	require.NotEmpty(t, headingStarts, "document should have headings")

	// Navigate forward through all headings with }.
	m.selection = nil
	var forwardVisited []int
	for i := 0; i < len(headingStarts)+2; i++ {
		if !m.SelectNext(isHeading) {
			break
		}
		forwardVisited = append(forwardVisited, m.selection.Start)

		// Verify the heading is visible in the viewport.
		li := m.findLineForOffset(m.selection.Start)
		assert.True(t, li >= m.lineOffset && li < m.lineOffset+m.pageSize,
			"heading at offset %d (line %d) should be visible in viewport [%d, %d)",
			m.selection.Start, li, m.lineOffset, m.lineOffset+m.pageSize)
	}
	assert.Equal(t, headingStarts, forwardVisited,
		"all headings should be reachable via forward navigation")

	// Navigate backward through all headings with {.
	var backwardVisited []int
	for i := 0; i < len(headingStarts)+2; i++ {
		if !m.SelectPrevious(isHeading) {
			break
		}
		backwardVisited = append(backwardVisited, m.selection.Start)
	}
	// Backward should visit all headings except the last (current position) in reverse.
	expected := make([]int, len(headingStarts)-1)
	for i, j := 0, len(headingStarts)-2; j >= 0; i, j = i+1, j-1 {
		expected[i] = headingStarts[j]
	}
	assert.Equal(t, expected, backwardVisited,
		"all prior headings should be reachable via backward navigation")
}
