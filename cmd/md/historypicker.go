package main

import (
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// historyEntry represents a single entry in the history picker list.
type historyEntry struct {
	name   string // page name or source basename
	source string // full source path/URL
	index  int    // index into pageStack (-1 for current page)
}

// historyPicker is a list selector with text filtering for navigating the page history.
type historyPicker struct {
	input     textinput.Model
	all       []historyEntry // full list
	filtered  []historyEntry // after text filter
	cursor    int
	minIdx    int
	maxIdx    int
	height    int
	width     int
	selected  int // selected entry index value (-2 = no selection)
	dismissed bool
}

// Style constants for the history picker.
var (
	hpCursorStyle   = lipgloss.NewStyle().Foreground(colorAccent)
	hpSelectedStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	hpNameStyle     = lipgloss.NewStyle()
	hpSourceStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	hpEmptyStyle    = lipgloss.NewStyle().Foreground(colorMuted)
)

// newHistoryPicker creates a history picker from the tab's page stack and current page.
// The list shows stack entries (oldest first) plus the current page at the bottom.
// The cursor starts on the current page.
func newHistoryPicker(stack []page, currentName, currentSource string, height, width int) historyPicker {
	ti := textinput.New()
	ti.Prompt = "  Filter: "
	ti.Placeholder = "type to filter..."
	innerW := width - 4 // account for border + padding
	ti.SetWidth(innerW - lipgloss.Width(ti.Prompt) - 1)

	// Build list with current page first (most recent at top).
	name := currentName
	if name == "" && currentSource != "" {
		name = filepath.Base(currentSource)
	}
	all := []historyEntry{{
		name:   name,
		source: currentSource,
		index:  -1,
	}}
	for i := len(stack) - 1; i >= 0; i-- {
		p := stack[i]
		pname := p.name
		if pname == "" && p.source != "" {
			pname = filepath.Base(p.source)
		}
		all = append(all, historyEntry{
			name:   pname,
			source: p.source,
			index:  i,
		})
	}

	listHeight := height - 1 // subtract input line
	if listHeight < 1 {
		listHeight = 1
	}

	cursor := 0
	minIdx := 0
	maxIdx := listHeight - 1

	return historyPicker{
		input:    ti,
		all:      all,
		filtered: append([]historyEntry(nil), all...),
		cursor:   cursor,
		minIdx:   minIdx,
		maxIdx:   maxIdx,
		height:   listHeight,
		width:    width,
		selected: -2,
	}
}

func (hp historyPicker) Update(msg tea.Msg) (historyPicker, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			hp.dismissed = true
			return hp, nil

		case "enter":
			if len(hp.filtered) == 0 || hp.cursor < 0 {
				return hp, nil
			}
			hp.selected = hp.filtered[hp.cursor].index
			return hp, nil

		case "up", "ctrl+p":
			hp.cursor--
			if hp.cursor < 0 {
				hp.cursor = 0
			}
			if hp.cursor < hp.minIdx {
				hp.minIdx = hp.cursor
				hp.maxIdx = hp.minIdx + hp.height - 1
			}
			return hp, nil

		case "down", "ctrl+n":
			hp.cursor++
			if hp.cursor >= len(hp.filtered) {
				hp.cursor = len(hp.filtered) - 1
			}
			if hp.cursor < 0 {
				hp.cursor = 0
			}
			if hp.cursor > hp.maxIdx {
				hp.maxIdx = hp.cursor
				hp.minIdx = hp.maxIdx - hp.height + 1
			}
			return hp, nil

		case "pgup":
			hp.cursor -= hp.height
			if hp.cursor < 0 {
				hp.cursor = 0
			}
			hp.minIdx -= hp.height
			if hp.minIdx < 0 {
				hp.minIdx = 0
			}
			hp.maxIdx = hp.minIdx + hp.height - 1
			return hp, nil

		case "pgdown":
			hp.cursor += hp.height
			if hp.cursor >= len(hp.filtered) {
				hp.cursor = max(0, len(hp.filtered)-1)
			}
			hp.maxIdx += hp.height
			if hp.maxIdx >= len(hp.filtered) {
				hp.maxIdx = max(0, len(hp.filtered)-1)
			}
			hp.minIdx = hp.maxIdx - hp.height + 1
			if hp.minIdx < 0 {
				hp.minIdx = 0
			}
			return hp, nil

		default:
			prevValue := hp.input.Value()
			var cmd tea.Cmd
			hp.input, cmd = hp.input.Update(msg)
			if hp.input.Value() != prevValue {
				hp.filter()
			}
			return hp, cmd
		}
	}

	var cmd tea.Cmd
	hp.input, cmd = hp.input.Update(msg)
	return hp, cmd
}

// filter applies case-insensitive subsequence matching on entry name and source.
func (hp *historyPicker) filter() {
	query := strings.ToLower(hp.input.Value())
	hp.filtered = nil
	for _, e := range hp.all {
		if query == "" || subsequenceMatch(strings.ToLower(e.name), query) || subsequenceMatch(strings.ToLower(e.source), query) {
			hp.filtered = append(hp.filtered, e)
		}
	}

	if hp.cursor < 0 {
		hp.cursor = 0
	}
	if hp.cursor >= len(hp.filtered) {
		hp.cursor = max(0, len(hp.filtered)-1)
	}
	hp.minIdx = 0
	hp.maxIdx = hp.height - 1
	if hp.cursor > hp.maxIdx {
		hp.minIdx = hp.cursor - hp.height + 1
		hp.maxIdx = hp.cursor
	}
}

func (hp historyPicker) View() string {
	var s strings.Builder

	s.WriteString(hp.input.View())
	s.WriteRune('\n')

	if len(hp.filtered) == 0 {
		s.WriteString(hpEmptyStyle.Render("  No matching entries."))
		s.WriteRune('\n')
	} else {
		for i, entry := range hp.filtered {
			if i < hp.minIdx || i > hp.maxIdx {
				continue
			}

			name := entry.name
			if name == "" {
				name = "(untitled)"
			}

			// Truncate source to fit.
			sourceMaxW := hp.width - ansi.StringWidth(name) - 6 // cursor + spaces + padding
			source := entry.source
			if sourceMaxW > 0 && ansi.StringWidth(source) > sourceMaxW {
				source = "..." + source[len(source)-sourceMaxW+3:]
			}

			if i == hp.cursor {
				line := " " + name
				if source != "" {
					line += "  " + source
				}
				s.WriteString(hpCursorStyle.Render(">") + hpSelectedStyle.Render(line))
			} else {
				s.WriteString(hpCursorStyle.Render(" "))
				s.WriteString(" " + hpNameStyle.Render(name))
				if source != "" {
					s.WriteString("  " + hpSourceStyle.Render(source))
				}
			}
			s.WriteRune('\n')
		}
	}

	// Pad remaining height.
	rendered := lipgloss.Height(s.String())
	for i := rendered; i <= hp.height+1; i++ {
		s.WriteRune('\n')
	}

	return s.String()
}

// DidSelect returns whether an entry was selected and its pageStack index.
// The index is -1 for the current page, or 0..N-1 for a stack entry.
func (hp historyPicker) DidSelect() (bool, int) {
	if hp.selected != -2 {
		return true, hp.selected
	}
	return false, 0
}
