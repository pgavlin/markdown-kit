package odt

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderMarkdown is a test helper that parses markdown source and renders it
// through the ODT renderer, returning the full XML output as a string.
func renderMarkdown(t *testing.T, source string) string {
	t.Helper()

	src := []byte(source)
	parser := goldmark.DefaultParser()
	document := parser.Parse(text.NewReader(src))

	var buf bytes.Buffer
	r := NewRenderer("serif", "monospace")
	err := r.Render(&buf, src, document)
	require.NoError(t, err)

	return buf.String()
}

func TestCodeBlock(t *testing.T) {
	// Indented code blocks require 4 spaces of indentation.
	source := "    code line 1\n    code line 2\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:p text:style-name="Code Block">`)
	assert.Contains(t, output, "code<text:s/>line<text:s/>1")
	assert.Contains(t, output, "code<text:s/>line<text:s/>2")
	assert.Contains(t, output, "</text:p>")
}

func TestFencedCodeBlock(t *testing.T) {
	source := "```go\nfmt.Println(\"hello\")\n```\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:p text:style-name="Code Block">`)
	assert.Contains(t, output, "fmt.Println(")
	assert.Contains(t, output, "&#34;hello&#34;")
	assert.Contains(t, output, "</text:p>")
}

func TestFencedCodeBlockNoLanguage(t *testing.T) {
	source := "```\nsome code\n```\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:p text:style-name="Code Block">`)
	assert.Contains(t, output, "some<text:s/>code")
}

func TestThematicBreak(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"dashes", "---\n"},
		{"asterisks", "***\n"},
		{"underscores", "___\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := renderMarkdown(t, tt.source)
			assert.Contains(t, output, `<text:p text:style-name="Thematic Break"/>`)
		})
	}
}

func TestAutoLink(t *testing.T) {
	source := "<https://example.com>\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:a xlink:href="https://example.com">`)
	assert.Contains(t, output, "https://example.com")
	assert.Contains(t, output, "</text:a>")
}

func TestAutoLinkEmail(t *testing.T) {
	source := "<user@example.com>\n"
	output := renderMarkdown(t, source)

	// goldmark parses email autolinks and renders them with the URL.
	assert.Contains(t, output, `xlink:href=`)
	assert.Contains(t, output, "user@example.com")
	assert.Contains(t, output, "</text:a>")
}

func TestXMLEscaping(t *testing.T) {
	// Use characters that need XML escaping but won't be parsed as HTML tags.
	// goldmark treats `<angle>` as an HTML inline element, so we avoid angle
	// brackets in the markdown source. Instead we test escaping via escapeText
	// directly and check &, ", and ' through the renderer.
	source := "Text with & \"quotes\" and 'apostrophes'\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, "&amp;")
	assert.Contains(t, output, "&#34;quotes&#34;")
	assert.Contains(t, output, "&#39;apostrophes&#39;")

	// Verify the raw special characters don't appear unescaped in the paragraph body.
	bodyStart := strings.Index(output, `<text:p text:style-name="Paragraph">`)
	require.NotEqual(t, -1, bodyStart, "expected to find a Paragraph element")
	bodyEnd := strings.Index(output[bodyStart:], "</text:p>")
	body := output[bodyStart : bodyStart+bodyEnd]
	assert.NotContains(t, body, "& ")       // raw & should be escaped
	assert.NotContains(t, body, `"quotes"`) // raw " should be escaped
}

func TestWhitespacePreservationInCode(t *testing.T) {
	// Test tabs in code blocks
	source := "```\n\tindented\n```\n"
	output := renderMarkdown(t, source)

	// Tabs should become 4 text:s elements
	assert.Contains(t, output, "<text:s/><text:s/><text:s/><text:s/>indented")

	// Test spaces in code blocks
	source2 := "```\na b\n```\n"
	output2 := renderMarkdown(t, source2)
	assert.Contains(t, output2, "a<text:s/>b")

	// Test newlines in multi-line code blocks
	source3 := "```\nline1\nline2\n```\n"
	output3 := renderMarkdown(t, source3)
	assert.Contains(t, output3, "line1<text:line-break/>")
	assert.Contains(t, output3, "line2")
}

func TestEmphasis(t *testing.T) {
	source := "*italic text*\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:span text:style-name="Emphasis">`)
	assert.Contains(t, output, "italic text")
	assert.Contains(t, output, "</text:span>")
}

func TestStrongEmphasis(t *testing.T) {
	source := "**bold text**\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:span text:style-name="Strong Emphasis">`)
	assert.Contains(t, output, "bold text")
	assert.Contains(t, output, "</text:span>")
}

func TestNestedEmphasis(t *testing.T) {
	source := "***bold and italic***\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:span text:style-name="Emphasis">`)
	assert.Contains(t, output, `<text:span text:style-name="Strong Emphasis">`)
	assert.Contains(t, output, "bold and italic")
}

func TestLink(t *testing.T) {
	source := "[click here](https://example.com)\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:a xlink:href="https://example.com">`)
	assert.Contains(t, output, "click here")
	assert.Contains(t, output, "</text:a>")
}

func TestLinkWithSpecialChars(t *testing.T) {
	// The renderer writes node.Destination directly into the href attribute
	// without XML-escaping. This tests the current behavior.
	source := "[link](https://example.com/path?q=1&r=2)\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:a xlink:href="https://example.com/path?q=1&r=2">`)
	assert.Contains(t, output, "link")
	assert.Contains(t, output, "</text:a>")
}

func TestHeadings(t *testing.T) {
	tests := []struct {
		name   string
		source string
		level  int
		text   string
	}{
		{"h1", "# Heading 1\n", 1, "Heading 1"},
		{"h2", "## Heading 2\n", 2, "Heading 2"},
		{"h3", "### Heading 3\n", 3, "Heading 3"},
		{"h4", "#### Heading 4\n", 4, "Heading 4"},
		{"h5", "##### Heading 5\n", 5, "Heading 5"},
		{"h6", "###### Heading 6\n", 6, "Heading 6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := renderMarkdown(t, tt.source)
			expected := strings.Replace(`<text:h text:outline-level="LEVEL">`, "LEVEL", strings.TrimSpace(string(rune('0'+tt.level))), 1)
			assert.Contains(t, output, expected)
			assert.Contains(t, output, tt.text)
			assert.Contains(t, output, "</text:h>")
		})
	}
}

func TestUnorderedList(t *testing.T) {
	source := "- item 1\n- item 2\n- item 3\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:list text:style-name="Unordered List">`)
	assert.Contains(t, output, "<text:list-item>")
	assert.Contains(t, output, "</text:list-item>")
	assert.Contains(t, output, "</text:list>")
	assert.Contains(t, output, "item 1")
	assert.Contains(t, output, "item 2")
	assert.Contains(t, output, "item 3")
}

func TestOrderedList(t *testing.T) {
	source := "1. first\n2. second\n3. third\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:list text:style-name="Ordered List">`)
	assert.Contains(t, output, `text:start-value="1"`)
	assert.Contains(t, output, "<text:list-item")
	assert.Contains(t, output, "</text:list-item>")
	assert.Contains(t, output, "</text:list>")
	assert.Contains(t, output, "first")
	assert.Contains(t, output, "second")
	assert.Contains(t, output, "third")
}

func TestOrderedListCustomStart(t *testing.T) {
	source := "3. third\n4. fourth\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:list text:style-name="Ordered List">`)
	assert.Contains(t, output, `text:start-value="3"`)
}

func TestCodeSpan(t *testing.T) {
	source := "Use the `fmt.Println` function.\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:span text:style-name="Code Span">`)
	assert.Contains(t, output, "fmt.Println")
	assert.Contains(t, output, "</text:span>")
}

func TestCodeSpanWithSpecialChars(t *testing.T) {
	source := "Use `a < b && c > d` in code.\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:span text:style-name="Code Span">`)
	assert.Contains(t, output, "a &lt; b &amp;&amp; c &gt; d")
}

func TestParagraph(t *testing.T) {
	source := "This is a paragraph.\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:p text:style-name="Paragraph">`)
	assert.Contains(t, output, "This is a paragraph.")
	assert.Contains(t, output, "</text:p>")
}

func TestMultipleParagraphs(t *testing.T) {
	source := "First paragraph.\n\nSecond paragraph.\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, "First paragraph.")
	assert.Contains(t, output, "Second paragraph.")
	count := strings.Count(output, `<text:p text:style-name="Paragraph">`)
	assert.Equal(t, 2, count, "expected two paragraph elements")
}

func TestBlockquote(t *testing.T) {
	source := "> This is a quote.\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:p text:style-name="Blockquote">`)
	assert.Contains(t, output, "This is a quote.")
}

func TestDocumentStructure(t *testing.T) {
	source := "Hello\n"
	output := renderMarkdown(t, source)

	// Check XML prolog
	assert.True(t, strings.HasPrefix(output, `<?xml version="1.0" encoding="UTF-8"?>`))

	// Check document wrapper elements
	assert.Contains(t, output, "<office:document-content")
	assert.Contains(t, output, "<office:body>")
	assert.Contains(t, output, "<office:text>")
	assert.Contains(t, output, "</office:text>")
	assert.Contains(t, output, "</office:body>")
	assert.Contains(t, output, "</office:document-content>")

	// Check style definitions are present
	assert.Contains(t, output, "<office:automatic-styles>")
	assert.Contains(t, output, "</office:automatic-styles>")
}

func TestHardLineBreak(t *testing.T) {
	// Two trailing spaces create a hard line break in markdown.
	source := "line one  \nline two\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, "line one")
	assert.Contains(t, output, "<text:line-break/>")
	assert.Contains(t, output, "line two")
}

func TestImageIsNoop(t *testing.T) {
	source := "![alt text](image.png)\n"
	output := renderMarkdown(t, source)

	// The image renderer is a no-op, so there should be no draw:image or similar.
	assert.NotContains(t, output, "draw:image")
	assert.NotContains(t, output, "image.png")
}

func TestEscapeTextDirect(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		preserveWhitespace bool
		expected           string
	}{
		{
			name:               "ampersand",
			input:              "a & b",
			preserveWhitespace: false,
			expected:           "a &amp; b",
		},
		{
			name:               "less than",
			input:              "a < b",
			preserveWhitespace: false,
			expected:           "a &lt; b",
		},
		{
			name:               "greater than",
			input:              "a > b",
			preserveWhitespace: false,
			expected:           "a &gt; b",
		},
		{
			name:               "double quote",
			input:              `say "hello"`,
			preserveWhitespace: false,
			expected:           "say &#34;hello&#34;",
		},
		{
			name:               "single quote",
			input:              "it's",
			preserveWhitespace: false,
			expected:           "it&#39;s",
		},
		{
			name:               "tab no preserve",
			input:              "a\tb",
			preserveWhitespace: false,
			expected:           "a&#x9;b",
		},
		{
			name:               "tab preserve whitespace",
			input:              "a\tb",
			preserveWhitespace: true,
			expected:           "a<text:s/><text:s/><text:s/><text:s/>b",
		},
		{
			name:               "newline no preserve",
			input:              "a\nb",
			preserveWhitespace: false,
			expected:           "a&#xA;b",
		},
		{
			name:               "newline preserve whitespace",
			input:              "a\nb",
			preserveWhitespace: true,
			expected:           "a<text:line-break/>b",
		},
		{
			name:               "space no preserve",
			input:              "a b",
			preserveWhitespace: false,
			expected:           "a b",
		},
		{
			name:               "space preserve whitespace",
			input:              "a b",
			preserveWhitespace: true,
			expected:           "a<text:s/>b",
		},
		{
			name:               "carriage return",
			input:              "a\rb",
			preserveWhitespace: false,
			expected:           "a&#xD;b",
		},
		{
			name:               "multiple special chars",
			input:              "<b>&amp;</b>",
			preserveWhitespace: false,
			expected:           "&lt;b&gt;&amp;amp;&lt;/b&gt;",
		},
		{
			name:               "empty string",
			input:              "",
			preserveWhitespace: false,
			expected:           "",
		},
		{
			name:               "plain text no escaping needed",
			input:              "hello world",
			preserveWhitespace: false,
			expected:           "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := escapeText(&buf, []byte(tt.input), tt.preserveWhitespace)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestStringNode(t *testing.T) {
	// String nodes can be produced by certain goldmark extensions.
	// We test the renderString method directly by constructing a String
	// AST node and calling the render method.
	var buf bytes.Buffer
	r := NewRenderer("serif", "monospace")

	node := &ast.String{}
	node.Value = []byte("hello <world> & 'friends'")

	_, err := r.renderString(&buf, nil, node, true)
	require.NoError(t, err)
	assert.Equal(t, "hello &lt;world&gt; &amp; &#39;friends&#39;", buf.String())

	// Also verify that enter=false is a no-op.
	buf.Reset()
	_, err = r.renderString(&buf, nil, node, false)
	require.NoError(t, err)
	assert.Equal(t, "", buf.String())
}

func TestComplexDocument(t *testing.T) {
	source := `# My Document

This is an *introductory* paragraph with **bold** and ` + "`code`" + `.

## Section 1

- Item A
- Item B

### Subsection

1. First
2. Second

---

> A famous quote.

` + "```" + `
some code
` + "```" + `

Visit [our site](https://example.com) for more.
`

	output := renderMarkdown(t, source)

	// Headings
	assert.Contains(t, output, `<text:h text:outline-level="1">My Document</text:h>`)
	assert.Contains(t, output, `<text:h text:outline-level="2">Section 1</text:h>`)
	assert.Contains(t, output, `<text:h text:outline-level="3">Subsection</text:h>`)

	// Emphasis and strong
	assert.Contains(t, output, `<text:span text:style-name="Emphasis">introductory</text:span>`)
	assert.Contains(t, output, `<text:span text:style-name="Strong Emphasis">bold</text:span>`)

	// Code span
	assert.Contains(t, output, `<text:span text:style-name="Code Span">code</text:span>`)

	// Unordered list
	assert.Contains(t, output, `<text:list text:style-name="Unordered List">`)
	assert.Contains(t, output, "Item A")
	assert.Contains(t, output, "Item B")

	// Ordered list
	assert.Contains(t, output, `<text:list text:style-name="Ordered List">`)
	assert.Contains(t, output, "First")
	assert.Contains(t, output, "Second")

	// Thematic break
	assert.Contains(t, output, `<text:p text:style-name="Thematic Break"/>`)

	// Blockquote
	assert.Contains(t, output, `<text:p text:style-name="Blockquote">`)
	assert.Contains(t, output, "A famous quote.")

	// Code block
	assert.Contains(t, output, `<text:p text:style-name="Code Block">`)
	assert.Contains(t, output, "some<text:s/>code")

	// Link
	assert.Contains(t, output, `<text:a xlink:href="https://example.com">`)
	assert.Contains(t, output, "our site")
	assert.Contains(t, output, "</text:a>")
}

func TestEmptyDocument(t *testing.T) {
	source := ""
	output := renderMarkdown(t, source)

	// Even an empty document should have the XML structure.
	assert.Contains(t, output, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, output, "<office:document-content")
	assert.Contains(t, output, "</office:document-content>")
}

func TestListStackReset(t *testing.T) {
	// Render two documents in a row to confirm listStack is reset.
	src1 := []byte("- item\n")
	src2 := []byte("1. item\n")

	parser := goldmark.DefaultParser()
	r := NewRenderer("serif", "monospace")

	var buf1 bytes.Buffer
	err := r.Render(&buf1, src1, parser.Parse(text.NewReader(src1)))
	require.NoError(t, err)
	assert.Contains(t, buf1.String(), `<text:list text:style-name="Unordered List">`)

	var buf2 bytes.Buffer
	err = r.Render(&buf2, src2, parser.Parse(text.NewReader(src2)))
	require.NoError(t, err)
	assert.Contains(t, buf2.String(), `<text:list text:style-name="Ordered List">`)
}

func TestNestedList(t *testing.T) {
	source := "- outer\n  - inner\n"
	output := renderMarkdown(t, source)

	// Should have nested list structures.
	listCount := strings.Count(output, "<text:list ")
	assert.GreaterOrEqual(t, listCount, 2, "expected at least two text:list elements for nested list")
	assert.Contains(t, output, "outer")
	assert.Contains(t, output, "inner")
}

func TestCodeBlockWithMultipleSpaces(t *testing.T) {
	source := "```\na  b   c\n```\n"
	output := renderMarkdown(t, source)

	// Each space in code should be replaced with <text:s/>
	assert.Contains(t, output, "a<text:s/><text:s/>b<text:s/><text:s/><text:s/>c")
}

func TestTextSoftLineBreak(t *testing.T) {
	// In markdown, a single newline within a paragraph is a soft break.
	// The renderer adds a space between sibling text nodes.
	source := "line one\nline two\n"
	output := renderMarkdown(t, source)

	assert.Contains(t, output, `<text:p text:style-name="Paragraph">`)
	assert.Contains(t, output, "line one")
	assert.Contains(t, output, "line two")
}
