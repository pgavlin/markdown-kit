package view

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopLine_TopOfViewport(t *testing.T) {
	md := "# Top\n\n## Setup\n\nUnique setup body line.\n\n## Usage\n\nUsage body.\n"
	m := newPositionModel(t, md)
	// lineOffset = 0, cursor not positioned. Reference line is the
	// first rendered line, the document title. The renderer preserves
	// the heading marker so the rendered snippet matches the source.
	snippet, occ := m.TopLine()
	assert.Equal(t, "# Top", snippet)
	assert.Equal(t, 0, occ)
}

func TestTopLine_PrefersCursorWhenPositioned(t *testing.T) {
	md := "# Top\n\n## Setup\n\nUnique setup body line.\n\n## Usage\n\nUsage body.\n"
	m := newPositionModel(t, md)
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "Unique setup") {
			m.cursorLine = i
			break
		}
	}
	require.Greater(t, m.cursorLine, 0)

	snippet, occ := m.TopLine()
	assert.Equal(t, "Unique setup body line.", snippet)
	assert.Equal(t, 0, occ)
}

func TestTopLine_DuplicateSnippetReturnsOccurrence(t *testing.T) {
	md := "# Top\n\n## Notes\n\nrepeat\n\nrepeat\n"
	m := newPositionModel(t, md)
	m.cursorMode = true
	m.cursorPositioned = true
	seen := 0
	for i, ln := range m.lines {
		if lineSnippet(ln.content) == "repeat" {
			if seen == 1 {
				m.cursorLine = i
				break
			}
			seen++
		}
	}
	require.Greater(t, m.cursorLine, 0)

	snippet, occ := m.TopLine()
	assert.Equal(t, "repeat", snippet)
	assert.Equal(t, 1, occ)
}

func TestTopLine_EmptyDoc(t *testing.T) {
	m := newPositionModel(t, "")
	snippet, occ := m.TopLine()
	assert.Equal(t, "", snippet)
	assert.Equal(t, 0, occ)
}
