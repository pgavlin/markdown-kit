package view

import (
	"bytes"
	"strings"

	"github.com/pgavlin/goldmark/ast"
	"golang.org/x/net/html"
)

// StripDataURIs is a [DocumentTransformer] that removes RawHTML nodes
// containing data: URIs (e.g. <img src="data:image/png;base64,...">).
func StripDataURIs(doc ast.Node, source []byte) {
	var toRemove []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		raw, ok := n.(*ast.RawHTML)
		if !ok {
			return ast.WalkContinue, nil
		}
		if rawHTMLHasDataURI(raw, source) {
			toRemove = append(toRemove, raw)
		}
		return ast.WalkContinue, nil
	})
	for _, n := range toRemove {
		if p := n.Parent(); p != nil {
			p.RemoveChild(p, n)
		}
	}
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
