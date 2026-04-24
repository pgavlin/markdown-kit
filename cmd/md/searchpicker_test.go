package main

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/markdown-kit/docsearch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A nil *docsearch.Index satisfies the field type; the Update paths we
// test here (navigation, esc, enter on result, message handling, View)
// never dereference the index, so this keeps the tests free of the
// sqlite_fts5 build tag. Tab-toggle is tested separately and explicitly
// verified as a no-op in this shape.

func pickerWithResults(t *testing.T, n int) searchPicker {
	t.Helper()
	sp := newSearchPicker(nil, 20, 80)
	results := make([]docsearch.Result, n)
	now := time.Now()
	for i := range results {
		results[i] = docsearch.Result{
			Path:       "/docs/page-" + itoa(i) + ".md",
			Title:      "Page " + itoa(i),
			LastOpened: now.Add(-time.Duration(i) * time.Hour),
		}
	}
	// Seed the picker as if a doSearch had returned these results.
	sp, _ = sp.Update(searchResultsMsg{results: results})
	return sp
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestSearchPicker_DownArrowAdvancesCursor(t *testing.T) {
	sp := pickerWithResults(t, 10)
	require.Equal(t, 0, sp.cursor)

	sp, _ = sp.Update(tea.KeyPressMsg{Text: "down"})
	assert.Equal(t, 1, sp.cursor)

	sp, _ = sp.Update(tea.KeyPressMsg{Text: "down"})
	assert.Equal(t, 2, sp.cursor)
}

func TestSearchPicker_UpArrowRetreats(t *testing.T) {
	sp := pickerWithResults(t, 10)
	sp.cursor = 5
	sp.minIdx = 0
	sp.maxIdx = sp.height - 1

	sp, _ = sp.Update(tea.KeyPressMsg{Text: "up"})
	assert.Equal(t, 4, sp.cursor)
}

func TestSearchPicker_UpArrowClampsAtZero(t *testing.T) {
	sp := pickerWithResults(t, 10)
	require.Equal(t, 0, sp.cursor)
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "up"})
	assert.Equal(t, 0, sp.cursor, "up on the first row should clamp at 0")
}

func TestSearchPicker_DownArrowClampsAtEnd(t *testing.T) {
	sp := pickerWithResults(t, 3)
	sp.cursor = 2 // last index
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "down"})
	assert.Equal(t, 2, sp.cursor, "down on the last row should clamp")
}

func TestSearchPicker_PgDownAdvancesByHeight(t *testing.T) {
	sp := pickerWithResults(t, 100)
	h := sp.height
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "pgdown"})
	assert.Equal(t, h, sp.cursor, "pgdown should advance cursor by one page")
}

func TestSearchPicker_PgDownClampsWhenPastEnd(t *testing.T) {
	sp := pickerWithResults(t, 5)
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "pgdown"})
	assert.Equal(t, 4, sp.cursor, "pgdown beyond the last result should clamp")
}

func TestSearchPicker_PgUpRetreats(t *testing.T) {
	sp := pickerWithResults(t, 50)
	sp.cursor = 30
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "pgup"})
	assert.Equal(t, 30-sp.height, sp.cursor)
}

func TestSearchPicker_PgUpClampsAtZero(t *testing.T) {
	sp := pickerWithResults(t, 10)
	sp.cursor = 2
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "pgup"})
	assert.Equal(t, 0, sp.cursor)
	assert.Equal(t, 0, sp.minIdx)
}

func TestSearchPicker_EscDismisses(t *testing.T) {
	sp := pickerWithResults(t, 3)
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "esc"})
	assert.True(t, sp.dismissed)
}

func TestSearchPicker_EnterSelectsResult(t *testing.T) {
	sp := pickerWithResults(t, 5)
	sp.cursor = 2
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "enter"})
	ok, path := sp.DidSelect()
	assert.True(t, ok)
	assert.Equal(t, "/docs/page-2.md", path)
}

func TestSearchPicker_EnterOnEmptyResultsIsNoOp(t *testing.T) {
	sp := newSearchPicker(nil, 20, 80)
	sp, _ = sp.Update(tea.KeyPressMsg{Text: "enter"})
	ok, _ := sp.DidSelect()
	assert.False(t, ok, "enter with no results should not select anything")
	assert.False(t, sp.dismissed)
}

func TestSearchPicker_TabNoopWithoutEmbedder(t *testing.T) {
	// With nil index (and thus no embedder), tab must be a no-op —
	// semantic search is unavailable.
	sp := pickerWithResults(t, 3)
	priorMode := sp.mode
	sp, cmd := sp.Update(tea.KeyPressMsg{Text: "tab"})
	assert.Equal(t, priorMode, sp.mode, "mode should not change without embedder")
	assert.Nil(t, cmd, "tab should issue no command when toggling is disabled")
}

func TestSearchPicker_TypingUpdatesQuery(t *testing.T) {
	// Typing into the textinput component should update the query and
	// return a doSearch command. We can't execute it (no index), but we
	// can verify the input state changed. The input must be focused first
	// (newSearchPicker doesn't focus; reader.go calls Focus() at dispatch
	// time).
	sp := newSearchPicker(nil, 20, 80)
	sp.input.Focus()

	sp, _ = sp.Update(tea.KeyPressMsg{Text: "h"})
	assert.Equal(t, "h", sp.input.Value())
}

func TestSearchPicker_DidSelectFalseByDefault(t *testing.T) {
	sp := newSearchPicker(nil, 20, 80)
	ok, path := sp.DidSelect()
	assert.False(t, ok)
	assert.Equal(t, "", path)
}

func TestSearchPicker_ViewRendersEmptyState(t *testing.T) {
	sp := newSearchPicker(nil, 20, 80)
	view := ansi.Strip(sp.View())
	assert.Contains(t, view, "[keyword]",
		"empty picker should show the mode indicator")
	assert.Contains(t, view, "Type to search indexed documents.",
		"empty state should prompt user to type")
}

func TestSearchPicker_ViewRendersResults(t *testing.T) {
	sp := pickerWithResults(t, 3)
	sp.input.SetValue("foo")
	view := ansi.Strip(sp.View())
	assert.Contains(t, view, "Page 0")
	assert.Contains(t, view, "Page 1")
	assert.Contains(t, view, "Page 2")
}

func TestSearchPicker_ViewRendersNoResultsWithQuery(t *testing.T) {
	sp := newSearchPicker(nil, 20, 80)
	sp.input.SetValue("xyz")
	view := ansi.Strip(sp.View())
	assert.Contains(t, view, "No results.")
}

func TestSearchPicker_ViewRendersSearchingState(t *testing.T) {
	sp := newSearchPicker(nil, 20, 80)
	sp.input.SetValue("foo")
	sp.searching = true
	view := ansi.Strip(sp.View())
	assert.Contains(t, view, "Searching...")
}

func TestSearchPicker_SearchResultsMsgResetsCursor(t *testing.T) {
	sp := pickerWithResults(t, 10)
	sp.cursor = 7
	sp.maxIdx = 15
	sp.minIdx = 3

	// A fresh results message must reset cursor/window regardless of
	// previous state (the user typed a new query).
	sp, _ = sp.Update(searchResultsMsg{results: []docsearch.Result{
		{Path: "/new.md", Title: "New"},
	}})
	assert.Equal(t, 0, sp.cursor, "new results should reset cursor")
	assert.Equal(t, 0, sp.minIdx)
	assert.False(t, sp.searching, "searching flag must clear on results")
}

func TestFormatRelativeTime_Ranges(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"now", now.Add(-30 * time.Second), "now"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-2 * 24 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatRelativeTime(c.t)
			assert.Equal(t, c.want, got, "case %q", c.name)
		})
	}
}

func TestFormatRelativeTime_OlderThanMonthUsesDate(t *testing.T) {
	// More than 30 days ago → shows month-day format.
	long := time.Now().Add(-60 * 24 * time.Hour)
	got := formatRelativeTime(long)
	// Must not contain "ago"; must look like "Jan 2" (month abbreviated).
	assert.NotContains(t, got, "ago",
		"older-than-a-month should switch to absolute date, got %q", got)
	assert.Regexp(t, `^[A-Z][a-z]{2} \d+$`, got,
		"expected 'Mon D' format, got %q", got)
}
