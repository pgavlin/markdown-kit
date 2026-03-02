package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewHistoryPicker_BuildsEntries(t *testing.T) {
	stack := []page{
		{name: "Page 1", source: "/page1.md"},
		{name: "Page 2", source: "/page2.md"},
	}

	hp := newHistoryPicker(stack, "Current", "/current.md", 20, 60)

	if len(hp.all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(hp.all))
	}
	// Most recent at top: current page, then stack in reverse order.
	if hp.all[0].name != "Current" || hp.all[0].index != -1 {
		t.Errorf("entry 0: name=%q index=%d", hp.all[0].name, hp.all[0].index)
	}
	if hp.all[1].name != "Page 2" || hp.all[1].index != 1 {
		t.Errorf("entry 1: name=%q index=%d", hp.all[1].name, hp.all[1].index)
	}
	if hp.all[2].name != "Page 1" || hp.all[2].index != 0 {
		t.Errorf("entry 2: name=%q index=%d", hp.all[2].name, hp.all[2].index)
	}
	// Cursor should start on current page (first entry).
	if hp.cursor != 0 {
		t.Errorf("cursor = %d, want 0", hp.cursor)
	}
}

func TestNewHistoryPicker_FallbackName(t *testing.T) {
	stack := []page{
		{name: "", source: "/docs/readme.md"},
	}

	hp := newHistoryPicker(stack, "", "/current.md", 20, 60)

	if hp.all[0].name != "current.md" {
		t.Errorf("expected fallback name 'current.md', got %q", hp.all[0].name)
	}
	if hp.all[1].name != "readme.md" {
		t.Errorf("expected fallback name 'readme.md', got %q", hp.all[1].name)
	}
}

func TestHistoryPicker_CursorNavigation(t *testing.T) {
	stack := []page{
		{name: "Page 1", source: "/page1.md"},
		{name: "Page 2", source: "/page2.md"},
		{name: "Page 3", source: "/page3.md"},
	}

	hp := newHistoryPicker(stack, "Current", "/current.md", 20, 60)
	// Cursor starts at 0 (current page, most recent at top).

	// Move up — should clamp at 0.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if hp.cursor != 0 {
		t.Errorf("after up from 0: cursor = %d, want 0", hp.cursor)
	}

	// Move down.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if hp.cursor != 1 {
		t.Errorf("after down: cursor = %d, want 1", hp.cursor)
	}

	// Move down twice more.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if hp.cursor != 3 {
		t.Errorf("after 3 downs: cursor = %d, want 3", hp.cursor)
	}

	// Move down again — should clamp at 3.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if hp.cursor != 3 {
		t.Errorf("after extra down: cursor = %d, want 3", hp.cursor)
	}

	// Move up.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if hp.cursor != 2 {
		t.Errorf("after up: cursor = %d, want 2", hp.cursor)
	}
}

func TestHistoryPicker_DidSelect(t *testing.T) {
	stack := []page{
		{name: "Page 1", source: "/page1.md"},
		{name: "Page 2", source: "/page2.md"},
	}

	hp := newHistoryPicker(stack, "Current", "/current.md", 20, 60)

	// No selection initially.
	didSelect, _ := hp.DidSelect()
	if didSelect {
		t.Error("expected no selection initially")
	}

	// Move to last entry (oldest stack entry, index 0) and select.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	didSelect, idx := hp.DidSelect()
	if !didSelect {
		t.Error("expected selection after enter")
	}
	if idx != 0 {
		t.Errorf("selected index = %d, want 0", idx)
	}
}

func TestHistoryPicker_DidSelect_CurrentPage(t *testing.T) {
	stack := []page{
		{name: "Page 1", source: "/page1.md"},
	}

	hp := newHistoryPicker(stack, "Current", "/current.md", 20, 60)
	// Cursor is on current page (index -1).
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	didSelect, idx := hp.DidSelect()
	if !didSelect {
		t.Error("expected selection")
	}
	if idx != -1 {
		t.Errorf("selected index = %d, want -1", idx)
	}
}

func TestHistoryPicker_Dismiss(t *testing.T) {
	stack := []page{
		{name: "Page 1", source: "/page1.md"},
	}

	hp := newHistoryPicker(stack, "Current", "/current.md", 20, 60)
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if !hp.dismissed {
		t.Error("expected dismissed=true after esc")
	}
}

func TestHistoryPicker_Filter(t *testing.T) {
	stack := []page{
		{name: "Alpha", source: "/alpha.md"},
		{name: "Beta", source: "/beta.md"},
		{name: "Gamma", source: "/gamma.md"},
	}

	hp := newHistoryPicker(stack, "Delta", "/delta.md", 20, 60)
	hp.input.Focus()

	// Type "al" to filter.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	hp, _ = hp.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})

	if len(hp.filtered) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(hp.filtered))
	}
	if hp.filtered[0].name != "Alpha" {
		t.Errorf("filtered entry name = %q, want 'Alpha'", hp.filtered[0].name)
	}
}

func TestHistoryPicker_FilterBySource(t *testing.T) {
	stack := []page{
		{name: "Page", source: "/docs/unique-file.md"},
		{name: "Other", source: "/other.md"},
	}

	hp := newHistoryPicker(stack, "Current", "/current.md", 20, 60)
	hp.input.Focus()

	// Filter by source substring.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	hp, _ = hp.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	hp, _ = hp.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})

	if len(hp.filtered) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(hp.filtered))
	}
	if hp.filtered[0].name != "Page" {
		t.Errorf("filtered entry name = %q, want 'Page'", hp.filtered[0].name)
	}
}

func TestHistoryPicker_View(t *testing.T) {
	stack := []page{
		{name: "Page 1", source: "/page1.md"},
	}

	hp := newHistoryPicker(stack, "Current", "/current.md", 20, 60)
	view := hp.View()

	if !strings.Contains(view, "Page 1") {
		t.Error("expected view to contain 'Page 1'")
	}
	if !strings.Contains(view, "Current") {
		t.Error("expected view to contain 'Current'")
	}
}

func TestHistoryPicker_EmptyFiltered(t *testing.T) {
	stack := []page{
		{name: "Page 1", source: "/page1.md"},
	}

	hp := newHistoryPicker(stack, "Current", "/current.md", 20, 60)
	hp.input.Focus()

	// Type something that matches nothing.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	hp, _ = hp.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	hp, _ = hp.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})

	view := hp.View()
	if !strings.Contains(view, "No matching entries") {
		t.Error("expected 'No matching entries' in view")
	}

	// Enter on empty should be a no-op.
	hp, _ = hp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	didSelect, _ := hp.DidSelect()
	if didSelect {
		t.Error("expected no selection on enter with empty filtered list")
	}
}
