package frontmatter

import (
	"bytes"
	"fmt"

	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/goldmark/parser"
	"github.com/pgavlin/goldmark/util"
	"gopkg.in/yaml.v3"
)

// Extension is a goldmark Extender that registers the frontmatter block
// parser. Use it with goldmark.New(goldmark.WithExtensions(frontmatter.Extension))
// or, for callers that build a parser via parser.NewParser, use
// parser.WithBlockParsers(util.Prioritized(frontmatter.NewParser(), 0)).
//
// The priority is 0 so it runs before the built-in thematic-break parser
// (priority 200), which would otherwise consume the opening "---".
var Extension goldmark.Extender = extension{}

type extension struct{}

func (extension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(NewParser(), 0),
	))
}

// Find walks doc and returns the first frontmatter Block child of the
// document, or nil if there is none. Frontmatter blocks always sit at
// the head of the document, so this is an O(1) lookup.
func Find(doc ast.Node) *Block {
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if b, ok := c.(*Block); ok {
			return b
		}
		// Frontmatter is always the very first block; any other first
		// block means the document has no frontmatter.
		return nil
	}
	return nil
}

// Body returns the raw bytes between the frontmatter fences, joined with
// newlines. The result excludes both fence lines.
func Body(b *Block, source []byte) []byte {
	if b == nil {
		return nil
	}
	lines := b.Lines()
	if lines == nil || lines.Len() == 0 {
		return nil
	}
	var buf bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(source[seg.Start:seg.Stop])
	}
	return buf.Bytes()
}

// Decode unmarshals the body of a frontmatter block into v according to
// the block's Format. Returns an error if the block's body fails to
// parse, including a wrapping message that identifies the format.
func Decode(b *Block, source []byte, v any) error {
	if b == nil {
		return fmt.Errorf("frontmatter: nil block")
	}
	body := Body(b, source)
	switch b.Format {
	case FormatYAML:
		if err := yaml.Unmarshal(body, v); err != nil {
			return fmt.Errorf("frontmatter: parse YAML: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("frontmatter: unsupported format %q", b.Format)
	}
}
