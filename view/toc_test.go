package view

import (
	"fmt"
	"os"
	"path/filepath"
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

// Two headings share the same text, one above and one below topOffset.
// Preselection must pick the one at or before topOffset (not the one after).
func TestTOC_OpenPreselectsWithDuplicateHeadingText(t *testing.T) {
	md := "# Examples\n\n"
	for i := 0; i < 5; i++ {
		md += "x\n\n"
	}
	md += "## Before\n\n"
	for i := 0; i < 5; i++ {
		md += "y\n\n"
	}
	md += "# Examples\n\n" // duplicate text, second occurrence
	for i := 0; i < 3; i++ {
		md += "z\n\n"
	}

	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(6),
	)
	m.SetText("dup.md", md)
	mp := &m
	// Scroll to the middle, which should land inside the "Before" section.
	mp.lineOffset = len(mp.lines) / 2
	mp.clampOffsets()

	pressKey(t, mp, "t")
	require.True(t, mp.TOCActive())

	// The cursor should point at the "Before" entry (entry index 1),
	// not at either "Examples" entry.
	current := mp.toc.allEntries[mp.toc.matches[mp.toc.cursor]]
	assert.Equal(t, "Before", current.text,
		"expected cursor on 'Before' when scrolled into its section, got %q at index %d",
		current.text, mp.toc.cursor)
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

func TestTOC_ViewIncludesOverlayWhenActive(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	out := m.View()
	// The rendered output should contain a rounded border corner.
	assert.Contains(t, ansi.Strip(out), "╭")
	assert.Contains(t, ansi.Strip(out), "╯")
	// And a known heading.
	assert.Contains(t, ansi.Strip(out), "Section A")
}

func TestTOC_ViewOmitsOverlayWhenInactive(t *testing.T) {
	m := newTestModelWithTOC(t)
	out := m.View()
	assert.NotContains(t, ansi.Strip(out), "╭")
}

func TestSubsequenceMatchPositions_Basic(t *testing.T) {
	pos := subsequenceMatchPositions("Introduction", "intro")
	assert.Equal(t, []int{0, 1, 2, 3, 4}, pos)
}

func TestSubsequenceMatchPositions_Gapped(t *testing.T) {
	// "i" matches at 0, "t" at 2, "d" at 5.
	pos := subsequenceMatchPositions("introduction", "itd")
	assert.Equal(t, []int{0, 2, 5}, pos)
}

func TestSubsequenceMatchPositions_CaseInsensitive(t *testing.T) {
	pos := subsequenceMatchPositions("HELLO", "hlo")
	assert.Equal(t, []int{0, 2, 4}, pos)
}

func TestSubsequenceMatchPositions_NoMatch(t *testing.T) {
	pos := subsequenceMatchPositions("abc", "xyz")
	assert.Nil(t, pos)
}

func TestSubsequenceMatchPositions_EmptyNeedle(t *testing.T) {
	pos := subsequenceMatchPositions("abc", "")
	assert.Equal(t, []int{}, pos)
}

func TestSubsequenceMatchPositions_WideChars(t *testing.T) {
	// Each CJK char has visual width 2; positions are visible column starts.
	pos := subsequenceMatchPositions("a日b", "ab")
	// "a" at col 0, "日" at cols 1-2, "b" at col 3.
	assert.Equal(t, []int{0, 3}, pos)
}

func TestTOC_SlashEntersFilterMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.Equal(t, tocModeTree, m.toc.mode)
	pressKey(t, m, "/")
	assert.Equal(t, tocModeFilter, m.toc.mode)
	assert.Equal(t, "", m.toc.query)
	assert.Len(t, m.toc.matches, len(m.toc.allEntries))
}

func TestTOC_FilterMatches(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "e")
	pressKey(t, m, "c")
	pressKey(t, m, "b")
	// Subsequence "secb" matches "Section B" but not "Section A" or "Subsection A1"
	// ("Subsection A1" has no 'b' after its 'c').
	require.Len(t, m.toc.matches, 1)
	assert.Equal(t, "Section B", m.toc.allEntries[m.toc.matches[0]].text)
}

func TestTOC_FilterBackspace(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "b")
	pressKey(t, m, "backspace")
	// Only "s" left: matches any entry with 's'.
	assert.Greater(t, len(m.toc.matches), 1)
	assert.Equal(t, "s", m.toc.query)
}

func TestTOC_FilterNoMatches(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "z")
	pressKey(t, m, "z")
	pressKey(t, m, "z")
	assert.Empty(t, m.toc.matches)
}

func TestRenderTOCBody_FilterMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "e")
	pressKey(t, m, "c")
	pressKey(t, m, "b")
	body := m.renderTOCBody(50)
	stripped := ansi.Strip(body)

	// Flat list: only Section B matches.
	require.Equal(t, 1, strings.Count(stripped, "\n")+1,
		"expected single-line filter result, got:\n%s", stripped)
	assert.Contains(t, stripped, "Section B")
	// Breadcrumb appended (matched entry's parent is "Top").
	assert.Contains(t, stripped, "Top")
}

func TestRenderTOCBody_FilterModeNoMatches(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "z")
	pressKey(t, m, "z")
	pressKey(t, m, "z")
	body := m.renderTOCBody(30)
	assert.Contains(t, ansi.Strip(body), "No matches")
}

// Breadcrumbs render root-first (matching the gutter convention in
// headingBreadcrumbs). For the fixture's "Subsection A1" entry with
// ancestors ["Top", "Section A"], the crumb reads "Top › Section A".
func TestRenderTOCBody_FilterModeBreadcrumbOrder(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	// Query uniquely matches "Subsection A1" (not Section A, not Section B).
	pressKey(t, m, "u")
	pressKey(t, m, "b")
	body := m.renderTOCBody(80)
	stripped := ansi.Strip(body)
	require.Contains(t, stripped, "Subsection A1")
	// Root-first ordering: "Top" must appear before "Section A" in the crumb.
	topIdx := strings.Index(stripped, "Top")
	secIdx := strings.Index(stripped, "Section A")
	require.Greater(t, topIdx, 0)
	require.Greater(t, secIdx, topIdx,
		"expected crumb to render root-first (Top before Section A), got:\n%s", stripped)
}

// In filter mode, the cursor row is wrapped in reverse-video as its
// selection indicator. A per-character match highlight on that row would
// emit \x1b[27m inside the label, turning the row's reverse off for the
// remainder — losing the selection indicator visually. The cursor row
// must therefore render without per-character highlights; only non-cursor
// rows show match highlights.
func TestRenderTOCBody_FilterModeCursorRowNoInnerSGR(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "e")
	pressKey(t, m, "c")
	pressKey(t, m, "b")
	require.Len(t, m.toc.matches, 1) // Section B
	body := m.renderTOCBody(50)
	// The cursor is on the only match (row 0). The rendered line must not
	// contain any "\x1b[27m" that would break reverse mid-line.
	lines := strings.Split(body, "\n")
	require.GreaterOrEqual(t, len(lines), 1)
	assert.NotContains(t, lines[0], "\x1b[27m",
		"cursor-row label should not contain a match-highlight reset, got: %q", lines[0])
}

func TestTOC_FilterEscReturnsToTree(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "esc")
	assert.Equal(t, tocModeTree, m.toc.mode)
	assert.Equal(t, "", m.toc.query)
	assert.Len(t, m.toc.matches, len(m.toc.allEntries))
}

func TestTOC_FilterEnterJumps(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "e")
	pressKey(t, m, "c")
	pressKey(t, m, "b")
	require.Len(t, m.toc.matches, 1)
	pressKey(t, m, "enter")
	assert.False(t, m.TOCActive())
	require.NotNil(t, m.selection)
}

func TestTOC_RenderSnapshot(t *testing.T) {
	const md = `# Overview

Intro text.

## Getting Started

Text.

### Installation

Text.

### Configuration

Text.

## Usage

Text.

## Reference

Text.
`
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(60),
		WithHeight(18),
	)
	m.SetText("snapshot.md", md)
	mp := &m
	pressKey(t, mp, "t")

	got := ansi.Strip(mp.View())

	fixturePath := filepath.Join("testdata", "toc_snapshot.txt")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
			require.NoError(t, os.MkdirAll("testdata", 0o755))
			require.NoError(t, os.WriteFile(fixturePath, []byte(got), 0o644))
			t.Skip("wrote new snapshot; rerun without UPDATE_SNAPSHOTS")
		}
		t.Fatalf("missing fixture %s; run with UPDATE_SNAPSHOTS=1 to create", fixturePath)
	}

	want, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}

// highlightMatch is only invoked for non-cursor rows in filter mode. This
// test forces that by building a fixture with multiple matching entries so
// rows other than the cursor position exist.
func TestHighlightMatch_WrapsMatchedColumns(t *testing.T) {
	got := highlightMatch("Section B", []int{0, 8})
	// 'S' at col 0 and 'B' at col 8 get wrapped in reverse-video SGR.
	assert.Contains(t, got, "\x1b[7mS\x1b[27m")
	assert.Contains(t, got, "\x1b[7mB\x1b[27m")
	// Interior chars stay plain.
	assert.NotContains(t, got, "\x1b[7me")
}

func TestHighlightMatch_EmptyPositionsReturnsUnchanged(t *testing.T) {
	assert.Equal(t, "Section B", highlightMatch("Section B", nil))
	assert.Equal(t, "Section B", highlightMatch("Section B", []int{}))
}

// Exercise filter-mode rendering where the cursor is not on a matched row,
// so the non-cursor branch of renderTOCFilterBody (which calls
// highlightMatch) actually runs.
func TestRenderTOCBody_FilterModeNonCursorHighlights(t *testing.T) {
	// Document with many "Section" headings so a single-char query matches
	// more than one.
	md := "# Top\n\n## Section One\n\n## Section Two\n\n## Section Three\n"
	mv := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	mv.SetText("multi.md", md)
	m := &mv
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s") // matches all three "Section" entries
	require.Greater(t, len(m.toc.matches), 1)
	// Move cursor off the first match so row 0 is a non-cursor match.
	pressKey(t, m, "down")
	body := m.renderTOCBody(60)
	lines := strings.Split(body, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	// Non-cursor row (lines[0]) must contain the per-char reverse SGR
	// sequence produced by highlightMatch.
	assert.Contains(t, lines[0], "\x1b[7m")
	assert.Contains(t, lines[0], "\x1b[27m")
}

// Exercise the breadcrumb-drop and label-truncation paths in
// renderTOCFilterBody by forcing a very narrow innerWidth.
func TestRenderTOCBody_FilterModeBreadcrumbDropped(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	// "Subsection A1" — has ancestors ["Top","Section A"], crumb is long.
	pressKey(t, m, "u")
	pressKey(t, m, "b")
	// Narrow inner width: label (13 cols) fits, breadcrumb doesn't.
	body := m.renderTOCBody(20)
	stripped := ansi.Strip(body)
	assert.Contains(t, stripped, "Subsection A1")
	// Breadcrumb must be absent — there's no room.
	assert.NotContains(t, stripped, "Section A")
}

func TestRenderTOCBody_FilterModeLabelTruncated(t *testing.T) {
	// Long heading that can't fit even without a breadcrumb.
	md := "# " + strings.Repeat("X", 100) + "\n"
	mv := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	mv.SetText("long.md", md)
	m := &mv
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "x")
	body := m.renderTOCBody(20)
	for _, ln := range strings.Split(body, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(ln), 20)
	}
	// Truncation ellipsis present.
	assert.Contains(t, ansi.Strip(body), "…")
}

// Exercise applyTOCScroll: cursor past the window advances scroll; cursor
// before scroll rewinds it; fitting content resets to 0. Called directly
// because View has a value receiver and scroll mutations there don't
// persist externally.
func TestApplyTOCScroll_WindowClamping(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())

	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	// Case 1: all fit — scroll resets to 0.
	m.toc.scroll = 3
	out := m.applyTOCScroll(lines, 10)
	assert.Equal(t, 0, m.toc.scroll)
	assert.Len(t, out, len(lines))

	// Case 2: cursor beyond window — scroll advances to keep cursor visible.
	m.toc.scroll = 0
	m.toc.cursor = 6
	out = m.applyTOCScroll(lines, 3) // window size 3, cursor at row 6
	assert.Equal(t, 4, m.toc.scroll, "scroll should advance to cursor-maxBody+1")
	assert.Len(t, out, 3)
	assert.Equal(t, []string{"e", "f", "g"}, out)

	// Case 3: cursor before scroll — scroll rewinds to cursor.
	m.toc.scroll = 5
	m.toc.cursor = 2
	out = m.applyTOCScroll(lines, 3)
	assert.Equal(t, 2, m.toc.scroll)
	assert.Equal(t, []string{"c", "d", "e"}, out)

	// Case 4: maxBody <= 0 — scroll resets, returns input.
	m.toc.scroll = 5
	out = m.applyTOCScroll(lines, 0)
	assert.Equal(t, 0, m.toc.scroll)
	assert.Equal(t, lines, out)
}

// Exercise renderTOCOverlay with a cursor past the visible window: the
// dialog should clip its rendered body and the top entry in the scroll
// window should no longer be the first allEntries entry.
func TestRenderTOCOverlay_ScrollsToKeepCursorVisible(t *testing.T) {
	md := "# Root\n\n"
	for i := 0; i < 40; i++ {
		md += fmt.Sprintf("## Heading %02d\n\nText.\n\n", i)
	}
	mv := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(12),
	)
	mv.SetText("many.md", md)
	m := &mv
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())

	// Move cursor to the last entry.
	pressKey(t, m, "G")

	out := ansi.Strip(m.View())
	// The last heading must be visible with the cursor marker.
	assert.Contains(t, out, "Heading 39")
	// A mid-document heading must be scrolled off of both the doc view
	// and the TOC dialog.
	assert.NotContains(t, out, "Heading 20")
}

// Filter-mode pgup/pgdown/ctrl+c paths.
func TestTOC_FilterPgDownPgUp(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s") // matches multiple entries
	require.Greater(t, len(m.toc.matches), 1)
	pressKey(t, m, "pgdown")
	assert.Equal(t, len(m.toc.matches)-1, m.toc.cursor)
	pressKey(t, m, "pgup")
	assert.Equal(t, 0, m.toc.cursor)
}

// Exercise renderTOCOverlay's filter-mode branch, including the prompt row
// with the active query and cursor marker.
func TestRenderTOCOverlay_FilterModePromptRow(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "e")
	pressKey(t, m, "c")
	out := ansi.Strip(m.View())
	// Filter prompt with the typed query appears above the entries.
	assert.Contains(t, out, "filter: sec")
}

func TestTOC_FilterCtrlCDismisses(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "ctrl+c")
	assert.False(t, m.TOCActive())
}
