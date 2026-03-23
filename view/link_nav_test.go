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

func TestBracketNavigation_NavigatesAllItems(t *testing.T) {
	// Document with headings, links, and code blocks — ] should navigate through all.
	source, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("getting-started.md", string(source))
	m.SetSize(80, 24)

	// Press ] — should select any navigable item.
	m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	require.NotNil(t, m.Selection())

	first := m.Selection().Start

	// Press ] again — should advance to next item.
	m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	require.NotNil(t, m.Selection())
	assert.Greater(t, m.Selection().Start, first, "should advance to next item")
}

func TestBracketNavigation_IncludesHTMLBlockAnchors(t *testing.T) {
	// An <a> tag split across lines is an HTMLBlock; ] should navigate to it.
	md := "# Heading\n\n<a id=\"target\">\n</a>\n\nSome text with a [link](#target).\n"

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", md)
	m.SetSize(80, 24)

	// Collect all navigable items via ] presses.
	var kinds []ast.NodeKind
	for i := 0; i < 10; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		if m.Selection() == nil {
			break
		}
		kinds = append(kinds, m.Selection().Node.Kind())
		prev := m.Selection()
		// peek if next press would advance
		test := m
		test, _ = test.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		if test.Selection() == prev {
			break
		}
	}

	assert.Contains(t, kinds, ast.KindHTMLBlock,
		"HTMLBlock anchor should be reachable via ] navigation")
}

func TestHeadingNavigation_IncludesHTMLBlockAnchors(t *testing.T) {
	// } should navigate through both headings and HTMLBlock anchors.
	md := "# First\n\n<a id=\"anchor\">\n</a>\n\n## Second\n\nText.\n"

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", md)
	m.SetSize(80, 24)

	// Collect items reachable via } (heading navigation).
	var kinds []ast.NodeKind
	for i := 0; i < 10; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: '}', Text: "}"})
		if m.Selection() == nil {
			break
		}
		kinds = append(kinds, m.Selection().Node.Kind())
		prev := m.Selection()
		test := m
		test, _ = test.Update(tea.KeyPressMsg{Code: '}', Text: "}"})
		if test.Selection() == prev {
			break
		}
	}

	assert.Contains(t, kinds, ast.KindHeading,
		"headings should be reachable via } navigation")
	assert.Contains(t, kinds, ast.KindHTMLBlock,
		"HTMLBlock anchors should be reachable via } navigation")
}

func TestBracketNavigation_SkipsNestedImages(t *testing.T) {
	// Image-wrapped links like [![badge](img)](url) should not cause double-steps.
	md := "# Title\n\n[![Badge1](https://img1)](https://link1) [![Badge2](https://img2)](https://link2)\n\nSome [text link](https://example.com).\n"

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", md)
	m.SetSize(80, 24)

	// Press ] repeatedly — should get distinct items with strictly increasing positions.
	var starts []int
	for i := 0; i < 4; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		require.NotNil(t, m.Selection(), "press %d should select something", i+1)
		starts = append(starts, m.Selection().Start)
	}

	// All starts should be strictly increasing (no duplicates, no image stops).
	for i := 1; i < len(starts); i++ {
		assert.Greater(t, starts[i], starts[i-1],
			"press %d should advance past press %d", i+1, i)
	}
}
