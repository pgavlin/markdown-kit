package indexer

import (
	"errors"
	"strings"
	"testing"

	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to parse markdown source into an ast.Document.
func parseMarkdown(t *testing.T, source []byte) *ast.Document {
	t.Helper()
	parser := goldmark.DefaultParser()
	node := parser.Parse(text.NewReader(source))
	doc, ok := node.(*ast.Document)
	require.True(t, ok, "parsed node should be *ast.Document")
	return doc
}

// ---------------------------------------------------------------------------
// GitHubFlavoredMarkdown tests
// ---------------------------------------------------------------------------

func TestGitHubFlavoredMarkdown_BasicLowercase(t *testing.T) {
	assert.Equal(t, "hello-world", GitHubFlavoredMarkdown("Hello World"))
}

func TestGitHubFlavoredMarkdown_SpacesToHyphens(t *testing.T) {
	assert.Equal(t, "foo-bar-baz", GitHubFlavoredMarkdown("foo bar baz"))
}

func TestGitHubFlavoredMarkdown_PunctuationStripping(t *testing.T) {
	assert.Equal(t, "whats-up", GitHubFlavoredMarkdown("What's Up?"))
}

func TestGitHubFlavoredMarkdown_MixedCaseSpecialChars(t *testing.T) {
	assert.Equal(t, "hello-world-2026", GitHubFlavoredMarkdown("Hello, World! (2026)"))
}

func TestGitHubFlavoredMarkdown_EmptyString(t *testing.T) {
	assert.Equal(t, "", GitHubFlavoredMarkdown(""))
}

func TestGitHubFlavoredMarkdown_HyphensPreserved(t *testing.T) {
	assert.Equal(t, "already-hyphenated", GitHubFlavoredMarkdown("already-hyphenated"))
}

func TestGitHubFlavoredMarkdown_OnlyPunctuation(t *testing.T) {
	assert.Equal(t, "", GitHubFlavoredMarkdown("!@#$%^&*()"))
}

func TestGitHubFlavoredMarkdown_NumbersAndUnderscores(t *testing.T) {
	// \w matches word characters including digits and underscores, so they should be kept.
	assert.Equal(t, "section_1_foo", GitHubFlavoredMarkdown("Section_1_Foo"))
}

func TestGitHubFlavoredMarkdown_MultipleSpaces(t *testing.T) {
	assert.Equal(t, "a---b", GitHubFlavoredMarkdown("a   b"))
}

// ---------------------------------------------------------------------------
// Index — simple document
// ---------------------------------------------------------------------------

func TestIndex_SimpleDocument(t *testing.T) {
	source := []byte("# Hello\n\nSome text.\n\n## World\n\nMore text.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	// TableOfContents returns the root section.
	toc := idx.TableOfContents()
	require.NotNil(t, toc)

	// Root section wraps the document node; level should be 0.
	assert.Equal(t, 0, toc.Level)
	assert.Equal(t, "", toc.Anchor)

	// The root section should have one top-level subsection (h1).
	require.Len(t, toc.Subsections, 1)

	h1 := toc.Subsections[0]
	assert.Equal(t, 1, h1.Level)
	assert.Equal(t, "hello", h1.Anchor)

	// The h1 section should have one subsection (h2).
	require.Len(t, h1.Subsections, 1)

	h2 := h1.Subsections[0]
	assert.Equal(t, 2, h2.Level)
	assert.Equal(t, "world", h2.Anchor)
	assert.Empty(t, h2.Subsections)
}

// ---------------------------------------------------------------------------
// Index — nested headings (h1 > h2 > h3)
// ---------------------------------------------------------------------------

func TestIndex_NestedHeadings(t *testing.T) {
	source := []byte("# A\n\n## B\n\n### C\n\nParagraph.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	toc := idx.TableOfContents()
	require.Len(t, toc.Subsections, 1, "root should have one h1")

	h1 := toc.Subsections[0]
	assert.Equal(t, "a", h1.Anchor)
	require.Len(t, h1.Subsections, 1, "h1 should have one h2")

	h2 := h1.Subsections[0]
	assert.Equal(t, "b", h2.Anchor)
	require.Len(t, h2.Subsections, 1, "h2 should have one h3")

	h3 := h2.Subsections[0]
	assert.Equal(t, "c", h3.Anchor)
	assert.Empty(t, h3.Subsections)
}

// ---------------------------------------------------------------------------
// Index — multiple top-level headings
// ---------------------------------------------------------------------------

func TestIndex_MultipleToplevelHeadings(t *testing.T) {
	source := []byte("# First\n\n# Second\n\n# Third\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	toc := idx.TableOfContents()
	require.Len(t, toc.Subsections, 3)

	assert.Equal(t, "first", toc.Subsections[0].Anchor)
	assert.Equal(t, "second", toc.Subsections[1].Anchor)
	assert.Equal(t, "third", toc.Subsections[2].Anchor)
}

// ---------------------------------------------------------------------------
// Index — heading level goes back up (h1 > h2 > h1)
// ---------------------------------------------------------------------------

func TestIndex_HeadingLevelGoesBack(t *testing.T) {
	source := []byte("# A\n\n## B\n\n# C\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	toc := idx.TableOfContents()
	require.Len(t, toc.Subsections, 2, "root should have two h1 sections")

	h1a := toc.Subsections[0]
	assert.Equal(t, "a", h1a.Anchor)
	require.Len(t, h1a.Subsections, 1)
	assert.Equal(t, "b", h1a.Subsections[0].Anchor)

	h1c := toc.Subsections[1]
	assert.Equal(t, "c", h1c.Anchor)
	assert.Empty(t, h1c.Subsections)
}

// ---------------------------------------------------------------------------
// Index — duplicate headings
// ---------------------------------------------------------------------------

func TestIndex_DuplicateHeadings(t *testing.T) {
	source := []byte("# Hello\n\nText.\n\n# Hello\n\nMore text.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	// Lookup should return both sections.
	sections, ok := idx.Lookup("hello")
	require.True(t, ok)
	require.Len(t, sections, 2, "two sections should share the same anchor")

	assert.Equal(t, 1, sections[0].Level)
	assert.Equal(t, 1, sections[1].Level)

	// They should be distinct section objects.
	assert.NotSame(t, sections[0], sections[1])
}

// ---------------------------------------------------------------------------
// Lookup — existing and non-existing anchors
// ---------------------------------------------------------------------------

func TestLookup_Existing(t *testing.T) {
	source := []byte("# Hello\n\n## World\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("hello")
	require.True(t, ok)
	require.Len(t, sections, 1)
	assert.Equal(t, 1, sections[0].Level)

	sections, ok = idx.Lookup("world")
	require.True(t, ok)
	require.Len(t, sections, 1)
	assert.Equal(t, 2, sections[0].Level)
}

func TestLookup_NonExisting(t *testing.T) {
	source := []byte("# Hello\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("does-not-exist")
	assert.False(t, ok)
	assert.Nil(t, sections)
}

// ---------------------------------------------------------------------------
// WithAnchors — custom anchor function
// ---------------------------------------------------------------------------

func TestWithAnchors_CustomFunction(t *testing.T) {
	source := []byte("# Hello World\n\n## Foo Bar\n")
	doc := parseMarkdown(t, source)

	// Custom anchor function: uppercase and replace spaces with underscores.
	customAnchor := func(heading string) string {
		return strings.ToUpper(strings.ReplaceAll(heading, " ", "_"))
	}

	idx := Index(doc, source, WithAnchors(customAnchor))

	// Default GFM anchors should not be present.
	_, ok := idx.Lookup("hello-world")
	assert.False(t, ok, "GFM anchor should not exist when custom func is used")

	// Custom anchors should be present.
	sections, ok := idx.Lookup("HELLO_WORLD")
	require.True(t, ok)
	require.Len(t, sections, 1)
	assert.Equal(t, 1, sections[0].Level)

	sections, ok = idx.Lookup("FOO_BAR")
	require.True(t, ok)
	require.Len(t, sections, 1)
	assert.Equal(t, 2, sections[0].Level)
}

// ---------------------------------------------------------------------------
// TableOfContents — structure verification
// ---------------------------------------------------------------------------

func TestTableOfContents_Structure(t *testing.T) {
	source := []byte("# Intro\n\n## Setup\n\n### Dependencies\n\n## Usage\n\n# FAQ\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	toc := idx.TableOfContents()
	require.NotNil(t, toc)

	// Root has two h1 sections: Intro and FAQ.
	require.Len(t, toc.Subsections, 2)

	intro := toc.Subsections[0]
	assert.Equal(t, "intro", intro.Anchor)
	assert.Equal(t, 1, intro.Level)
	require.Len(t, intro.Subsections, 2, "Intro should have Setup and Usage as children")

	setup := intro.Subsections[0]
	assert.Equal(t, "setup", setup.Anchor)
	assert.Equal(t, 2, setup.Level)
	require.Len(t, setup.Subsections, 1, "Setup should have Dependencies as child")

	deps := setup.Subsections[0]
	assert.Equal(t, "dependencies", deps.Anchor)
	assert.Equal(t, 3, deps.Level)
	assert.Empty(t, deps.Subsections)

	usage := intro.Subsections[1]
	assert.Equal(t, "usage", usage.Anchor)
	assert.Equal(t, 2, usage.Level)
	assert.Empty(t, usage.Subsections)

	faq := toc.Subsections[1]
	assert.Equal(t, "faq", faq.Anchor)
	assert.Equal(t, 1, faq.Level)
	assert.Empty(t, faq.Subsections)
}

// ---------------------------------------------------------------------------
// TableOfContents — empty document
// ---------------------------------------------------------------------------

func TestTableOfContents_EmptyDocument(t *testing.T) {
	source := []byte("Just a paragraph, no headings.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	toc := idx.TableOfContents()
	require.NotNil(t, toc)
	assert.Empty(t, toc.Subsections, "document with no headings should have no subsections")
	assert.Equal(t, 0, toc.Level)
}

// ---------------------------------------------------------------------------
// Section.Walk — verify it visits expected nodes
// ---------------------------------------------------------------------------

func TestSection_Walk(t *testing.T) {
	// Use two h1 sections so that the first section has a non-nil End.
	source := []byte("# Hello\n\nParagraph one.\n\nParagraph two.\n\n# Next\n\nUnder next.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("hello")
	require.True(t, ok)
	require.Len(t, sections, 1)
	helloSection := sections[0]

	// The hello section should end at "# Next" (same level).
	require.NotNil(t, helloSection.End, "section should have End set")

	// Walk the "Hello" section and collect the kinds of nodes visited on enter.
	var kinds []ast.NodeKind
	err := helloSection.Walk(func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if enter {
			kinds = append(kinds, n.Kind())
		}
		return ast.WalkContinue, nil
	})
	require.NoError(t, err)

	// The hello section runs from "# Hello" up to (but not including) "# Next".
	// We expect: Heading(Hello), Paragraph(one), Paragraph(two) at the top level,
	// plus their inline children (Text nodes).
	headingCount := 0
	paragraphCount := 0
	for _, k := range kinds {
		if k == ast.KindHeading {
			headingCount++
		}
		if k == ast.KindParagraph {
			paragraphCount++
		}
	}

	assert.Equal(t, 1, headingCount, "should visit only the Hello heading")
	assert.Equal(t, 2, paragraphCount, "should visit both paragraph nodes")
}

func TestSection_Walk_IncludesSubsections(t *testing.T) {
	// When a section has subsections (lower-level headings), Walk should include them.
	source := []byte("# Top\n\nIntro.\n\n## Sub\n\nSub content.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("top")
	require.True(t, ok)
	require.Len(t, sections, 1)
	topSection := sections[0]

	// "# Top" has no same-or-higher-level heading after it, so End is nil.
	// Walk should traverse all nodes from the heading to the end of the document.
	assert.Nil(t, topSection.End, "last h1 section should have nil End")

	var kinds []ast.NodeKind
	err := topSection.Walk(func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if enter {
			kinds = append(kinds, n.Kind())
		}
		return ast.WalkContinue, nil
	})
	require.NoError(t, err)

	headingCount := 0
	paragraphCount := 0
	for _, k := range kinds {
		if k == ast.KindHeading {
			headingCount++
		}
		if k == ast.KindParagraph {
			paragraphCount++
		}
	}

	// Should visit both the h1 and h2 headings and both paragraphs.
	assert.Equal(t, 2, headingCount, "should visit both headings (h1 and h2)")
	assert.Equal(t, 2, paragraphCount, "should visit both paragraphs")
}

func TestSection_Walk_LastSection(t *testing.T) {
	// The last section in a document has End == nil, meaning Walk iterates to the end.
	source := []byte("# Only\n\nSome content.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("only")
	require.True(t, ok)
	require.Len(t, sections, 1)

	var visited []ast.NodeKind
	err := sections[0].Walk(func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if enter {
			visited = append(visited, n.Kind())
		}
		return ast.WalkContinue, nil
	})
	require.NoError(t, err)

	// Should have visited the heading and paragraph (and their children).
	assert.Contains(t, visited, ast.KindHeading)
	assert.Contains(t, visited, ast.KindParagraph)
}

func TestSection_Walk_RootSection(t *testing.T) {
	// Walking the root (ToC) section should visit all nodes from the document start.
	source := []byte("Preamble.\n\n# Heading\n\nBody.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	toc := idx.TableOfContents()

	var topKinds []ast.NodeKind
	err := toc.Walk(func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if enter {
			topKinds = append(topKinds, n.Kind())
		}
		return ast.WalkContinue, nil
	})
	require.NoError(t, err)

	// The root section's Start is the Document node. Walk starts there and iterates
	// through siblings. Since Document is the root, its NextSibling is nil, so Walk
	// visits just the Document and all its children.
	assert.Contains(t, topKinds, ast.KindDocument)
}

// ---------------------------------------------------------------------------
// Section.End is set correctly when a same-or-higher-level heading follows
// ---------------------------------------------------------------------------

func TestSection_EndIsSet(t *testing.T) {
	source := []byte("# A\n\nText.\n\n# B\n\nText.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("a")
	require.True(t, ok)
	require.Len(t, sections, 1)

	sA := sections[0]
	// Section A should end at the heading for B.
	require.NotNil(t, sA.End, "section A should have End set to section B's heading")
	heading, ok := sA.End.(*ast.Heading)
	require.True(t, ok, "End should be a *ast.Heading")
	assert.Equal(t, string(heading.Text(source)), "B")
}

func TestSection_EndIsNil_ForLastSection(t *testing.T) {
	source := []byte("# A\n\n## B\n\nText.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("b")
	require.True(t, ok)
	require.Len(t, sections, 1)

	sB := sections[0]
	assert.Nil(t, sB.End, "last section in document should have nil End")
}

// ---------------------------------------------------------------------------
// Index with GFM-style anchor transformation edge cases
// ---------------------------------------------------------------------------

func TestIndex_AnchorTransformation(t *testing.T) {
	source := []byte("# What's New in v2.0?\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	// The heading "What's New in v2.0?" should become "whats-new-in-v20"
	sections, ok := idx.Lookup("whats-new-in-v20")
	require.True(t, ok)
	require.Len(t, sections, 1)
	assert.Equal(t, 1, sections[0].Level)
}

// ---------------------------------------------------------------------------
// Index — deeply nested headings
// ---------------------------------------------------------------------------

func TestSection_Walk_ReturnsError(t *testing.T) {
	source := []byte("# Hello\n\nSome text.\n\n# Next\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("hello")
	require.True(t, ok)
	require.Len(t, sections, 1)

	expectedErr := errors.New("walk error")
	err := sections[0].Walk(func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		return ast.WalkStop, expectedErr
	})
	assert.Equal(t, expectedErr, err, "Walk should propagate the walker error")
}

// ---------------------------------------------------------------------------
// HTML anchor tags — <a id="..."> and <a name="...">
// ---------------------------------------------------------------------------

func TestIndex_HTMLAnchorID(t *testing.T) {
	source := []byte("<a id=\"custom-section\"></a>\n\n# Hello\n\nSome text.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	// The custom anchor should resolve to the same section as the heading.
	sections, ok := idx.Lookup("custom-section")
	require.True(t, ok, "custom anchor should be found")
	require.Len(t, sections, 1)
	assert.Equal(t, 1, sections[0].Level)
	assert.Equal(t, "hello", sections[0].Anchor)

	// The heading's own GFM anchor should also work.
	sections2, ok := idx.Lookup("hello")
	require.True(t, ok)
	require.Len(t, sections2, 1)
	assert.Same(t, sections[0], sections2[0], "both anchors should point to the same section")
}

func TestIndex_HTMLAnchorName(t *testing.T) {
	source := []byte("<a name=\"my-anchor\"></a>\n\n## Section\n\nText.\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("my-anchor")
	require.True(t, ok, "name-based anchor should be found")
	require.Len(t, sections, 1)
	assert.Equal(t, 2, sections[0].Level)
}

func TestIndex_HTMLAnchorSingleQuotes(t *testing.T) {
	source := []byte("<a id='single-quoted'></a>\n\n# Heading\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("single-quoted")
	require.True(t, ok, "single-quoted anchor should be found")
	require.Len(t, sections, 1)
}

func TestIndex_HTMLAnchorNoFollowingHeading(t *testing.T) {
	// An anchor at the end of a document with no following heading
	// should be associated with the current section.
	source := []byte("# Hello\n\nText.\n\n<a id=\"end-anchor\"></a>\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("end-anchor")
	require.True(t, ok, "anchor without following heading should still be found")
	require.Len(t, sections, 1)
	// Should be associated with the "Hello" section (the current one).
	assert.Equal(t, "hello", sections[0].Anchor)
}

func TestIndex_HTMLAnchorMultipleBeforeHeading(t *testing.T) {
	source := []byte("<a id=\"alias1\"></a>\n<a id=\"alias2\"></a>\n\n# Target\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	for _, anchor := range []string{"alias1", "alias2", "target"} {
		sections, ok := idx.Lookup(anchor)
		require.True(t, ok, "anchor %q should be found", anchor)
		require.Len(t, sections, 1)
		assert.Equal(t, 1, sections[0].Level)
	}
}

func TestIndex_HTMLAnchorWithOtherAttributes(t *testing.T) {
	// id is not the first attribute — the old regex couldn't handle this.
	source := []byte("<a class=\"ref\" id=\"other-attrs\"></a>\n\n# Heading\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("other-attrs")
	require.True(t, ok, "anchor with preceding attributes should be found")
	require.Len(t, sections, 1)
	assert.Equal(t, 1, sections[0].Level)
}

func TestIndex_HTMLAnchorUnquoted(t *testing.T) {
	source := []byte("<a id=unquoted></a>\n\n# Heading\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	sections, ok := idx.Lookup("unquoted")
	require.True(t, ok, "unquoted anchor should be found")
	require.Len(t, sections, 1)
}

func TestIndex_DeeplyNested(t *testing.T) {
	source := []byte("# L1\n\n## L2\n\n### L3\n\n#### L4\n\n##### L5\n\n###### L6\n")
	doc := parseMarkdown(t, source)
	idx := Index(doc, source)

	toc := idx.TableOfContents()
	require.Len(t, toc.Subsections, 1)

	current := toc.Subsections[0]
	for expectedLevel := 1; expectedLevel <= 6; expectedLevel++ {
		assert.Equal(t, expectedLevel, current.Level, "expected level %d", expectedLevel)
		if expectedLevel < 6 {
			require.Len(t, current.Subsections, 1, "level %d should have one child", expectedLevel)
			current = current.Subsections[0]
		} else {
			assert.Empty(t, current.Subsections, "level 6 should have no children")
		}
	}
}
