package view

import (
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/indexer"
)

// tocMode distinguishes the tree vs filter view of the overlay.
type tocMode int

const (
	tocModeTree tocMode = iota
	tocModeFilter
)

// tocEntry is one heading in the overlay.
type tocEntry struct {
	anchor    string   // DocumentIndex anchor used for SelectAnchor
	level     int      // 1..6
	text      string   // heading plain text
	ancestors []string // ancestor heading texts (level 1 ... parent)
	lastChild bool     // true if last sibling at its level (for tree edges)
	matchCols []int    // filter-mode: visible column positions of matched chars
}

// tocState is the overlay's runtime state.
type tocState struct {
	active     bool
	mode       tocMode
	query      string
	allEntries []tocEntry
	matches    []int
	cursor     int
	scroll     int
}

// TOCActive reports whether the table-of-contents overlay is open.
// Embedders should gate their own key handling (see Searching()).
func (m *Model) TOCActive() bool {
	return m.toc.active
}

// buildTOCEntries walks the document's section tree and returns a flat,
// document-ordered list of entries suitable for rendering.
func (m *Model) buildTOCEntries() []tocEntry {
	if m.index == nil {
		return nil
	}
	root := m.index.TableOfContents()
	if root == nil {
		return nil
	}
	var entries []tocEntry
	var ancestors []string
	visitTOCSections(m.markdown, root.Subsections, ancestors, &entries)
	return entries
}

// visitTOCSections is a recursive helper that appends entries in document
// order, tracking ancestor heading texts along the recursion path.
func visitTOCSections(source []byte, sections []*indexer.Section, ancestors []string, out *[]tocEntry) {
	for i, s := range sections {
		h, ok := s.Start.(*ast.Heading)
		if !ok {
			continue
		}
		text := string(h.Text(source))
		entry := tocEntry{
			anchor:    s.Anchor,
			level:     s.Level,
			text:      text,
			ancestors: append([]string(nil), ancestors...),
			lastChild: i == len(sections)-1,
		}
		*out = append(*out, entry)

		if len(s.Subsections) > 0 {
			visitTOCSections(source, s.Subsections, append(ancestors, text), out)
		}
	}
}
