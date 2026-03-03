package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/markdown-kit/docsearch"
	mdk "github.com/pgavlin/markdown-kit/view"
	"github.com/skratchdot/open-golang/open"
)

// readerKeyMap combines the view KeyMap with md-specific bindings.
// It implements the help.KeyMap interface for use with bubbles/help.
type readerKeyMap struct {
	mdk.KeyMap // embed the view KeyMap

	ToggleRaw             key.Binding
	ToggleOriginalHTML    key.Binding
	ToggleReadabilityHTML key.Binding
	OpenFile              key.Binding
	OpenBrowser           key.Binding
	OpenFileNewTab        key.Binding
	OpenURL               key.Binding
	NextTab               key.Binding
	PrevTab               key.Binding
	CloseTab              key.Binding
	CloseAllTabs          key.Binding
	NewTab                key.Binding
	Reload                key.Binding
	History               key.Binding
	SearchDocuments       key.Binding
	FindSimilar           key.Binding
	UserGuide             key.Binding
	BugReport             key.Binding
	Help                  key.Binding
	Quit                  key.Binding
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
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "view source"),
		),
		OpenFile: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("ctrl+o", "open file"),
		),
		OpenBrowser: key.NewBinding(
			key.WithKeys("shift+enter"),
			key.WithHelp("shift+enter", "open in browser"),
		),
		OpenFileNewTab: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "open file in new tab"),
		),
		OpenURL: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "open URL"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev tab"),
		),
		CloseTab: key.NewBinding(
			key.WithKeys("ctrl+w"),
			key.WithHelp("ctrl+w", "close tab"),
		),
		CloseAllTabs: key.NewBinding(
			key.WithKeys("W"),
			key.WithHelp("W", "close all tabs"),
		),
		NewTab: key.NewBinding(
			key.WithKeys("T"),
			key.WithHelp("T", "open link in new tab"),
		),
		Reload: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "reload"),
		),
		History: key.NewBinding(
			key.WithKeys("H"),
			key.WithHelp("H", "history"),
		),
		SearchDocuments: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "search documents"),
		),
		FindSimilar: key.NewBinding(
			key.WithKeys("F"),
			key.WithHelp("F", "find similar"),
		),
		UserGuide: key.NewBinding(
			key.WithKeys("M"),
			key.WithHelp("M", "user guide"),
		),
		BugReport: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("ctrl+b", "bug report"),
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
	return append(km.KeyMap.ShortHelp(), km.OpenFile, km.OpenURL, km.ToggleRaw, km.Help, km.Quit)
}

// FullHelp returns the full set of key bindings for the expanded help view.
// We build the layout from scratch (rather than appending to the view's
// FullHelp) so that all 37 bindings fit into 5 balanced columns.
func (km readerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Movement
		{km.Up, km.Down, km.PageUp, km.PageDown, km.GotoTop, km.GotoEnd, km.Left, km.Right},
		// Navigation
		{km.Home, km.End, km.NextLink, km.PrevLink, km.NextHeading, km.PrevHeading, km.NextCodeBlock, km.PrevCodeBlock},
		// Actions
		{km.FollowLink, km.GoBack, km.History, km.SearchDocuments, km.FindSimilar, km.Reload, km.CopySelection, km.OpenFile, km.OpenURL, km.OpenBrowser, km.DecreaseWidth, km.IncreaseWidth},
		// Search & View
		{km.Search, km.NextMatch, km.PrevMatch, km.ClearSearch, km.ToggleRaw},
		// Tabs & General
		{km.NextTab, km.PrevTab, km.CloseTab, km.CloseAllTabs, km.NewTab, km.OpenFileNewTab, km.UserGuide, km.BugReport, km.Help, km.Quit},
	}
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
	name         string
	markdown     string
	source       string
	lineOffset   int
	columnOffset int
}

// tab holds all per-document state for a single tab.
type tab struct {
	view            mdk.Model
	currentSource   string
	pageStack       []page
	showRaw         bool
	rawOrigName     string
	rawOrigMarkdown string
}

// displayName returns the tab's display name: the document heading if available,
// otherwise the basename of the source path/URL.
func (t *tab) displayName() string {
	if name := t.view.GetName(); name != "" {
		return name
	}
	if t.currentSource != "" {
		return filepath.Base(t.currentSource)
	}
	return ""
}

type markdownReader struct {
	// Tab management.
	tabs      []tab
	activeTab int

	// Theme needed to create new tab views.
	theme *chroma.Style

	width, height int

	// Structured logger for I/O operations.
	logger *slog.Logger

	// Content converter for non-markdown URLs.
	converter converter

	// Format converter registry for non-markdown files and MIME types.
	registry *converterRegistry

	// Disk cache for conversion results.
	cache *conversionCache

	// HTTP client for fetching URLs.
	client httpClient

	// Filesystem abstraction.
	fsys fileSystem

	// Loading state for async page fetches.
	loading    bool
	loadingURL string
	spinner    spinner.Model

	// Help overlay.
	keys      readerKeyMap
	helpModel help.Model
	showHelp  bool

	// Error dialog state.
	showError bool
	errorText string
	errorURL  string // URL that failed, for "open in browser" fallback

	// File picker state.
	picker         fuzzyPicker
	showPicker     bool
	pickerStartup  bool            // true when picker is shown at startup (no content loaded yet)
	pickerNewTab   bool            // true when the picker should open the selected file in a new tab
	pickerURLMode  bool            // true when the picker is showing URL input instead of file list
	pickerURLInput textinput.Model // URL text input for picker URL mode

	// URL input state.
	showURLInput bool
	urlInput     textinput.Model
	urlNewTab    bool

	// History picker state.
	showHistory   bool
	historyPicker historyPicker

	// Search index for document search.
	searchIndex *docsearch.Index

	// Search picker state.
	showSearch   bool
	searchPicker searchPicker

	// Similar documents picker state.
	showSimilar   bool
	similarPicker searchPicker

	// Bug report state.
	showBugReport    bool
	bugReportInput   textinput.Model
	bugReportCapture bugReportData
}

// active returns a pointer to the active tab.
func (r *markdownReader) active() *tab {
	return &r.tabs[r.activeTab]
}

const defaultContentWidth = 160

// newTab creates a new tab with a fresh mdk.Model using the reader's theme and keys.
func (r *markdownReader) newTab() tab {
	view := mdk.NewModel(
		mdk.WithTheme(r.theme),
		mdk.WithGutter(true),
		mdk.WithContentWidth(defaultContentWidth),
	)
	view.KeyMap = r.keys.KeyMap
	if r.width > 0 && r.height > 0 {
		view.SetSize(r.width, r.viewHeight())
	}
	return tab{view: view}
}

func newMarkdownReader(name, markdown, source string, theme *chroma.Style, conv converter, registry *converterRegistry, cache *conversionCache, client httpClient, fsys fileSystem, searchIndex *docsearch.Index, logger *slog.Logger) markdownReader {
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

	wd, _ := fsys.Getwd()
	fp := newFuzzyPicker(wd, viewableExtsList(registry), fsys)

	return markdownReader{
		tabs: []tab{{
			view:          view,
			currentSource: source,
		}},
		activeTab:   0,
		theme:       theme,
		logger:      logger,
		converter:   conv,
		registry:    registry,
		cache:       cache,
		client:      client,
		fsys:        fsys,
		keys:        keys,
		helpModel:   helpModel,
		spinner:     spinner.New(spinner.WithSpinner(spinner.Dot)),
		picker:      fp,
		searchIndex: searchIndex,
	}
}

// tabBarHeight returns the height of the tab bar (1 when multiple tabs, 0 otherwise).
func (r *markdownReader) tabBarHeight() int {
	if len(r.tabs) > 1 {
		return 1
	}
	return 0
}

// viewHeight returns the height available for the document view.
func (r *markdownReader) viewHeight() int {
	return r.height - r.tabBarHeight()
}

// resizeAllViews updates the size of all tab views.
func (r *markdownReader) resizeAllViews() {
	vh := r.viewHeight()
	for i := range r.tabs {
		r.tabs[i].view.SetSize(r.width, vh)
	}
}

// nextTab switches to the next tab (wrapping around).
func (r *markdownReader) nextTab() {
	if len(r.tabs) <= 1 {
		return
	}
	r.activeTab = (r.activeTab + 1) % len(r.tabs)
}

// prevTab switches to the previous tab (wrapping around).
func (r *markdownReader) prevTab() {
	if len(r.tabs) <= 1 {
		return
	}
	r.activeTab = (r.activeTab - 1 + len(r.tabs)) % len(r.tabs)
}

// closeTab closes the tab at the given index.
func (r *markdownReader) closeTab(idx int) {
	if len(r.tabs) <= 1 {
		// Last tab — reset to a blank tab and show the file picker.
		r.tabs[0] = r.newTab()
		r.activeTab = 0
		r.showPicker = true
		r.pickerStartup = true
		r.pickerURLMode = false
		return
	}
	r.tabs = append(r.tabs[:idx], r.tabs[idx+1:]...)
	if r.activeTab >= len(r.tabs) {
		r.activeTab = len(r.tabs) - 1
	} else if r.activeTab > idx {
		r.activeTab--
	}
	r.resizeAllViews()
}

// closeAllTabs closes all tabs and shows the file picker.
func (r *markdownReader) closeAllTabs() {
	r.tabs = []tab{r.newTab()}
	r.activeTab = 0
	r.showPicker = true
	r.pickerStartup = true
	r.pickerURLMode = false
	r.resizeAllViews()
}

// openNewTab creates a new tab with the given content and makes it active.
func (r *markdownReader) openNewTab(name, markdown, source string) {
	hadOneTab := len(r.tabs) == 1
	t := r.newTab()
	t.view.SetText(name, markdown)
	t.currentSource = source
	r.tabs = append(r.tabs, t)
	r.activeTab = len(r.tabs) - 1
	if hadOneTab {
		// Tab bar just appeared — resize all views to account for it.
		r.resizeAllViews()
	}
}

// renderTabBar renders the tab bar when multiple tabs are open.
func (r *markdownReader) renderTabBar() string {
	if len(r.tabs) <= 1 {
		return ""
	}

	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(colorTabActiveFg).Background(colorTabActiveBg).Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().Faint(true).Padding(0, 1)

	var parts []string
	for i := range r.tabs {
		name := r.tabs[i].displayName()
		if name == "" {
			name = fmt.Sprintf("Tab %d", i+1)
		}
		// Truncate long names.
		if ansi.StringWidth(name) > 20 {
			name = ansi.Truncate(name, 17, "...")
		}
		if i == r.activeTab {
			parts = append(parts, activeStyle.Render(name))
		} else {
			parts = append(parts, inactiveStyle.Render(name))
		}
	}

	bar := strings.Join(parts, " ")
	// Truncate bar to terminal width.
	if ansi.StringWidth(bar) > r.width {
		bar = ansi.Truncate(bar, r.width, "...")
	}
	return bar
}

func (r markdownReader) Init() tea.Cmd {
	if r.showPicker {
		return r.picker.Init()
	}
	return nil
}

func (r markdownReader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle file picker state — all message types must reach the picker.
	if r.showPicker {
		if km, ok := msg.(tea.KeyPressMsg); ok {
			switch km.String() {
			case "ctrl+c":
				if r.pickerStartup {
					return r, tea.Quit
				}
				r.showPicker = false
				r.pickerURLMode = false
				return r, nil
			case "esc":
				if r.pickerURLMode {
					if r.pickerURLInput.Value() == "" {
						if r.pickerStartup {
							return r, tea.Quit
						}
						r.showPicker = false
						r.pickerURLMode = false
						return r, nil
					}
					r.pickerURLInput.SetValue("")
					return r, nil
				}
				if r.picker.input.Value() == "" {
					if r.pickerStartup {
						return r, tea.Quit
					}
					r.showPicker = false
					return r, nil
				}
				// Clear the filter text instead of dismissing.
				r.picker.input.SetValue("")
				r.picker.filter()
				return r, nil
			case "tab", "shift+tab":
				r.pickerURLMode = !r.pickerURLMode
				if r.pickerURLMode {
					r.pickerURLInput = textinput.New()
					r.pickerURLInput.Prompt = "  URL: "
					r.pickerURLInput.Placeholder = "https://..."
					fixedW := r.width * 3 / 4
					if fixedW < 40 {
						fixedW = min(r.width-4, 40)
					}
					innerW := fixedW - 4 // account for border + padding
					r.pickerURLInput.SetWidth(innerW - lipgloss.Width(r.pickerURLInput.Prompt) - 1)
					return r, r.pickerURLInput.Focus()
				}
				return r, r.picker.input.Focus()
			}
		}

		if r.pickerURLMode {
			// URL mode: handle enter, forward other messages to URL input.
			if km, ok := msg.(tea.KeyPressMsg); ok && km.String() == "enter" {
				url := r.pickerURLInput.Value()
				r.showPicker = false
				r.pickerURLMode = false
				r.pickerStartup = false
				if url != "" {
					r.loading = true
					r.loadingURL = url
					return r, tea.Batch(
						fetchURLPage(url, r.pickerNewTab, r.converter, r.registry, r.cache, r.client, r.logger),
						r.spinner.Tick,
					)
				}
				return r, nil
			}
			var cmd tea.Cmd
			r.pickerURLInput, cmd = r.pickerURLInput.Update(msg)
			if ws, ok := msg.(tea.WindowSizeMsg); ok {
				r.width = ws.Width
				r.height = ws.Height
				r.resizeAllViews()
				r.helpModel.SetWidth(ws.Width)
			}
			return r, cmd
		}

		// File mode.
		var cmd tea.Cmd
		r.picker, cmd = r.picker.Update(msg)
		if didSelect, path := r.picker.DidSelect(); didSelect {
			r.showPicker = false
			r.pickerStartup = false
			r.loading = true
			r.loadingURL = path
			var cmd tea.Cmd
			if isConvertibleFile(path, r.registry) {
				cmd = loadConvertFilePage(path, r.pickerNewTab, r.registry, r.cache, r.fsys, r.logger)
			} else {
				cmd = loadFilePage(path, r.pickerNewTab, r.fsys, r.logger)
			}
			return r, tea.Batch(cmd, r.spinner.Tick)
		}
		// Also handle window size for picker dimensions.
		if ws, ok := msg.(tea.WindowSizeMsg); ok {
			r.width = ws.Width
			r.height = ws.Height
			r.resizeAllViews()
			r.helpModel.SetWidth(ws.Width)
			r.picker.SetHeight(min(ws.Height-2, 20))
			r.picker.SetWidth(ws.Width)
		}
		return r, cmd
	}

	// Handle URL input modal.
	if r.showURLInput {
		if km, ok := msg.(tea.KeyPressMsg); ok {
			switch km.String() {
			case "esc":
				r.showURLInput = false
				return r, nil
			case "enter":
				url := r.urlInput.Value()
				newTab := r.urlNewTab
				r.showURLInput = false
				if url != "" {
					return r, r.handleLinkNavigation(url, newTab)
				}
				return r, nil
			}
		}
		var cmd tea.Cmd
		r.urlInput, cmd = r.urlInput.Update(msg)
		return r, cmd
	}

	// Handle bug report input modal.
	if r.showBugReport {
		if km, ok := msg.(tea.KeyPressMsg); ok {
			switch km.String() {
			case "esc":
				r.showBugReport = false
				return r, nil
			case "enter":
				desc := r.bugReportInput.Value()
				report := r.bugReportCapture.formatBugReport(desc)
				r.showBugReport = false
				return r, tea.SetClipboard(report)
			}
		}
		var cmd tea.Cmd
		r.bugReportInput, cmd = r.bugReportInput.Update(msg)
		return r, cmd
	}

	// Handle history picker modal.
	if r.showHistory {
		var cmd tea.Cmd
		r.historyPicker, cmd = r.historyPicker.Update(msg)
		if r.historyPicker.dismissed {
			r.showHistory = false
			return r, nil
		}
		if didSelect, idx := r.historyPicker.DidSelect(); didSelect {
			r.showHistory = false
			if idx != -1 {
				// Navigate to the selected stack entry.
				at := r.active()
				at.showRaw = false
				prev := at.pageStack[idx]
				at.pageStack = at.pageStack[:idx]
				at.view.SetText(prev.name, prev.markdown)
				at.currentSource = prev.source
				at.view.SetLineOffset(prev.lineOffset)
				at.view.SetColumnOffset(prev.columnOffset)
			}
			return r, nil
		}
		return r, cmd
	}

	// Handle search picker modal.
	if r.showSearch {
		var cmd tea.Cmd
		r.searchPicker, cmd = r.searchPicker.Update(msg)
		if r.searchPicker.dismissed {
			r.showSearch = false
			return r, nil
		}
		if didSelect, path := r.searchPicker.DidSelect(); didSelect {
			r.showSearch = false
			if path != "" {
				r.loading = true
				r.loadingURL = path
				if isConvertibleFile(path, r.registry) {
					return r, tea.Batch(loadConvertFilePage(path, false, r.registry, r.cache, r.fsys, r.logger), r.spinner.Tick)
				}
				return r, tea.Batch(loadFilePage(path, false, r.fsys, r.logger), r.spinner.Tick)
			}
			return r, nil
		}
		return r, cmd
	}

	// Handle similar documents picker modal.
	if r.showSimilar {
		var cmd tea.Cmd
		r.similarPicker, cmd = r.similarPicker.Update(msg)
		if r.similarPicker.dismissed {
			r.showSimilar = false
			return r, nil
		}
		if didSelect, path := r.similarPicker.DidSelect(); didSelect {
			r.showSimilar = false
			if path != "" {
				r.loading = true
				r.loadingURL = path
				if isConvertibleFile(path, r.registry) {
					return r, tea.Batch(loadConvertFilePage(path, false, r.registry, r.cache, r.fsys, r.logger), r.spinner.Tick)
				}
				return r, tea.Batch(loadFilePage(path, false, r.fsys, r.logger), r.spinner.Tick)
			}
			return r, nil
		}
		return r, cmd
	}

	switch msg := msg.(type) {
	case mdk.OpenLinkMsg:
		return r, r.handleLinkNavigation(msg.URL, false)

	case mdk.GoBackMsg:
		r.active().showRaw = false
		r.popPage()
		return r, nil

	case pageLoadedMsg:
		if msg.newTab {
			r.openNewTab(msg.name, msg.markdown, msg.source)
		} else {
			at := r.active()
			at.showRaw = false
			// Don't push to the stack on reload or when the page is empty.
			if !msg.reload && (at.view.GetName() != "" || len(at.view.GetMarkdown()) > 0) {
				r.pushCurrentPage()
			}
			at.view.SetText(msg.name, msg.markdown)
			at.currentSource = msg.source
		}
		r.loading = false
		r.loadingURL = ""

		// Index the document in the background.
		// Use the view's resolved name (extracted from the first heading)
		// rather than msg.name, which is often empty for local files.
		var cmds []tea.Cmd
		if r.searchIndex != nil && msg.source != "" {
			title := r.active().view.GetName()
			cmds = append(cmds, indexDocument(r.searchIndex, msg.source, title, msg.markdown))
		}
		if len(cmds) > 0 {
			return r, tea.Batch(cmds...)
		}
		return r, nil

	case findSimilarResultsMsg:
		r.showSimilar = true
		r.similarPicker = newSimilarPicker(
			r.searchIndex, msg.results,
			min(r.height*3/4, 20), r.width*3/4,
		)
		return r, r.similarPicker.input.Focus()

	case indexDocumentMsg:
		// Indexing completed successfully — nothing to do.
		return r, nil

	case indexDocumentErrorMsg:
		r.logger.Error("index_error", "error", msg.err)
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
		r.resizeAllViews()
		r.helpModel.SetWidth(msg.Width)
		r.picker.SetHeight(min(msg.Height-2, 20))
		r.picker.SetWidth(msg.Width)
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

		at := r.active()

		// Defer to view during search input.
		if at.view.Searching() {
			var cmd tea.Cmd
			at.view, cmd = at.view.Update(msg)
			return r, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return r, tea.Quit
		case "ctrl+u":
			if at.showRaw {
				at.view.SetText(at.rawOrigName, at.rawOrigMarkdown)
				at.showRaw = false
			} else {
				r.saveRawState()
				at.view.SetText(at.rawOrigName, fenceRaw(at.rawOrigMarkdown))
				at.showRaw = true
			}
			return r, nil
		case "ctrl+o":
			r.showPicker = true
			r.pickerStartup = false
			r.pickerNewTab = false
			r.pickerURLMode = false
			r.picker.selected = ""
			return r, r.picker.Init()
		case "ctrl+t":
			r.showPicker = true
			r.pickerStartup = false
			r.pickerNewTab = true
			r.pickerURLMode = false
			r.picker.selected = ""
			return r, r.picker.Init()
		case "ctrl+l":
			r.showURLInput = true
			r.urlNewTab = false
			r.urlInput = textinput.New()
			r.urlInput.Prompt = "  URL: "
			r.urlInput.Placeholder = "https://..."
			fixedW := r.width * 3 / 4
			if fixedW < 40 {
				fixedW = min(r.width-4, 40)
			}
			innerW := fixedW - 4 // account for border + padding
			r.urlInput.SetWidth(innerW - lipgloss.Width(r.urlInput.Prompt) - 1)
			return r, r.urlInput.Focus()
		case "shift+enter":
			link := at.view.FocusedLinkDestination()
			if err := openInBrowser(link, r.logger); err != nil {
				r.showError = true
				r.errorText = fmt.Sprintf("Error opening URL: %v", err)
			}
			return r, nil
		case "tab":
			r.nextTab()
			return r, nil
		case "shift+tab":
			r.prevTab()
			return r, nil
		case "ctrl+w":
			r.closeTab(r.activeTab)
			if r.showPicker {
				return r, r.picker.Init()
			}
			return r, nil
		case "W":
			r.closeAllTabs()
			return r, r.picker.Init()
		case "T":
			link := at.view.FocusedLinkDestination()
			if link != "" {
				return r, r.handleLinkNavigation(link, true)
			}
			return r, nil
		case "ctrl+r":
			return r, r.reloadCurrentPage()
		case "H":
			at := r.active()
			if len(at.pageStack) == 0 {
				return r, nil
			}
			r.showHistory = true
			r.historyPicker = newHistoryPicker(
				at.pageStack,
				at.view.GetName(), at.currentSource,
				min(r.height*3/4, 20), r.width*3/4,
			)
			return r, r.historyPicker.input.Focus()
		case "S":
			if r.searchIndex == nil {
				return r, nil
			}
			r.showSearch = true
			r.searchPicker = newSearchPicker(
				r.searchIndex,
				min(r.height*3/4, 20), r.width*3/4,
			)
			return r, tea.Batch(r.searchPicker.input.Focus(), r.searchPicker.doSearch())
		case "F":
			if r.searchIndex == nil || !r.searchIndex.HasEmbedder() {
				return r, nil
			}
			at := r.active()
			content := string(at.view.GetMarkdown())
			if content == "" {
				return r, nil
			}
			return r, findSimilar(r.searchIndex, content, at.currentSource)
		case "M":
			r.openNewTab("User Guide", renderHelpPage(r.keys), "")
			return r, nil
		case "?":
			r.showHelp = true
			return r, nil
		}

		if key.Matches(msg, r.keys.BugReport) {
			r.captureBugReport()
			r.bugReportInput = textinput.New()
			r.bugReportInput.Prompt = "  Description: "
			r.bugReportInput.Placeholder = "Describe the issue..."
			fixedW := r.width * 3 / 4
			if fixedW < 40 {
				fixedW = min(r.width-4, 40)
			}
			innerW := fixedW - 4
			r.bugReportInput.SetWidth(innerW - lipgloss.Width(r.bugReportInput.Prompt) - 1)
			r.showBugReport = true
			return r, r.bugReportInput.Focus()
		}

		// Pass other keys to the view.
		var cmd tea.Cmd
		at.view, cmd = at.view.Update(msg)
		return r, cmd
	}

	return r, nil
}

// reloadCurrentPage re-fetches and re-converts the current tab's source.
func (r *markdownReader) reloadCurrentPage() tea.Cmd {
	source := r.active().currentSource
	if source == "" {
		return nil
	}

	r.loading = true
	r.loadingURL = source

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		r.cache.evictHTTP(source, r.logger)
		return tea.Batch(reloadURLPage(source, r.converter, r.registry, r.cache, r.client, r.logger), r.spinner.Tick)
	}

	if isMarkdownFile(source) {
		return tea.Batch(reloadFilePage(source, r.fsys, r.logger), r.spinner.Tick)
	}

	if isConvertibleFile(source, r.registry) {
		return tea.Batch(reloadConvertFilePage(source, r.registry, r.cache, r.fsys, r.logger), r.spinner.Tick)
	}

	r.loading = false
	r.loadingURL = ""
	return nil
}

// handleLinkNavigation resolves and navigates to a link.
func (r *markdownReader) handleLinkNavigation(rawURL string, newTab bool) tea.Cmd {
	resolved := resolveLink(rawURL, r.active().currentSource)

	// HTTP/HTTPS URLs: fetch and convert (markdown or HTML via readability).
	if strings.HasPrefix(resolved, "http://") || strings.HasPrefix(resolved, "https://") {
		r.loading = true
		r.loadingURL = resolved
		return tea.Batch(fetchURLPage(resolved, newTab, r.converter, r.registry, r.cache, r.client, r.logger), r.spinner.Tick)
	}

	// Local markdown files.
	if isMarkdownFile(resolved) {
		r.loading = true
		r.loadingURL = resolved
		return tea.Batch(loadFilePage(resolved, newTab, r.fsys, r.logger), r.spinner.Tick)
	}

	// Local files with a registered converter.
	if isConvertibleFile(resolved, r.registry) {
		r.loading = true
		r.loadingURL = resolved
		return tea.Batch(loadConvertFilePage(resolved, newTab, r.registry, r.cache, r.fsys, r.logger), r.spinner.Tick)
	}

	// Non-markdown files, mailto:, etc. — open in browser.
	openInBrowser(resolved, r.logger)
	return nil
}

// pushCurrentPage saves the current page state onto the back stack.
func (r *markdownReader) pushCurrentPage() {
	at := r.active()
	at.pageStack = append(at.pageStack, page{
		name:         at.view.GetName(),
		markdown:     string(at.view.GetMarkdown()),
		source:       at.currentSource,
		lineOffset:   at.view.LineOffset(),
		columnOffset: at.view.ColumnOffset(),
	})
}

// popPage restores the previous page from the back stack.
func (r *markdownReader) popPage() {
	at := r.active()
	if len(at.pageStack) == 0 {
		return
	}
	prev := at.pageStack[len(at.pageStack)-1]
	at.pageStack = at.pageStack[:len(at.pageStack)-1]
	at.view.SetText(prev.name, prev.markdown)
	at.currentSource = prev.source
	at.view.SetLineOffset(prev.lineOffset)
	at.view.SetColumnOffset(prev.columnOffset)
}

func (r markdownReader) View() tea.View {
	if r.width == 0 || r.height == 0 {
		return tea.View{}
	}

	base := r.active().view.View()

	// Prepend tab bar when multiple tabs are open.
	tabBar := r.renderTabBar()
	if tabBar != "" {
		base = tabBar + "\n" + base
	}

	var result string
	if r.showPicker {
		header := renderOpenModeHeader(!r.pickerURLMode)
		fixedW := r.width * 3 / 4
		if fixedW < 40 {
			fixedW = min(r.width-4, 40)
		}
		if r.pickerURLMode {
			contentView := header + "\n\n" + r.pickerURLInput.View()
			result = r.renderFixedOverlay(base, contentView, fixedW, 5)
		} else {
			contentView := header + "\n\n" + r.picker.View()
			maxH := r.height * 3 / 4
			result = r.renderFixedOverlay(base, contentView, fixedW, maxH)
		}
	} else if r.showSearch {
		header := lipgloss.NewStyle().Bold(true).Render("Search Documents")
		searchView := header + "\n\n" + r.searchPicker.View()
		fixedW := r.width * 3 / 4
		if fixedW < 40 {
			fixedW = min(r.width-4, 40)
		}
		maxH := r.height * 3 / 4
		result = r.renderFixedOverlay(base, searchView, fixedW, maxH)
	} else if r.showSimilar {
		header := lipgloss.NewStyle().Bold(true).Render("Similar Documents")
		similarView := header + "\n\n" + r.similarPicker.View()
		fixedW := r.width * 3 / 4
		if fixedW < 40 {
			fixedW = min(r.width-4, 40)
		}
		maxH := r.height * 3 / 4
		result = r.renderFixedOverlay(base, similarView, fixedW, maxH)
	} else if r.showHistory {
		header := lipgloss.NewStyle().Bold(true).Render("History")
		historyView := header + "\n\n" + r.historyPicker.View()
		fixedW := r.width * 3 / 4
		if fixedW < 40 {
			fixedW = min(r.width-4, 40)
		}
		maxH := r.height * 3 / 4
		result = r.renderFixedOverlay(base, historyView, fixedW, maxH)
	} else if r.showURLInput {
		header := lipgloss.NewStyle().Bold(true).Render("Open URL")
		inputView := header + "\n\n" + r.urlInput.View()
		fixedW := r.width * 3 / 4
		if fixedW < 40 {
			fixedW = min(r.width-4, 40)
		}
		result = r.renderFixedOverlay(base, inputView, fixedW, 5)
	} else if r.showBugReport {
		header := lipgloss.NewStyle().Bold(true).Render("Bug Report")
		inputView := header + "\n\n" + r.bugReportInput.View()
		fixedW := r.width * 3 / 4
		if fixedW < 40 {
			fixedW = min(r.width-4, 40)
		}
		result = r.renderFixedOverlay(base, inputView, fixedW, 5)
	} else if r.loading {
		loadingText := r.spinner.View() + " Loading..."
		if r.loadingURL != "" {
			loadingText = r.spinner.View() + fmt.Sprintf(" Loading %s...", r.loadingURL)
		}
		result = r.overlayDialog(base, "Loading", loadingText)
	} else if r.showHelp {
		maxH := r.height * 3 / 4

		// Give the help model enough width to render all columns, then
		// let the overlay size itself to the actual rendered content.
		r.helpModel.SetWidth(r.width - 4) // account for border + padding
		content := r.helpModel.View(r.keys)

		// Skip wordWrap — help.Model already formats its own columns.
		result = r.renderOverlay(base, content, r.width-2, maxH)
	} else if r.showError {
		result = r.overlayDialog(base, "Error", r.errorText)
	} else {
		result = base
	}

	v := tea.NewView(result)
	v.AltScreen = true
	v.WindowTitle = r.active().displayName()
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

// renderOpenModeHeader renders the File/URL mode indicator for the open picker.
func renderOpenModeHeader(fileActive bool) string {
	active := lipgloss.NewStyle().Bold(true).Foreground(colorTabActiveFg).Background(colorTabActiveBg)
	inactive := lipgloss.NewStyle().Foreground(colorMuted)
	hint := lipgloss.NewStyle().Foreground(colorMuted)
	if fileActive {
		return active.Render("File") + "  " + inactive.Render("URL") + hint.Render("  (tab to switch)")
	}
	return inactive.Render("File") + "  " + active.Render("URL") + hint.Render("  (tab to switch)")
}

// renderFixedOverlay renders content in a bordered dialog with a fixed width, centered over base.
func (r markdownReader) renderFixedOverlay(base, content string, fixedW, maxH int) string {
	innerW := fixedW - 4 // border + padding
	lines := strings.Split(content, "\n")
	if len(lines) > maxH-2 {
		lines = lines[:maxH-2]
	}
	for i, line := range lines {
		if ansi.StringWidth(line) > innerW {
			lines[i] = ansi.Truncate(line, innerW-1, "\u2026")
		}
	}
	content = strings.Join(lines, "\n")

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted).
		Padding(0, 1).
		Width(fixedW)

	dialog := dialogStyle.Render(content)
	return placeOverlay(r.width, r.height, dialog, base)
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
		BorderForeground(colorMuted).
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

// bugReportData holds all captured state for a bug report.
type bugReportData struct {
	documentName   string
	source         string
	renderedView   string
	rawMarkdown    string
	viewportWidth  int
	viewportHeight int
	effectiveWidth int
	contentWidth   int
	lineOffset     int
	columnOffset   int
	totalLines     int
	visibleLines   int
	scrollPct      float64
	termWidth      int
	termHeight     int
	termEnv        string
	colorTerm      string
	themeName      string
	showRaw        bool
	tabCount       int
	activeTab      int
	goVersion      string
}

// captureBugReport populates bugReportCapture from the active tab state.
func (r *markdownReader) captureBugReport() {
	at := r.active()
	v := &at.view
	r.bugReportCapture = bugReportData{
		documentName:   v.GetName(),
		source:         at.currentSource,
		renderedView:   v.View(),
		rawMarkdown:    string(v.GetMarkdown()),
		viewportWidth:  v.Width(),
		viewportHeight: v.Height(),
		effectiveWidth: v.EffectiveWidth(),
		contentWidth:   v.ContentWidth(),
		lineOffset:     v.LineOffset(),
		columnOffset:   v.ColumnOffset(),
		totalLines:     v.TotalLineCount(),
		visibleLines:   v.VisibleLineCount(),
		scrollPct:      v.ScrollPercent(),
		termWidth:      r.width,
		termHeight:     r.height,
		termEnv:        os.Getenv("TERM"),
		colorTerm:      os.Getenv("COLORTERM"),
		themeName:      r.theme.Name,
		showRaw:        at.showRaw,
		tabCount:       len(r.tabs),
		activeTab:      r.activeTab,
		goVersion:      runtime.Version(),
	}
}

// formatBugReport formats the captured data and user description into clipboard text.
func (d bugReportData) formatBugReport(description string) string {
	var b strings.Builder
	b.WriteString("## Bug Report\n\n")
	b.WriteString("### Description\n")
	b.WriteString(description)
	b.WriteString("\n\n")

	b.WriteString("### Environment\n")
	fmt.Fprintf(&b, "- Go: %s\n", d.goVersion)
	fmt.Fprintf(&b, "- Theme: %s\n", d.themeName)
	fmt.Fprintf(&b, "- TERM: %s\n", d.termEnv)
	fmt.Fprintf(&b, "- COLORTERM: %s\n", d.colorTerm)
	fmt.Fprintf(&b, "- Terminal size: %dx%d\n", d.termWidth, d.termHeight)
	b.WriteString("\n")

	b.WriteString("### Document\n")
	fmt.Fprintf(&b, "- Name: %s\n", d.documentName)
	fmt.Fprintf(&b, "- Source: %s\n", d.source)
	fmt.Fprintf(&b, "- Tab: %d/%d\n", d.activeTab+1, d.tabCount)
	fmt.Fprintf(&b, "- Raw mode: %v\n", d.showRaw)
	b.WriteString("\n")

	b.WriteString("### Viewport\n")
	fmt.Fprintf(&b, "- Viewport: %dx%d\n", d.viewportWidth, d.viewportHeight)
	fmt.Fprintf(&b, "- Effective width: %d\n", d.effectiveWidth)
	fmt.Fprintf(&b, "- Content width: %d\n", d.contentWidth)
	fmt.Fprintf(&b, "- Scroll: line %d/%d (%.0f%%)\n", d.lineOffset, d.totalLines, d.scrollPct*100)
	fmt.Fprintf(&b, "- Column offset: %d\n", d.columnOffset)
	fmt.Fprintf(&b, "- Visible lines: %d\n", d.visibleLines)
	b.WriteString("\n")

	b.WriteString("### Rendered Output\n```\n")
	b.WriteString(d.renderedView)
	b.WriteString("\n```\n\n")

	b.WriteString("### Raw Markdown\n```markdown\n")
	b.WriteString(d.rawMarkdown)
	b.WriteString("\n```\n")

	return b.String()
}

// saveRawState saves the current view state for toggling back from a raw view.
func (r *markdownReader) saveRawState() {
	at := r.active()
	at.rawOrigName = at.view.GetName()
	at.rawOrigMarkdown = string(at.view.GetMarkdown())
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
