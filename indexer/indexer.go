package indexer

import (
	"regexp"
	"strings"

	"github.com/pgavlin/goldmark/ast"
)

var gfmPunctuationRegexp = regexp.MustCompile(`[^\w\- ]`)

// GitHubFlavoredMarkdown is an AnchorFunc that transforms heading text into GitHub Flavored
// Markdown anchors. Heading text is converted to a GFM anchor by first converting all text
// to lowercase, removing all non-word, non-hyphen, and non-space characters, and then
// replacing all spaces with hyphens.
//
// Ref: https://github.com/gjtorikian/html-pipeline/blob/main/lib/html/pipeline/toc_filter.rb
func GitHubFlavoredMarkdown(heading string) string {
	heading = strings.ToLower(heading)
	heading = gfmPunctuationRegexp.ReplaceAllString(heading, "")
	return strings.ReplaceAll(heading, " ", "-")
}

// An AnchorFunc is a function that converts raw header text into an anchor that is appropriate
// for use in a URL.
type AnchorFunc func(heading string) (anchor string)

// An IndexOption affects the behavior of the Index function.
type IndexOption func(i *indexer)

// WithAnchors configures the AnchorFunc used by the indexer to convert
func WithAnchors(anchors AnchorFunc) IndexOption {
	return func(i *indexer) {
		i.anchorFunc = anchors
	}
}

type indexer struct {
	anchorFunc AnchorFunc
	source     []byte

	sectionStack []*Section
	anchors      map[string][]*Section
}

func (i *indexer) walk(n ast.Node, enter bool) (ast.WalkStatus, error) {
	heading, ok := n.(*ast.Heading)
	if !ok {
		return ast.WalkContinue, nil
	}

	newSection := &Section{
		Level:  heading.Level,
		Anchor: i.anchorFunc(string(heading.Text(i.source))),
		Start:  heading,
	}
	i.anchors[newSection.Anchor] = append(i.anchors[newSection.Anchor], newSection)

	currentSection := i.sectionStack[len(i.sectionStack)-1]
	for heading.Level <= currentSection.Level {
		currentSection.End = heading

		i.sectionStack = i.sectionStack[:len(i.sectionStack)-1]
		currentSection = i.sectionStack[len(i.sectionStack)-1]
	}
	parent := currentSection

	parent.Subsections = append(parent.Subsections, newSection)
	i.sectionStack = append(i.sectionStack, newSection)
	return ast.WalkContinue, nil
}

// Index walks a Document, converts the raw text of each heading to an anchor, and returns a
// DocumentIndex that maps from anchors to lists of sections. Headings are converted to
// GitHub Flavored Markdown anchors by default. Each section begins with either
func Index(document *ast.Document, source []byte, options ...IndexOption) *DocumentIndex {
	indexer := &indexer{
		source:       source,
		anchorFunc:   GitHubFlavoredMarkdown,
		sectionStack: []*Section{{Start: document}},
		anchors:      map[string][]*Section{},
	}
	for _, o := range options {
		o(indexer)
	}

	ast.Walk(document, indexer.walk)

	return &DocumentIndex{
		toc:     indexer.sectionStack[0],
		anchors: indexer.anchors,
	}
}

// A Section represents a collection of nodes under a Heading (or the start of the document).
type Section struct {
	ID     int
	Level  int
	Anchor string

	Start ast.Node
	End   ast.Node

	Subsections []*Section
}

// Walk calls ast.Walk on each node in the section.
func (s *Section) Walk(walker ast.Walker) error {
	for cursor := s.Start; cursor != s.End; cursor = cursor.NextSibling() {
		if err := ast.Walk(cursor, walker); err != nil {
			return err
		}
	}
	return nil
}

// A DocumentIndex maps from anchors to Sections.
type DocumentIndex struct {
	toc     *Section
	anchors map[string][]*Section
}

// TableOfContents returns the root fo the document's section tree.
func (index *DocumentIndex) TableOfContents() *Section {
	return index.toc
}

// Lookup returns the list of sections with the given anchor. Sections appear in the list in
// the same order in which they appear in the source document.
func (index *DocumentIndex) Lookup(anchor string) ([]*Section, bool) {
	sections, ok := index.anchors[anchor]
	return sections, ok
}
