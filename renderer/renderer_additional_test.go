package renderer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/goldmark/extension"
	goldmark_parser "github.com/pgavlin/goldmark/parser"
	goldmark_renderer "github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderMarkdown is a helper that parses and renders markdown with the given options.
// It returns the rendered output, the Renderer, and any error.
func renderMarkdown(t *testing.T, input string, options ...RendererOption) (string, *Renderer) {
	t.Helper()

	source := []byte(input)
	parser := goldmark.DefaultParser()
	document := parser.Parse(text.NewReader(source))

	var buf bytes.Buffer
	r := New(options...)
	gmr := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	err := gmr.Render(&buf, source, document)
	require.NoError(t, err)

	return buf.String(), r
}

// renderMarkdownWithTables is a helper that parses and renders markdown with table support enabled.
func renderMarkdownWithTables(t *testing.T, input string, options ...RendererOption) (string, *Renderer) {
	t.Helper()

	source := []byte(input)
	parser := goldmark.DefaultParser()
	parser.AddOptions(goldmark_parser.WithParagraphTransformers(
		util.Prioritized(extension.NewTableParagraphTransformer(), 200),
	))
	document := parser.Parse(text.NewReader(source))

	var buf bytes.Buffer
	r := New(options...)
	gmr := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	err := gmr.Render(&buf, source, document)
	require.NoError(t, err)

	return buf.String(), r
}

// TestTableRendering verifies that GFM tables are rendered with Unicode box-drawing borders.
func TestTableRendering(t *testing.T) {
	input := "| A | B |\n| - | - |\n| 1 | 2 |\n"

	output, _ := renderMarkdownWithTables(t, input, WithTheme(styles.Pulumi))

	stripped := ansi.Strip(output)

	// Check for box-drawing characters in the output.
	assert.True(t, strings.ContainsRune(stripped, '╭'), "output should contain top-left corner (╭)")
	assert.True(t, strings.ContainsRune(stripped, '┬'), "output should contain top-join (┬)")
	assert.True(t, strings.ContainsRune(stripped, '╮'), "output should contain top-right corner (╮)")
	assert.True(t, strings.ContainsRune(stripped, '├'), "output should contain middle-left (├)")
	assert.True(t, strings.ContainsRune(stripped, '┼'), "output should contain middle-join (┼)")
	assert.True(t, strings.ContainsRune(stripped, '┤'), "output should contain middle-right (┤)")
	assert.True(t, strings.ContainsRune(stripped, '╰'), "output should contain bottom-left corner (╰)")
	assert.True(t, strings.ContainsRune(stripped, '┴'), "output should contain bottom-join (┴)")
	assert.True(t, strings.ContainsRune(stripped, '╯'), "output should contain bottom-right corner (╯)")
	assert.True(t, strings.ContainsRune(stripped, '│'), "output should contain vertical border (│)")
	assert.True(t, strings.ContainsRune(stripped, '─'), "output should contain horizontal border (─)")

	// The cell contents should be present.
	assert.True(t, strings.Contains(stripped, "A"), "output should contain header cell A")
	assert.True(t, strings.Contains(stripped, "B"), "output should contain header cell B")
	assert.True(t, strings.Contains(stripped, "1"), "output should contain data cell 1")
	assert.True(t, strings.Contains(stripped, "2"), "output should contain data cell 2")
}

// TestTableRenderingMultipleRows verifies tables with multiple rows render correctly.
func TestTableRenderingMultipleRows(t *testing.T) {
	input := "| Name | Value |\n| ---- | ----- |\n| foo  | 10    |\n| bar  | 20    |\n"

	output, _ := renderMarkdownWithTables(t, input, WithTheme(styles.Pulumi))
	stripped := ansi.Strip(output)

	// Verify both rows are present in the output.
	assert.True(t, strings.Contains(stripped, "foo"), "output should contain 'foo'")
	assert.True(t, strings.Contains(stripped, "bar"), "output should contain 'bar'")
	assert.True(t, strings.Contains(stripped, "10"), "output should contain '10'")
	assert.True(t, strings.Contains(stripped, "20"), "output should contain '20'")

	// Should have top, header-separator, and bottom borders.
	assert.True(t, strings.ContainsRune(stripped, '╭'), "should have top border")
	assert.True(t, strings.ContainsRune(stripped, '├'), "should have middle border")
	assert.True(t, strings.ContainsRune(stripped, '╰'), "should have bottom border")
}

// TestTableRenderingWithoutTheme verifies that tables render without a theme (no ANSI codes).
func TestTableRenderingWithoutTheme(t *testing.T) {
	input := "| X | Y |\n| - | - |\n| a | b |\n"

	output, _ := renderMarkdownWithTables(t, input)

	// Without a theme, there should be no ANSI escape sequences.
	assert.Equal(t, output, ansi.Strip(output), "output without theme should have no ANSI escape codes")

	// But box-drawing characters should still be present.
	assert.True(t, strings.ContainsRune(output, '╭'), "output should contain top-left corner")
	assert.True(t, strings.ContainsRune(output, '╯'), "output should contain bottom-right corner")
	assert.True(t, strings.Contains(output, "a"), "output should contain cell 'a'")
	assert.True(t, strings.Contains(output, "b"), "output should contain cell 'b'")
}

// TestHyperlinkRendering verifies that hyperlink mode changes link output.
func TestHyperlinkRendering(t *testing.T) {
	input := "[click here](http://example.com)\n"

	// Without hyperlinks: standard markdown link syntax should appear.
	outputNormal, _ := renderMarkdown(t, input)
	assert.True(t, strings.Contains(outputNormal, "[click here]"), "normal mode should contain link text in brackets")
	assert.True(t, strings.Contains(outputNormal, "(http://example.com)"), "normal mode should contain link destination")

	// With hyperlinks: the destination URL should be omitted from the text.
	outputHyper, _ := renderMarkdown(t, input, WithHyperlinks(true), WithTheme(styles.Pulumi))
	stripped := ansi.Strip(outputHyper)
	assert.True(t, strings.Contains(stripped, "click here"), "hyperlink mode should contain the link text")
	assert.False(t, strings.Contains(stripped, "(http://example.com)"), "hyperlink mode should not contain the URL destination")
	assert.False(t, strings.Contains(stripped, "[click here]"), "hyperlink mode should not have bracket wrapping")
}

// TestHyperlinkRenderingDiffersFromNormal verifies that output differs between hyperlink
// and non-hyperlink modes.
func TestHyperlinkRenderingDiffersFromNormal(t *testing.T) {
	input := "[Go docs](https://go.dev)\n"

	outputNormal, _ := renderMarkdown(t, input)
	outputHyper, _ := renderMarkdown(t, input, WithHyperlinks(true), WithTheme(styles.Pulumi))

	assert.NotEqual(t, outputNormal, outputHyper, "hyperlink output should differ from normal output")
}

// TestPushStylePopStyleWithTheme verifies that rendering with a theme produces ANSI SGR sequences.
func TestPushStylePopStyleWithTheme(t *testing.T) {
	input := "# Hello\n"

	output, _ := renderMarkdown(t, input, WithTheme(styles.Pulumi))

	// With the Pulumi theme, headings should have SGR sequences.
	assert.True(t, strings.Contains(output, "\033["), "themed output should contain ANSI escape sequences (ESC[)")
	assert.NotEqual(t, output, ansi.Strip(output), "themed output should differ from its stripped version")

	// The stripped output should still contain the heading text.
	stripped := ansi.Strip(output)
	assert.True(t, strings.Contains(stripped, "# Hello"), "stripped output should contain heading text")
}

// TestPushStylePopStyleNoTheme verifies that rendering without a theme produces no ANSI SGR sequences.
func TestPushStylePopStyleNoTheme(t *testing.T) {
	input := "# Hello\n"

	output, _ := renderMarkdown(t, input)

	// Without a theme, there should be no ANSI escape sequences.
	assert.Equal(t, output, ansi.Strip(output), "output without theme should have no ANSI escape sequences")
}

// TestPushStylePopStyleNested verifies that nested styled elements produce nested ANSI SGR sequences.
func TestPushStylePopStyleNested(t *testing.T) {
	input := "# **bold heading**\n"

	output, _ := renderMarkdown(t, input, WithTheme(styles.Pulumi))

	// Should contain ANSI sequences.
	assert.True(t, strings.Contains(output, "\033["), "output should contain ANSI escape sequences")

	stripped := ansi.Strip(output)
	assert.True(t, strings.Contains(stripped, "bold heading"), "stripped output should contain text")

	// The raw output should contain multiple SGR sequences (for heading and bold).
	count := strings.Count(output, "\033[")
	assert.True(t, count >= 2, "nested styled content should produce multiple SGR sequences, got %d", count)
}

// TestSpanTreeEmphasis verifies that rendering *emphasis* produces an Emphasis node in the span tree.
func TestSpanTreeEmphasis(t *testing.T) {
	input := "*emphasis*\n"

	_, r := renderMarkdown(t, input)
	tree := r.SpanTree()
	require.NotNil(t, tree, "span tree should not be nil")

	// Document -> Paragraph -> children should include emphasis.
	found := false
	var walk func(span *NodeSpan)
	walk = func(span *NodeSpan) {
		if _, ok := span.Node.(*ast.Emphasis); ok {
			found = true
		}
		for _, child := range span.Children {
			walk(child)
		}
	}
	walk(tree)

	assert.True(t, found, "span tree should contain an Emphasis node")
}

// TestSpanTreeEmphasisBounds verifies that the span for emphasis covers the correct byte range.
// Note: The emphasis span opens before writing the opening marker and closes before writing
// the closing marker, so it covers "*world" (opening marker + text) but not the trailing "*".
func TestSpanTreeEmphasisBounds(t *testing.T) {
	input := "hello *world*\n"

	output, r := renderMarkdown(t, input)
	tree := r.SpanTree()
	require.NotNil(t, tree)

	var emphSpan *NodeSpan
	var walk func(span *NodeSpan)
	walk = func(span *NodeSpan) {
		if _, ok := span.Node.(*ast.Emphasis); ok {
			emphSpan = span
		}
		for _, child := range span.Children {
			walk(child)
		}
	}
	walk(tree)

	require.NotNil(t, emphSpan, "should find an emphasis span")
	assert.True(t, emphSpan.Start >= 0 && emphSpan.Start < len(output), "emphasis start should be within output bounds")
	assert.True(t, emphSpan.End > emphSpan.Start && emphSpan.End <= len(output), "emphasis end should be after start and within output bounds")

	// The span opens before the opening "*" and closes before the closing "*",
	// so it covers the opening marker and the text content.
	spanText := output[emphSpan.Start:emphSpan.End]
	assert.True(t, strings.HasPrefix(spanText, "*"), "emphasis span should start with the opening marker")
	assert.True(t, strings.Contains(spanText, "world"), "emphasis span should contain the emphasized text")
}

// TestSpanTreeLink verifies that rendering a link produces a Link node in the span tree.
func TestSpanTreeLink(t *testing.T) {
	input := "[text](http://example.com)\n"

	_, r := renderMarkdown(t, input)
	tree := r.SpanTree()
	require.NotNil(t, tree)

	found := false
	var walk func(span *NodeSpan)
	walk = func(span *NodeSpan) {
		if _, ok := span.Node.(*ast.Link); ok {
			found = true
		}
		for _, child := range span.Children {
			walk(child)
		}
	}
	walk(tree)

	assert.True(t, found, "span tree should contain a Link node")
}

// TestSpanTreeLinkBounds verifies that the Link span covers the correct byte range.
// Note: The link span opens before writing "[" and closes before writing "](url)",
// so it covers the opening bracket and the link text, but not the destination.
func TestSpanTreeLinkBounds(t *testing.T) {
	input := "[text](http://example.com)\n"

	output, r := renderMarkdown(t, input)
	tree := r.SpanTree()
	require.NotNil(t, tree)

	var linkSpan *NodeSpan
	var walk func(span *NodeSpan)
	walk = func(span *NodeSpan) {
		if _, ok := span.Node.(*ast.Link); ok {
			linkSpan = span
		}
		for _, child := range span.Children {
			walk(child)
		}
	}
	walk(tree)

	require.NotNil(t, linkSpan, "should find a link span")
	spanText := output[linkSpan.Start:linkSpan.End]
	assert.True(t, strings.HasPrefix(spanText, "["), "link span should start with '['")
	assert.True(t, strings.Contains(spanText, "text"), "link span should contain the link text")
}

// TestOpenBlockCloseBlockMultipleParagraphs verifies that multiple paragraphs are separated
// by blank lines in the output.
func TestOpenBlockCloseBlockMultipleParagraphs(t *testing.T) {
	input := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph.\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "First paragraph."), "output should contain first paragraph")
	assert.True(t, strings.Contains(output, "Second paragraph."), "output should contain second paragraph")
	assert.True(t, strings.Contains(output, "Third paragraph."), "output should contain third paragraph")

	// The paragraphs should be separated by blank lines.
	assert.True(t, strings.Contains(output, "\n\n"), "paragraphs should be separated by blank lines")

	// Verify the exact output matches the input (since no styling is applied).
	assert.Equal(t, input, output, "output should match input for simple paragraphs without theme")
}

// TestOpenBlockCloseBlockHeadingsAndParagraphs verifies that blocks of different types
// are properly separated.
func TestOpenBlockCloseBlockHeadingsAndParagraphs(t *testing.T) {
	input := "# Heading\n\nA paragraph.\n"

	output, _ := renderMarkdown(t, input)

	assert.Equal(t, input, output, "output should match input for heading followed by paragraph")
}

// TestBlockquotePrefix verifies that blockquotes are rendered with the "> " prefix.
func TestBlockquotePrefix(t *testing.T) {
	input := "> This is a quote.\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "> "), "blockquote output should contain '> ' prefix")
	assert.True(t, strings.Contains(output, "This is a quote."), "blockquote output should contain the quoted text")
}

// TestBlockquotePrefixNested verifies that nested blockquotes have the correct indentation.
func TestBlockquotePrefixNested(t *testing.T) {
	input := "> > Nested quote.\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "> > "), "nested blockquote should contain '> > ' prefix")
	assert.True(t, strings.Contains(output, "Nested quote."), "nested blockquote should contain the quoted text")
}

// TestBlockquotePrefixMultiLine verifies that a multi-line blockquote paragraph renders correctly.
// The renderer produces a single blockquote prefix on the first line and continuation lines
// follow the paragraph's soft line break behavior.
func TestBlockquotePrefixMultiLine(t *testing.T) {
	input := "> Line one.\n> Line two.\n"

	output, _ := renderMarkdown(t, input)

	// The output should contain the blockquote text.
	assert.True(t, strings.Contains(output, "Line one."), "output should contain 'Line one.'")
	assert.True(t, strings.Contains(output, "Line two."), "output should contain 'Line two.'")

	// The first content line should have the "> " prefix.
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	require.True(t, len(lines) >= 1, "should have at least one line")
	assert.True(t, strings.HasPrefix(lines[0], "> "), "first blockquote line should start with '> ', got: %q", lines[0])
}

// TestBlockquoteWithTheme verifies that blockquotes have ANSI styling when a theme is used.
func TestBlockquoteWithTheme(t *testing.T) {
	input := "> Styled quote.\n"

	output, _ := renderMarkdown(t, input, WithTheme(styles.Pulumi))

	assert.True(t, strings.Contains(output, "\033["), "themed blockquote should contain ANSI escape sequences")

	stripped := ansi.Strip(output)
	assert.True(t, strings.Contains(stripped, "Styled quote."), "stripped output should contain the quote text")
}

// TestUnorderedListRendering verifies that unordered lists have the correct markers.
func TestUnorderedListRendering(t *testing.T) {
	input := "- Item one\n- Item two\n- Item three\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "- Item one"), "output should contain '- Item one'")
	assert.True(t, strings.Contains(output, "- Item two"), "output should contain '- Item two'")
	assert.True(t, strings.Contains(output, "- Item three"), "output should contain '- Item three'")
}

// TestOrderedListRendering verifies that ordered lists have the correct numeric markers.
func TestOrderedListRendering(t *testing.T) {
	input := "1. First\n2. Second\n3. Third\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "1."), "output should contain '1.'")
	assert.True(t, strings.Contains(output, "2."), "output should contain '2.'")
	assert.True(t, strings.Contains(output, "3."), "output should contain '3.'")
	assert.True(t, strings.Contains(output, "First"), "output should contain 'First'")
	assert.True(t, strings.Contains(output, "Second"), "output should contain 'Second'")
	assert.True(t, strings.Contains(output, "Third"), "output should contain 'Third'")
}

// TestOrderedListStartingIndex verifies that ordered lists starting at a number other than 1
// render correctly.
func TestOrderedListStartingIndex(t *testing.T) {
	input := "3. Three\n4. Four\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "3."), "output should contain '3.'")
	assert.True(t, strings.Contains(output, "4."), "output should contain '4.'")
}

// TestNestedListRendering verifies that nested lists are properly indented.
func TestNestedListRendering(t *testing.T) {
	input := "- Outer\n  - Inner\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "Outer"), "output should contain 'Outer'")
	assert.True(t, strings.Contains(output, "Inner"), "output should contain 'Inner'")

	// The inner list item should be indented relative to the outer one.
	lines := strings.Split(output, "\n")
	var outerLine, innerLine string
	for _, line := range lines {
		if strings.Contains(line, "Outer") {
			outerLine = line
		}
		if strings.Contains(line, "Inner") {
			innerLine = line
		}
	}
	require.NotEmpty(t, outerLine, "should find outer line")
	require.NotEmpty(t, innerLine, "should find inner line")

	// The inner item should have more leading whitespace/prefix than the outer.
	outerIndent := len(outerLine) - len(strings.TrimLeft(outerLine, " "))
	innerIndent := len(innerLine) - len(strings.TrimLeft(innerLine, " "))
	assert.True(t, innerIndent > outerIndent, "inner list item should be more indented than outer (inner: %d, outer: %d)", innerIndent, outerIndent)
}

// TestFencedCodeBlockRendering verifies that fenced code blocks are rendered with fence markers.
func TestFencedCodeBlockRendering(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "```"), "output should contain fence markers (```)")
	assert.True(t, strings.Contains(output, "go"), "output should contain the language specifier")
	assert.True(t, strings.Contains(output, "fmt.Println"), "output should contain the code content")
}

// TestFencedCodeBlockWithTheme verifies that fenced code blocks have syntax highlighting
// when a theme is used.
func TestFencedCodeBlockWithTheme(t *testing.T) {
	input := "```go\npackage main\n\nfunc main() {\n}\n```\n"

	output, _ := renderMarkdown(t, input, WithTheme(styles.Pulumi))

	// With a theme, fenced code should contain ANSI escape sequences for syntax highlighting.
	assert.True(t, strings.Contains(output, "\033["), "themed fenced code should contain ANSI escape sequences")

	stripped := ansi.Strip(output)
	assert.True(t, strings.Contains(stripped, "package main"), "stripped output should contain 'package main'")
	assert.True(t, strings.Contains(stripped, "func main()"), "stripped output should contain 'func main()'")
}

// TestFencedCodeBlockNoWrap verifies that word wrapping does not affect content inside
// fenced code blocks.
func TestFencedCodeBlockNoWrap(t *testing.T) {
	longLine := strings.Repeat("x", 200)
	input := "```\n" + longLine + "\n```\n"

	output, _ := renderMarkdown(t, input, WithWordWrap(80))

	// The long line should not be wrapped inside a code block.
	assert.True(t, strings.Contains(output, longLine), "code block content should not be word-wrapped")
}

// TestMeasureTextWithANSI verifies that measureText correctly handles ANSI escape codes
// by not counting them toward the visible width.
func TestMeasureTextWithANSI(t *testing.T) {
	r := New()

	// Plain text should have width equal to its rune count.
	assert.Equal(t, 5, r.measureText([]byte("hello")), "plain text 'hello' should have width 5")

	// Text with ANSI sequences should not count the escape sequences.
	ansiText := "\033[1mbold\033[0m"
	assert.Equal(t, 4, r.measureText([]byte(ansiText)), "ANSI-styled 'bold' should have visible width 4")

	// Empty string.
	assert.Equal(t, 0, r.measureText([]byte("")), "empty string should have width 0")

	// Text with color sequences.
	colorText := "\033[38;2;255;0;0mred\033[0m"
	assert.Equal(t, 3, r.measureText([]byte(colorText)), "colored 'red' should have visible width 3")
}

// TestMeasureTextWithWideCJK verifies that measureText correctly handles wide CJK characters.
func TestMeasureTextWithWideCJK(t *testing.T) {
	r := New()

	// CJK characters are typically double-width.
	cjkText := "\u4e16\u754c" // "world" in Chinese (2 chars, each double-width)
	width := r.measureText([]byte(cjkText))
	assert.Equal(t, 4, width, "two CJK characters should have width 4 (each is double-width)")
}

// TestRenderingPreservesSimpleMarkdown verifies that simple markdown without themes
// round-trips correctly through the renderer.
func TestRenderingPreservesSimpleMarkdown(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"paragraph", "Hello, world.\n"},
		{"heading level 1", "# Heading 1\n"},
		{"heading level 2", "## Heading 2\n"},
		{"heading level 3", "### Heading 3\n"},
		{"emphasis", "*emphasis*\n"},
		{"strong", "**strong**\n"},
		{"inline code", "`code`\n"},
		{"thematic break", "***\n"},
		{"link", "[link](http://example.com)\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, _ := renderMarkdown(t, tc.input)
			assert.Equal(t, tc.input, output, "simple markdown should round-trip without a theme")
		})
	}
}

// TestWordWrapBlockquote verifies that word wrapping works within blockquotes.
func TestWordWrapBlockquote(t *testing.T) {
	longText := "> " + strings.Repeat("word ", 30) + "\n"

	output, _ := renderMarkdown(t, longText, WithWordWrap(40), WithSoftBreak(true))

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	require.True(t, len(lines) > 1, "long blockquote text should wrap into multiple lines")

	// The first line of the blockquote should have the "> " prefix.
	assert.True(t, strings.HasPrefix(lines[0], "> "), "first line should start with '> ', got: %q", lines[0])

	// All lines should contain some of the blockquote text.
	for _, line := range lines {
		if line == "" {
			continue
		}
		assert.True(t, strings.Contains(line, "word"), "each line should contain text from the blockquote, got: %q", line)
	}
}

// TestCodeBlockIndentation verifies that indented code blocks are rendered with 4-space indentation.
func TestCodeBlockIndentation(t *testing.T) {
	input := "    code line 1\n    code line 2\n"

	output, _ := renderMarkdown(t, input)

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(line, "    "), "code block lines should have 4-space indent, got: %q", line)
	}
}

// TestHeadingLevels verifies that different heading levels are rendered with the correct
// number of # characters.
func TestHeadingLevels(t *testing.T) {
	for level := 1; level <= 6; level++ {
		prefix := strings.Repeat("#", level)
		input := prefix + " Heading\n"

		t.Run(input, func(t *testing.T) {
			output, _ := renderMarkdown(t, input)
			assert.True(t, strings.HasPrefix(output, prefix+" "), "heading level %d should start with '%s '", level, prefix)
		})
	}
}

// TestHeadingWithThemeLevels verifies that headings at different levels use different
// style tokens (GenericHeading for 1-2, GenericSubheading for 3+).
func TestHeadingWithThemeLevels(t *testing.T) {
	input1 := "# Level 1\n"
	input3 := "### Level 3\n"

	output1, _ := renderMarkdown(t, input1, WithTheme(styles.Pulumi))
	output3, _ := renderMarkdown(t, input3, WithTheme(styles.Pulumi))

	// Both should contain ANSI sequences.
	assert.True(t, strings.Contains(output1, "\033["), "level 1 heading should have ANSI styling")
	assert.True(t, strings.Contains(output3, "\033["), "level 3 heading should have ANSI styling")

	// The ANSI styling should differ between level 1 (GenericHeading/bold) and level 3 (GenericSubheading/no bold).
	stripped1 := ansi.Strip(output1)
	stripped3 := ansi.Strip(output3)
	assert.True(t, strings.Contains(stripped1, "Level 1"), "level 1 heading text should be present")
	assert.True(t, strings.Contains(stripped3, "Level 3"), "level 3 heading text should be present")

	// Level 1 uses GenericHeading which has "bold" in the Pulumi theme; level 3 uses GenericSubheading
	// which does not. The raw ANSI sequences should differ.
	// Extract just the ANSI prefix by stripping the text portions.
	ansi1 := strings.ReplaceAll(output1, stripped1, "")
	ansi3 := strings.ReplaceAll(output3, stripped3, "")
	assert.NotEqual(t, ansi1, ansi3, "ANSI styling should differ between heading levels 1 and 3")
}

// TestAutoLink verifies that autolinks are rendered with angle brackets.
func TestAutoLink(t *testing.T) {
	input := "<http://example.com>\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "<http://example.com>"), "autolink should be rendered with angle brackets")
}

// TestThematicBreak verifies that thematic breaks render as a dim horizontal line.
func TestThematicBreak(t *testing.T) {
	input := "***\n"

	output, _ := renderMarkdown(t, input, WithTheme(styles.Pulumi))

	assert.True(t, strings.Contains(output, "─"), "thematic break should render as a horizontal line")
	assert.True(t, strings.Contains(output, "\033[2m"), "thematic break should be dim")
}

// TestNodeSpanContains verifies the Contains method on NodeSpan.
func TestNodeSpanContains(t *testing.T) {
	span := &NodeSpan{Start: 5, End: 10}

	assert.True(t, span.Contains(5), "offset 5 should be contained (start is inclusive)")
	assert.True(t, span.Contains(7), "offset 7 should be contained")
	assert.True(t, span.Contains(9), "offset 9 should be contained")
	assert.False(t, span.Contains(4), "offset 4 should not be contained")
	assert.False(t, span.Contains(10), "offset 10 should not be contained (end is exclusive)")
	assert.False(t, span.Contains(100), "offset 100 should not be contained")
}

// TestSpanTreeStructure verifies the overall structure of the span tree for a simple document.
func TestSpanTreeStructure(t *testing.T) {
	input := "Hello.\n\nWorld.\n"

	_, r := renderMarkdown(t, input)
	tree := r.SpanTree()
	require.NotNil(t, tree)

	// Root should be the Document node.
	_, ok := tree.Node.(*ast.Document)
	assert.True(t, ok, "root span should be a Document node")

	// Should have two paragraph children.
	assert.Len(t, tree.Children, 2, "document should have two paragraph children")
	_, ok = tree.Children[0].Node.(*ast.Paragraph)
	assert.True(t, ok, "first child should be a Paragraph")
	_, ok = tree.Children[1].Node.(*ast.Paragraph)
	assert.True(t, ok, "second child should be a Paragraph")

	// Spans should cover the full output.
	assert.Equal(t, 0, tree.Start, "document span should start at 0")
}

// TestSpanTreePrevNext verifies that the Prev/Next pointers in the span tree form a valid
// preorder traversal.
func TestSpanTreePrevNext(t *testing.T) {
	input := "# Heading\n\nParagraph with *emphasis*.\n"

	_, r := renderMarkdown(t, input)
	tree := r.SpanTree()
	require.NotNil(t, tree)

	// Walk the tree using Next pointers and verify Prev links.
	var nodes []*NodeSpan
	for span := tree; span != nil; span = span.Next {
		nodes = append(nodes, span)
	}

	assert.True(t, len(nodes) > 1, "should have multiple spans in the tree")

	// Verify Prev pointers.
	for i := 1; i < len(nodes); i++ {
		assert.Equal(t, nodes[i-1], nodes[i].Prev, "Prev pointer at index %d should point to node at index %d", i, i-1)
	}

	// First node should have no Prev.
	assert.Nil(t, nodes[0].Prev, "first node should have nil Prev")
}

// TestSoftBreakEnabled verifies that with soft break enabled, soft line breaks become spaces.
func TestSoftBreakEnabled(t *testing.T) {
	input := "line one\nline two\n"

	output, _ := renderMarkdown(t, input, WithSoftBreak(true))

	// With soft break, newline within a paragraph becomes a space.
	assert.True(t, strings.Contains(output, "line one line two"), "soft break should convert newline to space")
}

// TestSoftBreakDisabled verifies that with soft break disabled (default), soft line breaks
// remain as newlines.
func TestSoftBreakDisabled(t *testing.T) {
	input := "line one\nline two\n"

	output, _ := renderMarkdown(t, input)

	// Without soft break, the lines should remain separate.
	assert.True(t, strings.Contains(output, "line one\nline two"), "without soft break, newlines should be preserved")
}

// TestPadToWrap verifies that with padding enabled, lines are padded to the wrap width.
func TestPadToWrap(t *testing.T) {
	input := "Short.\n"

	output, _ := renderMarkdown(t, input, WithWordWrap(40), WithPad(true))

	// The line should be padded to 40 characters wide.
	lines := strings.Split(output, "\n")
	require.True(t, len(lines) >= 1, "should have at least one line")

	// The first line (the paragraph content line) should be padded.
	firstLine := lines[0]
	width := ansi.StringWidth(firstLine)
	assert.Equal(t, 40, width, "padded line should have width equal to wrap width (40), got %d", width)
}

// TestCodeSpanRendering verifies that inline code spans are rendered with backticks.
func TestCodeSpanRendering(t *testing.T) {
	input := "Use `fmt.Println` to print.\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "`fmt.Println`"), "output should contain backtick-delimited code span")
}

// TestHardLineBreak verifies that a hard line break (backslash at end of line) is rendered correctly.
func TestHardLineBreak(t *testing.T) {
	input := "Line one\\\nLine two\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "Line one\\\n"), "hard line break should be preserved as backslash-newline")
	assert.True(t, strings.Contains(output, "Line two"), "text after hard line break should be present")
}

// TestRawHTML verifies that raw HTML is rendered as-is.
func TestRawHTML(t *testing.T) {
	input := "Text with <em>raw html</em> inside.\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "<em>"), "output should contain raw HTML tags")
	assert.True(t, strings.Contains(output, "</em>"), "output should contain closing raw HTML tags")
}

// TestTableRenderingWithThemeStyling verifies that the Pulumi theme applies distinct styles
// to table header and body rows.
func TestTableRenderingWithThemeStyling(t *testing.T) {
	input := "| H1 | H2 |\n| -- | -- |\n| D1 | D2 |\n| D3 | D4 |\n"

	output, _ := renderMarkdownWithTables(t, input, WithTheme(styles.Pulumi))

	// With the Pulumi theme, table output should contain ANSI sequences.
	assert.True(t, strings.Contains(output, "\033["), "themed table should contain ANSI escape sequences")

	stripped := ansi.Strip(output)
	assert.True(t, strings.Contains(stripped, "H1"), "table should contain header H1")
	assert.True(t, strings.Contains(stripped, "D1"), "table should contain data D1")
	assert.True(t, strings.Contains(stripped, "D3"), "table should contain data D3")
}

// TestWordWrapPreservesCodeSpan verifies that word wrapping does not break inside inline code spans.
func TestWordWrapPreservesCodeSpan(t *testing.T) {
	input := "This is some text with `inline code here` in the middle.\n"

	output, _ := renderMarkdown(t, input, WithWordWrap(30), WithSoftBreak(true))

	// The inline code should appear intact (not broken across lines).
	assert.True(t, strings.Contains(output, "`inline code here`"),
		"inline code span should not be broken by word wrap")
}

// TestRendererOptions verifies that renderer options are correctly applied.
func TestRendererOptions(t *testing.T) {
	r := New(
		WithWordWrap(80),
		WithHyperlinks(true),
		WithSoftBreak(true),
		WithPad(true),
	)

	assert.Equal(t, 80, r.wordWrap, "word wrap should be 80")
	assert.True(t, r.hyperlinks, "hyperlinks should be enabled")
	assert.True(t, r.softBreak, "soft break should be enabled")
	assert.Equal(t, []int{80}, r.padToWrap, "pad should be set to wrap width")
}

// TestRendererOptionsTheme verifies that WithTheme correctly sets the theme.
func TestRendererOptionsTheme(t *testing.T) {
	r := New(WithTheme(styles.Pulumi))

	assert.NotNil(t, r.theme, "theme should not be nil")
	assert.Equal(t, styles.Pulumi, r.theme, "theme should be the Pulumi theme")
}

// TestRendererOptionsDefaults verifies that a renderer created with no options has sensible defaults.
func TestRendererOptionsDefaults(t *testing.T) {
	r := New()

	assert.Nil(t, r.theme, "default theme should be nil")
	assert.Equal(t, 0, r.wordWrap, "default word wrap should be 0 (disabled)")
	assert.False(t, r.hyperlinks, "default hyperlinks should be false")
	assert.False(t, r.softBreak, "default soft break should be false")
	assert.Empty(t, r.padToWrap, "default pad should be empty")
}

// TestEmptyDocument verifies that rendering an empty document produces empty output.
func TestEmptyDocument(t *testing.T) {
	output, _ := renderMarkdown(t, "")

	assert.Equal(t, "", output, "empty document should produce empty output")
}

// TestImageSyntaxWithoutImageRendering verifies that image syntax is rendered as markdown
// when image rendering is disabled (the default).
func TestImageSyntaxWithoutImageRendering(t *testing.T) {
	input := "![alt text](http://example.com/image.png)\n"

	output, _ := renderMarkdown(t, input)

	assert.True(t, strings.Contains(output, "![alt text]"), "image should be rendered as markdown syntax")
	assert.True(t, strings.Contains(output, "(http://example.com/image.png)"), "image URL should be present")
}

// TestTableWrapping_ExceedsWidth verifies that when a table's total width exceeds the
// wrap width, no output line exceeds the wrap width.
func TestTableWrapping_ExceedsWidth(t *testing.T) {
	// Create a table with two roughly balanced columns that together exceed 40 columns.
	input := "| Header One | Header Two |\n| ---------- | ---------- |\n| Some content here | More content that is quite long |\n"

	output, _ := renderMarkdownWithTables(t, input, WithWordWrap(40), WithSoftBreak(true))
	stripped := ansi.Strip(output)

	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	for i, line := range lines {
		w := ansi.StringWidth(line)
		assert.True(t, w <= 40, "line %d should not exceed wrap width 40, got width %d: %q", i, w, line)
	}

	// All cell content should still be present (possibly split across lines).
	assert.True(t, strings.Contains(stripped, "Header"), "output should contain header text")
	assert.True(t, strings.Contains(stripped, "content"), "output should contain cell content")
	// Table should still have box-drawing borders.
	assert.True(t, strings.ContainsRune(stripped, '╭'), "output should contain top-left corner")
	assert.True(t, strings.ContainsRune(stripped, '╯'), "output should contain bottom-right corner")
}

// TestTableWrapping_CellContentWrapped verifies that long cell text appears across
// multiple lines within the row (multi-line rows).
func TestTableWrapping_CellContentWrapped(t *testing.T) {
	input := "| A | B |\n| - | - |\n| x | This text should be wrapped into multiple lines when rendered |\n"

	output, _ := renderMarkdownWithTables(t, input, WithWordWrap(30), WithSoftBreak(true))
	stripped := ansi.Strip(output)

	// The data row should span multiple lines (because the long cell wraps).
	// Count lines between the middle border (after header) and the bottom border.
	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	var dataLines int
	inData := false
	for _, line := range lines {
		if strings.ContainsRune(line, '├') {
			inData = true
			continue
		}
		if strings.ContainsRune(line, '╰') {
			break
		}
		if inData && strings.ContainsRune(line, '│') {
			dataLines++
		}
	}
	assert.True(t, dataLines > 1, "data row should span multiple lines due to cell wrapping, got %d data lines", dataLines)
}

// TestTableWrapping_NarrowTableUnchanged verifies that a table fitting within the
// wrap width renders identically to one rendered without a wrap width.
func TestTableWrapping_NarrowTableUnchanged(t *testing.T) {
	input := "| A | B |\n| - | - |\n| 1 | 2 |\n"

	outputNoWrap, _ := renderMarkdownWithTables(t, input)
	outputWithWrap, _ := renderMarkdownWithTables(t, input, WithWordWrap(80))

	assert.Equal(t, outputNoWrap, outputWithWrap, "narrow table should render identically with or without wrap width")
}

// TestTableWrapping_StyledCellNoStyleLeak verifies that styled text (e.g. link
// underline) in a wrapped table cell does not leak into padding or adjacent columns.
func TestTableWrapping_StyledCellNoStyleLeak(t *testing.T) {
	// Use a link with inline code in the first column and a long description
	// in the second so that ansi.Wrap splits the content across lines.
	input := "| Package | Description |\n|---------|-------------|\n| [`renderer`](https://pkg.go.dev/github.com/pgavlin/markdown-kit/renderer) | Terminal renderer with ANSI colorization, word wrapping, table rendering (Unicode box-drawing), image encoding (Kitty graphics protocol, ANSI), and document span tracking. |\n"

	output, _ := renderMarkdownWithTables(t, input, WithWordWrap(183), WithTheme(styles.GlamourDark), WithHyperlinks(true), WithSoftBreak(true))
	assertNoUnderlineLeak(t, output)
}

// TestTableWrapping_StyledCellStyleContinuity verifies that when cell content
// wraps with a theme that sets a table background, continuation lines preserve
// the row background color for padding.
func TestTableWrapping_StyledCellStyleContinuity(t *testing.T) {
	input := "| Package | Description |\n|---------|-------------|\n| [`renderer`](https://pkg.go.dev/github.com/pgavlin/markdown-kit/renderer) | Terminal renderer with ANSI colorization, word wrapping, table rendering (Unicode box-drawing), image encoding (Kitty graphics protocol, ANSI), and document span tracking. |\n"

	// Use the Pulumi theme which has an explicit table background color.
	output, _ := renderMarkdownWithTables(t, input, WithWordWrap(183), WithTheme(styles.Pulumi), WithHyperlinks(true), WithSoftBreak(true))

	lines := strings.Split(output, "\n")
	// Find the data row continuation line (second line of the first data row).
	// The data row starts after the header separator (├...┤). Look for the
	// second │-delimited line after that.
	var dataLines []string
	pastSep := false
	for _, line := range lines {
		stripped := ansi.Strip(line)
		if strings.ContainsRune(stripped, '├') {
			pastSep = true
			continue
		}
		if strings.ContainsRune(stripped, '╰') {
			break
		}
		if pastSep && strings.ContainsRune(stripped, '│') {
			dataLines = append(dataLines, line)
		}
	}

	require.True(t, len(dataLines) >= 2, "expected at least 2 data lines (wrapped content), got %d", len(dataLines))

	// The continuation line (dataLines[1]) should contain background color
	// sequences (48;2;...) for the row style, ensuring padding is styled.
	assert.True(t, strings.Contains(dataLines[1], "\033[48;2;"),
		"continuation line should have row background color for padding")
}

// TestTableWrapping_LinksInSpanTree verifies that links inside wrapped table
// cells appear in the renderer's span tree so they are navigable.
func TestTableWrapping_LinksInSpanTree(t *testing.T) {
	input := `| Package | Description |
|---------|-------------|
| [renderer](https://pkg.go.dev/renderer) | Terminal renderer |
| [view](https://pkg.go.dev/view) | Interactive model |
`

	_, r := renderMarkdownWithTables(t, input, WithWordWrap(60), WithTheme(styles.GlamourDark), WithHyperlinks(true), WithSoftBreak(true))

	tree := r.SpanTree()
	require.NotNil(t, tree, "span tree should not be nil")

	// Walk the span tree and collect Link nodes.
	var links []*NodeSpan
	for span := tree; span != nil; span = span.Next {
		if span.Node.Kind() == ast.KindLink {
			links = append(links, span)
		}
	}

	assert.Len(t, links, 2, "should find 2 link spans in the span tree")
	for i, link := range links {
		assert.True(t, link.Start < link.End, "link span %d should have positive width (start=%d, end=%d)", i, link.Start, link.End)
		l, ok := link.Node.(*ast.Link)
		require.True(t, ok, "link span %d should reference an ast.Link", i)
		assert.NotEmpty(t, string(l.Destination), "link span %d should have a destination", i)
	}
}

// TestTableWrapping_ColumnWidthDistribution verifies that narrow columns keep
// their natural width while wide columns absorb all the shrinkage.
func TestTableWrapping_ColumnWidthDistribution(t *testing.T) {
	// Column "ID" is narrow (natural width ~2), column "Description" is very wide.
	input := "| ID | Description |\n| -- | ----------- |\n| 42 | This is an extremely long description that will definitely need to be wrapped when rendered in a narrow terminal width |\n"

	output, _ := renderMarkdownWithTables(t, input, WithWordWrap(40), WithSoftBreak(true))
	stripped := ansi.Strip(output)

	// Find data row lines (between ├ separator and ╰ bottom border).
	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	var dataLines []string
	inData := false
	for _, line := range lines {
		if strings.ContainsRune(line, '├') {
			inData = true
			continue
		}
		if strings.ContainsRune(line, '╰') {
			break
		}
		if inData && strings.ContainsRune(line, '│') {
			dataLines = append(dataLines, line)
		}
	}
	require.True(t, len(dataLines) >= 1, "should have at least one data line")

	// The narrow "ID" column should keep its natural width — the value "42"
	// should appear on the first data line without being wrapped.
	// Extract the first cell content (between the first and second │).
	first := dataLines[0]
	parts := strings.SplitN(first, "│", 3)
	require.True(t, len(parts) >= 3, "data line should have at least 2 columns separated by │")
	narrowCell := strings.TrimSpace(parts[1])
	assert.Equal(t, "42", narrowCell, "narrow column should keep its natural width and show '42' without wrapping")

	// The wide column should have been wrapped (multiple data lines).
	assert.True(t, len(dataLines) > 1, "wide column should cause the data row to span multiple lines, got %d", len(dataLines))

	// On continuation lines, the narrow column cell should be empty (just spaces).
	for i := 1; i < len(dataLines); i++ {
		contParts := strings.SplitN(dataLines[i], "│", 3)
		if len(contParts) >= 3 {
			contNarrow := strings.TrimSpace(contParts[1])
			assert.Empty(t, contNarrow, "narrow column on continuation line %d should be empty (just padding)", i)
		}
	}
}

// TestTableWrapping_ExactWidth verifies that a table whose natural width
// (columns + borders) exactly equals the word wrap width renders correctly.
// This is a regression test: when the table width equaled wordWrap, the
// non-constraining path was taken and the renderer's word wrapping caused
// the trailing │ border to be pushed onto the next line.
func TestTableWrapping_ExactWidth(t *testing.T) {
	// Build a table whose natural width exactly equals the wrap width.
	// Col 1: 7 chars ("Header1"), Col 2: 7 chars ("Header2"), borders: 3 │.
	// Total = 7 + 7 + 3 = 17. Wrap at 17.
	input := "| Header1 | Header2 |\n| ------- | ------- |\n| AAAAAAA | BBBBBBB |\n"

	output, _ := renderMarkdownWithTables(t, input, WithWordWrap(17), WithSoftBreak(true))
	stripped := ansi.Strip(output)

	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	for i, line := range lines {
		w := ansi.StringWidth(line)
		assert.LessOrEqual(t, w, 17, "line %d should not exceed wrap width 17, got width %d: %q", i, w, line)
	}

	// Every content row (not border rows) should start and end with │.
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "╭") || strings.HasPrefix(trimmed, "├") || strings.HasPrefix(trimmed, "╰") {
			continue // border rows
		}
		assert.True(t, strings.HasPrefix(trimmed, "│"), "line %d should start with │: %q", i, trimmed)
		assert.True(t, strings.HasSuffix(trimmed, "│"), "line %d should end with │: %q", i, trimmed)
	}
}

// assertNoUnderlineLeak checks that no output line ends with underline active.
// When ansi.Wrap splits styled content across lines, inline styles (like link
// underline) can leak if the renderer doesn't reset them at cell boundaries.
func assertNoUnderlineLeak(t *testing.T, output string) {
	t.Helper()

	for i, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}

		underline := false
		j := 0
		for j < len(line) {
			if line[j] == '\033' && j+1 < len(line) && line[j+1] == '[' {
				end := strings.IndexByte(line[j:], 'm')
				if end < 0 {
					break
				}
				params := line[j+2 : j+end]
				j += end + 1
				for _, p := range strings.Split(params, ";") {
					switch p {
					case "0":
						underline = false
					case "4":
						underline = true
					case "24":
						underline = false
					}
				}
			} else {
				j++
			}
		}

		if underline {
			t.Errorf("line %d ends with underline active", i)
		}
	}
}

// TestExpandTabs verifies the expandTabs helper.
func TestExpandTabs(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		tabWidth int
		expected string
	}{
		{"no tabs", "hello", 8, "hello"},
		{"tab at start", "\thello", 8, "        hello"},
		{"tab at col 4", "1234\thello", 8, "1234    hello"},
		{"two tabs", "\t\thello", 8, "                hello"},
		{"tab width 4", "\thello", 4, "    hello"},
		{"mixed", "a\tb\tc", 8, "a       b       c"},
		{"newline resets col", "a\tb\na\tb", 8, "a       b\na       b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, expandTabs(tc.input, tc.tabWidth))
		})
	}
}

// TestFencedCodeBlockTabPadding verifies that fenced code blocks with tab characters
// have consistent line widths for background padding.
func TestFencedCodeBlockTabPadding(t *testing.T) {
	input := "```go\ntype Foo struct {\n\tBar string\n\tBaz int\n}\n```\n"

	output, _ := renderMarkdown(t, input, WithPad(true))

	// All non-empty lines within the code block should have the same width.
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	require.True(t, len(lines) >= 3, "should have code block lines")

	var widths []int
	for _, line := range lines {
		if line == "" {
			continue
		}
		widths = append(widths, ansi.StringWidth(line))
	}

	// All lines should have equal width (padded to the same maxWidth).
	for i, w := range widths {
		assert.Equal(t, widths[0], w, "line %d width %d should match first line width %d", i, w, widths[0])
	}
}

// TestFencedCodeBlockTabsExpandedToSpaces verifies that tab characters in code
// blocks are expanded to spaces in the rendered output.
func TestFencedCodeBlockTabsExpandedToSpaces(t *testing.T) {
	input := "```\n\thello\n```\n"

	output, _ := renderMarkdown(t, input)

	// The rendered output should not contain literal tab characters.
	assert.False(t, strings.Contains(output, "\t"), "rendered code block should not contain literal tabs")
	// The tab should be expanded to spaces.
	assert.True(t, strings.Contains(output, "        hello"), "tab should be expanded to 8 spaces")
}

// TestHyperlinkOSC8 verifies that OSC 8 sequences are emitted for links in hyperlink mode.
func TestHyperlinkOSC8(t *testing.T) {
	t.Run("regular link emits OSC 8", func(t *testing.T) {
		input := "[click here](https://example.com)\n"
		output, _ := renderMarkdown(t, input, WithHyperlinks(true))

		assert.Contains(t, output, ansi.SetHyperlink("https://example.com"))
		assert.Contains(t, output, ansi.ResetHyperlink())
		assert.Contains(t, output, "click here")
		// Should not contain markdown link syntax
		assert.NotContains(t, output, "](")
	})

	t.Run("autolink emits OSC 8", func(t *testing.T) {
		input := "<https://example.com>\n"
		output, _ := renderMarkdown(t, input, WithHyperlinks(true))

		assert.Contains(t, output, ansi.SetHyperlink("https://example.com"))
		assert.Contains(t, output, ansi.ResetHyperlink())
		// Should not contain angle brackets
		assert.NotContains(t, output, "<https://")
	})

	t.Run("no OSC 8 without hyperlinks mode", func(t *testing.T) {
		input := "[click here](https://example.com)\n"
		output, _ := renderMarkdown(t, input)

		assert.NotContains(t, output, ansi.SetHyperlink("https://example.com"))
		assert.NotContains(t, output, ansi.ResetHyperlink())
	})

	t.Run("OSC 8 contains correct URL", func(t *testing.T) {
		input := "[link1](https://one.com) and [link2](https://two.com)\n"
		output, _ := renderMarkdown(t, input, WithHyperlinks(true))

		assert.Contains(t, output, ansi.SetHyperlink("https://one.com"))
		assert.Contains(t, output, ansi.SetHyperlink("https://two.com"))
	})
}
