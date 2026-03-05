package view

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/pgavlin/goldmark/ast"
	"golang.org/x/net/html"
)

// StripDataURIs is a [DocumentTransformer] that removes nodes containing
// data: URIs. It handles RawHTML nodes (e.g. <img src="data:...">),
// Image and Link nodes with data: destinations, and LinkReferenceDefinition
// nodes with data: destinations.
func StripDataURIs(doc ast.Node, source []byte) {
	var toRemove []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.RawHTML:
			if rawHTMLHasDataURI(n, source) {
				toRemove = append(toRemove, n)
			}
		case *ast.Image:
			if bytes.HasPrefix(n.Destination, dataPrefix) {
				toRemove = append(toRemove, n)
			}
		case *ast.Link:
			if bytes.HasPrefix(n.Destination, dataPrefix) {
				toRemove = append(toRemove, n)
			}
		case *ast.LinkReferenceDefinition:
			if bytes.HasPrefix(n.Destination, dataPrefix) {
				toRemove = append(toRemove, n)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, n := range toRemove {
		p := n.Parent()
		if p == nil {
			continue
		}
		p.RemoveChild(p, n)
		// If removing the node left an empty paragraph, remove it too.
		if par, ok := p.(*ast.Paragraph); ok && !par.HasChildren() {
			if pp := par.Parent(); pp != nil {
				pp.RemoveChild(pp, par)
			}
		}
	}
}

var dataPrefix = []byte("data:")

var dataURIPattern = regexp.MustCompile(`data:[^\s)"'>]+`)

// StripDataURIText replaces data URIs in raw markdown text with a placeholder.
// This is used to reduce the size of bug reports that contain large base64-encoded images.
func StripDataURIText(s string) string {
	return dataURIPattern.ReplaceAllString(s, "[data URI removed]")
}

// rawHTMLHasDataURI reports whether a RawHTML node contains an HTML tag
// with an attribute value that starts with "data:".
func rawHTMLHasDataURI(raw *ast.RawHTML, source []byte) bool {
	var buf bytes.Buffer
	for i := 0; i < raw.Segments.Len(); i++ {
		seg := raw.Segments.At(i)
		buf.Write(seg.Value(source))
	}

	z := html.NewTokenizer(&buf)
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.SelfClosingTagToken:
			for {
				key, val, more := z.TagAttr()
				_ = key
				if strings.HasPrefix(string(val), "data:") {
					return true
				}
				if !more {
					break
				}
			}
		}
	}
}
