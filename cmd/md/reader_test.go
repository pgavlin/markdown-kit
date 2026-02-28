package main

import (
	"strings"
	"testing"
)

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
		// Should use 4+ backticks to avoid conflict.
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
		// Should use 4+ backticks.
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
