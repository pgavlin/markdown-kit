package view

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	"github.com/charmbracelet/x/ansi"
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

// renderTOCBody produces the body string (no border) for the overlay,
// respecting innerWidth. In tree mode it draws box-drawing tree edges;
// in filter mode (not yet implemented here) it falls back to simple
// indentation.
func (m *Model) renderTOCBody(innerWidth int) string {
	if innerWidth < 4 {
		innerWidth = 4
	}
	var b strings.Builder
	accent, muted := m.tocStyles()

	if len(m.toc.matches) == 0 {
		return muted.Render("  No matches")
	}

	// For tree mode we render all matches in order.
	for row, midx := range m.toc.matches {
		entry := m.toc.allEntries[midx]
		cursorMark := "  "
		if row == m.toc.cursor {
			cursorMark = "> "
		}

		prefix := m.tocTreePrefix(midx)
		label := entry.text
		// Truncate label to fit innerWidth - len(cursorMark) - ansi.StringWidth(prefix).
		avail := innerWidth - ansi.StringWidth(cursorMark) - ansi.StringWidth(prefix)
		if avail < 4 {
			// Deep nesting fallback: drop the box-drawing prefix.
			prefix = strings.Repeat("  ", entry.level-1)
			avail = innerWidth - ansi.StringWidth(cursorMark) - ansi.StringWidth(prefix)
			if avail < 1 {
				avail = 1
			}
		}
		if ansi.StringWidth(label) > avail {
			label = ansi.Truncate(label, avail, "…")
		}

		line := cursorMark + prefix + label
		// Pad to innerWidth so the selection highlight spans full width.
		w := ansi.StringWidth(line)
		if w < innerWidth {
			line = line + strings.Repeat(" ", innerWidth-w)
		}
		if row == m.toc.cursor {
			line = accent.Render(line)
		}
		if row > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// tocTreePrefix returns the box-drawing prefix for the entry at allEntries[i].
// Uses lastChild flags and ancestor depth to draw │/├/└ connectors.
func (m *Model) tocTreePrefix(i int) string {
	entry := m.toc.allEntries[i]
	if entry.level <= 1 {
		return ""
	}

	// For each ancestor depth (1..level-1), decide whether to draw "│   "
	// (branch continues) or "    " (branch done). Branch continues at
	// depth d when there is a later entry whose level <= d (meaning a
	// sibling of the ancestor at that depth appears after this entry).
	prefix := ""
	for depth := 1; depth < entry.level-1; depth++ {
		if m.tocBranchContinues(i, depth) {
			prefix += "│   "
		} else {
			prefix += "    "
		}
	}

	// Own connector:
	if entry.lastChild {
		prefix += "└── "
	} else {
		prefix += "├── "
	}
	return prefix
}

// tocBranchContinues reports whether the ancestor at depth `depth` has any
// further sibling after the entry at allEntries[i]. It scans forward for
// the first entry whose level is at most depth+1 (a sibling of the
// depth+1 ancestor or higher). If found and its level equals depth+1, the
// branch continues; if its level is shallower, the branch is done.
func (m *Model) tocBranchContinues(i, depth int) bool {
	for j := i + 1; j < len(m.toc.allEntries); j++ {
		lvl := m.toc.allEntries[j].level
		if lvl <= depth+1 {
			return lvl == depth+1
		}
	}
	return false
}

// tocStyles returns (accent, muted) lipgloss styles derived from the theme.
func (m *Model) tocStyles() (lipgloss.Style, lipgloss.Style) {
	accent := lipgloss.NewStyle().Reverse(true)
	muted := lipgloss.NewStyle()
	if m.theme != nil {
		if c := m.theme.Get(chroma.Comment).Colour; c.IsSet() {
			muted = muted.Foreground(lipgloss.Color(
				fmt.Sprintf("#%02x%02x%02x", c.Red(), c.Green(), c.Blue())))
		}
	}
	return accent, muted
}
