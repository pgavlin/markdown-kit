package indexer

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/pgavlin/goldmark/ast"
	"golang.org/x/net/html"
)

var gfmPunctuationRegexp = regexp.MustCompile(`[^\w\- ]`)

// anchorID extracts the id or name attribute from an <a> tag in raw HTML.
// Returns the attribute value and true if found, or ("", false) otherwise.
func anchorID(data []byte) (string, bool) {
	z := html.NewTokenizer(bytes.NewReader(data))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return "", false
		case html.StartTagToken, html.SelfClosingTagToken:
			tn, hasAttr := z.TagName()
			if !hasAttr || !bytes.EqualFold(tn, []byte("a")) {
				continue
			}
			for hasAttr {
				var key, val []byte
				key, val, hasAttr = z.TagAttr()
				k := string(key)
				if k == "id" || k == "name" {
					if v := string(val); v != "" {
						return v, true
					}
				}
			}
		}
	}
}

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

// pendingAnchor stores an HTML anchor ID along with the AST node it was found in.
type pendingAnchor struct {
	id   string
	node ast.Node
}

type indexer struct {
	anchorFunc AnchorFunc
	source     []byte

	sectionStack   []*Section
	anchors        map[string][]*Section
	nodeAnchors    map[string][]ast.Node
	pendingAnchors []pendingAnchor
}

func (i *indexer) walk(n ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		return ast.WalkContinue, nil
	}

	// Collect anchors from <a id="..."> or <a name="..."> raw HTML tags.
	if raw, ok := n.(*ast.RawHTML); ok {
		segs := raw.Segments
		for j := 0; j < segs.Len(); j++ {
			seg := segs.At(j)
			if id, ok := anchorID(seg.Value(i.source)); ok {
				i.pendingAnchors = append(i.pendingAnchors, pendingAnchor{
					id:   id,
					node: raw,
				})
			}
		}
		return ast.WalkContinue, nil
	}

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

	// Also register any pending anchors from preceding <a id="..."> tags.
	for _, pa := range i.pendingAnchors {
		i.anchors[pa.id] = append(i.anchors[pa.id], newSection)
		i.nodeAnchors[pa.id] = append(i.nodeAnchors[pa.id], pa.node)
	}
	i.pendingAnchors = i.pendingAnchors[:0]

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
		nodeAnchors:  map[string][]ast.Node{},
	}
	for _, o := range options {
		o(indexer)
	}

	ast.Walk(document, indexer.walk)

	// Flush any remaining pending anchors (not followed by a heading)
	// to the current (innermost) section.
	if len(indexer.pendingAnchors) > 0 {
		current := indexer.sectionStack[len(indexer.sectionStack)-1]
		for _, pa := range indexer.pendingAnchors {
			indexer.anchors[pa.id] = append(indexer.anchors[pa.id], current)
			indexer.nodeAnchors[pa.id] = append(indexer.nodeAnchors[pa.id], pa.node)
		}
	}

	return &DocumentIndex{
		toc:         indexer.sectionStack[0],
		anchors:     indexer.anchors,
		nodeAnchors: indexer.nodeAnchors,
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
	toc         *Section
	anchors     map[string][]*Section
	nodeAnchors map[string][]ast.Node
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

// LookupNode returns the AST nodes for HTML anchors (<a id="..."> or <a name="...">)
// with the given anchor ID. Returns nil, false if no HTML anchor defines this ID.
func (index *DocumentIndex) LookupNode(anchor string) ([]ast.Node, bool) {
	nodes, ok := index.nodeAnchors[anchor]
	return nodes, ok
}

// AnchorNodes returns the set of all AST nodes that define HTML anchors.
func (index *DocumentIndex) AnchorNodes() map[ast.Node]bool {
	if len(index.nodeAnchors) == 0 {
		return nil
	}
	nodes := make(map[ast.Node]bool)
	for _, ns := range index.nodeAnchors {
		for _, n := range ns {
			nodes[n] = true
		}
	}
	return nodes
}
