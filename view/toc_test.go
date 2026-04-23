package view

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestModelWithTOC builds a Model with a non-empty TOC-shaped document.
func newTestModelWithTOC(t *testing.T) *Model {
	t.Helper()
	md := `# Top

Intro paragraph.

## Section A

Text.

### Subsection A1

Text.

## Section B

Text.
`
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("test.md", md)
	return &m
}

func TestTOC_InitiallyInactive(t *testing.T) {
	m := newTestModelWithTOC(t)
	assert.False(t, m.TOCActive())
}

func TestTOC_ClearResetsState(t *testing.T) {
	m := newTestModelWithTOC(t)
	m.toc.active = true
	m.Clear()
	assert.False(t, m.TOCActive())
	assert.Nil(t, m.toc.allEntries)
}

func TestBuildTOCEntries_Structure(t *testing.T) {
	m := newTestModelWithTOC(t)
	entries := m.buildTOCEntries()

	// Expected sections: Top, Section A, Subsection A1, Section B
	require.Len(t, entries, 4)

	assert.Equal(t, "Top", entries[0].text)
	assert.Equal(t, 1, entries[0].level)
	assert.Empty(t, entries[0].ancestors)
	assert.True(t, entries[0].lastChild) // only root-level heading

	assert.Equal(t, "Section A", entries[1].text)
	assert.Equal(t, 2, entries[1].level)
	assert.Equal(t, []string{"Top"}, entries[1].ancestors)
	assert.False(t, entries[1].lastChild) // Section B follows at same level

	assert.Equal(t, "Subsection A1", entries[2].text)
	assert.Equal(t, 3, entries[2].level)
	assert.Equal(t, []string{"Top", "Section A"}, entries[2].ancestors)
	assert.True(t, entries[2].lastChild) // only level-3 under Section A

	assert.Equal(t, "Section B", entries[3].text)
	assert.Equal(t, 2, entries[3].level)
	assert.Equal(t, []string{"Top"}, entries[3].ancestors)
	assert.True(t, entries[3].lastChild)

	// Every entry has a non-empty anchor sourced from the indexer.
	for _, e := range entries {
		assert.NotEmpty(t, e.anchor)
	}
}

func TestBuildTOCEntries_EmptyDocument(t *testing.T) {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("empty.md", "Just a paragraph, no headings.\n")
	entries := m.buildTOCEntries()
	assert.Empty(t, entries)
}

func pressKey(t *testing.T, m *Model, k string) {
	t.Helper()
	updated, _ := m.Update(tea.KeyPressMsg{Text: k})
	*m = updated
}

func TestTOC_OpensOnToggleKey(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	assert.True(t, m.TOCActive())
	assert.Equal(t, tocModeTree, m.toc.mode)
	assert.NotEmpty(t, m.toc.allEntries)
	assert.Len(t, m.toc.matches, len(m.toc.allEntries))
}

func TestTOC_NoHeadingsShowsStatusMessage(t *testing.T) {
	mv := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	m := &mv
	m.SetText("flat.md", "Just a paragraph.\n")
	pressKey(t, m, "t")
	assert.False(t, m.TOCActive())
	assert.Equal(t, "No headings", m.statusMessage)
}

func TestTOC_TerminalTooSmallShowsStatusMessage(t *testing.T) {
	mv := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(30),
		WithHeight(5),
	)
	m := &mv
	m.SetText("test.md", "# H1\n\n## H2\n")
	pressKey(t, m, "t")
	assert.False(t, m.TOCActive())
	assert.Equal(t, "Terminal too small for TOC", m.statusMessage)
}

func TestTOC_NoIndexIsNoOp(t *testing.T) {
	mv := NewModel(
		WithTheme(styles.Pulumi),
		WithWidth(80),
		WithHeight(25),
	)
	m := &mv
	// No SetText -> m.index is nil.
	pressKey(t, m, "t")
	assert.False(t, m.TOCActive())
}
