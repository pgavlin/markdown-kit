package view

import (
	"testing"

	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBackstackModel sets up a model with a multi-section document long
// enough to scroll vertically and wide enough (one VERY long line) to
// scroll horizontally past column 0. Returns a pointer suitable for the
// *Model receivers used by Capture and Apply helpers.
func newBackstackModel(t *testing.T) *Model {
	t.Helper()
	long := "# Top\n\nIntro paragraph.\n\n"
	for i := 0; i < 30; i++ {
		long += "filler line\n\n"
	}
	long += "## Section A\n\nLink to [B](#section-b).\n\n"
	for i := 0; i < 30; i++ {
		long += "more filler\n\n"
	}
	// One long line so columnOffset can be non-zero without being clamped.
	long += "## Section B\n\n"
	for i := 0; i < 200; i++ {
		long += "x"
	}
	long += "\n\nB body.\n"

	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", long)
	return &m
}

// scrollNodeAt returns the AST node currently anchoring lineOffset in m.
func scrollNodeAt(t *testing.T, m *Model) ast.Node {
	t.Helper()
	require.NotNil(t, m.spanTree, "spanTree must be built")
	require.Greater(t, len(m.lines), m.lineOffset)
	span := m.findSpanAtOffset(m.lines[m.lineOffset].start)
	require.NotNil(t, span)
	return span.Node
}

func TestGoBack_RestoresScrollOffset(t *testing.T) {
	m := newBackstackModel(t)
	// Establish a selection (any internal anchor will do — picks up a
	// non-nil m.selection so the push will actually happen). After
	// SelectAnchor the viewport may have scrolled to make the selection
	// visible; that's fine, we capture the entry from the post-select
	// state, which is what FollowLink does in production.
	require.True(t, m.SelectAnchor("section-a"))
	require.NotNil(t, m.selection)

	// Move scroll to a non-trivial offset that still has a stable AST
	// node anchor.
	m.lineOffset = 5
	m.clampOffsets()
	require.Greater(t, m.lineOffset, 0, "fixture must support a non-zero scroll")
	preScrollNode := scrollNodeAt(t, m)

	pre := m.captureBackstackEntry()
	require.NotNil(t, pre.scrollNode)
	require.NotNil(t, pre.selectionNode)
	require.True(t, m.SelectAnchor("section-b"))
	m.backstack = append(m.backstack, pre)

	// Move scroll away.
	m.lineOffset = 0
	m.clampOffsets()

	// GoBack restores the line containing preScrollNode.
	require.True(t, m.GoBack())
	span := m.findSpanAtOffset(m.lines[m.lineOffset].start)
	require.NotNil(t, span)
	// The deepest span at the restored line's start may be a descendant
	// of preScrollNode; walk up the parent chain.
	found := false
	for n := span.Node; n != nil; n = n.Parent() {
		if n == preScrollNode {
			found = true
			break
		}
	}
	assert.True(t, found,
		"restored lineOffset should point at a line covering preScrollNode")
}

func TestGoBack_RestoresColumnOffset(t *testing.T) {
	m := newBackstackModel(t)
	m.columnOffset = 5

	// Synthesize a push.
	require.True(t, m.SelectAnchor("section-a"))
	pre := m.captureBackstackEntry()
	require.True(t, m.SelectAnchor("section-b"))
	m.backstack = append(m.backstack, pre)
	m.columnOffset = 0

	require.True(t, m.GoBack())
	assert.Equal(t, 5, m.columnOffset, "column offset should be restored")
}

func TestGoBack_RestoresCursorMode(t *testing.T) {
	m := newBackstackModel(t)
	// Position the cursor and enter cursor mode at a known location.
	m.cursorMode = true
	m.cursorPositioned = true
	m.cursorLine = 4
	m.cursorCol = 2
	preCursorNode := func() ast.Node {
		span := m.findSpanAtOffset(m.lines[m.cursorLine].start)
		require.NotNil(t, span)
		return span.Node
	}()

	// Push.
	require.True(t, m.SelectAnchor("section-a"))
	pre := m.captureBackstackEntry()
	require.NotNil(t, pre.cursorNode)
	require.True(t, m.SelectAnchor("section-b"))
	m.backstack = append(m.backstack, pre)

	// Disturb cursor state.
	m.cursorMode = false
	m.cursorPositioned = false
	m.cursorLine = 0
	m.cursorCol = 0

	require.True(t, m.GoBack())
	assert.True(t, m.cursorMode, "cursorMode should be restored")
	assert.True(t, m.cursorPositioned)
	assert.Equal(t, 2, m.cursorCol, "cursorCol should be restored")
	// Cursor line should resolve back to the same AST node.
	assert.Same(t, preCursorNode, scrollNodeAt(t, &Model{ // helper expects lineOffset; substitute cursorLine
		spanTree:   m.spanTree,
		lines:      m.lines,
		lineOffset: m.cursorLine,
	}), "cursor should land on the same AST line")
}

// captureBackstackEntry should fill scrollNode even when no selection
// was held. (push-side guard handles "no selection" — but capture still
// snapshots the rest.)
func TestCaptureBackstackEntry_NoSelection(t *testing.T) {
	m := newBackstackModel(t)
	m.lineOffset = 4
	m.clampOffsets()
	require.Nil(t, m.selection)

	e := m.captureBackstackEntry()
	assert.Nil(t, e.selectionNode, "no selection → entry has nil selectionNode")
	require.NotNil(t, e.scrollNode, "scrollNode should always reflect the current viewport top")
}

// The entry survives invalidateLines + render — width changes between
// FollowLink and GoBack must not break restoration. AST node identity
// is stable across the cycle for the same source, so the captured
// scrollNode still resolves to a meaningful line after re-render.
func TestGoBack_SurvivesReRender(t *testing.T) {
	m := newBackstackModel(t)
	require.True(t, m.SelectAnchor("section-a"))

	m.lineOffset = 5
	m.clampOffsets()
	preScrollNode := scrollNodeAt(t, m)

	pre := m.captureBackstackEntry()
	require.True(t, m.SelectAnchor("section-b"))
	m.backstack = append(m.backstack, pre)

	// Force re-render via width change. AST node pointers remain stable
	// across this cycle (no re-parse), so the entry's scrollNode is
	// still a valid lookup key in the new spanTree.
	m.SetSize(60, 20)
	m.SetSize(80, 24)

	m.lineOffset = 0
	require.True(t, m.GoBack())
	// After restore, the restored lineOffset must map back to a line
	// covering the preScrollNode (the deepest span at the line's start
	// may be a child of preScrollNode, so we walk up the parent chain).
	span := m.findSpanAtOffset(m.lines[m.lineOffset].start)
	require.NotNil(t, span)
	found := false
	for n := span.Node; n != nil; n = n.Parent() {
		if n == preScrollNode {
			found = true
			break
		}
	}
	assert.True(t, found,
		"restored line should be anchored at or under preScrollNode after re-render")
}

// TOC-jump pushes the same enriched entry as FollowLink: selection +
// scroll + cursor.
func TestTOCJump_BackstackHasFullSnapshot(t *testing.T) {
	m := newBackstackModel(t)
	require.True(t, m.SelectAnchor("section-a"))
	m.lineOffset = 5
	m.cursorMode = true
	m.cursorPositioned = true
	m.cursorLine = 3
	m.cursorCol = 7
	m.clampOffsets()

	// Drive a TOC jump by hand: capture, navigate, push.
	pre := m.captureBackstackEntry()
	require.NotNil(t, pre.selectionNode, "section-a selection must be captured")
	require.NotNil(t, pre.scrollNode, "scroll node must be captured")
	require.True(t, pre.cursorPositioned)
	require.NotNil(t, pre.cursorNode)
	require.Equal(t, 7, pre.cursorCol)

	require.True(t, m.SelectAnchor("section-b"))
	m.backstack = append(m.backstack, pre)

	// Move state away.
	m.lineOffset = 0
	m.cursorMode = false
	m.cursorPositioned = false
	m.cursorLine = 0
	m.cursorCol = 0

	// GoBack restores it all.
	require.True(t, m.GoBack())
	assert.True(t, m.cursorMode, "TOC-jump backstack must restore cursor mode")
	assert.True(t, m.cursorPositioned)
	assert.Equal(t, 7, m.cursorCol)
}

// Multi-level: push, push, pop, pop. Each pop restores the entry pushed
// at that level, not a flattened or shuffled one.
func TestGoBack_MultiLevelOrdering(t *testing.T) {
	m := newBackstackModel(t)
	require.True(t, m.SelectAnchor("section-a"))
	m.lineOffset = 4
	m.clampOffsets()
	level1Cols := 11
	m.columnOffset = level1Cols
	first := m.captureBackstackEntry()
	require.True(t, m.SelectAnchor("section-b"))
	m.backstack = append(m.backstack, first)

	// Now in a different state for level 2.
	m.lineOffset = 8
	level2Cols := 22
	m.columnOffset = level2Cols
	second := m.captureBackstackEntry()
	// Push a third anchor (any will do; pretend a sub-link).
	require.True(t, m.SelectAnchor("section-a"))
	m.backstack = append(m.backstack, second)

	require.Len(t, m.backstack, 2)

	// First pop should match level 2 state (most recent push).
	require.True(t, m.GoBack())
	assert.Equal(t, level2Cols, m.columnOffset, "first pop restores level 2")

	// Second pop should match level 1 state.
	require.True(t, m.GoBack())
	assert.Equal(t, level1Cols, m.columnOffset, "second pop restores level 1")

	// Now empty.
	assert.False(t, m.GoBack())
}

// applyBackstackEntry must not panic when a stored AST node fails to
// resolve in the current span tree. We synthesize an orphan heading
// node — never inserted into the spanTree — so findSpanForNode returns
// nil for it and the apply path falls through gracefully.
func TestApplyBackstackEntry_UnresolvableNode(t *testing.T) {
	m := newBackstackModel(t)
	orphan := ast.NewHeading(false, 2)
	e := backstackEntry{
		selectionNode:    orphan,
		scrollNode:       orphan,
		cursorNode:       orphan,
		cursorPositioned: true,
		cursorMode:       true,
	}
	assert.NotPanics(t, func() { m.applyBackstackEntry(e) })
	// Selection should be cleared since lookup failed.
	assert.Nil(t, m.selection)
}
