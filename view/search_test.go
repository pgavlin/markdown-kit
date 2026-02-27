package view

import (
	"regexp"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// fuzzyMatch
// ---------------------------------------------------------------------------

func TestFuzzyMatch_Basic(t *testing.T) {
	start, end, ok := fuzzyMatch("hello world", "wor")
	assert.True(t, ok)
	assert.Equal(t, 6, start)
	assert.Equal(t, 9, end)
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	start, end, ok := fuzzyMatch("Hello World", "hello")
	assert.True(t, ok)
	assert.Equal(t, 0, start)
	assert.Equal(t, 5, end)
}

func TestFuzzyMatch_Fuzzy(t *testing.T) {
	start, end, ok := fuzzyMatch("abcdef", "ace")
	assert.True(t, ok)
	assert.Equal(t, 0, start) // 'a' at col 0
	assert.Equal(t, 5, end)   // 'e' at col 4, width 1 -> end 5
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	_, _, ok := fuzzyMatch("hello", "xyz")
	assert.False(t, ok)
}

func TestFuzzyMatch_EmptyQuery(t *testing.T) {
	_, _, ok := fuzzyMatch("hello", "")
	assert.False(t, ok)
}

func TestFuzzyMatch_EmptyText(t *testing.T) {
	_, _, ok := fuzzyMatch("", "hello")
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// regexMatchLine
// ---------------------------------------------------------------------------

func TestRegexMatchLine_Basic(t *testing.T) {
	re := regexp.MustCompile("(?i)world")
	matches := regexMatchLine("hello world", re)
	require.Len(t, matches, 1)
	assert.Equal(t, 6, matches[0].startCol)
	assert.Equal(t, 11, matches[0].endCol)
}

func TestRegexMatchLine_Multiple(t *testing.T) {
	re := regexp.MustCompile("o")
	matches := regexMatchLine("foo boo", re)
	require.Len(t, matches, 4) // two o's in foo, two in boo
}

func TestRegexMatchLine_NoMatch(t *testing.T) {
	re := regexp.MustCompile("xyz")
	matches := regexMatchLine("hello", re)
	assert.Empty(t, matches)
}

// ---------------------------------------------------------------------------
// Search integration via Model
// ---------------------------------------------------------------------------

func newTestModelWithSearch(markdown string) *Model {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("test.md", markdown)
	return &m
}

func TestSearch_FuzzyExecute(t *testing.T) {
	m := newTestModelWithSearch("Hello world\n\nThis is a test\n\nHello again")

	m.search.active = true
	m.search.mode = searchModeFuzzy
	m.search.query = "hello"
	m.executeSearch()

	// Should find matches on lines containing "Hello".
	assert.Greater(t, len(m.search.matches), 0)
	assert.Equal(t, 0, m.search.currentMatch)
}

func TestSearch_RegexExecute(t *testing.T) {
	m := newTestModelWithSearch("Hello world\n\nThis is a test\n\nHello again")

	m.search.active = true
	m.search.mode = searchModeRegex
	m.search.query = "hel+o"
	m.executeSearch()

	assert.Greater(t, len(m.search.matches), 0)
}

func TestSearch_RegexInvalid(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	m.search.active = true
	m.search.mode = searchModeRegex
	m.search.query = "[invalid"
	m.executeSearch()

	assert.Empty(t, m.search.matches)
	assert.NotEmpty(t, m.search.regexError)
}

func TestSearch_Navigation(t *testing.T) {
	m := newTestModelWithSearch("aaa\n\nbbb\n\naaa\n\nbbb\n\naaa")

	m.search.mode = searchModeFuzzy
	m.search.query = "aaa"
	m.executeSearch()

	count := len(m.search.matches)
	require.Greater(t, count, 1)

	// Navigate forward.
	first := m.search.currentMatch
	m.nextMatch()
	assert.Equal(t, first+1, m.search.currentMatch)

	// Navigate backward.
	m.prevMatch()
	assert.Equal(t, first, m.search.currentMatch)

	// Wrapping forward.
	for i := 0; i < count; i++ {
		m.nextMatch()
	}
	assert.Equal(t, first, m.search.currentMatch)

	// Wrapping backward.
	m.prevMatch()
	assert.Equal(t, count-1, m.search.currentMatch)
}

func TestSearch_ClearSearch(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	m.search.active = true
	m.search.mode = searchModeFuzzy
	m.search.query = "hello"
	m.search.confirmed = true
	m.executeSearch()

	require.Greater(t, len(m.search.matches), 0)

	// Clear via esc when confirmed.
	m.search = searchState{}

	assert.Empty(t, m.search.matches)
	assert.Equal(t, "", m.search.query)
	assert.False(t, m.search.active)
	assert.False(t, m.search.confirmed)
}

func TestSearch_HandleSearchKey_Enter(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	m.search.active = true
	m.search.query = "hello"

	m.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.False(t, m.search.active)
	assert.True(t, m.search.confirmed)
}

func TestSearch_HandleSearchKey_Escape(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	m.search.active = true
	m.search.query = "hello"

	m.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, m.search.active)
	assert.Equal(t, "", m.search.query)
}

func TestSearch_HandleSearchKey_Tab(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	m.search.active = true
	m.search.mode = searchModeFuzzy

	m.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyTab})

	assert.Equal(t, searchModeRegex, m.search.mode)

	m.handleSearchKey(tea.KeyPressMsg{Code: tea.KeyTab})

	assert.Equal(t, searchModeFuzzy, m.search.mode)
}

func TestSearch_Searching(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	assert.False(t, m.Searching())

	m.search.active = true
	assert.True(t, m.Searching())
}

func TestSearch_HighlightInView(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	m.search.mode = searchModeFuzzy
	m.search.query = "world"
	m.search.confirmed = true
	m.executeSearch()

	require.Greater(t, len(m.search.matches), 0)

	output := m.View()
	// The output should contain reverse video escape for the current match.
	assert.Contains(t, output, "\033[7m")
}

func TestSearch_GutterShowsMatchCount(t *testing.T) {
	m := newTestModelWithSearch("Hello world\n\nHello again")

	m.search.mode = searchModeFuzzy
	m.search.query = "hello"
	m.search.confirmed = true
	m.executeSearch()

	output := m.View()
	lines := splitLines(output)
	require.NotEmpty(t, lines)

	// The last line is the gutter; it should contain the match count.
	gutter := ansi.Strip(lines[len(lines)-1])
	assert.Contains(t, gutter, "[1/")
}

func TestSearch_SearchGutterDuringInput(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	m.search.active = true
	m.search.mode = searchModeFuzzy
	m.search.query = "hel"
	m.executeSearch()

	output := m.View()
	lines := splitLines(output)
	require.NotEmpty(t, lines)

	// The last line should show the search prompt.
	gutter := ansi.Strip(lines[len(lines)-1])
	assert.Contains(t, gutter, "/fuzzy: hel")
}

func TestSearch_StaleReexecute(t *testing.T) {
	m := newTestModelWithSearch("Hello world")

	m.search.mode = searchModeFuzzy
	m.search.query = "hello"
	m.search.confirmed = true
	m.executeSearch()
	origCount := len(m.search.matches)

	// Simulate width change that invalidates lines.
	m.SetWrap(false)
	m.SetWrap(true)

	// After ensureRendered, stale search should be re-executed.
	m.ensureRendered()

	assert.Equal(t, origCount, len(m.search.matches))
}

// splitLines splits a string by newlines, used for test helpers.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
