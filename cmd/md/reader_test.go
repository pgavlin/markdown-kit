package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	mdk "github.com/pgavlin/markdown-kit/view"

	"github.com/pgavlin/markdown-kit/styles"
)

// testReader creates a markdownReader suitable for testing.
func testReader(name, markdown, source string) markdownReader {
	return newMarkdownReader(
		name, markdown, source,
		styles.GlamourDark,
		&fakeConverter{},
		nil,               // no registry
		nil,               // no cache
		&fakeHTTPClient{}, // unused default
		newMemFS(),
		nil, // no search index
		discardLogger(),
	)
}

// keyMsg constructs a tea.KeyPressMsg that produces the given String() value.
func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+t":
		return tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}
	case "shift+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	case "ctrl+o":
		return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	case "ctrl+l":
		return tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	case "ctrl+w":
		return tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	default:
		if len(s) == 1 {
			return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
		}
		return tea.KeyPressMsg{Code: -1, Text: s}
	}
}

// --- Existing pure-function tests ---

func TestFenceHTML(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		got := fenceHTML("<p>Hello</p>")
		if !strings.HasPrefix(got, "```html\n") {
			t.Errorf("expected to start with ```html\\n, got %q", got[:20])
		}
		if !strings.Contains(got, "<p>Hello</p>") {
			t.Error("expected HTML content to be preserved")
		}
		if !strings.HasSuffix(got, "\n```") {
			t.Error("expected to end with \\n```")
		}
	})

	t.Run("with_backticks", func(t *testing.T) {
		html := "<code>```</code>"
		got := fenceHTML(html)
		if strings.HasPrefix(got, "```html\n") {
			t.Error("expected longer fence for content with backticks")
		}
		if !strings.Contains(got, "html\n") {
			t.Error("expected html language marker")
		}
	})
}

func TestFenceSource(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		got := fenceSource("# Hello")
		if !strings.HasPrefix(got, "```markdown\n") {
			t.Errorf("expected to start with ```markdown\\n, got prefix %q", got[:20])
		}
		if !strings.Contains(got, "# Hello") {
			t.Error("expected markdown content to be preserved")
		}
	})

	t.Run("with_backticks", func(t *testing.T) {
		md := "```go\nfmt.Println()\n```"
		got := fenceSource(md)
		lines := strings.Split(got, "\n")
		fence := lines[0]
		if len(fence) <= 3 {
			t.Errorf("expected longer fence, got %q", fence)
		}
	})
}

func TestWordWrap(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		got := wordWrap("hello world foo bar", 10)
		lines := strings.Split(got, "\n")
		for _, line := range lines {
			if len(line) > 10 {
				t.Errorf("line %q exceeds width 10", line)
			}
		}
	})

	t.Run("zero_width", func(t *testing.T) {
		input := "hello world"
		got := wordWrap(input, 0)
		if got != input {
			t.Errorf("zero width should return input unchanged, got %q", got)
		}
	})

	t.Run("no_break_needed", func(t *testing.T) {
		got := wordWrap("short", 100)
		if got != "short" {
			t.Errorf("got %q, want %q", got, "short")
		}
	})

	t.Run("multi_paragraph", func(t *testing.T) {
		input := "first paragraph\n\nsecond paragraph"
		got := wordWrap(input, 100)
		if !strings.Contains(got, "\n\n") {
			t.Error("expected empty line between paragraphs to be preserved")
		}
	})
}

// --- ShortHelp / FullHelp ---

func TestShortHelp(t *testing.T) {
	km := defaultReaderKeyMap()
	bindings := km.ShortHelp()
	if len(bindings) == 0 {
		t.Fatal("expected non-empty short help bindings")
	}
}

func TestFullHelp(t *testing.T) {
	km := defaultReaderKeyMap()
	groups := km.FullHelp()
	if len(groups) != 5 {
		t.Fatalf("expected 5 help groups, got %d", len(groups))
	}
	// Columns range from 5-12 items each.
	for i, g := range groups {
		if len(g) < 5 || len(g) > 12 {
			t.Errorf("group %d has %d bindings, want 5-12", i, len(g))
		}
	}
}

// --- renderHelpPage ---

func TestRenderHelpPage(t *testing.T) {
	km := defaultReaderKeyMap()
	result := renderHelpPage(km)

	// Template should have been executed — no leftover {{.Foo}} placeholders.
	if strings.Contains(result, "{{.") {
		t.Error("rendered help page still contains unresolved template placeholders")
	}

	// Should contain actual default key bindings rendered by fmtKey.
	if !strings.Contains(result, "`j`") {
		t.Error("expected rendered help page to contain default Down key `j`")
	}
	if !strings.Contains(result, "`q`") {
		t.Error("expected rendered help page to contain default Quit key `q`")
	}
}

func TestRenderHelpPageCustomKeys(t *testing.T) {
	km := defaultReaderKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "quit"))

	result := renderHelpPage(km)

	if !strings.Contains(result, "`x`") {
		t.Error("expected rendered help page to contain custom Quit key `x`")
	}
}

// --- markdownReader.Update tests ---

func TestUpdate_Quit(t *testing.T) {
	r := testReader("test", "# Hello", "")
	m, cmd := r.Update(keyMsg("q"))
	_ = m
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestUpdate_QuitCtrlC(t *testing.T) {
	r := testReader("test", "# Hello", "")
	_, cmd := r.Update(keyMsg("ctrl+c"))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestUpdate_ShowHelp(t *testing.T) {
	r := testReader("test", "# Hello", "")
	m, _ := r.Update(keyMsg("?"))
	reader := m.(markdownReader)
	if !reader.showHelp {
		t.Error("expected showHelp=true")
	}
}

func TestUpdate_DismissHelp_Esc(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showHelp = true
	m, _ := r.Update(keyMsg("esc"))
	reader := m.(markdownReader)
	if reader.showHelp {
		t.Error("expected showHelp=false after esc")
	}
}

func TestUpdate_DismissHelp_Question(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showHelp = true
	m, _ := r.Update(keyMsg("?"))
	reader := m.(markdownReader)
	if reader.showHelp {
		t.Error("expected showHelp=false after ?")
	}
}

func TestUpdate_DismissHelp_Q(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showHelp = true
	m, _ := r.Update(keyMsg("q"))
	reader := m.(markdownReader)
	if reader.showHelp {
		t.Error("expected showHelp=false after q")
	}
}

func TestUpdate_HelpSwallowsOtherKeys(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showHelp = true
	m, _ := r.Update(keyMsg("j"))
	reader := m.(markdownReader)
	// Help should still be shown; key was swallowed.
	if !reader.showHelp {
		t.Error("expected showHelp to remain true for unrelated key")
	}
}

func TestUpdate_DismissError_Esc(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showError = true
	r.errorText = "some error"
	r.errorURL = "http://example.com"
	m, _ := r.Update(keyMsg("esc"))
	reader := m.(markdownReader)
	if reader.showError {
		t.Error("expected showError=false after esc")
	}
	if reader.errorText != "" {
		t.Error("expected errorText cleared")
	}
	if reader.errorURL != "" {
		t.Error("expected errorURL cleared")
	}
}

func TestUpdate_DismissError_Enter(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showError = true
	r.errorText = "err"
	m, _ := r.Update(keyMsg("enter"))
	reader := m.(markdownReader)
	if reader.showError {
		t.Error("expected showError=false after enter")
	}
}

func TestUpdate_DismissError_Q(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showError = true
	r.errorText = "err"
	m, _ := r.Update(keyMsg("q"))
	reader := m.(markdownReader)
	if reader.showError {
		t.Error("expected showError=false after q")
	}
}

func TestUpdate_ErrorSwallowsOtherKeys(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showError = true
	r.errorText = "err"
	m, _ := r.Update(keyMsg("j"))
	reader := m.(markdownReader)
	if !reader.showError {
		t.Error("expected showError to remain true for unrelated key")
	}
}

func TestUpdate_LoadingSwallowsKeys(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.loading = true
	m, _ := r.Update(keyMsg("q"))
	reader := m.(markdownReader)
	// Should not quit while loading — key is swallowed.
	if reader.loading != true {
		t.Error("expected loading to remain true")
	}
}

func TestUpdate_ToggleRaw(t *testing.T) {
	r := testReader("test", "# Hello", "")
	if r.active().showSource {
		t.Fatal("expected showSource=false initially")
	}

	// Toggle on.
	m, _ := r.Update(keyMsg("ctrl+u"))
	reader := m.(markdownReader)
	if !reader.active().showSource {
		t.Error("expected showSource=true after ctrl+u")
	}

	// Toggle off.
	m, _ = reader.Update(keyMsg("ctrl+u"))
	reader = m.(markdownReader)
	if reader.active().showSource {
		t.Error("expected showSource=false after second ctrl+r")
	}
}

func TestUpdate_PageLoadedMsg(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")
	msg := pageLoadedMsg{
		name:     "new page",
		markdown: "# New",
		source:   "http://example.com/new",
	}

	m, _ := r.Update(msg)
	reader := m.(markdownReader)

	if reader.active().currentSource != "http://example.com/new" {
		t.Errorf("currentSource = %q", reader.active().currentSource)
	}
	if reader.loading {
		t.Error("expected loading=false")
	}
	if reader.active().showSource {
		t.Error("expected showSource=false")
	}
	// Page stack should have the initial page.
	if len(reader.active().pageStack) != 1 {
		t.Errorf("pageStack length = %d, want 1", len(reader.active().pageStack))
	}
	if reader.active().pageStack[0].source != "/doc.md" {
		t.Errorf("pageStack[0].source = %q, want %q", reader.active().pageStack[0].source, "/doc.md")
	}
}

func TestUpdate_PageLoadedMsg_ClearsRawState(t *testing.T) {
	r := testReader("initial", "# Initial", "")
	r.active().showSource = true

	msg := pageLoadedMsg{name: "new", markdown: "# New", source: "new.md"}
	m, _ := r.Update(msg)
	reader := m.(markdownReader)
	if reader.active().showSource {
		t.Error("expected showSource=false after page load")
	}
}

func TestUpdate_PageLoadErrorMsg(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.loading = true
	r.loadingURL = "http://example.com"

	msg := pageLoadErrorMsg{url: "http://example.com", err: fmt.Errorf("404 not found")}
	m, _ := r.Update(msg)
	reader := m.(markdownReader)

	if reader.loading {
		t.Error("expected loading=false")
	}
	if !reader.showError {
		t.Error("expected showError=true")
	}
	if reader.errorURL != "http://example.com" {
		t.Errorf("errorURL = %q", reader.errorURL)
	}
	if !strings.Contains(reader.errorText, "404 not found") {
		t.Errorf("errorText = %q, expected to contain error", reader.errorText)
	}
}

func TestUpdate_GoBackMsg(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")
	r.active().showSource = true

	// Push a page via pageLoadedMsg.
	m, _ := r.Update(pageLoadedMsg{name: "second", markdown: "# Second", source: "/second.md"})
	reader := m.(markdownReader)

	// Go back.
	m, _ = reader.Update(mdk.GoBackMsg{})
	reader = m.(markdownReader)

	if reader.active().currentSource != "/doc.md" {
		t.Errorf("currentSource = %q, want %q", reader.active().currentSource, "/doc.md")
	}
	if reader.active().showSource {
		t.Error("expected showSource=false after go back")
	}
	if len(reader.active().pageStack) != 0 {
		t.Errorf("pageStack length = %d, want 0", len(reader.active().pageStack))
	}
}

func TestUpdate_GoBackMsg_EmptyStack(t *testing.T) {
	r := testReader("test", "# Hello", "/doc.md")
	// Go back on empty stack — should be a no-op.
	m, _ := r.Update(mdk.GoBackMsg{})
	reader := m.(markdownReader)
	if reader.active().currentSource != "/doc.md" {
		t.Errorf("currentSource = %q, should be unchanged", reader.active().currentSource)
	}
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	r := testReader("test", "# Hello", "")
	m, _ := r.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	reader := m.(markdownReader)
	if reader.width != 120 {
		t.Errorf("width = %d, want 120", reader.width)
	}
	if reader.height != 40 {
		t.Errorf("height = %d, want 40", reader.height)
	}
}

func TestUpdate_SpinnerTick_WhileLoading(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.loading = true
	_, cmd := r.Update(spinner.TickMsg{})
	// Should return a non-nil command (the next spinner tick).
	if cmd == nil {
		t.Error("expected non-nil command for spinner tick while loading")
	}
}

func TestUpdate_SpinnerTick_NotLoading(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.loading = false
	_, cmd := r.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Error("expected nil command for spinner tick when not loading")
	}
}

func TestUpdate_OpenLinkMsg(t *testing.T) {
	fs := newMemFS()
	fs.files["/docs/other.md"] = []byte("# Other")

	r := testReader("test", "# Hello", "/docs/test.md")
	r.fsys = fs

	m, cmd := r.Update(mdk.OpenLinkMsg{URL: "other.md"})
	reader := m.(markdownReader)

	if !reader.loading {
		t.Error("expected loading=true after OpenLinkMsg for local md file")
	}
	if cmd == nil {
		t.Error("expected non-nil command")
	}
}

func TestUpdate_UnknownMsg(t *testing.T) {
	r := testReader("test", "# Hello", "")
	type unknownMsg struct{}
	m, cmd := r.Update(unknownMsg{})
	_ = m
	if cmd != nil {
		t.Error("expected nil command for unknown message type")
	}
}

// --- handleLinkNavigation tests ---

func TestHandleLinkNavigation_HTTP(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.client = &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": {"text/markdown"}},
				Body:       io.NopCloser(strings.NewReader("# Remote")),
				Request:    req,
			}, nil
		},
	}

	cmd := r.handleLinkNavigation("http://example.com/page.md", false)
	if !r.loading {
		t.Error("expected loading=true for HTTP link")
	}
	if r.loadingURL != "http://example.com/page.md" {
		t.Errorf("loadingURL = %q", r.loadingURL)
	}
	if cmd == nil {
		t.Error("expected non-nil command")
	}
}

func TestHandleLinkNavigation_HTTPS(t *testing.T) {
	r := testReader("test", "# Hello", "")
	cmd := r.handleLinkNavigation("https://example.com/page", false)
	if !r.loading {
		t.Error("expected loading=true for HTTPS link")
	}
	if cmd == nil {
		t.Error("expected non-nil command")
	}
}

func TestHandleLinkNavigation_LocalMarkdown(t *testing.T) {
	fs := newMemFS()
	fs.files["/docs/other.md"] = []byte("# Other")
	r := testReader("test", "# Hello", "/docs/test.md")
	r.fsys = fs

	cmd := r.handleLinkNavigation("other.md", false)
	if !r.loading {
		t.Error("expected loading=true for local markdown file")
	}
	if r.loadingURL != "/docs/other.md" {
		t.Errorf("loadingURL = %q", r.loadingURL)
	}
	if cmd == nil {
		t.Error("expected non-nil command")
	}
}

func TestHandleLinkNavigation_NonMarkdownFile(t *testing.T) {
	r := testReader("test", "# Hello", "/docs/test.md")
	cmd := r.handleLinkNavigation("image.png", false)
	// Should try to open in browser and return nil.
	if r.loading {
		t.Error("expected loading=false for non-markdown file")
	}
	if cmd != nil {
		t.Error("expected nil command for non-markdown file")
	}
}

// --- pushCurrentPage / popPage tests ---

func TestPushPopPage(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")

	// Push current page.
	r.pushCurrentPage()
	if len(r.active().pageStack) != 1 {
		t.Fatalf("pageStack length = %d, want 1", len(r.active().pageStack))
	}
	if r.active().pageStack[0].source != "/page1.md" {
		t.Errorf("pageStack[0].source = %q", r.active().pageStack[0].source)
	}

	// Simulate navigating to page 2.
	r.active().view.SetText("page2", "# Page 2")
	r.active().currentSource = "/page2.md"

	// Pop back to page 1.
	r.popPage()
	if len(r.active().pageStack) != 0 {
		t.Fatalf("pageStack length = %d, want 0", len(r.active().pageStack))
	}
	if r.active().currentSource != "/page1.md" {
		t.Errorf("currentSource = %q, want /page1.md", r.active().currentSource)
	}
}

func TestPopPage_EmptyStack(t *testing.T) {
	r := testReader("test", "# Hello", "/test.md")
	// Should not panic on empty stack.
	r.popPage()
	if r.active().currentSource != "/test.md" {
		t.Errorf("currentSource should be unchanged, got %q", r.active().currentSource)
	}
}

func TestPushPopPage_MultiLevel(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.pushCurrentPage()

	r.active().view.SetText("page2", "# Page 2")
	r.active().currentSource = "/page2.md"
	r.pushCurrentPage()

	r.active().view.SetText("page3", "# Page 3")
	r.active().currentSource = "/page3.md"

	if len(r.active().pageStack) != 2 {
		t.Fatalf("pageStack length = %d, want 2", len(r.active().pageStack))
	}

	r.popPage()
	if r.active().currentSource != "/page2.md" {
		t.Errorf("after first pop: currentSource = %q", r.active().currentSource)
	}

	r.popPage()
	if r.active().currentSource != "/page1.md" {
		t.Errorf("after second pop: currentSource = %q", r.active().currentSource)
	}

	r.popPage() // empty stack — no-op
	if r.active().currentSource != "/page1.md" {
		t.Errorf("after third pop: currentSource = %q", r.active().currentSource)
	}
}

// --- Tab management tests ---

func TestOpenNewTab(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()

	if len(r.tabs) != 1 {
		t.Fatalf("expected 1 tab, got %d", len(r.tabs))
	}

	r.openNewTab("page2", "# Page 2", "/page2.md")

	if len(r.tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(r.tabs))
	}
	if r.activeTab != 1 {
		t.Errorf("activeTab = %d, want 1", r.activeTab)
	}
	if r.active().currentSource != "/page2.md" {
		t.Errorf("active tab source = %q", r.active().currentSource)
	}
}

func TestNextPrevTab(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()

	// Single tab — next/prev should be no-ops.
	r.nextTab()
	if r.activeTab != 0 {
		t.Errorf("activeTab = %d after nextTab with 1 tab", r.activeTab)
	}

	r.openNewTab("page2", "# Page 2", "/page2.md")
	r.openNewTab("page3", "# Page 3", "/page3.md")
	// Now at tab 2.

	r.nextTab()
	if r.activeTab != 0 {
		t.Errorf("activeTab = %d, want 0 (wrap around)", r.activeTab)
	}

	r.prevTab()
	if r.activeTab != 2 {
		t.Errorf("activeTab = %d, want 2 (wrap around)", r.activeTab)
	}

	r.prevTab()
	if r.activeTab != 1 {
		t.Errorf("activeTab = %d, want 1", r.activeTab)
	}
}

func TestCloseTab(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()

	r.openNewTab("page2", "# Page 2", "/page2.md")
	r.openNewTab("page3", "# Page 3", "/page3.md")
	// 3 tabs, active = 2

	// Close middle tab (index 1).
	r.activeTab = 1
	r.closeTab(1)
	if r.showPicker {
		t.Error("expected showPicker=false when closing non-last tab")
	}
	if len(r.tabs) != 2 {
		t.Fatalf("expected 2 tabs after close, got %d", len(r.tabs))
	}
	if r.activeTab != 1 {
		t.Errorf("activeTab = %d, want 1", r.activeTab)
	}
	// The remaining tab at index 1 should be page3.
	if r.active().currentSource != "/page3.md" {
		t.Errorf("active tab source = %q, want /page3.md", r.active().currentSource)
	}
}

func TestCloseTab_LastTab_ShowsPicker(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.closeTab(0)
	if !r.showPicker {
		t.Error("expected showPicker=true when closing the last tab")
	}
	if !r.pickerStartup {
		t.Error("expected pickerStartup=true when closing the last tab")
	}
}

func TestCloseTab_ActiveBeyondEnd(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()

	r.openNewTab("page2", "# Page 2", "/page2.md")
	// active = 1 (last tab)

	r.closeTab(1)
	if r.showPicker {
		t.Error("expected showPicker=false when other tabs remain")
	}
	if r.activeTab != 0 {
		t.Errorf("activeTab = %d, want 0", r.activeTab)
	}
}

func TestUpdate_PageLoadedMsg_NewTab(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()

	msg := pageLoadedMsg{
		name:     "new page",
		markdown: "# New",
		source:   "/new.md",
		newTab:   true,
	}

	m, _ := r.Update(msg)
	reader := m.(markdownReader)

	if len(reader.tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(reader.tabs))
	}
	if reader.activeTab != 1 {
		t.Errorf("activeTab = %d, want 1", reader.activeTab)
	}
	if reader.active().currentSource != "/new.md" {
		t.Errorf("active tab source = %q", reader.active().currentSource)
	}
	// Original tab should be unchanged.
	if reader.tabs[0].currentSource != "/doc.md" {
		t.Errorf("tab 0 source = %q, want /doc.md", reader.tabs[0].currentSource)
	}
}

func TestUpdate_TabSwitch(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.openNewTab("page2", "# Page 2", "/page2.md")
	// active = 1

	// Switch to next tab (wraps to 0).
	m, _ := r.Update(keyMsg("tab"))
	reader := m.(markdownReader)
	if reader.activeTab != 0 {
		t.Errorf("activeTab = %d, want 0 after tab key", reader.activeTab)
	}

	// Switch to prev tab (wraps to 1).
	m, _ = reader.Update(keyMsg("shift+tab"))
	reader = m.(markdownReader)
	if reader.activeTab != 1 {
		t.Errorf("activeTab = %d, want 1 after shift+tab key", reader.activeTab)
	}
}

func TestUpdate_CloseTab(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.openNewTab("page2", "# Page 2", "/page2.md")

	// Close active tab.
	m, cmd := r.Update(keyMsg("ctrl+w"))
	reader := m.(markdownReader)
	if len(reader.tabs) != 1 {
		t.Fatalf("expected 1 tab after close, got %d", len(reader.tabs))
	}
	if cmd != nil {
		t.Error("expected nil command (no quit) when other tabs remain")
	}
}

func TestUpdate_CloseLastTab_ShowsPicker(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	m, cmd := r.Update(keyMsg("ctrl+w"))
	reader := m.(markdownReader)
	if !reader.showPicker {
		t.Error("expected showPicker=true when closing the last tab")
	}
	if cmd == nil {
		t.Error("expected non-nil command (picker init)")
	}
}

func TestUpdate_CloseAllTabs(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.openNewTab("page2", "# Page 2", "/page2.md")
	r.openNewTab("page3", "# Page 3", "/page3.md")

	m, cmd := r.Update(keyMsg("W"))
	reader := m.(markdownReader)
	if len(reader.tabs) != 1 {
		t.Fatalf("expected 1 tab after close all, got %d", len(reader.tabs))
	}
	if !reader.showPicker {
		t.Error("expected showPicker=true after close all")
	}
	if cmd == nil {
		t.Error("expected non-nil command (picker init)")
	}
}

func TestCloseAllTabs(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.openNewTab("page2", "# Page 2", "/page2.md")

	r.closeAllTabs()
	if len(r.tabs) != 1 {
		t.Fatalf("expected 1 tab after closeAllTabs, got %d", len(r.tabs))
	}
	if !r.showPicker {
		t.Error("expected showPicker=true")
	}
	if !r.pickerStartup {
		t.Error("expected pickerStartup=true")
	}
}

func TestRenderTabBar_SingleTab(t *testing.T) {
	r := testReader("test", "# Hello", "")
	bar := r.renderTabBar()
	if bar != "" {
		t.Errorf("expected empty tab bar for single tab, got %q", bar)
	}
}

func TestRenderTabBar_MultipleTabs(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.openNewTab("page2", "# Page 2", "/page2.md")

	bar := r.renderTabBar()
	if bar == "" {
		t.Error("expected non-empty tab bar for multiple tabs")
	}
	if !strings.Contains(bar, "page1") {
		t.Error("expected tab bar to contain 'page1'")
	}
	if !strings.Contains(bar, "page2") {
		t.Error("expected tab bar to contain 'page2'")
	}
}

func TestTabBarHeight(t *testing.T) {
	r := testReader("test", "# Hello", "")
	if r.tabBarHeight() != 0 {
		t.Errorf("expected tabBarHeight=0 for single tab, got %d", r.tabBarHeight())
	}

	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.openNewTab("page2", "# Page 2", "/page2.md")
	if r.tabBarHeight() != 1 {
		t.Errorf("expected tabBarHeight=1 for multiple tabs, got %d", r.tabBarHeight())
	}
}

func TestTabDisplayName(t *testing.T) {
	// Name set via SetText (has a heading) — returns the heading.
	r := testReader("", "# My Document", "/docs/readme.md")
	name := r.active().displayName()
	if name != "My Document" {
		t.Errorf("displayName() = %q, want %q", name, "My Document")
	}

	// Explicit name passed — returns that name.
	r2 := testReader("Explicit", "no heading here", "/docs/readme.md")
	if got := r2.active().displayName(); got != "Explicit" {
		t.Errorf("displayName() = %q, want %q", got, "Explicit")
	}

	// No name, no heading — falls back to source basename.
	r3 := testReader("", "no heading here", "/docs/notes.md")
	if got := r3.active().displayName(); got != "notes.md" {
		t.Errorf("displayName() = %q, want %q", got, "notes.md")
	}

	// No name, no heading, no source — returns empty.
	r4 := testReader("", "no heading here", "")
	if got := r4.active().displayName(); got != "" {
		t.Errorf("displayName() = %q, want empty", got)
	}
}

func TestUpdate_OpenFileSetsPickerNewTabFalse(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.pickerNewTab = true // start with true to verify it gets cleared
	m, _ := r.Update(keyMsg("ctrl+o"))
	reader := m.(markdownReader)
	if !reader.showPicker {
		t.Error("expected showPicker=true after ctrl+o")
	}
	if reader.pickerNewTab {
		t.Error("expected pickerNewTab=false after ctrl+o")
	}
}

func TestUpdate_OpenFileNewTabSetsPickerNewTab(t *testing.T) {
	r := testReader("test", "# Hello", "")
	m, _ := r.Update(keyMsg("ctrl+t"))
	reader := m.(markdownReader)
	if !reader.showPicker {
		t.Error("expected showPicker=true after ctrl+t")
	}
	if !reader.pickerNewTab {
		t.Error("expected pickerNewTab=true after ctrl+t")
	}
}

// --- placeOverlay tests ---

func TestPlaceOverlay_Centered(t *testing.T) {
	bg := strings.Repeat(".", 20) + "\n" +
		strings.Repeat(".", 20) + "\n" +
		strings.Repeat(".", 20) + "\n" +
		strings.Repeat(".", 20) + "\n" +
		strings.Repeat(".", 20)

	dialog := "XX"

	result := placeOverlay(20, 5, dialog, bg)
	lines := strings.Split(result, "\n")

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	// Dialog should be on line 2 (index 2 = (5-1)/2).
	mid := lines[2]
	if !strings.Contains(mid, "XX") {
		t.Errorf("middle line should contain dialog, got %q", mid)
	}
}

func TestPlaceOverlay_PadShortBackground(t *testing.T) {
	bg := "short"
	dialog := "D"
	result := placeOverlay(10, 5, dialog, bg)
	lines := strings.Split(result, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines (padded), got %d", len(lines))
	}
}

func TestPlaceOverlay_DialogLargerThanBg(t *testing.T) {
	bg := "."
	dialog := "ABCDEF\nGHIJKL"
	// Should not panic even if dialog is wider/taller than bg.
	result := placeOverlay(3, 2, dialog, bg)
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestPlaceOverlay_EmptyDialog(t *testing.T) {
	bg := "background"
	result := placeOverlay(10, 1, "", bg)
	if !strings.Contains(result, "background") {
		t.Error("background should be preserved with empty dialog")
	}
}

// --- View / overlayDialog / renderOverlay tests ---

func TestView_ZeroSize(t *testing.T) {
	r := testReader("test", "# Hello", "")
	// width=0, height=0 → empty view.
	v := r.View()
	if v.Content != "" {
		t.Errorf("expected empty body for zero-size view, got %q", v.Content)
	}
}

func TestView_NormalRendering(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 80
	r.height = 24
	r.resizeAllViews()

	v := r.View()
	if v.Content == "" {
		t.Error("expected non-empty body for normal view")
	}
	if !v.AltScreen {
		t.Error("expected AltScreen=true")
	}
}

func TestView_LoadingOverlay(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.loading = true
	r.loadingURL = "http://example.com"

	v := r.View()
	if v.Content == "" {
		t.Error("expected non-empty body with loading overlay")
	}
	if !strings.Contains(v.Content, "Loading") {
		t.Error("expected 'Loading' text in view")
	}
}

func TestView_LoadingOverlay_NoURL(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.loading = true

	v := r.View()
	if !strings.Contains(v.Content, "Loading") {
		t.Error("expected 'Loading' text in view")
	}
}

func TestView_HelpOverlay(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.showHelp = true

	v := r.View()
	if v.Content == "" {
		t.Error("expected non-empty body with help overlay")
	}
}

func TestView_HelpOverlay_NarrowTerminal(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 30
	r.height = 24
	r.resizeAllViews()
	r.showHelp = true

	v := r.View()
	if v.Content == "" {
		t.Error("expected non-empty body even with narrow terminal")
	}
}

func TestView_ErrorOverlay(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.showError = true
	r.errorText = "Something went wrong"

	v := r.View()
	if v.Content == "" {
		t.Error("expected non-empty body with error overlay")
	}
	if !strings.Contains(v.Content, "Something went wrong") {
		t.Error("expected error text in view")
	}
}

func TestOverlayDialog_NarrowWidth(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 20
	r.height = 10

	bg := strings.Repeat(".", 20)
	result := r.overlayDialog(bg, "Title", "content text here")
	if result == "" {
		t.Error("expected non-empty overlay dialog")
	}
}

func TestRenderOverlay_TruncatesLongContent(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 40
	r.height = 10

	bg := strings.Repeat(strings.Repeat(".", 40)+"\n", 10)
	// Create content with many lines.
	var longContent strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&longContent, "line %d\n", i)
	}

	result := r.renderOverlay(bg, longContent.String(), 30, 8)
	if result == "" {
		t.Error("expected non-empty result")
	}
	// The dialog height limit is maxH-2 = 6 lines of content.
	// Result should not contain all 50 lines.
	if strings.Contains(result, "line 49") {
		t.Error("expected content to be truncated")
	}
}

// --- URL input tests ---

func TestUpdate_OpenURL_CtrlL(t *testing.T) {
	r := testReader("test", "# Hello", "")
	m, cmd := r.Update(keyMsg("ctrl+l"))
	reader := m.(markdownReader)
	if !reader.showURLInput {
		t.Error("expected showURLInput=true after ctrl+l")
	}
	if reader.urlNewTab {
		t.Error("expected urlNewTab=false after ctrl+l")
	}
	if cmd == nil {
		t.Error("expected non-nil command (focus)")
	}
}

func TestUpdate_URLInput_Dismiss_Esc(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showURLInput = true
	m, _ := r.Update(keyMsg("esc"))
	reader := m.(markdownReader)
	if reader.showURLInput {
		t.Error("expected showURLInput=false after esc")
	}
}

func TestUpdate_URLInput_Enter_WithURL(t *testing.T) {
	fs := newMemFS()
	r := testReader("test", "# Hello", "")
	r.fsys = fs
	r.showURLInput = true
	r.urlInput.SetValue("https://example.com/page.md")

	m, cmd := r.Update(keyMsg("enter"))
	reader := m.(markdownReader)
	if reader.showURLInput {
		t.Error("expected showURLInput=false after enter")
	}
	if !reader.loading {
		t.Error("expected loading=true after entering a URL")
	}
	if cmd == nil {
		t.Error("expected non-nil command for URL navigation")
	}
}

func TestUpdate_URLInput_Enter_Empty(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showURLInput = true
	// urlInput value is empty by default.
	m, cmd := r.Update(keyMsg("enter"))
	reader := m.(markdownReader)
	if reader.showURLInput {
		t.Error("expected showURLInput=false after enter")
	}
	if reader.loading {
		t.Error("expected loading=false for empty URL")
	}
	if cmd != nil {
		t.Error("expected nil command for empty URL")
	}
}

func TestUpdate_URLInput_SwallowsOtherKeys(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.showURLInput = true
	m, _ := r.Update(keyMsg("q"))
	reader := m.(markdownReader)
	// Should not quit — key is forwarded to the text input.
	if !reader.showURLInput {
		t.Error("expected showURLInput to remain true for non-esc/enter key")
	}
}

func TestView_URLInputOverlay(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.width = 80
	r.height = 24
	r.resizeAllViews()
	r.showURLInput = true

	v := r.View()
	if v.Content == "" {
		t.Error("expected non-empty body with URL input overlay")
	}
	if !strings.Contains(v.Content, "Open URL") {
		t.Error("expected 'Open URL' header in view")
	}
}

// --- History picker integration tests ---

func TestUpdate_CtrlH_WithStack(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")
	// Push a page to create a non-empty stack.
	m, _ := r.Update(pageLoadedMsg{name: "second", markdown: "# Second", source: "/second.md"})
	reader := m.(markdownReader)

	m, cmd := reader.Update(keyMsg("H"))
	reader = m.(markdownReader)
	if !reader.showHistory {
		t.Error("expected showHistory=true after H with non-empty stack")
	}
	if cmd == nil {
		t.Error("expected non-nil command (focus)")
	}
}

func TestUpdate_CtrlH_EmptyStack(t *testing.T) {
	r := testReader("test", "# Hello", "")
	m, cmd := r.Update(keyMsg("H"))
	reader := m.(markdownReader)
	if reader.showHistory {
		t.Error("expected showHistory=false after H with empty stack")
	}
	if cmd != nil {
		t.Error("expected nil command for empty stack")
	}
}

func TestUpdate_History_Dismiss_Esc(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")
	m, _ := r.Update(pageLoadedMsg{name: "second", markdown: "# Second", source: "/second.md"})
	reader := m.(markdownReader)

	m, _ = reader.Update(keyMsg("H"))
	reader = m.(markdownReader)

	m, _ = reader.Update(keyMsg("esc"))
	reader = m.(markdownReader)
	if reader.showHistory {
		t.Error("expected showHistory=false after esc")
	}
}

func TestUpdate_History_SelectEntry(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	// Navigate to page2.
	m, _ := r.Update(pageLoadedMsg{name: "page2", markdown: "# Page 2", source: "/page2.md"})
	reader := m.(markdownReader)
	// Navigate to page3.
	m, _ = reader.Update(pageLoadedMsg{name: "page3", markdown: "# Page 3", source: "/page3.md"})
	reader = m.(markdownReader)

	if len(reader.active().pageStack) != 2 {
		t.Fatalf("pageStack length = %d, want 2", len(reader.active().pageStack))
	}

	// Open history.
	m, _ = reader.Update(keyMsg("H"))
	reader = m.(markdownReader)

	// Move cursor to the last entry (page1, index 0 — oldest).
	// Cursor starts at top (current page). Move down twice to get to index 0.
	m, _ = reader.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	reader = m.(markdownReader)
	m, _ = reader.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	reader = m.(markdownReader)

	// Select.
	m, _ = reader.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	reader = m.(markdownReader)

	if reader.showHistory {
		t.Error("expected showHistory=false after selection")
	}
	if reader.active().currentSource != "/page1.md" {
		t.Errorf("currentSource = %q, want /page1.md", reader.active().currentSource)
	}
	if len(reader.active().pageStack) != 0 {
		t.Errorf("pageStack length = %d, want 0 (truncated)", len(reader.active().pageStack))
	}
}

func TestUpdate_History_SelectCurrentPage(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")
	m, _ := r.Update(pageLoadedMsg{name: "second", markdown: "# Second", source: "/second.md"})
	reader := m.(markdownReader)

	// Open history.
	m, _ = reader.Update(keyMsg("H"))
	reader = m.(markdownReader)

	// Cursor is on current page. Press enter.
	m, _ = reader.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	reader = m.(markdownReader)

	if reader.showHistory {
		t.Error("expected showHistory=false after selecting current page")
	}
	// Stack should be unchanged.
	if len(reader.active().pageStack) != 1 {
		t.Errorf("pageStack length = %d, want 1 (unchanged)", len(reader.active().pageStack))
	}
	if reader.active().currentSource != "/second.md" {
		t.Errorf("currentSource = %q, want /second.md (unchanged)", reader.active().currentSource)
	}
}

func TestView_HistoryOverlay(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")
	r.width = 80
	r.height = 24
	r.resizeAllViews()

	m, _ := r.Update(pageLoadedMsg{name: "second", markdown: "# Second", source: "/second.md"})
	reader := m.(markdownReader)

	m, _ = reader.Update(keyMsg("H"))
	reader = m.(markdownReader)

	v := reader.View()
	if v.Content == "" {
		t.Error("expected non-empty body with history overlay")
	}
	if !strings.Contains(v.Content, "History") {
		t.Error("expected 'History' header in view")
	}
}

// --- Reload tests ---

func TestUpdate_CtrlR_WithSource(t *testing.T) {
	r := testReader("doc", "# Doc", "/doc.md")
	r.width, r.height = 80, 24
	r.resizeAllViews()

	m, cmd := r.Update(keyMsg("ctrl+r"))
	reader := m.(markdownReader)

	if !reader.loading {
		t.Error("expected loading=true after ctrl+r")
	}
	if reader.loadingURL != "/doc.md" {
		t.Errorf("loadingURL = %q, want /doc.md", reader.loadingURL)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

func TestUpdate_CtrlR_NoSource(t *testing.T) {
	r := testReader("doc", "# Doc", "")
	r.width, r.height = 80, 24
	r.resizeAllViews()

	m, cmd := r.Update(keyMsg("ctrl+r"))
	reader := m.(markdownReader)

	if reader.loading {
		t.Error("expected loading=false when source is empty")
	}
	if cmd != nil {
		t.Error("expected nil cmd when source is empty")
	}
}

func TestUpdate_PageLoadedMsg_Reload_DoesNotPushStack(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")

	// Navigate to a new page first, so the stack has one entry.
	m, _ := r.Update(pageLoadedMsg{name: "second", markdown: "# Second", source: "/second.md"})
	reader := m.(markdownReader)

	if len(reader.active().pageStack) != 1 {
		t.Fatalf("pageStack length = %d, want 1", len(reader.active().pageStack))
	}

	// Now reload — stack should stay at 1.
	m, _ = reader.Update(pageLoadedMsg{
		name:     "second-reloaded",
		markdown: "# Second Reloaded",
		source:   "/second.md",
		reload:   true,
	})
	reader = m.(markdownReader)

	if len(reader.active().pageStack) != 1 {
		t.Errorf("pageStack length = %d after reload, want 1", len(reader.active().pageStack))
	}
	if reader.active().currentSource != "/second.md" {
		t.Errorf("currentSource = %q, want /second.md", reader.active().currentSource)
	}
	if string(reader.active().view.GetMarkdown()) != "# Second Reloaded" {
		t.Error("expected reloaded content")
	}
}
