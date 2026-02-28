package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

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
		nil,                // no cache
		&fakeHTTPClient{},  // unused default
		newMemFS(),
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
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+e":
		return tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}
	case "ctrl+t":
		return tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}
	case "ctrl+o":
		return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
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

func TestFenceRaw(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		got := fenceRaw("# Hello")
		if !strings.HasPrefix(got, "```\n") {
			t.Errorf("expected to start with ```\\n, got prefix %q", got[:10])
		}
		if !strings.Contains(got, "# Hello") {
			t.Error("expected markdown content to be preserved")
		}
	})

	t.Run("with_backticks", func(t *testing.T) {
		md := "```go\nfmt.Println()\n```"
		got := fenceRaw(md)
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
	if len(groups) == 0 {
		t.Fatal("expected non-empty full help groups")
	}
	// Last group should include reader-specific bindings.
	last := groups[len(groups)-1]
	if len(last) < 3 {
		t.Errorf("expected last group to have at least 3 bindings, got %d", len(last))
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
	if r.showRaw {
		t.Fatal("expected showRaw=false initially")
	}

	// Toggle on.
	m, _ := r.Update(keyMsg("ctrl+r"))
	reader := m.(markdownReader)
	if !reader.showRaw {
		t.Error("expected showRaw=true after ctrl+r")
	}

	// Toggle off.
	m, _ = reader.Update(keyMsg("ctrl+r"))
	reader = m.(markdownReader)
	if reader.showRaw {
		t.Error("expected showRaw=false after second ctrl+r")
	}
}

func TestUpdate_ToggleOriginalHTML_NoHTML(t *testing.T) {
	r := testReader("test", "# Hello", "")
	// No HTML content — ctrl+e should be a no-op.
	m, _ := r.Update(keyMsg("ctrl+e"))
	reader := m.(markdownReader)
	if reader.showRaw {
		t.Error("expected showRaw=false when no originalHTML")
	}
}

func TestUpdate_ToggleOriginalHTML_WithHTML(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.currentOriginalHTML = "<html><body>test</body></html>"

	// Toggle on.
	m, _ := r.Update(keyMsg("ctrl+e"))
	reader := m.(markdownReader)
	if !reader.showRaw {
		t.Error("expected showRaw=true after ctrl+e with HTML")
	}

	// Toggle off.
	m, _ = reader.Update(keyMsg("ctrl+e"))
	reader = m.(markdownReader)
	if reader.showRaw {
		t.Error("expected showRaw=false after second ctrl+e")
	}
}

func TestUpdate_ToggleReadabilityHTML_NoHTML(t *testing.T) {
	r := testReader("test", "# Hello", "")
	m, _ := r.Update(keyMsg("ctrl+t"))
	reader := m.(markdownReader)
	if reader.showRaw {
		t.Error("expected showRaw=false when no readabilityHTML")
	}
}

func TestUpdate_ToggleReadabilityHTML_WithHTML(t *testing.T) {
	r := testReader("test", "# Hello", "")
	r.currentReadabilityHTML = "<article>content</article>"

	m, _ := r.Update(keyMsg("ctrl+t"))
	reader := m.(markdownReader)
	if !reader.showRaw {
		t.Error("expected showRaw=true after ctrl+t with readability HTML")
	}

	m, _ = reader.Update(keyMsg("ctrl+t"))
	reader = m.(markdownReader)
	if reader.showRaw {
		t.Error("expected showRaw=false after second ctrl+t")
	}
}

func TestUpdate_PageLoadedMsg(t *testing.T) {
	r := testReader("initial", "# Initial", "/doc.md")
	msg := pageLoadedMsg{
		name:            "new page",
		markdown:        "# New",
		source:          "http://example.com/new",
		originalHTML:    "<html>orig</html>",
		readabilityHTML: "<article>read</article>",
	}

	m, _ := r.Update(msg)
	reader := m.(markdownReader)

	if reader.currentSource != "http://example.com/new" {
		t.Errorf("currentSource = %q", reader.currentSource)
	}
	if reader.currentOriginalHTML != "<html>orig</html>" {
		t.Errorf("currentOriginalHTML = %q", reader.currentOriginalHTML)
	}
	if reader.currentReadabilityHTML != "<article>read</article>" {
		t.Errorf("currentReadabilityHTML = %q", reader.currentReadabilityHTML)
	}
	if reader.loading {
		t.Error("expected loading=false")
	}
	if reader.showRaw {
		t.Error("expected showRaw=false")
	}
	// Page stack should have the initial page.
	if len(reader.pageStack) != 1 {
		t.Errorf("pageStack length = %d, want 1", len(reader.pageStack))
	}
	if reader.pageStack[0].source != "/doc.md" {
		t.Errorf("pageStack[0].source = %q, want %q", reader.pageStack[0].source, "/doc.md")
	}
}

func TestUpdate_PageLoadedMsg_ClearsRawState(t *testing.T) {
	r := testReader("initial", "# Initial", "")
	r.showRaw = true

	msg := pageLoadedMsg{name: "new", markdown: "# New", source: "new.md"}
	m, _ := r.Update(msg)
	reader := m.(markdownReader)
	if reader.showRaw {
		t.Error("expected showRaw=false after page load")
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
	r.showRaw = true

	// Push a page via pageLoadedMsg.
	m, _ := r.Update(pageLoadedMsg{name: "second", markdown: "# Second", source: "/second.md"})
	reader := m.(markdownReader)

	// Go back.
	m, _ = reader.Update(mdk.GoBackMsg{})
	reader = m.(markdownReader)

	if reader.currentSource != "/doc.md" {
		t.Errorf("currentSource = %q, want %q", reader.currentSource, "/doc.md")
	}
	if reader.showRaw {
		t.Error("expected showRaw=false after go back")
	}
	if len(reader.pageStack) != 0 {
		t.Errorf("pageStack length = %d, want 0", len(reader.pageStack))
	}
}

func TestUpdate_GoBackMsg_EmptyStack(t *testing.T) {
	r := testReader("test", "# Hello", "/doc.md")
	// Go back on empty stack — should be a no-op.
	m, _ := r.Update(mdk.GoBackMsg{})
	reader := m.(markdownReader)
	if reader.currentSource != "/doc.md" {
		t.Errorf("currentSource = %q, should be unchanged", reader.currentSource)
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

	cmd := r.handleLinkNavigation("http://example.com/page.md")
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
	cmd := r.handleLinkNavigation("https://example.com/page")
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

	cmd := r.handleLinkNavigation("other.md")
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
	cmd := r.handleLinkNavigation("image.png")
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
	r.currentOriginalHTML = "<html>1</html>"
	r.currentReadabilityHTML = "<article>1</article>"

	// Push current page.
	r.pushCurrentPage()
	if len(r.pageStack) != 1 {
		t.Fatalf("pageStack length = %d, want 1", len(r.pageStack))
	}
	if r.pageStack[0].source != "/page1.md" {
		t.Errorf("pageStack[0].source = %q", r.pageStack[0].source)
	}
	if r.pageStack[0].originalHTML != "<html>1</html>" {
		t.Errorf("pageStack[0].originalHTML = %q", r.pageStack[0].originalHTML)
	}

	// Simulate navigating to page 2.
	r.view.SetText("page2", "# Page 2")
	r.currentSource = "/page2.md"
	r.currentOriginalHTML = ""
	r.currentReadabilityHTML = ""

	// Pop back to page 1.
	r.popPage()
	if len(r.pageStack) != 0 {
		t.Fatalf("pageStack length = %d, want 0", len(r.pageStack))
	}
	if r.currentSource != "/page1.md" {
		t.Errorf("currentSource = %q, want /page1.md", r.currentSource)
	}
	if r.currentOriginalHTML != "<html>1</html>" {
		t.Errorf("currentOriginalHTML = %q", r.currentOriginalHTML)
	}
}

func TestPopPage_EmptyStack(t *testing.T) {
	r := testReader("test", "# Hello", "/test.md")
	// Should not panic on empty stack.
	r.popPage()
	if r.currentSource != "/test.md" {
		t.Errorf("currentSource should be unchanged, got %q", r.currentSource)
	}
}

func TestPushPopPage_MultiLevel(t *testing.T) {
	r := testReader("page1", "# Page 1", "/page1.md")
	r.pushCurrentPage()

	r.view.SetText("page2", "# Page 2")
	r.currentSource = "/page2.md"
	r.pushCurrentPage()

	r.view.SetText("page3", "# Page 3")
	r.currentSource = "/page3.md"

	if len(r.pageStack) != 2 {
		t.Fatalf("pageStack length = %d, want 2", len(r.pageStack))
	}

	r.popPage()
	if r.currentSource != "/page2.md" {
		t.Errorf("after first pop: currentSource = %q", r.currentSource)
	}

	r.popPage()
	if r.currentSource != "/page1.md" {
		t.Errorf("after second pop: currentSource = %q", r.currentSource)
	}

	r.popPage() // empty stack — no-op
	if r.currentSource != "/page1.md" {
		t.Errorf("after third pop: currentSource = %q", r.currentSource)
	}
}

func TestUpdateHTMLKeyBindings(t *testing.T) {
	r := testReader("test", "# Hello", "")

	r.currentOriginalHTML = "<html>yes</html>"
	r.updateHTMLKeyBindings()
	if !r.keys.ToggleOriginalHTML.Enabled() {
		t.Error("expected ToggleOriginalHTML enabled when HTML present")
	}

	r.currentOriginalHTML = ""
	r.updateHTMLKeyBindings()
	if r.keys.ToggleOriginalHTML.Enabled() {
		t.Error("expected ToggleOriginalHTML disabled when no HTML")
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
	r.view.SetSize(80, 24)

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
	r.view.SetSize(80, 24)
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
	r.view.SetSize(80, 24)
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
	r.view.SetSize(80, 24)
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
	r.view.SetSize(30, 24)
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
	r.view.SetSize(80, 24)
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
