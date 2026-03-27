package view

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	xast "github.com/pgavlin/goldmark/extension/ast"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/pgavlin/tea-grid/grid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGridFocusedModel creates a Model with a focused grid on a simple table.
// The table has two data rows: Alice/30 and Bob/25.
func setupGridFocusedModel(t *testing.T) Model {
	t.Helper()

	md := "# Title\n\n| Name | Age |\n| ---- | --- |\n| Alice | 30 |\n| Bob | 25 |\n"
	gtr := NewGridTableRenderer(styles.Pulumi)
	m := NewModel(WithTheme(styles.Pulumi))
	m.KeyMap.FollowLink.SetEnabled(true)
	m.SetGridTableRenderer(gtr)
	m.SetText("test.md", md)
	m.SetSize(80, 24)
	_ = m.View()

	// Navigate to the table.
	found := false
	for i := 0; i < 20; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		if m.selection != nil && m.selection.Node.Kind() == xast.KindTable {
			found = true
			break
		}
	}
	require.True(t, found, "should navigate to the table")

	// Enter grid focus.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.True(t, m.gridFocused, "grid should be focused")

	return m
}

func TestGridEventMsg_FocusChanged(t *testing.T) {
	m := setupGridFocusedModel(t)

	// Press Down — should move focus and emit a GridEventMsg wrapping FocusChangedMsg.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.True(t, m.gridFocused, "grid should remain focused after Down")

	require.NotNil(t, cmd, "Down in grid should produce a command")
	msg := cmd()
	ge, ok := msg.(GridEventMsg)
	require.True(t, ok, "command should produce a GridEventMsg, got %T", msg)

	_, isFocusChanged := ge.Event.(grid.FocusChangedMsg)
	assert.True(t, isFocusChanged, "GridEventMsg.Event should be FocusChangedMsg, got %T", ge.Event)
}

func TestGridEventMsg_NilForNoOp(t *testing.T) {
	m := setupGridFocusedModel(t)

	// Press a key that does not produce a grid command (e.g. an unbound key).
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	assert.True(t, m.gridFocused, "grid should remain focused")
	assert.Nil(t, cmd, "unhandled key should not produce a command")
}

func TestFocusedGridCell(t *testing.T) {
	m := setupGridFocusedModel(t)

	row, col, ok := m.FocusedGridCell()
	assert.True(t, ok, "should report focused grid cell when grid is focused")
	assert.Equal(t, 0, row, "initial focused row should be 0")
	assert.Equal(t, 0, col, "initial focused col should be 0")

	// Move focus down.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	row, col, ok = m.FocusedGridCell()
	assert.True(t, ok)
	assert.Equal(t, 1, row, "after Down, focused row should be 1")
}

func TestFocusedGridCell_NotFocused(t *testing.T) {
	md := "# Title\n\nSome text.\n"
	m := NewModel(WithTheme(styles.Pulumi))
	m.SetText("test.md", md)
	m.SetSize(80, 24)

	row, col, ok := m.FocusedGridCell()
	assert.False(t, ok, "should return false when no grid is focused")
	assert.Equal(t, -1, row)
	assert.Equal(t, -1, col)
}

func TestGridFocusedLinkDestination_NoLinks(t *testing.T) {
	m := setupGridFocusedModel(t)

	// The simple table has no links, so destination should be empty.
	url := m.FocusedGridLinkDestination()
	assert.Empty(t, url, "should be empty when cell has no links")
}

func TestGridFocusedLinkDestination_WithLink(t *testing.T) {
	md := "# Title\n\n| Name | URL |\n| ---- | --- |\n| Go | [golang.org](https://golang.org) |\n| Rust | [rust-lang.org](https://rust-lang.org) |\n"

	gtr := NewGridTableRenderer(styles.Pulumi)
	m := NewModel(WithTheme(styles.Pulumi))
	m.KeyMap.FollowLink.SetEnabled(true)
	m.SetGridTableRenderer(gtr)
	m.SetText("test.md", md)
	m.SetSize(80, 24)
	_ = m.View()

	// Navigate to the table.
	found := false
	for i := 0; i < 20; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		if m.selection != nil && m.selection.Node.Kind() == xast.KindTable {
			found = true
			break
		}
	}
	require.True(t, found, "should navigate to the table")

	// Enter grid focus.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.True(t, m.gridFocused)

	// Move to the URL column (col 1).
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	url := m.FocusedGridLinkDestination()
	assert.Equal(t, "https://golang.org", url, "should return the link destination from the focused cell")
}

func TestGridLinkNavigation_ExternalLink(t *testing.T) {
	md := "# Title\n\n| Name | URL |\n| ---- | --- |\n| Go | [golang.org](https://golang.org) |\n"

	gtr := NewGridTableRenderer(styles.Pulumi)
	m := NewModel(WithTheme(styles.Pulumi))
	m.KeyMap.FollowLink.SetEnabled(true)
	m.SetGridTableRenderer(gtr)
	m.SetText("test.md", md)
	m.SetSize(80, 24)
	_ = m.View()

	// Navigate to the table and enter grid focus.
	for i := 0; i < 20; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		if m.selection != nil && m.selection.Node.Kind() == xast.KindTable {
			break
		}
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.True(t, m.gridFocused)

	// Move to URL column.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	// Press Enter to activate the link.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.NotNil(t, cmd, "Enter on link cell should produce a command")
	msg := cmd()
	openLink, ok := msg.(OpenLinkMsg)
	require.True(t, ok, "should produce OpenLinkMsg, got %T", msg)
	assert.Equal(t, "https://golang.org", openLink.URL)
}

func TestGridLinkNavigation_InternalAnchor(t *testing.T) {
	md := "# Title\n\n## Target\n\nSome content.\n\n| Name | Link |\n| ---- | ---- |\n| Jump | [go to target](#target) |\n"

	gtr := NewGridTableRenderer(styles.Pulumi)
	m := NewModel(WithTheme(styles.Pulumi))
	m.KeyMap.FollowLink.SetEnabled(true)
	m.SetGridTableRenderer(gtr)
	m.SetText("test.md", md)
	m.SetSize(80, 24)
	_ = m.View()

	// Navigate to the table and enter grid focus.
	for i := 0; i < 20; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		if m.selection != nil && m.selection.Node.Kind() == xast.KindTable {
			break
		}
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.True(t, m.gridFocused)

	// Move to Link column.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	// Press Enter to follow the internal anchor.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Should have exited grid focus and navigated internally.
	assert.False(t, m.gridFocused, "grid focus should exit after following internal anchor")
	assert.Nil(t, cmd, "internal anchor navigation should not produce a command")
}

func TestGridFocusEscExits(t *testing.T) {
	m := setupGridFocusedModel(t)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, m.gridFocused, "Esc should exit grid focus")

	row, col, ok := m.FocusedGridCell()
	assert.False(t, ok)
	assert.Equal(t, -1, row)
	assert.Equal(t, -1, col)
}

func TestGridEventMsg_ArrowKeysProduceEvents(t *testing.T) {
	m := setupGridFocusedModel(t)

	// Right arrow should produce a FocusChangedMsg.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	assert.True(t, m.gridFocused)
	require.NotNil(t, cmd)

	msg := cmd()
	ge, ok := msg.(GridEventMsg)
	require.True(t, ok, "should produce GridEventMsg, got %T", msg)
	_, isFocusChanged := ge.Event.(grid.FocusChangedMsg)
	assert.True(t, isFocusChanged, "right arrow should produce FocusChangedMsg")
}

func TestGridLinkNavigation_EnterOnNonLinkCell(t *testing.T) {
	// Table with links, but Enter on a non-link cell should NOT produce OpenLinkMsg.
	md := "# Title\n\n| Name | URL |\n| ---- | --- |\n| Go | [golang.org](https://golang.org) |\n"

	gtr := NewGridTableRenderer(styles.Pulumi)
	m := NewModel(WithTheme(styles.Pulumi))
	m.KeyMap.FollowLink.SetEnabled(true)
	m.SetGridTableRenderer(gtr)
	m.SetText("test.md", md)
	m.SetSize(80, 24)
	_ = m.View()

	// Navigate to the table and enter grid focus.
	for i := 0; i < 20; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		if m.selection != nil && m.selection.Node.Kind() == xast.KindTable {
			break
		}
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.True(t, m.gridFocused)

	// Focus is on col 0 (Name column, no links). Press Enter.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Should remain in grid focus — Enter forwarded to grid (sort on header, or no-op on data).
	assert.True(t, m.gridFocused, "should remain in grid focus when no link in cell")

	// Should NOT produce OpenLinkMsg.
	if cmd != nil {
		msg := cmd()
		_, isOpenLink := msg.(OpenLinkMsg)
		assert.False(t, isOpenLink, "should not produce OpenLinkMsg for non-link cell")
	}
}
