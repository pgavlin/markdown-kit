package main

import (
	"fmt"
	"log/slog"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
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

	ToggleRaw            key.Binding
	ToggleOriginalHTML   key.Binding
	ToggleReadabilityHTML key.Binding
	OpenBrowser          key.Binding
	Help                 key.Binding
	Quit                 key.Binding
}

func defaultReaderKeyMap() readerKeyMap {
	km := mdk.DefaultKeyMap()
	km.DecreaseWidth.SetEnabled(true)
	km.IncreaseWidth.SetEnabled(true)
	km.FollowLink.SetEnabled(true)
	km.GoBack.SetEnabled(true)
	return readerKeyMap{
		KeyMap: km,
		ToggleRaw: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "toggle raw"),
		),
		ToggleOriginalHTML: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("ctrl+e", "view original HTML"),
			key.WithDisabled(),
		),
		ToggleReadabilityHTML: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "view readability HTML"),
			key.WithDisabled(),
		),
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
	return append(km.KeyMap.ShortHelp(), km.ToggleRaw, km.Help, km.Quit)
}

// FullHelp returns the full set of key bindings for the expanded help view.
func (km readerKeyMap) FullHelp() [][]key.Binding {
	groups := km.KeyMap.FullHelp()
	groups = append(groups, []key.Binding{
		km.ToggleRaw, km.ToggleOriginalHTML, km.ToggleReadabilityHTML,
		km.OpenBrowser, km.Help, km.Quit,
	})
	return groups
}

func openInBrowser(url string, logger *slog.Logger) error {
	if url == "" {
		return fmt.Errorf("missing URL")
	}
	logger.Info("open_browser", "url", url)
	return open.Run(url)
}

func sendToClipboard(value string, logger *slog.Logger) {
	if !clipboard.Unsupported {
		logger.Info("clipboard_write", "length", len(value))
		clipboard.WriteAll(value)
	}
}

// page stores the state of a viewed page for the back stack.
type page struct {
	name            string
	markdown        string
	source          string
	originalHTML    string
	readabilityHTML string
	lineOffset      int
	columnOffset    int
}

type markdownReader struct {
	view mdk.Model

	width, height int

	// Structured logger for I/O operations.
	logger *slog.Logger

	// Content converter for non-markdown URLs.
	converter converter

	// Disk cache for conversion results.
	cache *conversionCache

	// HTTP client for fetching URLs.
	client httpClient

	// Filesystem abstraction.
	fsys fileSystem

	// The source path or URL of the current document.
	currentSource string

	// HTML content for pages that originated from HTML.
	currentOriginalHTML    string
	currentReadabilityHTML string

	// Page navigation back stack.
	pageStack []page

	// Loading state for async page fetches.
	loading    bool
	loadingURL string
	spinner    spinner.Model

	// Help overlay.
	keys      readerKeyMap
	helpModel help.Model
	showHelp  bool

	// Raw markdown toggle.
	showRaw         bool
	rawOrigName     string
	rawOrigMarkdown string

	// Error dialog state.
	showError bool
	errorText string
	errorURL  string // URL that failed, for "open in browser" fallback
}

const defaultContentWidth = 160

func newMarkdownReader(name, markdown, source string, theme *chroma.Style, conv converter, cache *conversionCache, client httpClient, fsys fileSystem, logger *slog.Logger) markdownReader {
	keys := defaultReaderKeyMap()

	view := mdk.NewModel(
		mdk.WithTheme(theme),
		mdk.WithGutter(true),
		mdk.WithContentWidth(defaultContentWidth),
	)
	view.SetText(name, markdown)
	view.KeyMap = keys.KeyMap

	helpModel := help.New()
	helpModel.ShowAll = true

	return markdownReader{
		view:          view,
		logger:        logger,
		converter:     conv,
		cache:         cache,
		client:        client,
		fsys:          fsys,
		currentSource: source,
		keys:          keys,
		helpModel:     helpModel,
		spinner:       spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

func (r markdownReader) Init() tea.Cmd {
	return nil
}

func (r markdownReader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mdk.OpenLinkMsg:
		return r, r.handleLinkNavigation(msg.URL)

	case mdk.GoBackMsg:
		r.showRaw = false
		r.popPage()
		return r, nil

	case pageLoadedMsg:
		r.showRaw = false
		r.pushCurrentPage()
		r.view.SetText(msg.name, msg.markdown)
		r.currentSource = msg.source
		r.currentOriginalHTML = msg.originalHTML
		r.currentReadabilityHTML = msg.readabilityHTML
		r.updateHTMLKeyBindings()
		r.loading = false
		r.loadingURL = ""
		return r, nil

	case pageLoadErrorMsg:
		r.loading = false
		r.loadingURL = ""
		r.showError = true
		r.errorURL = msg.url
		r.errorText = fmt.Sprintf("Error loading %s: %v\n\nPress 'o' to open in browser", msg.url, msg.err)
		return r, nil

	case spinner.TickMsg:
		if r.loading {
			var cmd tea.Cmd
			r.spinner, cmd = r.spinner.Update(msg)
			return r, cmd
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
			if msg.String() == "o" && r.errorURL != "" {
				url := r.errorURL
				r.showError = false
				r.errorURL = ""
				r.errorText = ""
				openInBrowser(url, r.logger)
				return r, nil
			}
			if msg.String() == "esc" || msg.String() == "enter" || msg.String() == "q" {
				r.showError = false
				r.errorURL = ""
				r.errorText = ""
				return r, nil
			}
			return r, nil
		}
		if r.loading {
			// Ignore input while loading.
			return r, nil
		}

		// Defer to view during search input.
		if r.view.Searching() {
			var cmd tea.Cmd
			r.view, cmd = r.view.Update(msg)
			return r, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return r, tea.Quit
		case "ctrl+r":
			if r.showRaw {
				r.view.SetText(r.rawOrigName, r.rawOrigMarkdown)
				r.showRaw = false
			} else {
				r.saveRawState()
				r.view.SetText(r.rawOrigName, fenceRaw(r.rawOrigMarkdown))
				r.showRaw = true
			}
			return r, nil
		case "ctrl+e":
			if r.currentOriginalHTML == "" {
				return r, nil
			}
			if r.showRaw {
				r.view.SetText(r.rawOrigName, r.rawOrigMarkdown)
				r.showRaw = false
			} else {
				r.saveRawState()
				r.view.SetText(r.rawOrigName, fenceHTML(r.currentOriginalHTML))
				r.showRaw = true
			}
			return r, nil
		case "ctrl+t":
			if r.currentReadabilityHTML == "" {
				return r, nil
			}
			if r.showRaw {
				r.view.SetText(r.rawOrigName, r.rawOrigMarkdown)
				r.showRaw = false
			} else {
				r.saveRawState()
				r.view.SetText(r.rawOrigName, fenceHTML(r.currentReadabilityHTML))
				r.showRaw = true
			}
			return r, nil
		case "ctrl+o":
			link := r.view.FocusedLinkDestination()
			if err := openInBrowser(link, r.logger); err != nil {
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

// handleLinkNavigation resolves and navigates to a link.
func (r *markdownReader) handleLinkNavigation(rawURL string) tea.Cmd {
	resolved := resolveLink(rawURL, r.currentSource)

	// HTTP/HTTPS URLs: fetch and convert (markdown or HTML via readability).
	if strings.HasPrefix(resolved, "http://") || strings.HasPrefix(resolved, "https://") {
		r.loading = true
		r.loadingURL = resolved
		return tea.Batch(fetchURLPage(resolved, r.converter, r.cache, r.client, r.logger), r.spinner.Tick)
	}

	// Local markdown files.
	if isMarkdownFile(resolved) {
		r.loading = true
		r.loadingURL = resolved
		return tea.Batch(loadFilePage(resolved, r.fsys, r.logger), r.spinner.Tick)
	}

	// Non-markdown files, mailto:, etc. — open in browser.
	openInBrowser(resolved, r.logger)
	return nil
}

// pushCurrentPage saves the current page state onto the back stack.
func (r *markdownReader) pushCurrentPage() {
	r.pageStack = append(r.pageStack, page{
		name:            r.view.GetName(),
		markdown:        string(r.view.GetMarkdown()),
		source:          r.currentSource,
		originalHTML:    r.currentOriginalHTML,
		readabilityHTML: r.currentReadabilityHTML,
		lineOffset:      r.view.LineOffset(),
		columnOffset:    r.view.ColumnOffset(),
	})
}

// popPage restores the previous page from the back stack.
func (r *markdownReader) popPage() {
	if len(r.pageStack) == 0 {
		return
	}
	prev := r.pageStack[len(r.pageStack)-1]
	r.pageStack = r.pageStack[:len(r.pageStack)-1]
	r.view.SetText(prev.name, prev.markdown)
	r.currentSource = prev.source
	r.currentOriginalHTML = prev.originalHTML
	r.currentReadabilityHTML = prev.readabilityHTML
	r.updateHTMLKeyBindings()
	r.view.SetLineOffset(prev.lineOffset)
	r.view.SetColumnOffset(prev.columnOffset)
}

// updateHTMLKeyBindings enables or disables the HTML view key bindings
// based on whether the current page has HTML content.
func (r *markdownReader) updateHTMLKeyBindings() {
	hasHTML := r.currentOriginalHTML != ""
	r.keys.ToggleOriginalHTML.SetEnabled(hasHTML)
	r.keys.ToggleReadabilityHTML.SetEnabled(hasHTML)
}

func (r markdownReader) View() tea.View {
	if r.width == 0 || r.height == 0 {
		return tea.View{}
	}

	base := r.view.View()

	var result string
	if r.loading {
		loadingText := r.spinner.View() + " Loading..."
		if r.loadingURL != "" {
			loadingText = r.spinner.View() + fmt.Sprintf(" Loading %s...", r.loadingURL)
		}
		result = r.overlayDialog(base, "Loading", loadingText)
	} else if r.showHelp {
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

// saveRawState saves the current view state for toggling back from a raw view.
func (r *markdownReader) saveRawState() {
	r.rawOrigName = r.view.GetName()
	r.rawOrigMarkdown = string(r.view.GetMarkdown())
}

// fenceHTML wraps HTML in a fenced code block with html syntax highlighting.
func fenceHTML(html string) string {
	maxRun := 0
	run := 0
	for _, c := range html {
		if c == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(maxRun+1, 3))
	return fence + "html\n" + html + "\n" + fence
}

// fenceRaw wraps markdown in a fenced code block for raw display.
// It scans the content for the longest run of consecutive backticks
// and uses one more to avoid conflicts.
func fenceRaw(markdown string) string {
	maxRun := 0
	run := 0
	for _, c := range markdown {
		if c == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(maxRun+1, 3))
	return fence + "\n" + markdown + "\n" + fence
}
