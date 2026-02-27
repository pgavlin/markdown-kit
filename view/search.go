package view

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

type searchMode int

const (
	searchModeFuzzy searchMode = iota
	searchModeRegex
)

type searchMatch struct {
	lineIndex int // index into m.lines
	startCol  int // visible character start (0-based)
	endCol    int // visible character end (exclusive)
}

type searchState struct {
	active       bool // input prompt is showing
	mode         searchMode
	query        string
	confirmed    bool // enter was pressed
	matches      []searchMatch
	currentMatch int // -1 if none
	regexError   string
	stale        bool // matches need recomputing after re-render
}

// Searching returns true if the search input prompt is active.
func (m *Model) Searching() bool {
	return m.search.active
}

// fuzzyMatch finds a case-insensitive fuzzy match of query in text.
// It returns the column span (startCol, endCol) of the match, where columns
// are measured in display width. ok is false if no match is found.
func fuzzyMatch(text, query string) (startCol, endCol int, ok bool) {
	if query == "" {
		return 0, 0, false
	}

	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)

	firstCol := -1
	lastCol := 0
	lastWidth := 0
	col := 0
	qi := 0 // byte index into lowerQuery

	qr, qSize := utf8.DecodeRuneInString(lowerQuery[qi:])

	for i := 0; i < len(lowerText); {
		tr, tSize := utf8.DecodeRuneInString(lowerText[i:])
		w := runewidth.RuneWidth(tr)

		if tr == qr {
			if firstCol == -1 {
				firstCol = col
			}
			lastCol = col
			lastWidth = w
			qi += qSize
			if qi >= len(lowerQuery) {
				// All query runes matched.
				return firstCol, lastCol + lastWidth, true
			}
			qr, qSize = utf8.DecodeRuneInString(lowerQuery[qi:])
		}

		col += w
		i += tSize
	}

	return 0, 0, false
}

// regexMatchLine finds all regex matches in a stripped line.
func regexMatchLine(stripped string, re *regexp.Regexp) []searchMatch {
	locs := re.FindAllStringIndex(stripped, -1)
	if len(locs) == 0 {
		return nil
	}

	matches := make([]searchMatch, 0, len(locs))
	for _, loc := range locs {
		if loc[0] == loc[1] {
			continue // skip zero-width matches
		}
		startCol := ansi.StringWidth(stripped[:loc[0]])
		endCol := ansi.StringWidth(stripped[:loc[1]])
		matches = append(matches, searchMatch{
			startCol: startCol,
			endCol:   endCol,
		})
	}
	return matches
}

// executeSearch runs the current search query against all lines.
func (m *Model) executeSearch() {
	m.search.matches = nil
	m.search.currentMatch = -1
	m.search.regexError = ""
	m.search.stale = false

	if m.search.query == "" || m.lines == nil {
		return
	}

	var re *regexp.Regexp
	if m.search.mode == searchModeRegex {
		pattern := m.search.query
		// Auto-prepend case-insensitive flag unless user specified flags.
		if !strings.HasPrefix(pattern, "(?") {
			pattern = "(?i)" + pattern
		}
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			m.search.regexError = err.Error()
			return
		}
	}

	for i, ln := range m.lines {
		stripped := ansi.Strip(expandTabs(ln.content, 8))

		if m.search.mode == searchModeFuzzy {
			startCol, endCol, ok := fuzzyMatch(stripped, m.search.query)
			if ok {
				m.search.matches = append(m.search.matches, searchMatch{
					lineIndex: i,
					startCol:  startCol,
					endCol:    endCol,
				})
			}
		} else {
			lineMatches := regexMatchLine(stripped, re)
			for _, match := range lineMatches {
				match.lineIndex = i
				m.search.matches = append(m.search.matches, match)
			}
		}
	}

	if len(m.search.matches) > 0 {
		// Find the first match at or after the current viewport.
		m.search.currentMatch = 0
		for i, match := range m.search.matches {
			if match.lineIndex >= m.lineOffset {
				m.search.currentMatch = i
				break
			}
		}
		m.scrollToMatch(m.search.currentMatch)
	}
}

// scrollToMatch centers the viewport on the given match index.
func (m *Model) scrollToMatch(idx int) {
	if idx < 0 || idx >= len(m.search.matches) {
		return
	}
	match := m.search.matches[idx]
	target := match.lineIndex - m.pageSize/2
	if target < 0 {
		target = 0
	}
	m.lineOffset = target
	m.clampOffsets()
}

// nextMatch advances to the next match with wrapping.
func (m *Model) nextMatch() {
	if len(m.search.matches) == 0 {
		return
	}
	m.search.currentMatch = (m.search.currentMatch + 1) % len(m.search.matches)
	m.scrollToMatch(m.search.currentMatch)
}

// prevMatch goes to the previous match with wrapping.
func (m *Model) prevMatch() {
	if len(m.search.matches) == 0 {
		return
	}
	m.search.currentMatch--
	if m.search.currentMatch < 0 {
		m.search.currentMatch = len(m.search.matches) - 1
	}
	m.scrollToMatch(m.search.currentMatch)
}

// handleSearchKey handles key events during search input mode.
func (m *Model) handleSearchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		// Cancel search, clear all state.
		m.search = searchState{}
		return nil
	case "enter":
		// Confirm search.
		m.search.active = false
		m.search.confirmed = true
		return nil
	case "tab":
		// Toggle mode.
		if m.search.mode == searchModeFuzzy {
			m.search.mode = searchModeRegex
		} else {
			m.search.mode = searchModeFuzzy
		}
		m.executeSearch()
		return nil
	case "backspace":
		if len(m.search.query) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.search.query)
			m.search.query = m.search.query[:len(m.search.query)-size]
			m.executeSearch()
		}
		return nil
	default:
		if msg.Text != "" {
			m.search.query += msg.Text
			m.executeSearch()
		}
		return nil
	}
}

// applySearchHighlights applies search match highlighting to a line's content.
func (m *Model) applySearchHighlights(lineIdx int, content string) string {
	// Collect matches for this line.
	type matchInfo struct {
		startCol int
		endCol   int
		current  bool
	}
	var lineMatches []matchInfo
	for i, match := range m.search.matches {
		if match.lineIndex == lineIdx {
			lineMatches = append(lineMatches, matchInfo{
				startCol: match.startCol,
				endCol:   match.endCol,
				current:  i == m.search.currentMatch,
			})
		}
	}
	if len(lineMatches) == 0 {
		return content
	}

	lineWidth := ansi.StringWidth(content)

	// Build result by processing matches from left to right.
	var result strings.Builder
	pos := 0
	for _, match := range lineMatches {
		sc := match.startCol
		ec := match.endCol
		if ec > lineWidth {
			ec = lineWidth
		}
		if sc >= lineWidth || sc >= ec {
			continue
		}

		// Content before this match.
		if sc > pos {
			result.WriteString(ansiCut(content, pos, sc))
		}

		// The match itself.
		matchContent := ansiCut(content, sc, ec)
		if match.current {
			result.WriteString("\033[7m")
			result.WriteString(matchContent)
			result.WriteString("\033[27m")
		} else {
			result.WriteString("\033[43m")
			result.WriteString(matchContent)
			result.WriteString("\033[49m")
		}

		pos = ec
	}

	// Remaining content after last match.
	if pos < lineWidth {
		result.WriteString(ansiCut(content, pos, lineWidth))
	}

	return result.String()
}

// renderSearchGutter renders the gutter line during search input.
func (m *Model) renderSearchGutter(width int) string {
	var modeStr string
	if m.search.mode == searchModeFuzzy {
		modeStr = "fuzzy"
	} else {
		modeStr = "regex"
	}

	prompt := fmt.Sprintf("/%s: %s", modeStr, m.search.query)

	// Match count.
	var info string
	if m.search.regexError != "" {
		info = " [error]"
	} else if len(m.search.matches) > 0 {
		info = fmt.Sprintf(" [%d/%d]", m.search.currentMatch+1, len(m.search.matches))
	} else if m.search.query != "" {
		info = " [0]"
	}

	// Cursor indicator.
	cursor := "_"

	full := prompt + cursor + info
	fullWidth := ansi.StringWidth(full)

	if fullWidth > width {
		// Truncate the prompt portion.
		avail := width - ansi.StringWidth(cursor+info)
		if avail > 3 {
			prompt = ansiTruncate(prompt, avail-3) + "..."
		}
		full = prompt + cursor + info
		fullWidth = ansi.StringWidth(full)
	}

	pad := width - fullWidth
	if pad < 0 {
		pad = 0
	}

	return full + strings.Repeat(" ", pad)
}

// searchGutterInfo returns the search match info string for the normal gutter.
func (m *Model) searchGutterInfo() string {
	if !m.search.confirmed || len(m.search.matches) == 0 {
		return ""
	}
	return fmt.Sprintf("[%d/%d]", m.search.currentMatch+1, len(m.search.matches))
}
