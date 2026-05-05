package view

// TopLine returns the snippet and occurrence index of the reference
// line -- the cursor line when cursor mode has been positioned,
// otherwise the top of the viewport. The pair is what the snippet-
// search-based source-line resolver in cmd/md uses to find the
// matching line in the on-disk source markdown.
//
// Mirrors Position()'s reference-line selection so the editor lands
// on the same line RestorePosition would re-anchor to.
func (m *Model) TopLine() (snippet string, occurrence int) {
	refLine := m.lineOffset
	if m.cursorPositioned && m.cursorLine >= 0 && m.cursorLine < len(m.lines) {
		refLine = m.cursorLine
	}
	if len(m.lines) == 0 || refLine < 0 || refLine >= len(m.lines) {
		return "", 0
	}

	snippet = lineSnippet(m.lines[refLine].content)
	if snippet == "" {
		return "", 0
	}

	heading, _ := m.enclosingHeading(refLine)
	startLine, _ := m.sectionLineRangeForHeading(heading)
	occurrence = m.countSnippetOccurrences(snippet, startLine, refLine)
	return snippet, occurrence
}
