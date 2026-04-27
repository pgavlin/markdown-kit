package view

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/indexer"
)

// Position is a portable snapshot of the user's reading location. It is
// decoupled from AST node identity (which changes on every re-parse) so
// it survives a fresh SetText — making it suitable for "remember where
// I was before reload, restore after reload" flows.
//
// All fields are primitives; Position is JSON-serializable if a caller
// later wants to persist reading positions across processes.
type Position struct {
	// HeadingAnchor is the GFM anchor of the heading whose section
	// contains the reference line, or "" if no heading precedes the
	// reference line.
	HeadingAnchor string
	// HeadingIndex disambiguates among sections that share the same
	// HeadingAnchor (e.g., a document with two "## Notes" sections).
	// 0-based ordinal in document order.
	HeadingIndex int

	// LineSnippet is the ANSI-stripped, trimmed text of the reference
	// line, used for fine-grained re-anchoring within the heading's
	// section. Empty when the line had no displayable text.
	LineSnippet string
	// LineOccurrence is the 0-based index of LineSnippet among matching
	// lines within the section, used to disambiguate when the same
	// snippet text appears on multiple lines of the same section.
	LineOccurrence int

	// CursorMode reports whether cursor mode was active.
	CursorMode bool
	// CursorPositioned reports whether the cursor had been placed at
	// least once before capture.
	CursorPositioned bool
	// CursorCol is the visible column of the cursor on its line.
	CursorCol int
}

// IsZero reports whether p carries no useful information.
func (p Position) IsZero() bool {
	return p.HeadingAnchor == "" && p.LineSnippet == "" && !p.CursorMode && !p.CursorPositioned
}

// Position captures the user's current location into a portable
// snapshot. The reference line is the cursor line when cursor mode has
// been positioned, otherwise the top of the viewport.
func (m *Model) Position() Position {
	var p Position
	p.CursorMode = m.cursorMode
	p.CursorPositioned = m.cursorPositioned
	p.CursorCol = m.cursorCol

	refLine := m.lineOffset
	if m.cursorPositioned && m.cursorLine >= 0 && m.cursorLine < len(m.lines) {
		refLine = m.cursorLine
	}
	if len(m.lines) == 0 || refLine >= len(m.lines) || refLine < 0 {
		return p
	}

	heading, _ := m.enclosingHeading(refLine)
	if heading != nil {
		text := string(heading.Text(m.markdown))
		p.HeadingAnchor = indexer.GitHubFlavoredMarkdown(text)
		// HeadingIndex must be the ordinal AMONG sections that share
		// this anchor — not the global heading ordinal — so it lines up
		// with the slice indexer.DocumentIndex.Lookup returns.
		if m.index != nil {
			if sections, ok := m.index.Lookup(p.HeadingAnchor); ok {
				for i, s := range sections {
					if s.Start == heading {
						p.HeadingIndex = i
						break
					}
				}
			}
		}
	}

	snippet := lineSnippet(m.lines[refLine].content)
	if snippet != "" {
		p.LineSnippet = snippet
		startLine, _ := m.sectionLineRangeForHeading(heading)
		p.LineOccurrence = m.countSnippetOccurrences(snippet, startLine, refLine)
	}
	return p
}

// RestorePosition applies a previously-captured Position to the current
// document. Falls back gracefully — anchor not found → whole-doc snippet
// search → top of doc — and never panics on nil/empty state.
func (m *Model) RestorePosition(p Position) {
	if len(m.lines) == 0 {
		return
	}

	target := -1
	sectionStart, sectionEnd := 0, len(m.lines)
	anchorMatched := false

	if p.HeadingAnchor != "" && m.index != nil {
		if sections, ok := m.index.Lookup(p.HeadingAnchor); ok && len(sections) > 0 {
			i := p.HeadingIndex
			if i < 0 || i >= len(sections) {
				i = 0
			}
			if span := m.findSpanForNode(sections[i].Start); span != nil {
				heading, _ := sections[i].Start.(*ast.Heading)
				sectionStart, sectionEnd = m.sectionLineRangeForHeading(heading)
				target = sectionStart
				anchorMatched = true
			}
		}
	}

	if p.LineSnippet != "" {
		if li, ok := m.findSnippetLine(p.LineSnippet, p.LineOccurrence, sectionStart, sectionEnd); ok {
			target = li
		} else if !anchorMatched {
			// Section search degenerated to whole-doc, no match — try
			// occurrence 0 across the whole document as a final attempt.
			if li, ok := m.findSnippetLine(p.LineSnippet, 0, 0, len(m.lines)); ok {
				target = li
			}
		}
	}

	if target < 0 {
		m.lineOffset = 0
		return
	}
	m.lineOffset = target
	if p.CursorPositioned {
		m.cursorPositioned = true
		m.cursorMode = p.CursorMode
		m.cursorLine = target
		m.cursorCol = p.CursorCol
	}
	m.clampOffsets()
}

// sectionLineRangeForHeading returns the [startLine, endLine) range of
// m.lines covered by the section whose heading is the given AST node.
// The range starts at the heading's own line and ends at the line of
// the next same-or-shallower heading (or len(m.lines) when there is
// none). nil heading or empty/missing data returns the whole document.
func (m *Model) sectionLineRangeForHeading(heading *ast.Heading) (int, int) {
	if heading == nil || m.spanTree == nil || len(m.lines) == 0 {
		return 0, len(m.lines)
	}
	startSpan := m.findSpanForNode(heading)
	if startSpan == nil {
		return 0, len(m.lines)
	}
	startLine := m.findLineForOffset(startSpan.Start)
	if startLine < 0 {
		startLine = 0
	}
	endLine := len(m.lines)
	for s := m.spanTree; s != nil; s = s.Next {
		if s.Start <= startSpan.Start {
			continue
		}
		if h, ok := s.Node.(*ast.Heading); ok && h.Level <= heading.Level {
			if li := m.findLineForOffset(s.Start); li >= 0 {
				endLine = li
			}
			break
		}
	}
	return startLine, endLine
}

// findSnippetLine returns the index of the n-th line in [start,end)
// whose stripped/trimmed content equals snippet, where n is occurrence
// (0-based). Returns (idx, true) on success, (-1, false) on miss.
func (m *Model) findSnippetLine(snippet string, occurrence, start, end int) (int, bool) {
	if start < 0 {
		start = 0
	}
	if end > len(m.lines) {
		end = len(m.lines)
	}
	if start >= end {
		return -1, false
	}
	seen := 0
	for i := start; i < end; i++ {
		if lineSnippet(m.lines[i].content) == snippet {
			if seen == occurrence {
				return i, true
			}
			seen++
		}
	}
	return -1, false
}

// countSnippetOccurrences counts lines in [start,end) whose stripped
// content equals snippet.
func (m *Model) countSnippetOccurrences(snippet string, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(m.lines) {
		end = len(m.lines)
	}
	n := 0
	for i := start; i < end; i++ {
		if lineSnippet(m.lines[i].content) == snippet {
			n++
		}
	}
	return n
}

// lineSnippet returns the ANSI-stripped, whitespace-trimmed content of
// a rendered line — the form Position uses as a stable text key.
func lineSnippet(content string) string {
	return strings.TrimSpace(ansi.Strip(content))
}
