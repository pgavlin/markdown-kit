package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	mdk "github.com/pgavlin/markdown-kit/view"
	"github.com/skratchdot/open-golang/open"
)

// readerKeyMap combines the view KeyMap with md-specific bindings.
// It implements the help.KeyMap interface for use with bubbles/help.
type readerKeyMap struct {
	mdk.KeyMap // embed the view KeyMap

	OpenBrowser key.Binding
	Help        key.Binding
	Quit        key.Binding
}

func defaultReaderKeyMap() readerKeyMap {
	km := mdk.DefaultKeyMap()
	km.DecreaseWidth.SetEnabled(true)
	km.IncreaseWidth.SetEnabled(true)
	km.FollowLink.SetEnabled(true)
	km.GoBack.SetEnabled(true)
	return readerKeyMap{
		KeyMap: km,
		OpenBrowser: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("ctrl+o", "open in browser"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp returns a short list of key bindings for the compact help view.
func (km readerKeyMap) ShortHelp() []key.Binding {
	return append(km.KeyMap.ShortHelp(), km.Help, km.Quit)
}

// FullHelp returns the full set of key bindings for the expanded help view.
func (km readerKeyMap) FullHelp() [][]key.Binding {
	groups := km.KeyMap.FullHelp()
	groups = append(groups, []key.Binding{km.OpenBrowser, km.Help, km.Quit})
	return groups
}

func openInBrowser(url string) error {
	if url == "" {
		return fmt.Errorf("missing URL")
	}
	return open.Run(url)
}

func sendToClipboard(value string) {
	if !clipboard.Unsupported {
		clipboard.WriteAll(value)
	}
}

type markdownReader struct {
	view mdk.Model

	width, height int

	// Help overlay.
	keys      readerKeyMap
	helpModel help.Model
	showHelp  bool

	// Error dialog state.
	showError bool
	errorText string
}

const defaultContentWidth = 160

func newMarkdownReader(name, source string, theme *chroma.Style) markdownReader {
	keys := defaultReaderKeyMap()

	view := mdk.NewModel(
		mdk.WithTheme(theme),
		mdk.WithGutter(true),
		mdk.WithContentWidth(defaultContentWidth),
	)
	view.SetText(name, source)
	view.KeyMap = keys.KeyMap

	helpModel := help.New()
	helpModel.ShowAll = true

	return markdownReader{
		view:      view,
		keys:      keys,
		helpModel: helpModel,
	}
}

func (r markdownReader) Init() tea.Cmd {
	return nil
}

func (r markdownReader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mdk.OpenLinkMsg:
		if err := openInBrowser(msg.URL); err != nil {
			r.showError = true
			r.errorText = fmt.Sprintf("Error opening URL: %v", err)
		}
		return r, nil

	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		r.view.SetSize(msg.Width, msg.Height)
		r.helpModel.SetWidth(msg.Width)
		return r, nil

	case tea.KeyPressMsg:
		// Handle dialog dismissal first.
		if r.showHelp {
			if msg.String() == "esc" || msg.String() == "?" || msg.String() == "q" {
				r.showHelp = false
				return r, nil
			}
			return r, nil
		}
		if r.showError {
			if msg.String() == "esc" || msg.String() == "enter" || msg.String() == "q" {
				r.showError = false
				return r, nil
			}
			return r, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return r, tea.Quit
		case "ctrl+o":
			link := r.view.FocusedLinkDestination()
			if err := openInBrowser(link); err != nil {
				r.showError = true
				r.errorText = fmt.Sprintf("Error opening URL: %v", err)
			}
			return r, nil
		case "?":
			r.showHelp = true
			return r, nil
		}

		// Pass other keys to the view.
		var cmd tea.Cmd
		r.view, cmd = r.view.Update(msg)
		return r, cmd
	}

	return r, nil
}

func (r markdownReader) View() tea.View {
	if r.width == 0 || r.height == 0 {
		return tea.View{}
	}

	base := r.view.View()

	var result string
	if r.showHelp {
		// Use a wider overlay for the columnar help layout.
		maxW := r.width * 3 / 4
		if maxW < 40 {
			maxW = min(r.width-4, 40)
		}
		maxH := r.height * 3 / 4

		// Render help at the dialog's inner width so columns fit.
		r.helpModel.SetWidth(maxW - 4) // account for border + padding
		content := r.helpModel.View(r.keys)

		// Skip wordWrap — help.Model already formats its own columns.
		result = r.renderOverlay(base, content, maxW, maxH)
	} else if r.showError {
		result = r.overlayDialog(base, "Error", r.errorText)
	} else {
		result = base
	}

	v := tea.NewView(result)
	v.AltScreen = true
	return v
}

// overlayDialog renders a word-wrapped, centered dialog over the base view.
func (r markdownReader) overlayDialog(base, _, content string) string {
	maxW := r.width / 2
	if maxW < 30 {
		maxW = min(r.width-4, 30)
	}
	maxH := r.height / 2

	wrapped := wordWrap(content, maxW-4) // account for border + padding
	return r.renderOverlay(base, wrapped, maxW, maxH)
}

// renderOverlay renders pre-formatted content in a bordered dialog centered over base.
func (r markdownReader) renderOverlay(base, content string, maxW, maxH int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > maxH-2 {
		lines = lines[:maxH-2]
	}
	content = strings.Join(lines, "\n")

	contentWidth := 0
	for _, line := range lines {
		w := ansi.StringWidth(line)
		if w > contentWidth {
			contentWidth = w
		}
	}

	dialogWidth := contentWidth + 4 // border + padding
	if dialogWidth > maxW {
		dialogWidth = maxW
	}

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(dialogWidth)

	dialog := dialogStyle.Render(content)
	return placeOverlay(r.width, r.height, dialog, base)
}

// placeOverlay places a dialog string centered over a background string.
func placeOverlay(width, height int, dialog, background string) string {
	bgLines := strings.Split(background, "\n")

	// Pad background to full height if needed.
	for len(bgLines) < height {
		bgLines = append(bgLines, strings.Repeat(" ", width))
	}

	dialogLines := strings.Split(dialog, "\n")
	dh := len(dialogLines)
	dw := 0
	for _, dl := range dialogLines {
		w := ansi.StringWidth(dl)
		if w > dw {
			dw = w
		}
	}

	// Calculate offsets to center.
	startY := (height - dh) / 2
	startX := (width - dw) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i, dl := range dialogLines {
		y := startY + i
		if y >= len(bgLines) {
			break
		}
		bgLine := bgLines[y]
		dlWidth := ansi.StringWidth(dl)

		// Build: left part of bg + dialog line + right part of bg
		left := ansi.Truncate(bgLine, startX, "")
		leftWidth := ansi.StringWidth(left)

		// Pad left if needed.
		if leftWidth < startX {
			left += strings.Repeat(" ", startX-leftWidth)
		}

		rightStart := startX + dlWidth
		right := ""
		bgWidth := ansi.StringWidth(bgLine)
		if rightStart < bgWidth {
			right = ansi.TruncateLeft(bgLine, rightStart, "")
		}

		bgLines[y] = left + dl + right
	}

	return strings.Join(bgLines, "\n")
}

// wordWrap wraps text to the given width.
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	for _, paragraph := range strings.Split(text, "\n") {
		if result.Len() > 0 {
			result.WriteByte('\n')
		}

		lineWidth := 0
		for _, word := range strings.Fields(paragraph) {
			wordWidth := ansi.StringWidth(word)
			if lineWidth > 0 && lineWidth+1+wordWidth > width {
				result.WriteByte('\n')
				lineWidth = 0
			}
			if lineWidth > 0 {
				result.WriteByte(' ')
				lineWidth++
			}
			result.WriteString(word)
			lineWidth += wordWidth
		}
	}
	return result.String()
}
