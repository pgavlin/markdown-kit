package view

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ansiSlice
// ---------------------------------------------------------------------------

func TestAnsiSlice_PlainText(t *testing.T) {
	result := ansiSlice("hello world", 2, 7)
	// When start > 0, a \033[0m preamble is emitted to reset prior state.
	stripped := ansi.Strip(result)
	assert.Equal(t, "llo w", stripped)
	assert.True(t, strings.HasPrefix(result, "\033[0m"), "should start with SGR reset preamble")
}

func TestAnsiSlice_StartZero(t *testing.T) {
	result := ansiSlice("hello world", 0, 5)
	assert.Equal(t, "hello", result)
}

func TestAnsiSlice_FullString(t *testing.T) {
	input := "hello world"
	result := ansiSlice(input, 0, 11)
	assert.Equal(t, input, result)
}

func TestAnsiSlice_StartGEEnd(t *testing.T) {
	assert.Equal(t, "", ansiSlice("hello", 5, 3))
	assert.Equal(t, "", ansiSlice("hello", 3, 3))
}

func TestAnsiSlice_WithSGR(t *testing.T) {
	// Bold "hello" followed by normal " world".
	input := "\033[1mhello\033[0m world"
	result := ansiSlice(input, 0, 5)
	stripped := ansi.Strip(result)
	assert.Equal(t, "hello", stripped)
	assert.Contains(t, result, "\033[1m")
}

func TestAnsiSlice_MiddleWithSGR(t *testing.T) {
	// Cut from middle: the preamble should restore SGR state.
	input := "\033[1mhello\033[0m world"
	result := ansiSlice(input, 3, 8)
	stripped := ansi.Strip(result)
	assert.Equal(t, "lo wo", stripped)
	// Should start with a reset preamble since start > 0.
	assert.True(t, strings.HasPrefix(result, "\033[0m"), "should start with SGR reset preamble")
}

func TestAnsiSlice_OSC8Hyperlink(t *testing.T) {
	// "Before " + hyperlink("anchor link", "#target") + " after"
	osc8Set := ansi.SetHyperlink("#target")
	osc8Rst := ansi.ResetHyperlink()
	input := "Before " + osc8Set + "anchor link" + osc8Rst + " after"

	// Slice the hyperlink text exactly.
	result := ansiSlice(input, 7, 18)
	stripped := ansi.Strip(result)
	assert.Equal(t, "anchor link", stripped)
	// Should contain the hyperlink set and reset.
	assert.Contains(t, result, osc8Set, "should contain OSC 8 set")
	assert.Contains(t, result, osc8Rst, "should contain OSC 8 reset")
}

func TestAnsiSlice_NoEmptyHyperlinks(t *testing.T) {
	// Split at the hyperlink boundary: the "before" slice should NOT contain
	// the OSC 8 set, and no empty hyperlinks should appear.
	osc8Set := ansi.SetHyperlink("#target")
	osc8Rst := ansi.ResetHyperlink()
	input := "Before " + osc8Set + "anchor link" + osc8Rst + " after"

	before := ansiSlice(input, 0, 7)
	assert.NotContains(t, before, osc8Set, "before should not contain OSC 8 set")
	assert.NotContains(t, before, osc8Rst, "before should not contain OSC 8 reset")

	after := ansiSlice(input, 18, 24)
	assert.NotContains(t, after, osc8Set, "after should not contain OSC 8 set")
}

func TestAnsiSlice_EmptyHyperlinkStripped(t *testing.T) {
	// An input with an already-empty hyperlink (set immediately followed by reset).
	osc8Set := ansi.SetHyperlink("url")
	osc8Rst := ansi.ResetHyperlink()
	input := "text" + osc8Set + osc8Rst + " more"

	result := ansiSlice(input, 0, 9)
	stripped := ansi.Strip(result)
	assert.Equal(t, "text more", stripped)
	// The empty hyperlink should be eliminated.
	assert.NotContains(t, result, osc8Set, "empty hyperlink set should be stripped")
}

func TestAnsiSlice_PartialHyperlink(t *testing.T) {
	// Slice through the middle of a hyperlink: should include OSC 8 set/reset
	// for just the visible portion.
	osc8Set := ansi.SetHyperlink("url")
	osc8Rst := ansi.ResetHyperlink()
	input := "aa" + osc8Set + "bbcc" + osc8Rst + "dd"

	result := ansiSlice(input, 3, 6)
	stripped := ansi.Strip(result)
	assert.Equal(t, "bcc", stripped)
	// Should contain hyperlink for the partial content.
	assert.Contains(t, result, osc8Set)
	assert.Contains(t, result, osc8Rst)
}

// ---------------------------------------------------------------------------
// ansiCut
// ---------------------------------------------------------------------------

func TestAnsiCut_PlainText(t *testing.T) {
	result := ansiCut("hello world", 2, 7)
	assert.Equal(t, "llo w", result)
}

func TestAnsiCut_WithANSICodes(t *testing.T) {
	// Bold "hello" followed by normal " world".
	input := "\033[1mhello\033[0m world"
	result := ansiCut(input, 0, 5)
	// The visible text should be "hello".
	stripped := ansi.Strip(result)
	assert.Equal(t, "hello", stripped)
	// The result should still contain the bold ANSI escape.
	assert.Contains(t, result, "\033[1m")
}

func TestAnsiCut_StartGEEnd(t *testing.T) {
	assert.Equal(t, "", ansiCut("hello", 5, 3))
	assert.Equal(t, "", ansiCut("hello", 3, 3))
}

func TestAnsiCut_StartLEZero(t *testing.T) {
	result := ansiCut("hello world", 0, 5)
	assert.Equal(t, "hello", ansi.Strip(result))

	result = ansiCut("hello world", -2, 5)
	assert.Equal(t, "hello", ansi.Strip(result))
}

func TestAnsiCut_FullWidth(t *testing.T) {
	input := "hello world"
	result := ansiCut(input, 0, len(input))
	assert.Equal(t, input, result)
}

func TestAnsiCut_MiddleWithANSI(t *testing.T) {
	// "he\033[31mllo wo\033[0mrld" — cut the red portion in the middle.
	input := "he\033[31mllo wo\033[0mrld"
	result := ansiCut(input, 3, 7)
	stripped := ansi.Strip(result)
	assert.Equal(t, "lo w", stripped)
}

// ---------------------------------------------------------------------------
// ansiTruncate
// ---------------------------------------------------------------------------

func TestAnsiTruncate_PlainText(t *testing.T) {
	result := ansiTruncate("hello world", 5)
	assert.Equal(t, "hello", result)
}

func TestAnsiTruncate_WidthZero(t *testing.T) {
	assert.Equal(t, "", ansiTruncate("hello", 0))
}

func TestAnsiTruncate_NegativeWidth(t *testing.T) {
	assert.Equal(t, "", ansiTruncate("hello", -1))
}

func TestAnsiTruncate_WithANSICodes(t *testing.T) {
	input := "\033[1mhello\033[0m world"
	result := ansiTruncate(input, 5)
	stripped := ansi.Strip(result)
	assert.Equal(t, "hello", stripped)
	// Bold escape should be preserved.
	assert.Contains(t, result, "\033[1m")
}

func TestAnsiTruncate_WidthExceedsContent(t *testing.T) {
	input := "short"
	result := ansiTruncate(input, 100)
	assert.Equal(t, "short", ansi.Strip(result))
}

// ---------------------------------------------------------------------------
// applySelection
// ---------------------------------------------------------------------------

func TestApplySelection_Highlighted(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Hello\n\nSome text here.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)
	require.True(t, len(m.lines) > 0)

	// Select the first heading span and enable highlighting.
	require.NotNil(t, m.spanTree)
	m.SelectSpan(m.spanTree, true)

	output := m.View()
	// Reverse video should appear somewhere in the output.
	assert.Contains(t, output, "\033[7m", "selection should contain reverse video start")
	assert.Contains(t, output, "\033[27m", "selection should contain reverse video end")
}

func TestApplySelection_NoOverlap(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Hello\n\nSome text here.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)
	require.True(t, len(m.lines) >= 2)

	// Create a selection that doesn't overlap with line 1 (the blank line / "Some text" line).
	ln := m.lines[0]
	content := ln.content

	// Manually call applySelection with a selection that doesn't overlap this line.
	m.selection = m.spanTree
	m.highlightSelection = true
	m.selectionStart = ln.end + 100
	m.selectionEnd = ln.end + 200

	result := m.applySelection(ln, content)
	assert.Equal(t, content, result, "non-overlapping selection should return content unchanged")
}

func TestApplySelection_PartialLine(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello world here.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)
	require.True(t, len(m.lines) > 0)

	ln := m.lines[0]
	content := ln.content

	// Set up a partial selection in the middle of the line.
	m.selection = m.spanTree
	m.highlightSelection = true
	// Select bytes corresponding to "world" if the line starts at offset 0.
	// We pick safe byte offsets within the line.
	m.selectionStart = ln.start + 2
	m.selectionEnd = ln.start + 6

	result := m.applySelection(ln, content)
	// Should contain both reverse video markers.
	assert.Contains(t, result, "\033[7m")
	assert.Contains(t, result, "\033[27m")
}

// ---------------------------------------------------------------------------
// SelectAnchor
// ---------------------------------------------------------------------------

func TestSelectAnchor_FindsHeading(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# First\n\nText\n\n## Second\n\nMore text\n")
	m.SetSize(80, 24)

	found := m.SelectAnchor("second")
	assert.True(t, found, "should find the ## Second heading")
	require.NotNil(t, m.Selection())
}

func TestSelectAnchor_NotFound(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# First\n\nText\n")
	m.SetSize(80, 24)

	found := m.SelectAnchor("nonexistent")
	assert.False(t, found, "should not find a nonexistent anchor")
}

func TestSelectAnchor_NoIndex(t *testing.T) {
	m := NewModel()
	// Don't set any text, so index is nil.
	found := m.SelectAnchor("anything")
	assert.False(t, found, "should return false when no index exists")
}

func TestSelectAnchor_FirstHeading(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# First\n\nText\n\n## Second\n\nMore text\n")
	m.SetSize(80, 24)

	found := m.SelectAnchor("first")
	assert.True(t, found, "should find the # First heading")
	require.NotNil(t, m.Selection())
}

// ---------------------------------------------------------------------------
// View edge cases
// ---------------------------------------------------------------------------

func TestView_ZeroWidthHeight(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Hello\n\nSome text.\n")
	// Don't set size (width=0, height=0).
	assert.Equal(t, "", m.View())

	// Width set but height 0.
	m.SetSize(80, 0)
	assert.Equal(t, "", m.View())

	// Height set but width 0.
	m.SetSize(0, 24)
	assert.Equal(t, "", m.View())
}

func TestView_NoContent(t *testing.T) {
	m := NewModel()
	m.SetSize(80, 24)

	output := m.View()
	lines := strings.Split(output, "\n")
	assert.Equal(t, 24, len(lines), "viewport should have exactly 24 lines")
	for i, ln := range lines {
		w := ansi.StringWidth(ln)
		assert.Equal(t, 80, w, "line %d should have width 80, got %d", i, w)
	}
}

func TestView_LongDocumentScrolling(t *testing.T) {
	// Build a document with many lines.
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("Line of text number.\n\n")
	}

	m := NewModel()
	m.SetText("test.md", sb.String())
	m.SetSize(80, 10)

	output := m.View()
	require.NotEmpty(t, output)

	lines := strings.Split(output, "\n")
	assert.Equal(t, 10, len(lines), "viewport should have exactly 10 lines")

	// Scroll to the end with G.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	outputEnd := m.View()
	assert.NotEqual(t, output, outputEnd, "scrolled-to-end should differ from start")
}

// ---------------------------------------------------------------------------
// handleKey comprehensive
// ---------------------------------------------------------------------------

func keyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func specialKeyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestHandleKey_GGoesToEnd(t *testing.T) {
	m := setupLongDoc(t)

	m, _ = m.Update(keyMsg('G'))
	expected := len(m.lines) - m.pageSize
	if expected < 0 {
		expected = 0
	}
	assert.Equal(t, expected, m.lineOffset, "'G' should scroll to end")
	assert.Equal(t, 0, m.columnOffset, "'G' should reset column offset")
}

func TestHandleKey_gGoesToTop(t *testing.T) {
	m := setupLongDoc(t)

	// First scroll down, then go to top.
	m, _ = m.Update(keyMsg('G'))
	m, _ = m.Update(keyMsg('g'))
	assert.Equal(t, 0, m.lineOffset, "'g' should scroll to top")
	assert.Equal(t, 0, m.columnOffset, "'g' should reset column offset")
}

func TestHandleKey_PgDownCtrlF(t *testing.T) {
	m := setupLongDoc(t)
	pageSize := m.pageSize

	m, _ = m.Update(specialKeyMsg(tea.KeyPgDown))
	assert.Equal(t, pageSize, m.lineOffset, "pgdown should move by pageSize")

	// Reset.
	m, _ = m.Update(keyMsg('g'))

	m, _ = m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	assert.Equal(t, pageSize, m.lineOffset, "ctrl+f should move by pageSize")
}

func TestHandleKey_PgUpCtrlB(t *testing.T) {
	m := setupLongDoc(t)
	pageSize := m.pageSize

	// Go to end, then page up.
	m, _ = m.Update(keyMsg('G'))
	endOffset := m.lineOffset

	m, _ = m.Update(specialKeyMsg(tea.KeyPgUp))
	expected := endOffset - pageSize
	if expected < 0 {
		expected = 0
	}
	assert.Equal(t, expected, m.lineOffset, "pgup should move back by pageSize")

	// Also test ctrl+b.
	m, _ = m.Update(keyMsg('G'))
	m, _ = m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	assert.Equal(t, expected, m.lineOffset, "ctrl+b should move back by pageSize")
}

func TestHandleKey_HLColumnScroll(t *testing.T) {
	// Create model with wrap disabled so horizontal scrolling is meaningful.
	m := NewModel()
	m.SetWrap(false)
	longLine := strings.Repeat("x", 200)
	m.SetText("test.md", longLine+"\n")
	m.SetSize(80, 24)

	// 'l' should increase column offset.
	m, _ = m.Update(keyMsg('l'))
	assert.Equal(t, 1, m.columnOffset, "'l' should increment columnOffset")

	// 'h' should decrease column offset.
	m, _ = m.Update(keyMsg('h'))
	assert.Equal(t, 0, m.columnOffset, "'h' should decrement columnOffset")

	// 'h' at 0 should clamp to 0.
	m, _ = m.Update(keyMsg('h'))
	assert.Equal(t, 0, m.columnOffset, "'h' at 0 should stay at 0")
}

func TestHandleKey_BraceHeadingNavigation(t *testing.T) {
	md := "# First\n\nText.\n\n## Second\n\nMore text.\n\n### Third\n\nFinal.\n"

	m := NewModel()
	m.SetText("test.md", md)
	m.SetSize(80, 24)

	// '}' should navigate to next heading.
	m, _ = m.Update(keyMsg('}'))
	require.NotNil(t, m.Selection(), "'}' should select a heading")
	firstSel := m.Selection().Start

	// '}' again should advance.
	m, _ = m.Update(keyMsg('}'))
	require.NotNil(t, m.Selection())
	assert.Greater(t, m.Selection().Start, firstSel, "second '}' should advance")

	// '{' should go back.
	m, _ = m.Update(keyMsg('{'))
	require.NotNil(t, m.Selection())
	assert.Equal(t, firstSel, m.Selection().Start, "'{' should go back to first heading")
}

func TestHandleKey_JKUpDown(t *testing.T) {
	m := setupLongDoc(t)

	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, 1, m.lineOffset, "'j' should move down one line")

	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, 2, m.lineOffset)

	m, _ = m.Update(keyMsg('k'))
	assert.Equal(t, 1, m.lineOffset, "'k' should move up one line")

	// Also test arrow keys.
	m, _ = m.Update(specialKeyMsg(tea.KeyDown))
	assert.Equal(t, 2, m.lineOffset, "down arrow should move down")

	m, _ = m.Update(specialKeyMsg(tea.KeyUp))
	assert.Equal(t, 1, m.lineOffset, "up arrow should move up")
}

func TestHandleKey_HomeEnd(t *testing.T) {
	m := setupLongDoc(t)

	// Scroll somewhere in the middle.
	m, _ = m.Update(keyMsg('G'))
	m, _ = m.Update(specialKeyMsg(tea.KeyHome))
	assert.Equal(t, 0, m.lineOffset, "home should go to top")
	assert.Equal(t, 0, m.columnOffset, "home should reset column offset")
}

// setupLongDoc creates a model with enough content to exercise scrolling.
func setupLongDoc(t *testing.T) Model {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("A line of text.\n\n")
	}

	m := NewModel()
	m.SetText("test.md", sb.String())
	m.SetSize(80, 24)
	require.NotNil(t, m.lines)
	require.True(t, len(m.lines) > 24, "document should be longer than viewport")
	return m
}

// ---------------------------------------------------------------------------
// calculateSelectionSpan
// ---------------------------------------------------------------------------

func TestCalculateSelectionSpan_TrimsWhitespace(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Heading\n\nSome text.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.spanTree)

	// Find the heading span. The rendered heading line typically starts with
	// some whitespace or ANSI codes. calculateSelectionSpan should trim
	// leading/trailing whitespace.
	span := m.spanTree
	m.calculateSelectionSpan(span)

	// The trimmed start should be >= the original start.
	assert.GreaterOrEqual(t, m.selectionStart, span.Start,
		"trimmed selectionStart should be >= span.Start")
	assert.LessOrEqual(t, m.selectionEnd, span.End,
		"trimmed selectionEnd should be <= span.End")

	// The selected range should be non-empty for a heading.
	assert.Less(t, m.selectionStart, m.selectionEnd,
		"selection should be non-empty for a heading")
}

func TestCalculateSelectionSpan_AllWhitespace(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Hello\n\nSome text.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)

	// Create a fake span that covers only whitespace/newline region.
	// Find a blank line (line between heading and paragraph).
	var blankLineIdx int
	for i, ln := range m.lines {
		stripped := ansi.Strip(ln.content)
		if strings.TrimSpace(stripped) == "" && i > 0 {
			blankLineIdx = i
			break
		}
	}

	if blankLineIdx > 0 {
		ln := m.lines[blankLineIdx]
		fakeSpan := &renderer.NodeSpan{
			Start: ln.start,
			End:   ln.end,
		}
		m.calculateSelectionSpan(fakeSpan)
		// If all content is whitespace, start == end.
		assert.Equal(t, m.selectionStart, m.selectionEnd,
			"all-whitespace span should have zero-length selection")
	}
}

// ---------------------------------------------------------------------------
// findLineForOffset
// ---------------------------------------------------------------------------

func TestFindLineForOffset_StartOfLine(t *testing.T) {
	// Use lineWriter directly to create predictable lines with known offsets.
	m := Model{}
	m.lines = []line{
		{start: 0, end: 5, content: "hello"},
		{start: 6, end: 11, content: "world"},
		{start: 12, end: 17, content: "three"},
	}

	// Offset at start of line 0 should return 0.
	idx := m.findLineForOffset(0)
	assert.Equal(t, 0, idx)

	// Offset at start of line 1 should return 1.
	idx = m.findLineForOffset(6)
	assert.Equal(t, 1, idx)

	// Offset at start of line 2 should return 2.
	idx = m.findLineForOffset(12)
	assert.Equal(t, 2, idx)
}

func TestFindLineForOffset_MiddleOfLine(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello world.\n\nAnother line.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)
	require.True(t, len(m.lines) > 0)

	// Middle of first line.
	mid := m.lines[0].start + (m.lines[0].end-m.lines[0].start)/2
	idx := m.findLineForOffset(mid)
	assert.Equal(t, 0, idx, "offset in middle of first line should return line 0")
}

func TestFindLineForOffset_GapBetweenLines(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "First line.\n\nSecond line.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)
	require.True(t, len(m.lines) >= 2)

	// The gap between lines is the newline byte after line 0.
	// line[0].end is the byte after the last content byte of line 0.
	// The newline byte is at line[0].end (it was consumed to flush the line).
	// The next line starts at line[1].start.
	gapOffset := m.lines[0].end
	idx := m.findLineForOffset(gapOffset)
	// findLineForOffset uses sort.Search with lines[i].end > offset,
	// so for a gap byte that equals line[0].end, line[0].end > gapOffset is false,
	// so it should return line 1 (or beyond).
	assert.True(t, idx >= 1, "gap offset should resolve to line 1 or later")
}

func TestFindLineForOffset_BeyondEnd(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)

	// Offset well beyond the last line.
	idx := m.findLineForOffset(999999)
	assert.Equal(t, len(m.lines), idx, "offset beyond end should return len(lines)")
}

// ---------------------------------------------------------------------------
// isOffsetWhitespace
// ---------------------------------------------------------------------------

func TestIsOffsetWhitespace_Space(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello world.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)
	require.True(t, len(m.lines) > 0)

	// Find the space between "Hello" and "world" in the rendered content.
	ln := m.lines[0]
	stripped := ansi.Strip(ln.content)
	spaceIdx := strings.Index(stripped, " ")
	require.True(t, spaceIdx >= 0, "should find a space in the content")

	// Map to byte offset. We need to find the actual byte position in content
	// that corresponds to the visible space. For plain text (no theme), the
	// byte offsets align.
	// Use the byte offset of the space in the raw content.
	contentSpaceIdx := strings.Index(ln.content, " ")
	if contentSpaceIdx >= 0 {
		offset := ln.start + contentSpaceIdx
		assert.True(t, m.isOffsetWhitespace(offset), "space should be whitespace")
	}
}

func TestIsOffsetWhitespace_NonWhitespace(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello world.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)
	require.True(t, len(m.lines) > 0)

	ln := m.lines[0]
	// Find the first non-ANSI, non-whitespace byte.
	for i := 0; i < len(ln.content); i++ {
		b := ln.content[i]
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '\033' {
			offset := ln.start + i
			assert.False(t, m.isOffsetWhitespace(offset),
				"non-whitespace byte %q at offset %d should return false", b, offset)
			break
		}
	}
}

func TestIsOffsetWhitespace_OutOfBounds(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)

	// Offset far beyond content.
	assert.True(t, m.isOffsetWhitespace(999999), "out-of-bounds offset should return true")

	// Negative offset (effectively before any line).
	assert.True(t, m.isOffsetWhitespace(-1), "negative offset should return true (no line found)")
}

// ---------------------------------------------------------------------------
// lineWriter
// ---------------------------------------------------------------------------

func TestLineWriter_NewlinesSplitLines(t *testing.T) {
	w := &lineWriter{}
	data := []byte("hello\nworld\n")
	n, err := w.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)

	// "hello\nworld\n" has two newlines, so two flushLine calls.
	// After "hello\n": line 0 is "hello" (bytes 0-5), then newline at 5 advances offset to 6.
	// After "world\n": line 1 is "world" (bytes 6-11), then newline at 11 advances offset to 12.
	assert.Equal(t, 2, len(w.lines))
	assert.Equal(t, "hello", w.lines[0].content)
	assert.Equal(t, "world", w.lines[1].content)
}

func TestLineWriter_ByteOffsets(t *testing.T) {
	w := &lineWriter{}
	_, err := w.Write([]byte("abc\ndef\n"))
	require.NoError(t, err)

	assert.Equal(t, 2, len(w.lines))

	// "abc" occupies bytes 0-3 (exclusive end), then newline at byte 3.
	assert.Equal(t, 0, w.lines[0].start)
	assert.Equal(t, 3, w.lines[0].end)

	// "def" starts at byte 4 (after "abc\n"), occupies 4-7, then newline at 7.
	assert.Equal(t, 4, w.lines[1].start)
	assert.Equal(t, 7, w.lines[1].end)
}

func TestLineWriter_MultipleWrites(t *testing.T) {
	w := &lineWriter{}
	_, err := w.Write([]byte("hel"))
	require.NoError(t, err)
	_, err = w.Write([]byte("lo\nwor"))
	require.NoError(t, err)
	_, err = w.Write([]byte("ld\n"))
	require.NoError(t, err)

	assert.Equal(t, 2, len(w.lines))
	assert.Equal(t, "hello", w.lines[0].content)
	assert.Equal(t, "world", w.lines[1].content)
}

func TestLineWriter_NoTrailingNewline(t *testing.T) {
	w := &lineWriter{}
	_, err := w.Write([]byte("no newline"))
	require.NoError(t, err)

	// Without a trailing newline, the buffered content is not flushed.
	assert.Equal(t, 0, len(w.lines))
	assert.Equal(t, "no newline", w.buf.String())

	// Manually flush the remaining buffer (as the View code does).
	w.flushLine()
	assert.Equal(t, 1, len(w.lines))
	assert.Equal(t, "no newline", w.lines[0].content)
}

func TestLineWriter_LongestLine(t *testing.T) {
	w := &lineWriter{}
	_, err := w.Write([]byte("short\nthis is a longer line\nmed\n"))
	require.NoError(t, err)

	assert.Equal(t, ansi.StringWidth("this is a longer line"), w.longestLine)
}

func TestLineWriter_ANSIState(t *testing.T) {
	w := &lineWriter{}
	_, err := w.Write([]byte("\033[1mbold text\n"))
	require.NoError(t, err)

	assert.Equal(t, 1, len(w.lines))
	// After processing the first line, the ANSI state should include bold.
	assert.Equal(t, "\033[1m", w.ansiState)

	// Write a second line; it should inherit the state as prefix.
	_, err = w.Write([]byte("still bold\n"))
	require.NoError(t, err)

	assert.Equal(t, 2, len(w.lines))
	assert.Equal(t, "\033[1m", w.lines[1].ansiPrefix)
}

func TestLineWriter_EmptyLines(t *testing.T) {
	w := &lineWriter{}
	_, err := w.Write([]byte("\n\n\n"))
	require.NoError(t, err)

	assert.Equal(t, 3, len(w.lines))
	for _, ln := range w.lines {
		assert.Equal(t, "", ln.content)
	}
}

// ---------------------------------------------------------------------------
// updateANSIState (additional tests)
// ---------------------------------------------------------------------------

func TestUpdateANSIState_EmptyState(t *testing.T) {
	result := updateANSIState("", "no ansi here")
	assert.Equal(t, "", result)
}

func TestUpdateANSIState_MultipleSequences(t *testing.T) {
	result := updateANSIState("", "\033[1m\033[31mhello")
	// Output order is canonical: bg, fg, bold, italic, underline.
	assert.Equal(t, "\033[31m\033[1m", result)
}

func TestUpdateANSIState_ResetInMiddle(t *testing.T) {
	result := updateANSIState("\033[1m", "text\033[0m\033[32mgreen")
	assert.Equal(t, "\033[32m", result)
}

func TestUpdateANSIState_IncompleteEscape(t *testing.T) {
	// An incomplete escape sequence (no terminator) should not change state.
	result := updateANSIState("", "\033[1")
	assert.Equal(t, "", result)
}

// ---------------------------------------------------------------------------
// Model: SetText, Clear, GetMarkdown
// ---------------------------------------------------------------------------

func TestModel_Clear(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Hello\n")
	m.SetSize(80, 24)

	require.NotEmpty(t, m.View())

	m.Clear()
	assert.Nil(t, m.lines)
	assert.Nil(t, m.markdown)
}

func TestModel_GetMarkdown(t *testing.T) {
	m := NewModel()
	md := "# Hello\n\nWorld.\n"
	m.SetText("test.md", md)
	assert.Equal(t, []byte(md), m.GetMarkdown())
}

// ---------------------------------------------------------------------------
// Model: SetWrap clears lines
// ---------------------------------------------------------------------------

func TestModel_SetWrapClearsLines(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Hello\n\nSome text.\n")
	m.SetSize(80, 24)

	require.NotNil(t, m.lines)

	m.SetWrap(false)
	assert.Nil(t, m.lines, "SetWrap should clear lines when wrap changes")

	// Toggling back also clears.
	m.SetSize(80, 24)
	require.NotNil(t, m.lines)
	m.SetWrap(true)
	assert.Nil(t, m.lines)
}

// ---------------------------------------------------------------------------
// Model: Selection returns nil when nothing is selected
// ---------------------------------------------------------------------------

func TestModel_SelectionNil(t *testing.T) {
	m := NewModel()
	assert.Nil(t, m.Selection())
}

// ---------------------------------------------------------------------------
// Model: SelectNext / SelectPrevious with nil spanTree
// ---------------------------------------------------------------------------

func TestSelectNext_NilSpanTree(t *testing.T) {
	m := NewModel()
	// No text set, spanTree is nil.
	assert.False(t, m.SelectNext(isHeading))
	assert.False(t, m.SelectPrevious(isHeading))
}

// ---------------------------------------------------------------------------
// Model: View with gutter
// ---------------------------------------------------------------------------

func TestView_WithGutter(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", "# Hello\n\nSome text.\n")
	m.SetGutter(true)
	m.SetSize(80, 24)

	output := m.View()
	require.NotEmpty(t, output)

	lines := strings.Split(output, "\n")
	assert.Equal(t, 24, len(lines), "should have 24 lines including gutter")

	// The last line (gutter) should contain the document name.
	lastLine := ansi.Strip(lines[len(lines)-1])
	assert.Contains(t, lastLine, "test.md")
	// And should contain a percentage.
	assert.Contains(t, lastLine, "%")
}

// ---------------------------------------------------------------------------
// Model: Init returns nil
// ---------------------------------------------------------------------------

func TestModel_Init(t *testing.T) {
	m := NewModel()
	cmd := m.Init()
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Clamp offsets
// ---------------------------------------------------------------------------

func TestClampOffsets_NilLines(t *testing.T) {
	m := NewModel()
	m.lineOffset = 100
	m.columnOffset = 50
	m.clampOffsets()
	// With nil lines, clampOffsets should be a no-op.
	assert.Equal(t, 100, m.lineOffset)
	assert.Equal(t, 50, m.columnOffset)
}

func TestClampOffsets_NegativeValues(t *testing.T) {
	m := setupLongDoc(t)

	m.lineOffset = -5
	m.columnOffset = -10
	m.clampOffsets()
	assert.Equal(t, 0, m.lineOffset, "negative lineOffset should clamp to 0")
	assert.Equal(t, 0, m.columnOffset, "negative columnOffset should clamp to 0")
}

// ---------------------------------------------------------------------------
// isLink and isHeading selectors
// ---------------------------------------------------------------------------

func TestIsLink(t *testing.T) {
	// For non-link node kinds, isLink returns false, false.
	heading := &testNode{kind: headingKind}
	highlight, ok := isLink(heading)
	assert.False(t, highlight)
	assert.False(t, ok)
}

func TestIsHeading(t *testing.T) {
	heading := &testNode{kind: headingKind}
	highlight, ok := isHeading(heading)
	assert.True(t, highlight)
	assert.True(t, ok)

	nonHeading := &testNode{kind: linkKind}
	highlight, ok = isHeading(nonHeading)
	assert.False(t, highlight)
	assert.False(t, ok)
}

// testNode is a minimal ast.Node implementation for testing selectors.
var (
	headingKind = ast.KindHeading
	linkKind    = ast.KindLink
)

type testNode struct {
	ast.BaseBlock
	kind ast.NodeKind
}

func (n *testNode) Kind() ast.NodeKind                         { return n.kind }
func (n *testNode) Dump(w io.Writer, source []byte, level int) {}

// ---------------------------------------------------------------------------
// scrollToOffset
// ---------------------------------------------------------------------------

func TestScrollToOffset(t *testing.T) {
	m := setupLongDoc(t)

	// Scroll to an offset at the start of a line in the middle of the document.
	midLine := len(m.lines) / 2
	offset := m.lines[midLine].start
	m.scrollToOffset(offset)

	// findLineForOffset returns the line whose range contains the offset.
	expectedLine := m.findLineForOffset(offset)
	assert.Equal(t, expectedLine, m.lineOffset)
}

// ---------------------------------------------------------------------------
// selected helper
// ---------------------------------------------------------------------------

func TestSelected(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Hello\n\nWorld.\n")
	m.SetSize(80, 24)

	// No selection: selected should return false.
	assert.False(t, m.selected(0))

	// Select first span.
	require.NotNil(t, m.spanTree)
	m.SelectSpan(m.spanTree, true)

	// Offset within selection should return true.
	assert.True(t, m.selected(m.selectionStart))
	// Offset before selection should return false.
	if m.selectionStart > 0 {
		assert.False(t, m.selected(m.selectionStart-1))
	}
	// Offset at end (exclusive) should return false.
	assert.False(t, m.selected(m.selectionEnd))
}

// ---------------------------------------------------------------------------
// Visual mode
// ---------------------------------------------------------------------------

func TestVisualMode_EnterExit(t *testing.T) {
	m := setupLongDoc(t)

	assert.False(t, m.VisualMode(), "should not start in visual mode")

	// Press 'v' to enter visual mode.
	m, _ = m.Update(keyMsg('v'))
	assert.True(t, m.VisualMode(), "should be in visual mode after 'v'")
	assert.Equal(t, m.lineOffset, m.cursorLine, "cursor should start at top of viewport")
	assert.Equal(t, 0, m.cursorCol, "cursor should start at column 0")

	// Press Esc to exit.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape, Text: ""})
	assert.False(t, m.VisualMode(), "should exit visual mode on Esc")
}

func TestVisualMode_VAgainExits(t *testing.T) {
	m := setupLongDoc(t)

	m, _ = m.Update(keyMsg('v'))
	assert.True(t, m.VisualMode())

	m, _ = m.Update(keyMsg('v'))
	assert.False(t, m.VisualMode(), "pressing 'v' again should exit visual mode")
}

func TestVisualMode_CursorMovement(t *testing.T) {
	m := setupLongDoc(t)

	m, _ = m.Update(keyMsg('v'))
	startLine := m.cursorLine

	// Move down with j.
	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, startLine+1, m.cursorLine, "j should move cursor down")

	// Move up with k.
	m, _ = m.Update(keyMsg('k'))
	assert.Equal(t, startLine, m.cursorLine, "k should move cursor back up")

	// Move right with l.
	m, _ = m.Update(keyMsg('l'))
	assert.Equal(t, 1, m.cursorCol, "l should move cursor right")

	// Move left with h.
	m, _ = m.Update(keyMsg('h'))
	assert.Equal(t, 0, m.cursorCol, "h should move cursor left")

	// h at column 0 should stay at 0.
	m, _ = m.Update(keyMsg('h'))
	assert.Equal(t, 0, m.cursorCol, "h at col 0 should stay at 0")
}

func TestVisualMode_GotoTopBottom(t *testing.T) {
	m := setupLongDoc(t)

	m, _ = m.Update(keyMsg('v'))

	// G should move cursor to last line.
	m, _ = m.Update(keyMsg('G'))
	assert.Equal(t, len(m.lines)-1, m.cursorLine, "G should move cursor to last line")

	// g should move cursor to first line.
	m, _ = m.Update(keyMsg('g'))
	assert.Equal(t, 0, m.cursorLine, "g should move cursor to first line")
}

func TestVisualMode_LineStartEnd(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello world here.\n")
	m.SetSize(80, 24)

	m, _ = m.Update(keyMsg('v'))

	// $ should go to end of line.
	m, _ = m.Update(keyMsg('$'))
	lineWidth := m.lineVisibleWidth(m.cursorLine)
	assert.Equal(t, lineWidth-1, m.cursorCol, "$ should move to last column")

	// 0 should go to start of line.
	m, _ = m.Update(keyMsg('0'))
	assert.Equal(t, 0, m.cursorCol, "0 should move to column 0")
}

func TestVisualMode_SelectionBounds(t *testing.T) {
	m := setupLongDoc(t)

	m, _ = m.Update(keyMsg('v'))
	anchorLine := m.cursorLine

	// Move cursor down two lines.
	m, _ = m.Update(keyMsg('j'))
	m, _ = m.Update(keyMsg('j'))

	sl, sc, el, ec := m.visualSelectionBounds()
	assert.Equal(t, anchorLine, sl, "start line should be anchor")
	assert.Equal(t, 0, sc, "start col should be anchor col")
	assert.Equal(t, anchorLine+2, el, "end line should be cursor")
	assert.Equal(t, m.cursorCol, ec, "end col should be cursor col")
}

func TestVisualMode_SelectionBoundsReversed(t *testing.T) {
	m := setupLongDoc(t)

	// Scroll down first so we have room to move up.
	m, _ = m.Update(keyMsg('G'))
	m, _ = m.Update(keyMsg('v'))
	anchorLine := m.cursorLine

	// Move cursor up - selection should be reversed.
	m, _ = m.Update(keyMsg('k'))
	m, _ = m.Update(keyMsg('k'))

	sl, _, el, _ := m.visualSelectionBounds()
	assert.Equal(t, anchorLine-2, sl, "start should be cursor (earlier)")
	assert.Equal(t, anchorLine, el, "end should be anchor (later)")
}

func TestVisualMode_YankContent(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello world.\n\nSecond line.\n")
	m.SetSize(80, 24)

	m, _ = m.Update(keyMsg('v'))
	// Move to end of first line.
	m, _ = m.Update(keyMsg('$'))

	content := m.yankVisualSelection()
	stripped := m.strippedLineContent(0)
	assert.Equal(t, stripped, content, "yank of full first line should match stripped content")
}

func TestVisualMode_YankMultipleLines(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Line one.\n\nLine two.\n")
	m.SetSize(80, 24)

	m, _ = m.Update(keyMsg('v'))
	// Move down to include second line.
	m, _ = m.Update(keyMsg('j'))
	m, _ = m.Update(keyMsg('$'))

	content := m.yankVisualSelection()
	assert.Contains(t, content, "\n", "multi-line yank should contain newline")
}

func TestVisualMode_GutterIndicator(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", "# Hello\n\nSome text.\n")
	m.SetGutter(true)
	m.SetSize(80, 24)

	m, _ = m.Update(keyMsg('v'))

	output := m.View()
	lines := strings.Split(output, "\n")
	lastLine := ansi.Strip(lines[len(lines)-1])
	assert.Contains(t, lastLine, "-- VISUAL --", "gutter should show visual mode indicator")
}

func TestVisualMode_RenderHighlighting(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello world here.\n")
	m.SetSize(80, 24)

	m, _ = m.Update(keyMsg('v'))
	// Move right to select a few characters.
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))

	output := m.View()
	// Should contain reverse video for the visual selection.
	assert.Contains(t, output, "\033[7m", "visual selection should use reverse video")
}

func TestVisualMode_ClearsNodeSelection(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "# Hello\n\nSome text.\n")
	m.SetSize(80, 24)

	// Select a heading first.
	m, _ = m.Update(keyMsg('}'))
	require.NotNil(t, m.Selection())

	// Enter visual mode - should clear node selection.
	m, _ = m.Update(keyMsg('v'))
	assert.Nil(t, m.Selection(), "visual mode should clear node selection")
}

func TestVisualMode_WordMotions(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Hello world foo bar.\n")
	m.SetSize(80, 24)

	m, _ = m.Update(keyMsg('v'))

	// w should move to next word.
	m, _ = m.Update(keyMsg('w'))
	assert.True(t, m.cursorCol > 0, "w should advance cursor")
	wCol := m.cursorCol

	// b should move back.
	m, _ = m.Update(keyMsg('b'))
	assert.True(t, m.cursorCol < wCol, "b should move cursor back")

	// e should move to end of word.
	m, _ = m.Update(keyMsg('e'))
	assert.True(t, m.cursorCol > 0, "e should advance cursor")
}
