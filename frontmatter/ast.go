// Package frontmatter implements detection and parsing of YAML frontmatter
// blocks at the head of a Markdown document. A frontmatter block begins
// with a fence line ("---") at column 0 of the very first line of the
// document, contains zero or more YAML lines, and ends with a closing
// fence ("---") on its own line. The block is exposed as an AST node of
// kind KindFrontmatter so renderers can decide whether to render or hide
// its contents.
package frontmatter

import (
	"io"

	"github.com/pgavlin/goldmark/ast"
)

// Format identifies the frontmatter syntax flavor. Only YAML is supported
// today; TOML ("+++") and JSON ("{...}") may be added later.
type Format string

const (
	// FormatYAML denotes a YAML frontmatter block fenced with "---".
	FormatYAML Format = "yaml"
)

// KindFrontmatter is the AST node kind for frontmatter blocks.
var KindFrontmatter = ast.NewNodeKind("Frontmatter")

// Block is the AST node representing a frontmatter block. Its inner
// content (everything between the fences, exclusive) is accessible
// through the embedded BaseBlock's Lines() segments.
type Block struct {
	ast.BaseBlock

	// Format identifies the syntax flavor of the block's body.
	Format Format
}

// NewBlock returns a new frontmatter Block with the given format.
func NewBlock(format Format) *Block {
	return &Block{Format: format}
}

// Kind implements ast.Node.
func (b *Block) Kind() ast.NodeKind { return KindFrontmatter }

// IsRaw reports that the block's body must not be inline-parsed. Without
// this override, goldmark would feed the YAML body to the inline parser,
// producing spurious Text/Emphasis/Whitespace children that break
// rendering and complicate downstream processing.
func (b *Block) IsRaw() bool { return true }

// Dump implements ast.Node.
func (b *Block) Dump(w io.Writer, source []byte, level int) {
	ast.DumpHelper(w, b, source, level, map[string]string{
		"Format": string(b.Format),
	}, nil)
}
