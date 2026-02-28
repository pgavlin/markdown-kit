package view

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testdataPath = filepath.Join("..", "internal", "testdata")

func TestModel_Render(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("getting-started.md", string(source))
	m.SetGutter(true)
	m.SetSize(80, 24)

	// Trigger a window size to initialize rendering.

	output := m.View()
	require.NotEmpty(t, output)

	// Verify basic properties of the rendered output.
	lines := strings.Split(output, "\n")
	assert.Equal(t, 24, len(lines), "should have exactly 24 lines for 80x24 viewport")

	// Each line should be at most 80 visible characters wide.
	for i, line := range lines {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, 80, "line %d is %d chars wide", i, w)
	}

	// The last line should be the gutter with the document name.
	lastLine := ansi.Strip(lines[len(lines)-1])
	assert.Contains(t, lastLine, "getting-started.md", "gutter should contain document name")
}

func TestModel_Navigation(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("getting-started.md", string(source))
	m.SetSize(80, 24)

	_ = m.View() // trigger rendering

	// Scroll down.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	assert.Equal(t, 1, m.lineOffset)

	// Scroll up.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	assert.Equal(t, 0, m.lineOffset)

	// Go to top.
	m.lineOffset = 10
	m, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	assert.Equal(t, 0, m.lineOffset)
	assert.Equal(t, 0, m.columnOffset)

	// Next link selection.
	m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	assert.NotNil(t, m.Selection(), "should have selected a link")
}

func TestUpdateANSIState(t *testing.T) {
	// A reset clears accumulated state.
	assert.Equal(t, "", updateANSIState("\033[48;2;50;50;50m", "\033[0m"))

	// Non-reset sequences are tracked by category (bg, fg, bold, etc.).
	assert.Equal(t, "\033[48;2;50;50;50m\033[1m",
		updateANSIState("\033[48;2;50;50;50m", "\033[1mhello"))

	// Reset followed by new sequence keeps only the new sequence.
	assert.Equal(t, "\033[38;2;255;0;0m",
		updateANSIState("\033[48;2;50;50;50m", "\033[0m\033[38;2;255;0;0mred"))

	// Plain text does not change state.
	assert.Equal(t, "\033[1m", updateANSIState("\033[1m", "hello world"))

	// Repeated foreground color changes replace rather than accumulate.
	// This is critical for syntax-highlighted code blocks where each token
	// changes the foreground color without resetting.
	state := ""
	for i := 0; i < 100; i++ {
		state = updateANSIState(state, fmt.Sprintf("\033[38;2;%d;%d;%dmtoken", i, i, i))
	}
	assert.Equal(t, "\033[38;2;99;99;99m", state, "state should contain only the last foreground color")
	assert.Less(t, len(state), 100, "state should not grow with repeated foreground changes")
}

func TestModel_ANSIPrefixAfterScroll(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("getting-started.md", string(source))
	m.SetSize(80, 24)

	// Get the initial output (not scrolled).
	initial := m.View()

	// Scroll down several lines.
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	scrolled := m.View()

	// The scrolled output should still contain ANSI escape sequences
	// (background color should be re-established via the prefix).
	assert.NotEqual(t, initial, scrolled)
	assert.True(t, strings.Contains(scrolled, "\033["), "scrolled output should contain ANSI sequences")

	// The first visible line when scrolled should start with the ANSI state
	// prefix, not raw text.
	assert.True(t, strings.HasPrefix(scrolled, "\033["),
		"scrolled output should start with ANSI prefix to restore theme colors")
}

func TestView_NoLineExceedsTerminalWidth(t *testing.T) {
	// Regression test: tab characters in code blocks caused ansi.StringWidth
	// to undercount the visible width (tabs return 0). The right padding was
	// then too large, making lines wider than the terminal and causing wrapping.
	md := "# Heading\n\n```go\npackage main\n\nimport (\n\t\"context\"\n\t\"fmt\"\n\t\"log\"\n\n\tagentsdk \"example.com/sdk\"\n)\n\nfunc main() {\n\tctx := context.Background()\n\topts := &Options{\n\t\tMode: ModePlan,\n\t}\n\tQuery(ctx, \"prompt\", opts)\n}\n```\n"

	for _, tc := range []struct {
		name          string
		width, height int
		contentWidth  int
	}{
		{"215x37 cw160", 215, 37, 160},
		{"80x24 cw0", 80, 24, 0},
		{"120x40 cw100", 120, 40, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(WithTheme(styles.Pulumi))
			m.SetText("test.md", md)
			m.SetGutter(true)
			if tc.contentWidth > 0 {
				m.SetContentWidth(tc.contentWidth)
			}
			m.SetSize(tc.width, tc.height)

			output := m.View()
			lines := strings.Split(output, "\n")
			assert.Equal(t, tc.height, len(lines), "should have exactly %d lines", tc.height)

			for i, line := range lines {
				w := ansi.StringWidth(line)
				assert.LessOrEqual(t, w, tc.width,
					"line %d visible width %d exceeds terminal width %d", i, w, tc.width)
			}
		})
	}
}

func TestModel_WrapToggle(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("getting-started.md", string(source))
	m.SetSize(80, 24)

	// Render with wrapping on (default).
	wrappedOutput := m.View()

	// Render with wrapping off.
	m.SetWrap(false)
	m.SetSize(80, 24)
	unwrappedOutput := m.View()

	// Outputs should differ (wrapping changes line breaks).
	assert.NotEqual(t, wrappedOutput, unwrappedOutput)
}

func TestExpandTabs(t *testing.T) {
	// No tabs — pass through unchanged.
	assert.Equal(t, "hello", expandTabs("hello", 8))

	// Single leading tab at column 0 → 8 spaces.
	assert.Equal(t, "        x", expandTabs("\tx", 8))

	// Tab after 3 characters → advances to column 8 (5 spaces).
	assert.Equal(t, "abc     x", expandTabs("abc\tx", 8))

	// Two leading tabs → 16 spaces.
	assert.Equal(t, "                x", expandTabs("\t\tx", 8))

	// Tab with ANSI codes: codes don't affect column counting.
	assert.Equal(t, "\033[1m        x", expandTabs("\033[1m\tx", 8))

	// ANSI codes between tab and content.
	assert.Equal(t, "        \033[31mred", expandTabs("\t\033[31mred", 8))

	// Tab width 4.
	assert.Equal(t, "    x", expandTabs("\tx", 4))
	assert.Equal(t, "ab  x", expandTabs("ab\tx", 4))
}

func TestView_CodeBlockTabWidth(t *testing.T) {
	// Markdown with a fenced code block containing tab-indented lines.
	md := "# Test\n\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n"

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", string(md))
	m.SetGutter(true)
	m.SetContentWidth(160)
	m.SetSize(215, 37)

	output := m.View()
	require.NotEmpty(t, output)

	lines := strings.Split(output, "\n")
	assert.Equal(t, 37, len(lines), "should have exactly 37 lines for 215x37 viewport with gutter")

	// No line should exceed 215 visible characters (would cause terminal wrapping).
	for i, line := range lines {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, 215, "line %d is %d chars wide (exceeds terminal width 215)", i, w)
	}

	// The tab-indented line should contain spaces, not tabs, in the output.
	// Find the line containing "Println".
	found := false
	for _, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, "Println") {
			found = true
			assert.NotContains(t, stripped, "\t",
				"tab characters should be expanded to spaces in view output")
			// The indentation should be 8 spaces (one tab at column 0).
			trimmed := strings.TrimLeft(stripped, " ")
			indent := len(stripped) - len(trimmed)
			// Account for centering margin (215-160)/2 = 27.
			assert.Equal(t, 27+8, indent,
				"tab should expand to 8 spaces at column 0 (plus 27 char margin)")
			break
		}
	}
	assert.True(t, found, "should find the Println line in the output")
}

func TestView_CodeBlockNoExtraBlanks(t *testing.T) {
	// Simulate the problematic README content: a fenced code block with
	// consecutive tab-indented lines (no blank lines between them).
	md := "```go\nimport (\n\t\"context\"\n\t\"fmt\"\n\t\"log\"\n)\n```\n"

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", string(md))
	m.SetGutter(true)
	m.SetContentWidth(160)
	m.SetSize(215, 37)

	output := m.View()
	lines := strings.Split(output, "\n")
	assert.Equal(t, 37, len(lines), "should have exactly 37 lines")

	// All lines must fit within the terminal width.
	for i, line := range lines {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, 215, "line %d is %d chars wide", i, w)
	}

	// Find the "context", "fmt", "log" lines and verify they are consecutive
	// (no extra blank lines between them).
	var importLines []int
	for i, line := range lines {
		stripped := ansi.Strip(line)
		stripped = strings.TrimSpace(stripped)
		if stripped == `"context"` || stripped == `"fmt"` || stripped == `"log"` {
			importLines = append(importLines, i)
		}
	}
	require.Equal(t, 3, len(importLines),
		"should find exactly 3 import lines (context, fmt, log)")

	// Lines should be consecutive (no extra blank lines between them).
	assert.Equal(t, importLines[0]+1, importLines[1],
		"fmt should immediately follow context (no extra blank line)")
	assert.Equal(t, importLines[1]+1, importLines[2],
		"log should immediately follow fmt (no extra blank line)")
}

// ---------------------------------------------------------------------------
// Programmatic scroll methods
// ---------------------------------------------------------------------------

func setupScrollDoc(t *testing.T) Model {
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

func TestScrollDown(t *testing.T) {
	m := setupScrollDoc(t)
	m.ScrollDown(5)
	assert.Equal(t, 5, m.lineOffset)
}

func TestScrollUp(t *testing.T) {
	m := setupScrollDoc(t)
	m.ScrollDown(10)
	m.ScrollUp(3)
	assert.Equal(t, 7, m.lineOffset)
}

func TestScrollUpClampsToZero(t *testing.T) {
	m := setupScrollDoc(t)
	m.ScrollUp(100)
	assert.Equal(t, 0, m.lineOffset)
}

func TestGotoTop(t *testing.T) {
	m := setupScrollDoc(t)
	m.ScrollDown(50)
	m.ScrollRight(10)
	m.GotoTop()
	assert.Equal(t, 0, m.lineOffset)
	assert.Equal(t, 0, m.columnOffset)
}

func TestGotoBottom(t *testing.T) {
	m := setupScrollDoc(t)
	m.GotoBottom()
	expected := len(m.lines) - m.pageSize
	if expected < 0 {
		expected = 0
	}
	assert.Equal(t, expected, m.lineOffset)
	assert.Equal(t, 0, m.columnOffset)
}

func TestPageDown(t *testing.T) {
	m := setupScrollDoc(t)
	m.PageDown()
	assert.Equal(t, m.pageSize, m.lineOffset)
}

func TestPageUp(t *testing.T) {
	m := setupScrollDoc(t)
	m.GotoBottom()
	endOffset := m.lineOffset
	m.PageUp()
	expected := endOffset - m.pageSize
	if expected < 0 {
		expected = 0
	}
	assert.Equal(t, expected, m.lineOffset)
}

func TestScrollLeft(t *testing.T) {
	m := NewModel()
	m.SetWrap(false)
	m.SetText("test.md", strings.Repeat("x", 200)+"\n")
	m.SetSize(80, 24)

	m.ScrollRight(10)
	assert.Equal(t, 10, m.columnOffset)
	m.ScrollLeft(3)
	assert.Equal(t, 7, m.columnOffset)
}

func TestScrollRight(t *testing.T) {
	m := NewModel()
	m.SetWrap(false)
	m.SetText("test.md", strings.Repeat("x", 200)+"\n")
	m.SetSize(80, 24)

	m.ScrollRight(5)
	assert.Equal(t, 5, m.columnOffset)
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

func TestAtTop(t *testing.T) {
	m := setupScrollDoc(t)
	assert.True(t, m.AtTop())
	m.ScrollDown(1)
	assert.False(t, m.AtTop())
}

func TestAtBottom(t *testing.T) {
	m := setupScrollDoc(t)
	assert.False(t, m.AtBottom())
	m.GotoBottom()
	assert.True(t, m.AtBottom())
}

func TestScrollPercent(t *testing.T) {
	m := setupScrollDoc(t)
	assert.Equal(t, 0.0, m.ScrollPercent())
	m.GotoBottom()
	assert.Equal(t, 1.0, m.ScrollPercent())
}

func TestScrollPercent_ShortDoc(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Short.\n")
	m.SetSize(80, 24)
	assert.Equal(t, 1.0, m.ScrollPercent())
}

func TestLineOffset(t *testing.T) {
	m := setupScrollDoc(t)
	assert.Equal(t, 0, m.LineOffset())
	m.ScrollDown(7)
	assert.Equal(t, 7, m.LineOffset())
}

func TestSetLineOffset(t *testing.T) {
	m := setupScrollDoc(t)
	m.SetLineOffset(10)
	assert.Equal(t, 10, m.lineOffset)
	// Setting beyond max should be clamped.
	m.SetLineOffset(999999)
	assert.LessOrEqual(t, m.lineOffset, len(m.lines)-m.pageSize)
}

func TestColumnOffset(t *testing.T) {
	m := NewModel()
	m.SetWrap(false)
	m.SetText("test.md", strings.Repeat("x", 200)+"\n")
	m.SetSize(80, 24)

	assert.Equal(t, 0, m.ColumnOffset())
	m.ScrollRight(5)
	assert.Equal(t, 5, m.ColumnOffset())
}

func TestSetColumnOffset(t *testing.T) {
	m := NewModel()
	m.SetWrap(false)
	m.SetText("test.md", strings.Repeat("x", 200)+"\n")
	m.SetSize(80, 24)

	m.SetColumnOffset(10)
	assert.Equal(t, 10, m.columnOffset)
}

func TestTotalLineCount(t *testing.T) {
	m := setupScrollDoc(t)
	assert.Equal(t, len(m.lines), m.TotalLineCount())
}

func TestVisibleLineCount(t *testing.T) {
	m := setupScrollDoc(t)
	assert.Equal(t, m.pageSize, m.VisibleLineCount())
}

func TestVisibleLineCount_ShortDoc(t *testing.T) {
	m := NewModel()
	m.SetText("test.md", "Short.\n")
	m.SetSize(80, 24)
	assert.LessOrEqual(t, m.VisibleLineCount(), m.TotalLineCount())
}

// ---------------------------------------------------------------------------
// Size accessors
// ---------------------------------------------------------------------------

func TestWidthHeight(t *testing.T) {
	m := NewModel()
	m.SetSize(120, 40)
	assert.Equal(t, 120, m.Width())
	assert.Equal(t, 40, m.Height())
}

func TestContentWidthAccessor(t *testing.T) {
	m := NewModel()
	assert.Equal(t, 0, m.ContentWidth())
	m.SetContentWidth(100)
	assert.Equal(t, 100, m.ContentWidth())
}

func TestEffectiveWidth(t *testing.T) {
	m := NewModel()
	m.SetSize(120, 40)
	assert.Equal(t, 120, m.EffectiveWidth())
	m.SetContentWidth(80)
	assert.Equal(t, 80, m.EffectiveWidth())
}

func TestDecreaseContentWidth(t *testing.T) {
	m := NewModel()
	m.SetSize(120, 40)
	m.SetText("test.md", "Hello.\n")

	m.DecreaseContentWidth()
	assert.Equal(t, 110, m.contentWidth)
	assert.Nil(t, m.lines, "lines should be cleared after width change")
}

func TestDecreaseContentWidth_Min(t *testing.T) {
	m := NewModel()
	m.SetSize(120, 40)
	m.SetContentWidth(45)
	m.SetText("test.md", "Hello.\n")

	m.DecreaseContentWidth()
	assert.Equal(t, 40, m.contentWidth, "should not go below 40")
}

func TestIncreaseContentWidth(t *testing.T) {
	m := NewModel()
	m.SetSize(120, 40)
	m.SetContentWidth(80)
	m.SetText("test.md", "Hello.\n")

	m.IncreaseContentWidth()
	assert.Equal(t, 90, m.contentWidth)
	assert.Nil(t, m.lines)
}

func TestIncreaseContentWidth_FullWidth(t *testing.T) {
	m := NewModel()
	m.SetSize(120, 40)
	m.SetContentWidth(115)
	m.SetText("test.md", "Hello.\n")

	m.IncreaseContentWidth()
	assert.Equal(t, 0, m.contentWidth, "should reset to 0 (full width) when near viewport width")
}

// ---------------------------------------------------------------------------
// KeyMap tests
// ---------------------------------------------------------------------------

func TestKeyMapDisabled(t *testing.T) {
	m := setupScrollDoc(t)
	m.KeyMap.SetEnabled(false)

	// All key presses should be ignored.
	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, 0, m.lineOffset, "disabled KeyMap should ignore 'j'")

	m, _ = m.Update(keyMsg('G'))
	assert.Equal(t, 0, m.lineOffset, "disabled KeyMap should ignore 'G'")

	m, _ = m.Update(specialKeyMsg(tea.KeyPgDown))
	assert.Equal(t, 0, m.lineOffset, "disabled KeyMap should ignore pgdown")
}

func TestKeyMapCustomBinding(t *testing.T) {
	m := setupScrollDoc(t)

	// Remap Down to 'n' instead of 'j'/'down'.
	m.KeyMap.Down = key.NewBinding(key.WithKeys("n"))

	// 'j' should no longer work.
	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, 0, m.lineOffset, "'j' should not work after rebinding")

	// 'n' should now scroll down.
	m, _ = m.Update(keyMsg('n'))
	assert.Equal(t, 1, m.lineOffset, "'n' should scroll down after rebinding")
}

func TestDefaultKeyMap_WidthBindingsDisabled(t *testing.T) {
	km := DefaultKeyMap()
	// DecreaseWidth and IncreaseWidth should be disabled by default.
	assert.False(t, km.DecreaseWidth.Enabled(), "DecreaseWidth should be disabled by default")
	assert.False(t, km.IncreaseWidth.Enabled(), "IncreaseWidth should be disabled by default")
}

func TestKeyMapSetEnabled_ReEnables(t *testing.T) {
	m := setupScrollDoc(t)
	m.KeyMap.SetEnabled(false)

	// Verify disabled.
	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, 0, m.lineOffset)

	// Re-enable.
	m.KeyMap.SetEnabled(true)
	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, 1, m.lineOffset, "re-enabled KeyMap should respond to 'j'")
}

// ---------------------------------------------------------------------------
// Regression: SetText with stale lineOffset
// ---------------------------------------------------------------------------

func TestSetText_ClampsLineOffset(t *testing.T) {
	// Regression test for a panic in SelectFirstVisible when lineOffset
	// exceeds len(lines) after SetText replaces a long document with a
	// shorter one.
	//
	// Scenario: user scrolls far down in a long document, then follows a
	// link to a shorter page. SetText replaces the content but lineOffset
	// was not reset, so the next key press that calls SelectFirstVisible
	// panicked with "index out of range".

	// Build a long document with links so SelectFirstVisible has work to do.
	var longDoc strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&longDoc, "Line %d with a [link](https://example.com/%d).\n\n", i, i)
	}

	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("long.md", longDoc.String())
	m.SetSize(80, 24)

	require.True(t, len(m.lines) > 100, "long document should produce many lines")

	// Scroll far down.
	m.GotoBottom()
	require.True(t, m.lineOffset > 100, "should be scrolled well past what the short doc will have")

	// Replace with a much shorter document (simulates navigating to a new page).
	shortDoc := "# Short\n\nJust a [link](https://example.com).\n"
	m.SetText("short.md", shortDoc)

	// lineOffset must now be valid for the new document.
	require.Less(t, m.lineOffset, len(m.lines),
		"lineOffset should be clamped after SetText with shorter content")

	// Pressing NextLink (]) must not panic.
	assert.NotPanics(t, func() {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	}, "SelectFirstVisible should not panic after SetText with shorter content")
}
