package frontmatter

import (
	"strings"
	"testing"

	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/goldmark/parser"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parse builds a Markdown parser with the frontmatter extension and
// parses source. Returns the document plus source bytes for downstream
// inspection.
func parse(t *testing.T, source string) (ast.Node, []byte) {
	t.Helper()
	md := goldmark.New(goldmark.WithExtensions(Extension))
	src := []byte(source)
	doc := md.Parser().Parse(text.NewReader(src))
	return doc, src
}

func TestParser_DocumentWithoutFrontmatter(t *testing.T) {
	doc, _ := parse(t, "# Heading\n\nBody text.\n")
	assert.Nil(t, Find(doc), "no frontmatter expected")
	// First child should be the heading, untouched.
	first := doc.FirstChild()
	require.NotNil(t, first)
	assert.Equal(t, ast.KindHeading, first.Kind())
}

func TestParser_BasicYAMLBlock(t *testing.T) {
	source := "---\ntitle: Hello\nauthor: world\n---\n\n# Body\n"
	doc, src := parse(t, source)

	b := Find(doc)
	require.NotNil(t, b, "expected frontmatter block")
	assert.Equal(t, FormatYAML, b.Format)

	body := string(Body(b, src))
	assert.Equal(t, "title: Hello\nauthor: world\n", body)

	// First non-frontmatter sibling is the heading.
	require.Equal(t, ast.KindHeading, b.NextSibling().Kind())
}

func TestParser_DecodeYAMLIntoStruct(t *testing.T) {
	source := "---\ntitle: Hello\ntags:\n  - a\n  - b\ndraft: true\n---\n\nbody\n"
	doc, src := parse(t, source)
	b := Find(doc)
	require.NotNil(t, b)

	var meta struct {
		Title string   `yaml:"title"`
		Tags  []string `yaml:"tags"`
		Draft bool     `yaml:"draft"`
	}
	require.NoError(t, Decode(b, src, &meta))
	assert.Equal(t, "Hello", meta.Title)
	assert.Equal(t, []string{"a", "b"}, meta.Tags)
	assert.True(t, meta.Draft)
}

func TestParser_DecodeIntoMap(t *testing.T) {
	source := "---\nfoo: 1\nbar: two\n---\n"
	doc, src := parse(t, source)
	b := Find(doc)
	require.NotNil(t, b)

	var m map[string]any
	require.NoError(t, Decode(b, src, &m))
	assert.EqualValues(t, 1, m["foo"])
	assert.Equal(t, "two", m["bar"])
}

func TestParser_EmptyBlock(t *testing.T) {
	// An empty frontmatter block is valid: parses to no fields.
	source := "---\n---\n\n# Body\n"
	doc, src := parse(t, source)
	b := Find(doc)
	require.NotNil(t, b, "empty block should still be detected")

	var m map[string]any
	require.NoError(t, Decode(b, src, &m))
	assert.Nil(t, m, "empty body decodes to nil map")
}

func TestParser_NoOpeningFenceMeansNoFrontmatter(t *testing.T) {
	// Document begins with a heading — first line is not "---".
	doc, _ := parse(t, "# Heading\n\n---\n\nThe ones above are thematic breaks.\n")
	assert.Nil(t, Find(doc))
}

func TestParser_LeadingBlankLineDisqualifies(t *testing.T) {
	// Per Jekyll/Hugo convention, a leading blank line means no
	// frontmatter — the "---" is a thematic break.
	doc, _ := parse(t, "\n---\ntitle: x\n---\n")
	assert.Nil(t, Find(doc),
		"frontmatter with leading blank line must not be detected")
}

func TestParser_OpeningFenceWithLeadingSpacesDisqualifies(t *testing.T) {
	// A "---" with leading whitespace is at column > 0.
	doc, _ := parse(t, " ---\ntitle: x\n---\n")
	assert.Nil(t, Find(doc))
}

func TestParser_NoClosingFenceMeansNoFrontmatter(t *testing.T) {
	// Without a closing fence we refuse to consume the document.
	source := "---\ntitle: x\nThis line is not YAML and there's no closing fence.\n"
	doc, _ := parse(t, source)
	assert.Nil(t, Find(doc),
		"missing closing fence should not silently swallow the body")
}

func TestParser_ThematicBreakInBodyNotConfusedWithFrontmatter(t *testing.T) {
	// A thematic break later in the document must not be misread as a
	// closing fence — frontmatter only opens at line 1.
	source := "# Title\n\nIntro\n\n---\n\nMore content.\n"
	doc, _ := parse(t, source)
	assert.Nil(t, Find(doc))
	// The body should still contain a thematic break node.
	var found bool
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Kind() == ast.KindThematicBreak {
			found = true
		}
	}
	assert.True(t, found, "thematic break in body should still parse")
}

func TestParser_ClosingFenceWithTrailingWhitespace(t *testing.T) {
	// "---  \n" with trailing spaces should still close the block.
	source := "---\ntitle: x\n---  \n\nbody\n"
	doc, src := parse(t, source)
	b := Find(doc)
	require.NotNil(t, b)
	body := string(Body(b, src))
	assert.Equal(t, "title: x\n", body)
}

func TestParser_FrontmatterWithoutTrailingNewline(t *testing.T) {
	// Source ends right at the closing fence (no trailing body, no
	// trailing newline).
	source := "---\ntitle: x\n---"
	doc, src := parse(t, source)
	b := Find(doc)
	require.NotNil(t, b)
	body := string(Body(b, src))
	assert.Equal(t, "title: x\n", body)
}

func TestParser_BodyAfterFrontmatterParsesNormally(t *testing.T) {
	source := "---\ntitle: x\n---\n\n# Heading\n\nA paragraph.\n"
	doc, _ := parse(t, source)

	// Walk children: frontmatter, then heading, then paragraph.
	kinds := []ast.NodeKind{}
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		kinds = append(kinds, c.Kind())
	}
	require.Equal(t, 3, len(kinds), "expected frontmatter + heading + paragraph; got %v", kinds)
	assert.Equal(t, KindFrontmatter, kinds[0])
	assert.Equal(t, ast.KindHeading, kinds[1])
	assert.Equal(t, ast.KindParagraph, kinds[2])
}

func TestParser_OnlyFrontmatter(t *testing.T) {
	// A document that is entirely frontmatter, nothing after.
	source := "---\ntitle: solo\n---\n"
	doc, src := parse(t, source)
	b := Find(doc)
	require.NotNil(t, b)

	var meta struct {
		Title string `yaml:"title"`
	}
	require.NoError(t, Decode(b, src, &meta))
	assert.Equal(t, "solo", meta.Title)
}

func TestParser_DecodeMalformedYAMLReturnsError(t *testing.T) {
	source := "---\nfoo: [unclosed\n---\n"
	doc, src := parse(t, source)
	b := Find(doc)
	require.NotNil(t, b, "fenced block should still be detected; YAML failure is a Decode concern")

	var m map[string]any
	err := Decode(b, src, &m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "frontmatter: parse YAML")
}

func TestParser_NilBlockHelpersDoNotPanic(t *testing.T) {
	assert.Nil(t, Body(nil, []byte("anything")))
	err := Decode(nil, []byte{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil block")
}

// CRLF line endings are common from Windows-authored files.
func TestParser_CRLFLineEndings(t *testing.T) {
	source := "---\r\ntitle: x\r\n---\r\n\r\n# Body\r\n"
	doc, src := parse(t, source)
	b := Find(doc)
	require.NotNil(t, b, "CRLF should not block detection")

	var meta struct {
		Title string `yaml:"title"`
	}
	require.NoError(t, Decode(b, src, &meta))
	assert.Equal(t, "x", meta.Title)
}

// The trigger char is '-'; lines starting with other characters must not
// invoke our parser. Cross-check by ensuring documents with leading
// "+++ " (TOML, not yet supported) or other punctuation aren't taken.
func TestParser_TOMLFenceIgnored(t *testing.T) {
	source := "+++\ntitle = \"x\"\n+++\n\n# Body\n"
	doc, _ := parse(t, source)
	assert.Nil(t, Find(doc), "TOML frontmatter is not yet supported")
}

func TestParser_LongFenceIsThematicBreak(t *testing.T) {
	// "----" (4 dashes) is a thematic break, not a YAML fence.
	source := "----\ntitle: x\n----\n"
	doc, _ := parse(t, source)
	assert.Nil(t, Find(doc),
		"a 4+ dash line is a thematic break, not a frontmatter fence")
}

// Ensure the parser can be registered via the parser-only API the
// rest of this repo uses (goldmark.DefaultParser() + AddOptions(...)),
// not only through goldmark.New(WithExtensions(...)).
func TestParser_WiresViaDefaultParser(t *testing.T) {
	p := goldmark.DefaultParser()
	p.AddOptions(parser.WithBlockParsers(util.Prioritized(NewParser(), 0)))

	src := []byte("---\ntitle: x\n---\n")
	doc := p.Parse(text.NewReader(src))
	b := Find(doc)
	require.NotNil(t, b)
	body := strings.TrimSpace(string(Body(b, src)))
	assert.Equal(t, "title: x", body)
}
