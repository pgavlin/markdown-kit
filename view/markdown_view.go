package view

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/goldmark/extension"
	goldmark_parser "github.com/pgavlin/goldmark/parser"
	goldmark_renderer "github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/indexer"
	"github.com/pgavlin/markdown-kit/renderer"
)

type line struct {
	start      int    // byte offset of start in rendered output
	end        int    // byte offset of end in rendered output
	content    string // raw content including ANSI codes
	ansiPrefix string // ANSI SGR state to prepend for standalone rendering
}

type lineWriter struct {
	byteOffset  int
	buf         bytes.Buffer
	lines       []line
	longestLine int
	ansiState   string // running ANSI SGR state
}

func (w *lineWriter) flushLine() {
	content := w.buf.String()
	start := w.byteOffset
	w.byteOffset += len(content)

	width := ansi.StringWidth(expandTabs(content, 8))
	if width > w.longestLine {
		w.longestLine = width
	}

	w.lines = append(w.lines, line{
		start:      start,
		end:        w.byteOffset,
		content:    content,
		ansiPrefix: w.ansiState,
	})

	w.ansiState = updateANSIState(w.ansiState, content)
	w.buf.Reset()
}

// sgrState tracks the effective SGR (Select Graphic Rendition) attributes.
// Instead of accumulating raw ANSI sequences (which grows without bound
// inside syntax-highlighted code blocks), this tracks just the last value
// for each attribute category and emits a minimal prefix.
type sgrState struct {
	fg        string // last foreground sequence (e.g. "\033[38;2;R;G;Bm"), empty if unset
	bg        string // last background sequence
	bold      string // "\033[1m" or "\033[22m" or ""
	italic    string // "\033[3m" or "\033[23m" or ""
	underline string // "\033[4m" or "\033[24m" or ""
}

func (s *sgrState) reset() {
	s.fg = ""
	s.bg = ""
	s.bold = ""
	s.italic = ""
	s.underline = ""
}

func (s sgrState) String() string {
	var b strings.Builder
	if s.bg != "" {
		b.WriteString(s.bg)
	}
	if s.fg != "" {
		b.WriteString(s.fg)
	}
	if s.bold != "" {
		b.WriteString(s.bold)
	}
	if s.italic != "" {
		b.WriteString(s.italic)
	}
	if s.underline != "" {
		b.WriteString(s.underline)
	}
	return b.String()
}

// classifySGR classifies a CSI SGR sequence and applies it to the state.
// The seq is the full sequence including ESC[ and m.
func (s *sgrState) applySGR(seq string) {
	// Extract the parameter string between "\033[" and "m".
	params := seq[2 : len(seq)-1]
	if params == "" || params == "0" {
		s.reset()
		return
	}

	// Parse semicolon-separated parameters.
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		switch p {
		case "1":
			s.bold = seq
		case "22":
			s.bold = seq
		case "3":
			s.italic = seq
		case "23":
			s.italic = seq
		case "4":
			s.underline = seq
		case "24":
			s.underline = seq
		case "38":
			// Foreground color: consume remaining params as part of this sequence.
			s.fg = seq
			return
		case "39":
			s.fg = ""
		case "48":
			// Background color: consume remaining params as part of this sequence.
			s.bg = seq
			return
		case "49":
			s.bg = ""
		default:
			// Basic foreground colors (30-37, 90-97).
			if n := atoiSimple(p); (n >= 30 && n <= 37) || (n >= 90 && n <= 97) {
				s.fg = seq
				return
			}
			// Basic background colors (40-47, 100-107).
			if n := atoiSimple(p); (n >= 40 && n <= 47) || (n >= 100 && n <= 107) {
				s.bg = seq
				return
			}
		}
	}
}

// atoiSimple parses a simple non-negative integer from a string. Returns -1 on failure.
func atoiSimple(s string) int {
	if len(s) == 0 {
		return -1
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// parseSGRState parses an ANSI state string (as produced by sgrState.String)
// back into an sgrState.
func parseSGRState(state string) sgrState {
	var s sgrState
	for i := 0; i < len(state); {
		if state[i] != '\033' {
			i++
			continue
		}
		if i+1 >= len(state) || state[i+1] != '[' {
			i++
			continue
		}
		j := i + 2
		for j < len(state) && ((state[j] >= '0' && state[j] <= '9') || state[j] == ';') {
			j++
		}
		if j < len(state) && state[j] == 'm' {
			seq := state[i : j+1]
			s.applySGR(seq)
			i = j + 1
			continue
		}
		i++
	}
	return s
}

// updateANSIState scans s for ANSI CSI SGR sequences and computes the
// effective SGR state at the end of the string. The state parameter is
// the state string from the previous line. Returns a minimal state string
// that, when emitted, restores the effective SGR attributes.
func updateANSIState(state, s string) string {
	st := parseSGRState(state)

	for i := 0; i < len(s); {
		if s[i] != '\033' {
			i++
			continue
		}
		if i+1 >= len(s) || s[i+1] != '[' {
			i++
			continue
		}
		// Scan parameter bytes (0x30-0x3f: digits and semicolons).
		j := i + 2
		for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';') {
			j++
		}
		// Check for SGR terminator 'm'.
		if j < len(s) && s[j] == 'm' {
			seq := s[i : j+1]
			st.applySGR(seq)
			i = j + 1
			continue
		}
		i++
	}
	return st.String()
}

func (w *lineWriter) Write(b []byte) (int, error) {
	n := len(b)
	for {
		newline := bytes.IndexByte(b, '\n')
		if newline == -1 {
			w.buf.Write(b)
			return n, nil
		}

		w.buf.Write(b[:newline])
		w.flushLine()
		// Account for the newline byte in byte offset
		w.byteOffset++
		b = b[newline+1:]
	}
}

func isLink(n ast.Node) (bool, bool) {
	switch n.Kind() {
	case ast.KindAutoLink, ast.KindLink:
		return true, true
	default:
		return false, false
	}
}

func isCodeBlock(n ast.Node) (bool, bool) {
	switch n.Kind() {
	case ast.KindCodeBlock, ast.KindFencedCodeBlock:
		return true, true
	default:
		return false, false
	}
}

func isHeading(n ast.Node) (bool, bool) {
	return n.Kind() == ast.KindHeading, n.Kind() == ast.KindHeading
}

// firstHeadingText returns the plain text of the first heading in the document,
// or "" if no heading is found.
func firstHeadingText(doc ast.Node, source []byte) string {
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if n.Kind() == ast.KindHeading {
			if s := string(n.Text(source)); s != "" {
				return s
			}
		}
	}
	return ""
}

// isHeadingOrAnchor matches headings and HTML anchor nodes for navigation.
func (m *Model) isHeadingOrAnchor(n ast.Node) (bool, bool) {
	if n.Kind() == ast.KindHeading {
		return true, true
	}
	if m.anchorNodes[n] {
		return false, true // selectable but not highlighted
	}
	return false, false
}

// Selector is a function that determines whether a node should be selected.
type Selector func(n ast.Node) (highlight, ok bool)

// Model is a bubbletea model that displays rendered Markdown content.
type Model struct {
	// KeyMap defines the key bindings for this model. Customize individual
	// bindings or call KeyMap.SetEnabled(false) to disable all input.
	KeyMap KeyMap

	// The colorscheme to use, if any.
	theme *chroma.Style

	// The name of the document.
	name string

	// The raw Markdown.
	markdown []byte

	// The parsed Markdown.
	document ast.Node

	// Node span tree.
	spanTree *renderer.NodeSpan

	// The document index.
	index *indexer.DocumentIndex

	// Set of AST nodes that are HTML anchor targets (<a id="...">).
	// Used by heading navigation to include anchor nodes.
	anchorNodes map[ast.Node]bool

	// The selection, if any.
	selection *renderer.NodeSpan

	// Navigation backstack for internal link following.
	backstack []*renderer.NodeSpan

	// The selected span byte offsets (trimmed of whitespace).
	selectionStart, selectionEnd int

	// True if the selected span should be highlighted.
	highlightSelection bool

	// The processed lines.
	lines []line

	// The last width for which the content was rendered.
	lastWidth int

	// The screen width of the longest line.
	longestLine int

	// The index of the first line shown.
	lineOffset int

	// The number of characters to be skipped on each line (horizontal scroll).
	columnOffset int

	// The viewport dimensions.
	width, height int

	// The height available for text (height minus gutter).
	pageSize int

	// If true, lines longer than available width are wrapped.
	wrap bool

	// If true, render a gutter with document name and position.
	showGutter bool

	// Transient status message shown in gutter instead of name/breadcrumbs.
	statusMessage string

	// The desired content width. 0 means use full viewport width.
	contentWidth int

	// Search state.
	search searchState

	// Document transformers to apply after parsing.
	documentTransformers []DocumentTransformer
}

// effectiveWidth returns the width to use for rendering content.
func (m *Model) effectiveWidth() int {
	if m.contentWidth > 0 && m.contentWidth < m.width {
		return m.contentWidth
	}
	return m.width
}

// NewModel creates a new Model with the given options.
func NewModel(opts ...Option) Model {
	m := Model{
		KeyMap: DefaultKeyMap(),
		wrap:   true,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// Clear removes all text from the buffer and resets all document-dependent
// state including selections, the navigation backstack, and the span tree.
func (m *Model) Clear() {
	m.lines = nil
	m.markdown = nil
	m.document = nil
	m.spanTree = nil
	m.index = nil
	m.anchorNodes = nil
	m.selection = nil
	m.backstack = nil
	m.selectionStart = 0
	m.selectionEnd = 0
	m.highlightSelection = false
	m.lineOffset = 0
	m.columnOffset = 0
	m.search = searchState{}
}

// GetName returns the document name.
func (m *Model) GetName() string {
	return m.name
}

// GetMarkdown returns the raw markdown bytes.
func (m *Model) GetMarkdown() []byte {
	return m.markdown
}

// SetText sets the text of this view. Previously contained text will be removed.
// If name is empty, the name is inferred from the first heading in the document.
func (m *Model) SetText(name, markdown string) {
	m.Clear()
	m.markdown = []byte(markdown)
	parser := goldmark.DefaultParser()
	parser.AddOptions(goldmark_parser.WithParagraphTransformers(
		util.Prioritized(extension.NewTableParagraphTransformer(), 200),
	))
	m.document = parser.Parse(text.NewReader(m.markdown))
	for _, t := range m.documentTransformers {
		t(m.document, m.markdown)
	}
	if name == "" {
		name = firstHeadingText(m.document, m.markdown)
	}
	m.name = name
	if doc, ok := m.document.(*ast.Document); ok {
		m.index = indexer.Index(doc, m.markdown)
		m.anchorNodes = m.index.AnchorNodes()
	}
	m.ensureRendered()
}

// SetWrap sets whether long lines should be wrapped.
func (m *Model) SetWrap(wrap bool) {
	if m.wrap != wrap {
		m.lines = nil
		m.search.stale = true
	}
	m.wrap = wrap
}

// SetContentWidth sets the desired content width. 0 means use full viewport width.
func (m *Model) SetContentWidth(width int) {
	m.contentWidth = width
	m.lines = nil
	m.search.stale = true
}

// SetGutter sets whether to show the gutter with document name and position.
func (m *Model) SetGutter(showGutter bool) {
	m.showGutter = showGutter
}

// SetStatusMessage sets a transient status message to display in the gutter.
func (m *Model) SetStatusMessage(msg string) {
	m.statusMessage = msg
}

// ClearStatusMessage clears the transient gutter status message.
func (m *Model) ClearStatusMessage() {
	m.statusMessage = ""
}

// SetSize sets the viewport dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	// Re-render if width changed and wrapping
	if m.wrap {
		m.lines = nil
		m.search.stale = true
	}
	m.ensureRendered()
}

// render renders the markdown into lines for display.
func (m *Model) render(width int) {
	if m.lines != nil {
		return
	}

	if m.document == nil {
		m.lines = []line{}
		return
	}

	wrap := 0
	if m.wrap {
		wrap = width
	}

	r := renderer.New(
		renderer.WithTheme(m.theme),
		renderer.WithHyperlinks(true),
		renderer.WithWordWrap(wrap),
		renderer.WithSoftBreak(wrap != 0))

	w := lineWriter{}
	gmRenderer := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	if err := gmRenderer.Render(&w, m.markdown, m.document); err != nil {
		m.lines = []line{{content: fmt.Sprintf("error rendering Markdown: %v", err)}}
		return
	}
	if w.buf.Len() > 0 {
		w.flushLine()
	}

	m.spanTree, m.lines, m.longestLine = r.SpanTree(), w.lines, w.longestLine
	if m.lines == nil {
		m.lines = []line{}
	}

	// Re-execute search after re-render if stale.
	if m.search.stale && m.search.query != "" {
		m.executeSearch()
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.ensureRendered()
		cmd := m.handleKey(msg)
		m.ensureRendered()
		m.clampOffsets()
		return m, cmd
	}
	m.clampOffsets()
	return m, nil
}

// clampOffsets keeps lineOffset and columnOffset within valid bounds.
func (m *Model) clampOffsets() {
	if m.lines == nil {
		return
	}
	if m.lineOffset+m.pageSize > len(m.lines) {
		m.lineOffset = len(m.lines) - m.pageSize
	}
	if m.lineOffset < 0 {
		m.lineOffset = 0
	}
	ew := m.effectiveWidth()
	if m.columnOffset+ew > m.longestLine {
		m.columnOffset = m.longestLine - ew
	}
	if m.columnOffset < 0 {
		m.columnOffset = 0
	}
}

// ScrollDown scrolls down by n lines.
func (m *Model) ScrollDown(n int) {
	m.lineOffset += n
	m.clampOffsets()
}

// ScrollUp scrolls up by n lines.
func (m *Model) ScrollUp(n int) {
	m.lineOffset -= n
	m.clampOffsets()
}

// ScrollLeft scrolls left by n columns.
func (m *Model) ScrollLeft(n int) {
	m.columnOffset -= n
	m.clampOffsets()
}

// ScrollRight scrolls right by n columns.
func (m *Model) ScrollRight(n int) {
	m.columnOffset += n
	m.clampOffsets()
}

// PageDown scrolls down by one page.
func (m *Model) PageDown() {
	m.lineOffset += m.pageSize
	m.clampOffsets()
}

// PageUp scrolls up by one page.
func (m *Model) PageUp() {
	m.lineOffset -= m.pageSize
	m.clampOffsets()
}

// GotoTop scrolls to the top of the document and resets horizontal scroll.
func (m *Model) GotoTop() {
	m.lineOffset = 0
	m.columnOffset = 0
}

// GotoBottom scrolls to the bottom of the document and resets horizontal scroll.
func (m *Model) GotoBottom() {
	m.columnOffset = 0
	if len(m.lines) > m.pageSize {
		m.lineOffset = len(m.lines) - m.pageSize
	}
}

// Width returns the current viewport width.
func (m *Model) Width() int {
	return m.width
}

// Height returns the current viewport height.
func (m *Model) Height() int {
	return m.height
}

// ContentWidth returns the current content width setting. 0 means full viewport width.
func (m *Model) ContentWidth() int {
	return m.contentWidth
}

// EffectiveWidth returns the computed effective width used for rendering.
func (m *Model) EffectiveWidth() int {
	return m.effectiveWidth()
}

// DecreaseContentWidth reduces the content width by 10 columns (minimum 40).
func (m *Model) DecreaseContentWidth() {
	if m.contentWidth == 0 {
		m.contentWidth = m.width - 10
	} else {
		m.contentWidth -= 10
	}
	if m.contentWidth < 40 {
		m.contentWidth = 40
	}
	m.lines = nil
	m.search.stale = true
}

// IncreaseContentWidth increases the content width by 10 columns.
// If the result would be within 10 of the viewport width, resets to 0 (full width).
func (m *Model) IncreaseContentWidth() {
	if m.contentWidth > 0 {
		m.contentWidth += 10
		if m.contentWidth+10 >= m.width {
			m.contentWidth = 0
		}
	}
	m.lines = nil
	m.search.stale = true
}

// AtTop reports whether the viewport is scrolled to the top.
func (m *Model) AtTop() bool {
	return m.lineOffset <= 0
}

// AtBottom reports whether the viewport is scrolled to the bottom.
func (m *Model) AtBottom() bool {
	if m.lines == nil {
		return true
	}
	return m.lineOffset+m.pageSize >= len(m.lines)
}

// ScrollPercent returns the scroll position as a value between 0.0 and 1.0.
func (m *Model) ScrollPercent() float64 {
	if m.lines == nil || len(m.lines) <= m.pageSize {
		return 1.0
	}
	maxOffset := len(m.lines) - m.pageSize
	if maxOffset <= 0 {
		return 1.0
	}
	pct := float64(m.lineOffset) / float64(maxOffset)
	if pct < 0 {
		return 0
	}
	if pct > 1 {
		return 1
	}
	return pct
}

// LineOffset returns the current vertical scroll offset (first visible line index).
func (m *Model) LineOffset() int {
	return m.lineOffset
}

// SetLineOffset sets the vertical scroll offset.
func (m *Model) SetLineOffset(n int) {
	m.lineOffset = n
	m.clampOffsets()
}

// ColumnOffset returns the current horizontal scroll offset.
func (m *Model) ColumnOffset() int {
	return m.columnOffset
}

// SetColumnOffset sets the horizontal scroll offset.
func (m *Model) SetColumnOffset(n int) {
	m.columnOffset = n
	m.clampOffsets()
}

// TotalLineCount returns the total number of rendered lines.
func (m *Model) TotalLineCount() int {
	return len(m.lines)
}

// VisibleLineCount returns the number of lines visible in the viewport.
func (m *Model) VisibleLineCount() int {
	if m.lines == nil {
		return 0
	}
	visible := m.pageSize
	if visible > len(m.lines) {
		visible = len(m.lines)
	}
	return visible
}

// SetHeight sets the height of the viewport.
func (m *Model) SetHeight(height int) {
	m.height = height
	m.ensureRendered()
}

// SetWidth sets the width of the viewport.
func (m *Model) SetWidth(width int) {
	m.width = width
	// Re-render if width changed and wrapping
	if m.wrap {
		m.lines = nil
		m.search.stale = true
	}
	m.ensureRendered()
}

// ensureRendered triggers rendering if not already done.
func (m *Model) ensureRendered() {
	if m.width == 0 {
		return
	}

	textHeight := m.height
	if m.showGutter {
		textHeight--
	}
	m.pageSize = textHeight

	ew := m.effectiveWidth()
	if ew != m.lastWidth && m.wrap {
		m.lines = nil
		m.search.stale = true
	}
	m.lastWidth = ew

	m.render(ew)
}

// OpenLinkMsg is sent when the user activates a link that is not an
// internal document anchor. Embedders should handle this message to
// open the link in a browser or otherwise.
type OpenLinkMsg struct {
	URL string
}

// GoBackMsg is sent when the user presses the back key and the internal
// backstack is empty. Embedders should handle this message to navigate
// to the previous page.
type GoBackMsg struct{}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.search.active {
		return m.handleSearchKey(msg)
	}

	// Handle search-related keys when search is confirmed.
	if m.search.confirmed {
		switch {
		case key.Matches(msg, m.KeyMap.NextMatch):
			m.nextMatch()
			return nil
		case key.Matches(msg, m.KeyMap.PrevMatch):
			m.prevMatch()
			return nil
		case key.Matches(msg, m.KeyMap.ClearSearch):
			m.search = searchState{}
			return nil
		}
	}

	switch {
	case key.Matches(msg, m.KeyMap.Search):
		m.search = searchState{active: true}
		return nil

	case key.Matches(msg, m.KeyMap.GotoTop):
		m.GotoTop()
	case key.Matches(msg, m.KeyMap.GotoEnd):
		m.GotoBottom()
	case key.Matches(msg, m.KeyMap.Down):
		m.ScrollDown(1)
	case key.Matches(msg, m.KeyMap.Up):
		m.ScrollUp(1)
	case key.Matches(msg, m.KeyMap.Left):
		m.ScrollLeft(1)
	case key.Matches(msg, m.KeyMap.Right):
		m.ScrollRight(1)
	case key.Matches(msg, m.KeyMap.PrevLink):
		if m.isSelectionVisible() || !m.SelectLastVisible(isLink) {
			m.SelectPrevious(isLink)
		}
	case key.Matches(msg, m.KeyMap.NextLink):
		if m.isSelectionVisible() || !m.SelectFirstVisible(isLink) {
			m.SelectNext(isLink)
		}
	case key.Matches(msg, m.KeyMap.PrevCodeBlock):
		if m.isSelectionVisible() || !m.SelectLastVisible(isCodeBlock) {
			m.SelectPrevious(isCodeBlock)
		}
	case key.Matches(msg, m.KeyMap.NextCodeBlock):
		if m.isSelectionVisible() || !m.SelectFirstVisible(isCodeBlock) {
			m.SelectNext(isCodeBlock)
		}
	case key.Matches(msg, m.KeyMap.PrevHeading):
		m.SelectPrevious(m.isHeadingOrAnchor)
	case key.Matches(msg, m.KeyMap.NextHeading):
		m.SelectNext(m.isHeadingOrAnchor)
	case key.Matches(msg, m.KeyMap.Home):
		m.GotoTop()
	case key.Matches(msg, m.KeyMap.End):
		m.columnOffset = 0
	case key.Matches(msg, m.KeyMap.PageDown):
		m.PageDown()
	case key.Matches(msg, m.KeyMap.PageUp):
		m.PageUp()
	case key.Matches(msg, m.KeyMap.DecreaseWidth):
		m.DecreaseContentWidth()
	case key.Matches(msg, m.KeyMap.IncreaseWidth):
		m.IncreaseContentWidth()
	case key.Matches(msg, m.KeyMap.FollowLink):
		if !m.FollowLink() {
			if url := m.FocusedLinkDestination(); url != "" {
				return func() tea.Msg { return OpenLinkMsg{URL: url} }
			}
		}
	case key.Matches(msg, m.KeyMap.GoBack):
		if !m.GoBack() {
			return func() tea.Msg { return GoBackMsg{} }
		}
	case key.Matches(msg, m.KeyMap.CopySelection):
		if content := m.focusedContent(); content != "" {
			return tea.SetClipboard(content)
		}
	}
	return nil
}

// View implements tea.Model.
func (m Model) View() string {
	width := m.width
	height := m.height
	if width == 0 || height == 0 {
		return ""
	}

	if m.lines == nil {
		return ""
	}

	textHeight := height
	if m.showGutter {
		textHeight = height - 1
	}

	// Clamp line offset.
	lineOffset := m.lineOffset
	if lineOffset+textHeight > len(m.lines) {
		lineOffset = len(m.lines) - textHeight
	}
	if lineOffset < 0 {
		lineOffset = 0
	}

	// Effective content width and centering margin.
	ew := m.effectiveWidth()
	margin := 0
	if ew < width {
		margin = (width - ew) / 2
	}

	// Clamp column offset.
	columnOffset := m.columnOffset
	if columnOffset+ew > m.longestLine {
		columnOffset = m.longestLine - ew
	}
	if columnOffset < 0 {
		columnOffset = 0
	}

	// Build visible lines.
	lastLine := lineOffset + textHeight
	if lastLine > len(m.lines) {
		lastLine = len(m.lines)
	}

	var buf strings.Builder

	leftPad := strings.Repeat(" ", margin)
	rightMargin := width - margin - ew
	if rightMargin < 0 {
		rightMargin = 0
	}
	rightMarginPad := strings.Repeat(" ", rightMargin)

	// Compute theme background SGR sequence for the content area.
	// Content lines carry their own background via ansiPrefix, but
	// short lines need padding within the effective width to fill
	// the content column with the theme background.
	var bgSeq string
	if m.theme != nil {
		if bg := m.theme.Get(chroma.Background).Background; bg.IsSet() {
			bgSeq = fmt.Sprintf("\033[48;2;%d;%d;%dm", bg.Red(), bg.Green(), bg.Blue())
		}
	}

	for i, ln := range m.lines[lineOffset:lastLine] {
		if i > 0 {
			buf.WriteByte('\n')
		}

		content := expandTabs(ln.content, 8)

		// Apply selection highlighting if needed.
		if m.selection != nil && m.highlightSelection {
			content = m.applySelection(ln, content)
		}

		// Apply search match highlighting.
		if len(m.search.matches) > 0 {
			content = m.applySearchHighlights(lineOffset+i, content)
		}

		// Handle horizontal scrolling and width truncation.
		if columnOffset > 0 {
			content = ansiCut(content, columnOffset, columnOffset+ew)
		} else {
			content = ansiTruncate(content, ew)
		}

		lineWidth := ansi.StringWidth(content)

		// Left margin for centering (unstyled).
		buf.WriteString(leftPad)

		// Theme background for the content column.
		buf.WriteString(bgSeq)

		// Restore the ANSI SGR state expected at the start of this line,
		// then write content. Each line is self-contained: the reset at
		// the end clears content styling, and emitting ansiPrefix here
		// restores state (e.g. theme colors) even when scrolled.
		buf.WriteString(ln.ansiPrefix)
		buf.WriteString(content)

		// Reset content styling, then restore theme background to pad
		// the content area to the effective width.
		buf.WriteString("\033[0m")
		buf.WriteString(bgSeq)
		contentPad := ew - lineWidth
		if contentPad > 0 {
			buf.WriteString(strings.Repeat(" ", contentPad))
		}

		// Clear background and add right margin (unstyled).
		if bgSeq != "" {
			buf.WriteString("\033[0m")
		}
		buf.WriteString(rightMarginPad)
	}

	// Pad remaining empty lines.
	for i := lastLine - lineOffset; i < textHeight; i++ {
		if i > 0 || lastLine > lineOffset {
			buf.WriteByte('\n')
		}
		buf.WriteString(leftPad)
		buf.WriteString(bgSeq)
		buf.WriteString(strings.Repeat(" ", ew))
		if bgSeq != "" {
			buf.WriteString("\033[0m")
		}
		buf.WriteString(rightMarginPad)
	}

	// Draw gutter.
	if m.showGutter && m.theme != nil {
		buf.WriteByte('\n')
		if m.search.active {
			buf.WriteString(m.renderSearchGutter(width))
		} else {
			buf.WriteString(m.renderGutter(width, lineOffset, lastLine))
		}
	}

	return buf.String()
}

// applySelection applies reverse video to the selected portion of a line.
func (m *Model) applySelection(ln line, content string) string {
	// Check if this line overlaps with the selection.
	if ln.end <= m.selectionStart || ln.start >= m.selectionEnd {
		return content
	}

	// Calculate the character offsets within this line that correspond to the selection.
	// We need to map byte offsets to visible character positions.
	selStart := 0
	selEnd := ansi.StringWidth(content)

	if m.selectionStart > ln.start {
		byteOffset := m.selectionStart - ln.start
		prefix := content
		if byteOffset < len(content) {
			prefix = content[:byteOffset]
		}
		selStart = ansi.StringWidth(prefix)
	}

	if m.selectionEnd < ln.end {
		byteOffset := m.selectionEnd - ln.start
		prefix := content
		if byteOffset < len(content) {
			prefix = content[:byteOffset]
		}
		selEnd = ansi.StringWidth(prefix)
	}

	if selStart >= selEnd {
		return content
	}

	lineWidth := ansi.StringWidth(content)

	// Split into before, selected, and after portions using ANSI-aware operations.
	before := ansiTruncate(content, selStart)
	middle := ansiCut(content, selStart, selEnd)
	after := ansiCut(content, selEnd, lineWidth)

	// Use raw ANSI reverse video codes instead of lipgloss, which requires
	// TTY detection and may not emit codes in all environments.
	return before + "\033[7m" + middle + "\033[27m" + after
}

// headingBreadcrumbs returns the heading hierarchy at the given line offset.
func (m *Model) headingBreadcrumbs(lineOffset int) []string {
	if m.spanTree == nil || len(m.lines) == 0 || lineOffset >= len(m.lines) {
		return nil
	}

	topOffset := m.lines[lineOffset].start

	type heading struct {
		level int
		text  string
	}
	var stack []heading

	for s := m.spanTree; s != nil; s = s.Next {
		if s.Start > topOffset {
			break
		}
		if h, ok := s.Node.(*ast.Heading); ok {
			// Pop headings of same or deeper level.
			for len(stack) > 0 && stack[len(stack)-1].level >= h.Level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, heading{level: h.Level, text: string(h.Text(m.markdown))})
		}
	}

	parts := make([]string, len(stack))
	for i, h := range stack {
		parts[i] = h.text
	}
	return parts
}

// renderGutter renders the bottom gutter line.
func (m *Model) renderGutter(width, lineOffset, lastLine int) string {
	if width < 6 { // minimum for " 100% "
		return strings.Repeat(" ", width)
	}

	entry := m.theme.Get(chroma.Comment)
	gutterStyle := lipgloss.NewStyle()
	if entry.Colour.IsSet() {
		gutterStyle = gutterStyle.Foreground(lipgloss.Color(
			fmt.Sprintf("#%02x%02x%02x", entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue())))
	}

	textEntry := m.theme.Get(chroma.Text)
	pctStyle := lipgloss.NewStyle()
	if textEntry.Colour.IsSet() {
		pctStyle = pctStyle.Foreground(lipgloss.Color(
			fmt.Sprintf("#%02x%02x%02x", textEntry.Colour.Red(), textEntry.Colour.Green(), textEntry.Colour.Blue())))
	}

	pct := fmt.Sprintf(" %3d%% ", lastLine*100/max(len(m.lines), 1))
	pctWidth := len(pct)

	nameWidth := width - pctWidth
	var name string
	var nameVisWidth int

	if m.statusMessage != "" {
		// Show transient status message instead of normal gutter content.
		name = m.statusMessage
		nameVisWidth = ansi.StringWidth(name)
		if nameVisWidth > nameWidth {
			if nameWidth > 3 {
				name = ansiTruncate(name, nameWidth-3) + "..."
			} else {
				name = ""
			}
			nameVisWidth = ansi.StringWidth(name)
		}
	} else {
		name = m.name
		nameVisWidth = ansi.StringWidth(name)

		if nameVisWidth > nameWidth {
			if nameWidth > 3 {
				name = ansiTruncate(name, nameWidth-3) + "..."
			} else {
				name = ""
			}
			nameVisWidth = ansi.StringWidth(name)
		}

		// Append heading breadcrumbs.
		crumbs := m.headingBreadcrumbs(lineOffset)
		bcAvail := nameWidth - nameVisWidth - 3
		if bcAvail > 0 && len(crumbs) > 0 {
			totalCrumbs := len(crumbs)
			for len(crumbs) > 0 && ansi.StringWidth(strings.Join(crumbs, " > ")) > bcAvail {
				crumbs = crumbs[1:]
			}
			// If crumbs were dropped but the remainder still doesn't fit with
			// an ellipsis prefix, keep dropping until it does.
			if len(crumbs) > 0 && len(crumbs) < totalCrumbs {
				for len(crumbs) > 0 && ansi.StringWidth("... > "+strings.Join(crumbs, " > ")) > bcAvail {
					crumbs = crumbs[1:]
				}
			}
			if len(crumbs) > 0 {
				breadcrumb := strings.Join(crumbs, " > ")
				if len(crumbs) < totalCrumbs {
					breadcrumb = "... > " + breadcrumb
				}
				bcWidth := ansi.StringWidth(breadcrumb)
				name = name + " | " + breadcrumb
				nameVisWidth += 3 + bcWidth
			}
		}

		// Append link target when a link is selected.
		if m.selection != nil {
			var linkTarget string
			switch node := m.selection.Node.(type) {
			case *ast.Link:
				linkTarget = string(node.Destination)
			case *ast.AutoLink:
				linkTarget = string(node.URL(m.markdown))
			}
			if linkTarget != "" {
				ltAvail := nameWidth - nameVisWidth - 3
				if ltAvail > 0 {
					ltWidth := ansi.StringWidth(linkTarget)
					if ltWidth > ltAvail {
						if ltAvail > 3 {
							linkTarget = ansiTruncate(linkTarget, ltAvail-3) + "..."
						} else {
							linkTarget = ""
						}
					}
					if linkTarget != "" {
						name = name + " | " + linkTarget
						nameVisWidth += 3 + ansi.StringWidth(linkTarget)
					}
				}
			}
		}

		// Append search match info when search is confirmed with results.
		if searchInfo := m.searchGutterInfo(); searchInfo != "" {
			siAvail := nameWidth - nameVisWidth - 1
			siWidth := ansi.StringWidth(searchInfo)
			if siWidth <= siAvail {
				name = name + " " + searchInfo
				nameVisWidth += 1 + siWidth
			}
		}
	}

	padding := nameWidth - nameVisWidth
	if padding < 0 {
		padding = 0
	}

	return gutterStyle.Render(name+strings.Repeat(" ", padding)) + pctStyle.Render(pct)
}

// scrollToOffset scrolls to make the given byte offset visible.
func (m *Model) scrollToOffset(offset int) {
	li := m.findLineForOffset(offset)
	if li < len(m.lines) {
		m.lineOffset = li
	}
}

// findLineForOffset returns the line index containing the given byte offset.
func (m *Model) findLineForOffset(offset int) int {
	return sort.Search(len(m.lines), func(i int) bool {
		return m.lines[i].end > offset
	})
}

// isOffsetWhitespace checks if the character at the given byte offset is whitespace.
func (m *Model) isOffsetWhitespace(offset int) bool {
	li := m.findLineForOffset(offset)
	if li >= len(m.lines) {
		return true
	}
	ln := m.lines[li]
	if offset < ln.start || offset >= ln.end {
		return true
	}
	byteIdx := offset - ln.start
	if byteIdx >= len(ln.content) {
		return true
	}
	// Check the byte directly in the content (which includes ANSI codes).
	// ANSI escape bytes are not whitespace, so they won't cause false positives.
	b := ln.content[byteIdx]
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// calculateSelectionSpan trims whitespace from selection boundaries.
func (m *Model) calculateSelectionSpan(selection *renderer.NodeSpan) {
	start, end := selection.Start, selection.End

	for start < end {
		if !m.isOffsetWhitespace(start) {
			break
		}
		start++
	}

	for end > start {
		if !m.isOffsetWhitespace(end - 1) {
			break
		}
		end--
	}

	m.selectionStart, m.selectionEnd = start, end
}

func (m *Model) selected(offset int) bool {
	return m.selection != nil && m.selectionStart <= offset && offset < m.selectionEnd
}

// Selection returns the current selection span.
func (m *Model) Selection() *renderer.NodeSpan {
	return m.selection
}

// isSelectionVisible reports whether the current selection is within the viewport.
func (m *Model) isSelectionVisible() bool {
	if m.selection == nil || len(m.lines) == 0 {
		return false
	}
	li := m.findLineForOffset(m.selection.Start)
	return li >= m.lineOffset && li < m.lineOffset+m.pageSize
}

// SelectFirstVisible selects the first node within the viewport that matches the given selector.
func (m *Model) SelectFirstVisible(selector Selector) bool {
	if m.spanTree == nil || len(m.lines) == 0 || m.lineOffset >= len(m.lines) {
		return false
	}
	vpStart := m.lines[m.lineOffset].start
	endLine := m.lineOffset + m.pageSize
	if endLine > len(m.lines) {
		endLine = len(m.lines)
	}
	vpEnd := m.lines[endLine-1].end

	for s := m.spanTree; s != nil; s = s.Next {
		if s.Start >= vpEnd {
			break
		}
		if s.Start >= vpStart {
			if highlight, ok := selector(s.Node); ok {
				m.SelectSpan(s, highlight)
				return true
			}
		}
	}
	return false
}

// SelectLastVisible selects the last node within the viewport that matches the given selector.
func (m *Model) SelectLastVisible(selector Selector) bool {
	if m.spanTree == nil || len(m.lines) == 0 || m.lineOffset >= len(m.lines) {
		return false
	}
	vpStart := m.lines[m.lineOffset].start
	endLine := m.lineOffset + m.pageSize
	if endLine > len(m.lines) {
		endLine = len(m.lines)
	}
	vpEnd := m.lines[endLine-1].end

	var last *renderer.NodeSpan
	var lastHighlight bool
	for s := m.spanTree; s != nil; s = s.Next {
		if s.Start >= vpEnd {
			break
		}
		if s.Start >= vpStart {
			if highlight, ok := selector(s.Node); ok {
				last = s
				lastHighlight = highlight
			}
		}
	}
	if last != nil {
		m.SelectSpan(last, lastHighlight)
		return true
	}
	return false
}

// SelectPrevious selects the first node before the current selection that matches the given selector.
func (m *Model) SelectPrevious(selector Selector) bool {
	cursor := m.selection
	if cursor == nil {
		cursor = m.spanTree
	}
	if cursor == nil {
		return false
	}
	cursor = cursor.Prev
	if cursor == nil {
		return false
	}

	for ; cursor != nil; cursor = cursor.Prev {
		if highlight, ok := selector(cursor.Node); ok {
			m.SelectSpan(cursor, highlight)
			return true
		}
	}

	return false
}

// SelectNext selects the first node after the current selection that matches the given selector.
func (m *Model) SelectNext(selector Selector) bool {
	cursor := m.selection
	if cursor == nil {
		cursor = m.spanTree
	}
	if cursor == nil {
		return false
	}
	cursor = cursor.Next
	if cursor == nil {
		return false
	}

	for ; cursor != nil; cursor = cursor.Next {
		if highlight, ok := selector(cursor.Node); ok {
			m.SelectSpan(cursor, highlight)
			return true
		}
	}

	return false
}

// SelectAnchor selects the next node with the given anchor.
// For anchors defined by HTML anchor tags (<a id="...">), this navigates to
// the anchor node itself. For heading-derived anchors, it navigates to the heading.
func (m *Model) SelectAnchor(anchor string) bool {
	if m.index == nil {
		return false
	}

	// Prefer navigating to the HTML anchor node if one exists.
	if nodes, ok := m.index.LookupNode(anchor); ok {
		selector := func(node ast.Node) (bool, bool) {
			for _, n := range nodes {
				if n == node {
					return false, true // selectable but not highlighted
				}
			}
			return false, false
		}
		if !m.SelectNext(selector) {
			m.selection = nil
			m.SelectNext(selector)
		}
		return true
	}

	// Fall back to section-based navigation for heading anchors.
	sections, ok := m.index.Lookup(anchor)
	if !ok {
		return false
	}
	selector := func(node ast.Node) (bool, bool) {
		for _, s := range sections {
			if s.Start == node {
				return true, true
			}
		}
		return false, false
	}

	if !m.SelectNext(selector) {
		m.selection = nil
		m.SelectNext(selector)
	}
	return true
}

// documentAnchor checks whether url is an internal anchor link (e.g. "#foo"
// or "docName.md#foo") and returns the anchor portion if so.
func (m *Model) documentAnchor(url string) (string, bool) {
	if strings.HasPrefix(url, "#") {
		return url[1:], true
	}
	if m.name != "" {
		if rest, ok := strings.CutPrefix(url, m.name); ok && strings.HasPrefix(rest, "#") {
			return rest[1:], true
		}
	}
	return "", false
}

// FocusedLinkDestination returns the URL of the currently selected link,
// or "" if the selection is not a link.
func (m *Model) FocusedLinkDestination() string {
	if m.selection == nil {
		return ""
	}
	switch node := m.selection.Node.(type) {
	case *ast.AutoLink:
		return string(node.URL(m.markdown))
	case *ast.Link:
		return string(node.Destination)
	}
	return ""
}

// FollowLink follows the currently selected internal anchor link.
// Returns true if navigation occurred (the link was an internal anchor).
func (m *Model) FollowLink() bool {
	link := m.FocusedLinkDestination()
	anchor, ok := m.documentAnchor(link)
	if !ok {
		return false
	}
	selection := m.selection
	if m.SelectAnchor(anchor) && selection != nil {
		m.backstack = append(m.backstack, selection)
	}
	return true
}

// GoBack returns to the previous selection from the backstack.
// Returns true if there was a previous selection to return to.
func (m *Model) GoBack() bool {
	if len(m.backstack) == 0 {
		return false
	}
	last := m.backstack[len(m.backstack)-1]
	m.backstack = m.backstack[:len(m.backstack)-1]
	m.SelectSpan(last, true)
	return true
}

// SelectSpan selects the given node span.
func (m *Model) SelectSpan(span *renderer.NodeSpan, highlight bool) {
	m.highlightSelection = highlight
	m.selection = span
	m.calculateSelectionSpan(span)
	m.ensureOffsetVisible(span.Start)
}

// ensureOffsetVisible scrolls the viewport only if the given byte offset is
// not already visible. When scrolling is needed, the target line is positioned
// so that 3 lines of content remain below it at the bottom of the viewport.
func (m *Model) ensureOffsetVisible(offset int) {
	li := m.findLineForOffset(offset)
	if li >= len(m.lines) {
		return
	}
	// Already visible — don't scroll.
	if li >= m.lineOffset && li < m.lineOffset+m.pageSize-4 {
		return
	}
	// Scroll so the target line has 3 lines of content below it.
	target := li - m.pageSize + 4
	if target < 0 {
		target = 0
	}
	m.lineOffset = target
}

// expandTabs replaces tab characters with spaces, advancing to the next
// tab stop (every tabWidth columns). ANSI escape sequences are passed
// through without affecting the column counter.
func expandTabs(s string, tabWidth int) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var result strings.Builder
	result.Grow(len(s) + 16) // pre-allocate a bit extra
	col := 0
	for i := 0; i < len(s); {
		if s[i] == '\033' {
			// Scan past the ANSI escape sequence.
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';') {
					j++
				}
				if j < len(s) && s[j] == 'm' {
					j++
				}
			}
			result.WriteString(s[i:j])
			i = j
		} else if s[i] == '\t' {
			spaces := tabWidth - (col % tabWidth)
			for k := 0; k < spaces; k++ {
				result.WriteByte(' ')
			}
			col += spaces
			i++
		} else {
			result.WriteByte(s[i])
			// Only advance column for non-continuation bytes (avoid double-counting UTF-8).
			if s[i]&0xC0 != 0x80 {
				col++
			}
			i++
		}
	}
	return result.String()
}

// ansiTruncate truncates a string to the given visible width, preserving ANSI codes.
func ansiTruncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "")
}

// ansiCut extracts a substring by visible character positions, preserving ANSI codes.
func ansiCut(s string, start, end int) string {
	if start >= end {
		return ""
	}
	// First truncate to end, then skip from start.
	truncated := ansi.Truncate(s, end, "")
	if start <= 0 {
		return truncated
	}
	return ansi.TruncateLeft(truncated, start, "")
}

// focusedContent returns the currently-selected content.
//
// - Links are reutrned as their URL
// - Headers and code blocks are returned as their text
func (m *Model) focusedContent() string {
	if m.selection == nil {
		return ""
	}
	switch node := m.selection.Node.(type) {
	case *ast.AutoLink:
		return string(node.URL(m.markdown))
	case *ast.Link:
		return string(node.Destination)
	case *ast.Heading:
		return string(node.Text(m.markdown))
	case *ast.CodeBlock, *ast.FencedCodeBlock:
		var sb strings.Builder
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			sb.WriteString(string(line.Value(m.markdown)))
		}
		return sb.String()
	}
	return ""
}
