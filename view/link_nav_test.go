package view

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBracketNavigation_PrefersLinks(t *testing.T) {
	// Document with both links and headings — ] should prefer links.
	source, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("getting-started.md", string(source))
	m.SetSize(80, 24)

	// Press ] — should select a link (not a heading).
	m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	require.NotNil(t, m.Selection())
	assert.Equal(t, ast.KindLink, m.Selection().Node.Kind(), "] should select a link when links exist")

	first := m.Selection().Start

	// Press ] again — should advance to next link.
	m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	require.NotNil(t, m.Selection())
	assert.Equal(t, ast.KindLink, m.Selection().Node.Kind())
	assert.Greater(t, m.Selection().Start, first, "should advance to next link")
}

func TestBracketNavigation_SkipsNestedImages(t *testing.T) {
	// Image-wrapped links like [![badge](img)](url) should not cause double-steps.
	md := "# Title\n\n[![Badge1](https://img1)](https://link1) [![Badge2](https://img2)](https://link2)\n\nSome [text link](https://example.com).\n"

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", md)
	m.SetSize(80, 24)

	// Press ] three times — should get three distinct links, no image stops.
	var starts []int
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		require.NotNil(t, m.Selection(), "press %d should select something", i+1)
		assert.Equal(t, ast.KindLink, m.Selection().Node.Kind(),
			"press %d should select a Link, not an Image", i+1)
		starts = append(starts, m.Selection().Start)
	}

	// All three starts should be strictly increasing (no duplicates, no image stops).
	for i := 1; i < len(starts); i++ {
		assert.Greater(t, starts[i], starts[i-1],
			"press %d should advance past press %d", i+1, i)
	}
}
