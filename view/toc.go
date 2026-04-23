package view

import (
	tea "charm.land/bubbletea/v2"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/indexer"
)

// tocMode distinguishes the tree vs filter view of the overlay.
type tocMode int

const (
	tocModeTree tocMode = iota
	tocModeFilter
)

// tocEntry is one heading in the overlay.
type tocEntry struct {
	anchor    string   // DocumentIndex anchor used for SelectAnchor
	level     int      // 1..6
	text      string   // heading plain text
	ancestors []string // ancestor heading texts (level 1 ... parent)
	lastChild bool     // true if last sibling at its level (for tree edges)
	matchCols []int    // filter-mode: visible column positions of matched chars
}

// tocState is the overlay's runtime state.
type tocState struct {
	active     bool
	mode       tocMode
	query      string
	allEntries []tocEntry
	matches    []int
	cursor     int
	scroll     int
}

// TOCActive reports whether the table-of-contents overlay is open.
// Embedders should gate their own key handling (see Searching()).
func (m *Model) TOCActive() bool {
	return m.toc.active
}

// buildTOCEntries walks the document's section tree and returns a flat,
// document-ordered list of entries suitable for rendering.
func (m *Model) buildTOCEntries() []tocEntry {
	if m.index == nil {
		return nil
	}
	root := m.index.TableOfContents()
	if root == nil {
		return nil
	}
	var entries []tocEntry
	var ancestors []string
	visitTOCSections(m.markdown, root.Subsections, ancestors, &entries)
	return entries
}

// visitTOCSections is a recursive helper that appends entries in document
// order, tracking ancestor heading texts along the recursion path.
func visitTOCSections(source []byte, sections []*indexer.Section, ancestors []string, out *[]tocEntry) {
	for i, s := range sections {
		h, ok := s.Start.(*ast.Heading)
		if !ok {
			continue
		}
		text := string(h.Text(source))
		entry := tocEntry{
			anchor:    s.Anchor,
			level:     s.Level,
			text:      text,
			ancestors: append([]string(nil), ancestors...),
			lastChild: i == len(sections)-1,
		}
		*out = append(*out, entry)

		if len(s.Subsections) > 0 {
			visitTOCSections(source, s.Subsections, append(ancestors, text), out)
		}
	}
}

// Minimum viewport dimensions required to open the TOC overlay.
const (
	tocMinWidth  = 40
	tocMinHeight = 6
)

// openTOC initializes and activates the overlay. Guards for missing index,
// empty headings, and undersized viewport set a transient status message
// instead of opening.
func (m *Model) openTOC() {
	if m.index == nil {
		return
	}
	if m.width < tocMinWidth || m.height < tocMinHeight {
		m.SetStatusMessage("Terminal too small for TOC")
		return
	}
	entries := m.buildTOCEntries()
	if len(entries) == 0 {
		m.SetStatusMessage("No headings")
		return
	}
	matches := make([]int, len(entries))
	for i := range entries {
		matches[i] = i
	}
	cursor := m.findEnclosingTOCEntry(entries)
	m.toc = tocState{
		active:     true,
		mode:       tocModeTree,
		allEntries: entries,
		matches:    matches,
		cursor:     cursor,
		scroll:     0,
	}
}

// findEnclosingTOCEntry returns the index of the entry whose section covers
// the current scroll position. Falls back to 0 if no heading precedes
// m.lineOffset.
func (m *Model) findEnclosingTOCEntry(entries []tocEntry) int {
	if len(entries) == 0 || m.spanTree == nil || len(m.lines) == 0 {
		return 0
	}
	lineOffset := m.lineOffset
	if lineOffset >= len(m.lines) {
		lineOffset = len(m.lines) - 1
	}
	if lineOffset < 0 {
		return 0
	}
	topOffset := m.lines[lineOffset].start

	// Walk the span tree in document order; track the most recent heading
	// whose start is at or before topOffset.
	var lastHeadingText string
	found := false
	for s := m.spanTree; s != nil; s = s.Next {
		if s.Start > topOffset {
			break
		}
		if h, ok := s.Node.(*ast.Heading); ok {
			lastHeadingText = string(h.Text(m.markdown))
			found = true
		}
	}
	if !found {
		return 0
	}

	// Map heading text back to entry index. Multiple headings may share
	// text; prefer the last occurrence at or before topOffset, so search
	// from the end.
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].text == lastHeadingText {
			return i
		}
	}
	return 0
}

// handleTOCKey routes keys while the overlay is active.
func (m *Model) handleTOCKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.toc.mode == tocModeFilter {
		return m.handleTOCFilterKey(msg)
	}
	return m.handleTOCTreeKey(msg)
}

// handleTOCTreeKey handles keys in tree mode.
func (m *Model) handleTOCTreeKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "t", "ctrl+c":
		m.toc = tocState{}
		return nil
	case "j", "down":
		m.moveTOCCursor(1)
		return nil
	case "k", "up":
		m.moveTOCCursor(-1)
		return nil
	case "pgdown", "ctrl+f":
		m.moveTOCCursor(10)
		return nil
	case "pgup", "ctrl+b":
		m.moveTOCCursor(-10)
		return nil
	case "g", "home":
		m.toc.cursor = 0
		m.toc.scroll = 0
		return nil
	case "G", "end":
		m.toc.cursor = len(m.toc.matches) - 1
		if m.toc.cursor < 0 {
			m.toc.cursor = 0
		}
		return nil
	case "enter":
		m.jumpToSelectedTOCEntry()
		return nil
	}
	return nil
}

// handleTOCFilterKey is implemented in a later task; default to tree keys so
// the filter mode can be added incrementally without breaking tree behavior.
func (m *Model) handleTOCFilterKey(msg tea.KeyPressMsg) tea.Cmd {
	return m.handleTOCTreeKey(msg)
}

// moveTOCCursor shifts the cursor by n (positive = down) with clamping.
func (m *Model) moveTOCCursor(n int) {
	if len(m.toc.matches) == 0 {
		m.toc.cursor = 0
		return
	}
	m.toc.cursor += n
	if m.toc.cursor < 0 {
		m.toc.cursor = 0
	}
	if m.toc.cursor >= len(m.toc.matches) {
		m.toc.cursor = len(m.toc.matches) - 1
	}
}

// jumpToSelectedTOCEntry navigates to the heading at the current cursor,
// pushes the prior selection onto the backstack (matching FollowLink), and
// dismisses the overlay. If no entry is selected (empty matches), it only
// dismisses.
func (m *Model) jumpToSelectedTOCEntry() {
	if len(m.toc.matches) == 0 || m.toc.cursor < 0 || m.toc.cursor >= len(m.toc.matches) {
		m.toc = tocState{}
		return
	}
	entry := m.toc.allEntries[m.toc.matches[m.toc.cursor]]
	prev := m.selection
	if m.SelectAnchor(entry.anchor) && prev != nil {
		m.backstack = append(m.backstack, prev)
	}
	m.toc = tocState{}
}
