package view

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestTOC_TreeNavigation(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	start := m.toc.cursor

	pressKey(t, m, "j")
	assert.Equal(t, start+1, m.toc.cursor)

	pressKey(t, m, "k")
	assert.Equal(t, start, m.toc.cursor)

	// G jumps to last.
	pressKey(t, m, "G")
	assert.Equal(t, len(m.toc.matches)-1, m.toc.cursor)

	// g jumps to first.
	pressKey(t, m, "g")
	assert.Equal(t, 0, m.toc.cursor)

	// j at last stays at last (no wrap).
	pressKey(t, m, "G")
	pressKey(t, m, "j")
	assert.Equal(t, len(m.toc.matches)-1, m.toc.cursor)

	// k at first stays at first.
	pressKey(t, m, "g")
	pressKey(t, m, "k")
	assert.Equal(t, 0, m.toc.cursor)
}

func TestTOC_EscClosesTreeMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	pressKey(t, m, "esc")
	assert.False(t, m.TOCActive())
}

func TestTOC_ToggleKeyClosesTreeMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	pressKey(t, m, "t")
	assert.False(t, m.TOCActive())
}

func TestTOC_EnterJumpsAndDismisses(t *testing.T) {
	m := newTestModelWithTOC(t)
	// Open TOC and move cursor to Section B (index 3 in the fixture).
	pressKey(t, m, "t")
	for m.toc.allEntries[m.toc.matches[m.toc.cursor]].text != "Section B" {
		if m.toc.cursor == len(m.toc.matches)-1 {
			t.Fatal("could not find Section B entry")
		}
		pressKey(t, m, "j")
	}
	pressKey(t, m, "enter")

	assert.False(t, m.TOCActive())
	require.NotNil(t, m.selection)
	// Selection should correspond to the Section B heading.
	// We don't assert the exact node here — just that a selection exists.
}

func TestTOC_EnterBackstackPreservesPriorSelection(t *testing.T) {
	m := newTestModelWithTOC(t)
	// Establish a prior selection by jumping to Section A first.
	m.SelectAnchor("section-a")
	priorSelection := m.selection
	require.NotNil(t, priorSelection)

	pressKey(t, m, "t")
	// Move cursor to Section B.
	for m.toc.allEntries[m.toc.matches[m.toc.cursor]].text != "Section B" {
		if m.toc.cursor == len(m.toc.matches)-1 {
			t.Fatal("could not find Section B entry")
		}
		pressKey(t, m, "j")
	}
	pressKey(t, m, "enter")

	assert.False(t, m.TOCActive())
	require.Len(t, m.backstack, 1)
	assert.Same(t, priorSelection, m.backstack[0])
}

func TestTOC_EnterWithNoPriorSelectionDoesNotPushBackstack(t *testing.T) {
	m := newTestModelWithTOC(t)
	require.Nil(t, m.selection)
	pressKey(t, m, "t")
	pressKey(t, m, "enter")
	assert.False(t, m.TOCActive())
	assert.Empty(t, m.backstack)
}

func TestTOC_OpenPreselectsEnclosingHeading(t *testing.T) {
	// Use single-word paragraphs separated by blank lines so each paragraph
	// renders as exactly two output lines (content + blank separator). This
	// produces enough rendered lines that the document is scrollable past the
	// "Target" heading even with a small (but tocMinHeight-satisfying) viewport.
	md := "# Top\n\n"
	for i := 0; i < 7; i++ {
		md += "x\n\n"
	}
	md += "## Target\n\n"
	for i := 0; i < 3; i++ {
		md += "x\n\n"
	}

	// height=6 satisfies tocMinHeight (6); with gutter, pageSize=5.
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(6),
	)
	m.SetText("test.md", md)
	mp := &m

	// Scroll well past the "Target" heading; clampOffsets will cap it to the
	// last valid page, which should still be within the "Target" section.
	mp.lineOffset = 999
	mp.clampOffsets()

	// Sanity: ensure we actually scrolled past the "Top" heading.
	require.Greater(t, mp.lineOffset, 0, "lineOffset should be non-zero after clamping")

	pressKey(t, mp, "t")
	require.True(t, mp.TOCActive())
	// The cursor should point at the "Target" entry.
	require.Greater(t, len(mp.toc.matches), mp.toc.cursor)
	current := mp.toc.allEntries[mp.toc.matches[mp.toc.cursor]]
	assert.Equal(t, "Target", current.text)
}

func TestTOC_DismissedOnSetText(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	m.SetText("other.md", "# Other\n")
	assert.False(t, m.TOCActive())
}

func TestRenderTOCBody_TreeMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	body := m.renderTOCBody(40)

	lines := strings.Split(body, "\n")
	stripped := make([]string, len(lines))
	for i, ln := range lines {
		stripped[i] = ansi.Strip(ln)
	}
	require.GreaterOrEqual(t, len(stripped), 4)

	// Fixture order: Top, Section A, Subsection A1, Section B.
	// Tree-edge expectations:
	//   Top           (no prefix, it's the sole level-1 root)
	//   ├── Section A
	//   │   └── Subsection A1
	//   └── Section B
	assert.Contains(t, stripped[0], "Top")
	assert.Contains(t, stripped[1], "├──")
	assert.Contains(t, stripped[1], "Section A")
	assert.Contains(t, stripped[2], "│")
	assert.Contains(t, stripped[2], "└──")
	assert.Contains(t, stripped[2], "Subsection A1")
	assert.Contains(t, stripped[3], "└──")
	assert.Contains(t, stripped[3], "Section B")

	// The line at the cursor position should have the ">" marker.
	cursorLine := stripped[m.toc.cursor]
	assert.Contains(t, cursorLine, ">")
}

func TestRenderTOCBody_TruncatesLongLabels(t *testing.T) {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("long.md", "# "+strings.Repeat("A", 200)+"\n")
	mp := &m
	pressKey(t, mp, "t")
	body := mp.renderTOCBody(20)
	for _, ln := range strings.Split(body, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(ln), 20)
	}
}
