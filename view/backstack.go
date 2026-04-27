package view

import (
	"github.com/pgavlin/goldmark/ast"
)

// backstackEntry is a snapshot of view location-state pushed onto the
// navigation backstack each time the user follows an internal link or a
// TOC entry. On Backspace the entry is popped and the snapshot is
// applied, restoring not only the focused element but also the cursor
// position, cursor mode, and scroll offsets.
//
// AST node pointers are stable across the existing
// invalidateLines/render cycle that re-renders the same document under
// a different width or after a re-parse of identical source. Storing
// pointers directly here keeps the entry valid through that cycle
// without the parallel saved-state shadow that the old simpler
// backstack required.
type backstackEntry struct {
	// Selection (focused element) at the time of the push.
	selectionNode ast.Node
	// Whether the selection should be rendered highlighted on restore.
	highlight bool

	// AST node at the line offset that was at the top of the viewport,
	// used to restore vertical scroll position.
	scrollNode ast.Node
	// Visible-column horizontal scroll position.
	columnOffset int

	// Cursor-mode state.
	cursorMode       bool
	cursorPositioned bool
	// AST node at the cursor's line, used to restore the cursor's
	// vertical position after re-render or re-parse. nil iff
	// cursorPositioned is false.
	cursorNode ast.Node
	// Visible column of the cursor on its line.
	cursorCol int
}

// captureBackstackEntry snapshots the model's current location-state
// into a backstackEntry. The selection field may be nil; callers are
// responsible for skipping push when no selection was held at the time.
func (m *Model) captureBackstackEntry() backstackEntry {
	e := backstackEntry{
		highlight:        m.highlightSelection,
		columnOffset:     m.columnOffset,
		cursorMode:       m.cursorMode,
		cursorPositioned: m.cursorPositioned,
		cursorCol:        m.cursorCol,
	}
	if m.selection != nil {
		e.selectionNode = m.selection.Node
	}
	if len(m.lines) > 0 && m.spanTree != nil {
		off := m.lineOffset
		if off >= len(m.lines) {
			off = len(m.lines) - 1
		}
		if off >= 0 {
			if s := m.findSpanAtOffset(m.lines[off].start); s != nil {
				e.scrollNode = s.Node
			}
		}
	}
	if m.cursorPositioned && m.cursorLine >= 0 && m.cursorLine < len(m.lines) && m.spanTree != nil {
		if s := m.findSpanAtOffset(m.lines[m.cursorLine].start); s != nil {
			e.cursorNode = s.Node
		}
	}
	return e
}

// applyBackstackEntry restores the model's location-state from a
// previously-captured entry. Selection is set without going through
// SelectSpan (which would re-scroll via ensureOffsetVisible) so the
// scroll/cursor values from the snapshot are honored exactly.
func (m *Model) applyBackstackEntry(e backstackEntry) {
	if e.selectionNode != nil {
		if span := m.findSpanForNode(e.selectionNode); span != nil {
			m.selection = span
			m.highlightSelection = e.highlight
			m.calculateSelectionSpan(span)
		} else {
			m.selection = nil
			m.highlightSelection = false
			m.selectionStart = 0
			m.selectionEnd = 0
		}
	}

	if e.scrollNode != nil {
		if span := m.findSpanForNode(e.scrollNode); span != nil {
			m.lineOffset = m.findLineForOffset(span.Start)
		}
	}
	m.columnOffset = e.columnOffset

	m.cursorMode = e.cursorMode
	m.cursorPositioned = e.cursorPositioned
	if e.cursorPositioned && e.cursorNode != nil {
		if span := m.findSpanForNode(e.cursorNode); span != nil {
			m.cursorLine = m.findLineForOffset(span.Start)
		}
	}
	m.cursorCol = e.cursorCol

	m.clampOffsets()
}
