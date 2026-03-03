package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/markdown-kit/docsearch"
)

type searchMode int

const (
	searchModeKeyword searchMode = iota
	searchModeSemantic
)

// searchPicker is a modal for searching the document index.
type searchPicker struct {
	input   textinput.Model
	index   *docsearch.Index
	results []docsearch.Result
	mode    searchMode
	cursor  int
	minIdx  int
	maxIdx  int
	height  int
	width   int

	selected  string // selected path ("" = no selection)
	dismissed bool
	searching bool // true while async search is in flight
}

// Style constants for the search picker.
var (
	spCursorStyle   = lipgloss.NewStyle().Foreground(colorAccent)
	spSelectedStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	spNameStyle     = lipgloss.NewStyle()
	spPathStyle     = lipgloss.NewStyle().Foreground(colorMuted)
	spEmptyStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	spModeStyle     = lipgloss.NewStyle().Bold(true).Foreground(colorTabActiveFg).Background(colorTabActiveBg)
	spDateStyle     = lipgloss.NewStyle().Foreground(colorMuted)
)

func newSearchPicker(index *docsearch.Index, height, width int) searchPicker {
	ti := textinput.New()
	ti.Prompt = "  Search: "
	ti.Placeholder = "type to search..."
	innerW := width - 4 // account for border + padding
	ti.SetWidth(innerW - lipgloss.Width(ti.Prompt) - 1)

	listHeight := height - 2 // subtract input line + mode line
	if listHeight < 1 {
		listHeight = 1
	}

	return searchPicker{
		input:  ti,
		index:  index,
		mode:   searchModeKeyword,
		cursor: 0,
		minIdx: 0,
		maxIdx: listHeight - 1,
		height: listHeight,
		width:  width,
	}
}

// searchResultsMsg carries results from an async search.
type searchResultsMsg struct {
	results []docsearch.Result
}

func (sp searchPicker) Update(msg tea.Msg) (searchPicker, tea.Cmd) {
	switch msg := msg.(type) {
	case searchResultsMsg:
		sp.results = msg.results
		sp.searching = false
		sp.cursor = 0
		sp.minIdx = 0
		sp.maxIdx = sp.height - 1
		return sp, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			sp.dismissed = true
			return sp, nil

		case "enter":
			if len(sp.results) == 0 || sp.cursor < 0 {
				return sp, nil
			}
			sp.selected = sp.results[sp.cursor].Path
			return sp, nil

		case "tab":
			if sp.index != nil && sp.index.HasEmbedder() {
				if sp.mode == searchModeKeyword {
					sp.mode = searchModeSemantic
				} else {
					sp.mode = searchModeKeyword
				}
				return sp, sp.doSearch()
			}
			return sp, nil

		case "up", "ctrl+p":
			sp.cursor--
			if sp.cursor < 0 {
				sp.cursor = 0
			}
			if sp.cursor < sp.minIdx {
				sp.minIdx = sp.cursor
				sp.maxIdx = sp.minIdx + sp.height - 1
			}
			return sp, nil

		case "down", "ctrl+n":
			sp.cursor++
			if sp.cursor >= len(sp.results) {
				sp.cursor = len(sp.results) - 1
			}
			if sp.cursor < 0 {
				sp.cursor = 0
			}
			if sp.cursor > sp.maxIdx {
				sp.maxIdx = sp.cursor
				sp.minIdx = sp.maxIdx - sp.height + 1
			}
			return sp, nil

		case "pgup":
			sp.cursor -= sp.height
			if sp.cursor < 0 {
				sp.cursor = 0
			}
			sp.minIdx -= sp.height
			if sp.minIdx < 0 {
				sp.minIdx = 0
			}
			sp.maxIdx = sp.minIdx + sp.height - 1
			return sp, nil

		case "pgdown":
			sp.cursor += sp.height
			if sp.cursor >= len(sp.results) {
				sp.cursor = max(0, len(sp.results)-1)
			}
			sp.maxIdx += sp.height
			if sp.maxIdx >= len(sp.results) {
				sp.maxIdx = max(0, len(sp.results)-1)
			}
			sp.minIdx = sp.maxIdx - sp.height + 1
			if sp.minIdx < 0 {
				sp.minIdx = 0
			}
			return sp, nil

		default:
			prevValue := sp.input.Value()
			var cmd tea.Cmd
			sp.input, cmd = sp.input.Update(msg)
			if sp.input.Value() != prevValue {
				return sp, tea.Batch(cmd, sp.doSearch())
			}
			return sp, cmd
		}
	}

	var cmd tea.Cmd
	sp.input, cmd = sp.input.Update(msg)
	return sp, cmd
}

// doSearch fires an async search command.
func (sp *searchPicker) doSearch() tea.Cmd {
	query := sp.input.Value()
	mode := sp.mode
	index := sp.index
	sp.searching = true

	return func() tea.Msg {
		ctx := context.Background()
		var results []docsearch.Result
		var err error

		switch mode {
		case searchModeKeyword:
			results, err = index.SearchKeyword(ctx, query, 50)
		case searchModeSemantic:
			results, err = index.SearchSemantic(ctx, query, 50)
		}

		if err != nil {
			return searchResultsMsg{results: nil}
		}
		return searchResultsMsg{results: results}
	}
}

func (sp searchPicker) View() string {
	var s strings.Builder

	// Mode indicator.
	modeLabel := "keyword"
	if sp.mode == searchModeSemantic {
		modeLabel = "semantic"
	}
	modeHint := ""
	if sp.index != nil && sp.index.HasEmbedder() {
		modeHint = "  (tab to toggle)"
	}
	s.WriteString(spModeStyle.Render(fmt.Sprintf("  [%s]", modeLabel)))
	s.WriteString(spPathStyle.Render(modeHint))
	s.WriteRune('\n')

	s.WriteString(sp.input.View())
	s.WriteRune('\n')

	if len(sp.results) == 0 {
		if sp.searching {
			s.WriteString(spEmptyStyle.Render("  Searching..."))
		} else if sp.input.Value() != "" {
			s.WriteString(spEmptyStyle.Render("  No results."))
		} else {
			s.WriteString(spEmptyStyle.Render("  Type to search indexed documents."))
		}
		s.WriteRune('\n')
	} else {
		for i, result := range sp.results {
			if i < sp.minIdx || i > sp.maxIdx {
				continue
			}

			title := result.Title
			if title == "" {
				title = "(untitled)"
			}

			// Format last opened date.
			dateStr := formatRelativeTime(result.LastOpened)

			// Truncate path to fit.
			pathMaxW := sp.width - ansi.StringWidth(title) - ansi.StringWidth(dateStr) - 10
			path := result.Path
			if pathMaxW > 0 && ansi.StringWidth(path) > pathMaxW {
				path = "..." + path[len(path)-pathMaxW+3:]
			}

			if i == sp.cursor {
				line := " " + title
				if path != "" {
					line += "  " + path
				}
				line += "  " + dateStr
				s.WriteString(spCursorStyle.Render(">") + spSelectedStyle.Render(line))
			} else {
				s.WriteString(spCursorStyle.Render(" "))
				s.WriteString(" " + spNameStyle.Render(title))
				if path != "" {
					s.WriteString("  " + spPathStyle.Render(path))
				}
				s.WriteString("  " + spDateStyle.Render(dateStr))
			}
			s.WriteRune('\n')
		}
	}

	// Pad remaining height.
	rendered := lipgloss.Height(s.String())
	for i := rendered; i <= sp.height+2; i++ {
		s.WriteRune('\n')
	}

	return s.String()
}

// DidSelect returns whether a document was selected and its path.
func (sp searchPicker) DidSelect() (bool, string) {
	if sp.selected != "" {
		return true, sp.selected
	}
	return false, ""
}

// formatRelativeTime formats a time as a relative string like "2h ago", "3d ago".
func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// findSimilarResultsMsg carries results from an async find-similar command.
type findSimilarResultsMsg struct {
	results []docsearch.Result
}

// findSimilar returns a tea.Cmd that finds documents similar to the given content.
func findSimilar(index *docsearch.Index, content, excludePath string) tea.Cmd {
	return func() tea.Msg {
		results, err := index.FindSimilar(context.Background(), content, excludePath, 20)
		if err != nil {
			return findSimilarResultsMsg{results: nil}
		}
		return findSimilarResultsMsg{results: results}
	}
}

// indexDocumentMsg signals that document indexing completed.
type indexDocumentMsg struct{}

// indexDocumentErrorMsg signals that document indexing failed.
type indexDocumentErrorMsg struct {
	err error
}

// indexDocument returns a tea.Cmd that indexes a document in the background.
func indexDocument(index *docsearch.Index, path, title, markdown string) tea.Cmd {
	return func() tea.Msg {
		if err := index.Add(context.Background(), path, title, markdown); err != nil {
			return indexDocumentErrorMsg{err: err}
		}
		return indexDocumentMsg{}
	}
}

// newSimilarPicker creates a search picker pre-populated with find-similar results.
func newSimilarPicker(index *docsearch.Index, results []docsearch.Result, height, width int) searchPicker {
	sp := newSearchPicker(index, height, width)
	sp.results = results
	sp.input.Placeholder = "similar documents"
	return sp
}
