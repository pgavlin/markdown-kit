package docsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "heading prefixes",
			input:    "# Title\n## Subtitle\n### Section",
			expected: "Title\nSubtitle\nSection",
		},
		{
			name:     "setext headings",
			input:    "Title\n=====\nSubtitle\n-----",
			expected: "Title\nSubtitle",
		},
		{
			name:     "bold and italic",
			input:    "This is **bold** and *italic* and __also bold__ and _also italic_.",
			expected: "This is bold and italic and also bold and also italic.",
		},
		{
			name:     "strikethrough",
			input:    "This is ~~deleted~~ text.",
			expected: "This is deleted text.",
		},
		{
			name:     "links",
			input:    "Click [here](https://example.com) for more.",
			expected: "Click here for more.",
		},
		{
			name:     "images",
			input:    "An image: ![alt text](image.png)",
			expected: "An image: alt text",
		},
		{
			name:     "inline code",
			input:    "Use the `fmt.Println` function.",
			expected: "Use the fmt.Println function.",
		},
		{
			name:     "code fences",
			input:    "Before\n```go\nfmt.Println(\"hello\")\n```\nAfter",
			expected: "Before\nfmt.Println(\"hello\")\nAfter",
		},
		{
			name:     "tilde code fences",
			input:    "Before\n~~~\ncode here\n~~~\nAfter",
			expected: "Before\ncode here\nAfter",
		},
		{
			name:     "HTML tags",
			input:    "Some <b>bold</b> and <a href=\"url\">link</a> text.",
			expected: "Some bold and link text.",
		},
		{
			name:     "table",
			input:    "| Name | Value |\n|------|-------|\n| foo  | bar   |",
			expected: "Name Value\nfoo bar",
		},
		{
			name:     "multiple blank lines collapsed",
			input:    "First\n\n\n\nSecond",
			expected: "First\n\nSecond",
		},
		{
			name:     "mixed formatting",
			input:    "# Hello **World**\n\nThis is a [link](url) with `code`.",
			expected: "Hello World\n\nThis is a link with code.",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   \n  \n   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripMarkdown(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
