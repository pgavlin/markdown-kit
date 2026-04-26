package frontmatter

import (
	"bytes"

	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/goldmark/parser"
	"github.com/pgavlin/goldmark/text"
	textm "github.com/pgavlin/goldmark/text"
)

// blockParser is a goldmark BlockParser that recognizes a YAML
// frontmatter block at the very start of a document.
//
// Acceptance rules:
//   - The opening fence "---" must be on the first non-empty parse line.
//     In practice this means: parent is the Document node, the Document
//     has no children yet, and the line is "---" at column 0 with no
//     trailing content.
//   - A matching closing fence "---" must exist on its own line later in
//     the source. If no closing fence is found, we refuse to open the
//     block and the document is parsed with no frontmatter — keeping us
//     from accidentally swallowing the entire document.
//   - Lines between the fences are captured verbatim as the block's body.
type blockParser struct{}

// NewParser returns a goldmark BlockParser that detects YAML frontmatter
// blocks. Register it via parser.WithBlockParsers; the suggested priority
// is < 100 so it runs before thematic-break (which would otherwise eat
// the opening "---").
func NewParser() parser.BlockParser { return &blockParser{} }

// Trigger limits the parser to lines starting with the YAML fence char.
// Other formats (TOML "+++") would extend this in the future.
func (p *blockParser) Trigger() []byte { return []byte{'-'} }

func (p *blockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	// Only valid as the very first child of the Document.
	if _, ok := parent.(*ast.Document); !ok {
		return nil, parser.NoChildren
	}
	if parent.HasChildren() {
		return nil, parser.NoChildren
	}

	line, seg := reader.PeekLine()
	if !isYAMLFence(line) {
		return nil, parser.NoChildren
	}
	// The opening fence must be the very first bytes of the source —
	// no leading blank lines, no leading whitespace. Goldmark skips
	// blank lines before invoking block parsers, so without this guard
	// a document like "\n---\nx: 1\n---" would falsely match.
	if seg.Start != 0 {
		return nil, parser.NoChildren
	}

	// Verify a closing fence exists somewhere after the opening one in
	// the source bytes. Without this check, a document without a closing
	// fence would have its entire body silently consumed as YAML.
	source := reader.Source()
	if !hasYAMLClosingFence(source, seg.Stop) {
		return nil, parser.NoChildren
	}

	// Consume the opening fence line. The framework advances past the
	// trailing newline itself, hence the -1.
	reader.Advance(seg.Len() - 1)
	return NewBlock(FormatYAML), parser.NoChildren
}

func (p *blockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, seg := reader.PeekLine()
	if isYAMLFence(line) {
		// Consume the closing fence and stop. The framework advances
		// past the trailing newline itself, hence -1.
		reader.Advance(seg.Len() - 1)
		return parser.Close
	}
	// Capture this body line verbatim, then advance past it (again,
	// leaving the trailing newline for the framework).
	node.Lines().Append(textm.NewSegment(seg.Start, seg.Stop))
	reader.Advance(seg.Len() - 1)
	return parser.Continue | parser.NoChildren
}

func (p *blockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

// CanInterruptParagraph reports whether this parser can interrupt an
// in-progress paragraph. Frontmatter only opens at the document head,
// where no paragraph can be open, so the answer is false.
func (p *blockParser) CanInterruptParagraph() bool { return false }

// CanAcceptIndentedLine reports whether an indented line can open this
// block. Frontmatter requires column 0, so always false.
func (p *blockParser) CanAcceptIndentedLine() bool { return false }

// isYAMLFence reports whether line is exactly "---" possibly followed by
// whitespace then a line terminator. The leading "-" is our trigger so we
// only need to examine the rest.
func isYAMLFence(line []byte) bool {
	if len(line) < 3 || line[0] != '-' || line[1] != '-' || line[2] != '-' {
		return false
	}
	// Anything after the three dashes must be whitespace through to the
	// line terminator.
	for i := 3; i < len(line); i++ {
		c := line[i]
		if c == '\n' || c == '\r' {
			return true
		}
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

// hasYAMLClosingFence reports whether a line starting with "---" exists
// somewhere in source at or after offset. The fence must be on its own
// line, so we look for a newline followed by "---" followed by an EOL or
// blank-only suffix.
func hasYAMLClosingFence(source []byte, offset int) bool {
	if offset < 0 || offset >= len(source) {
		return false
	}
	rest := source[offset:]
	for {
		// Each iteration scans forward until we find a candidate fence.
		idx := bytes.Index(rest, []byte("---"))
		if idx == -1 {
			return false
		}
		// The match must start at the beginning of a line.
		if idx == 0 || rest[idx-1] == '\n' {
			// And the rest of the line (after "---") must be whitespace
			// only, terminated by EOL or EOF.
			tail := rest[idx+3:]
			if isBlankToEOL(tail) {
				return true
			}
		}
		rest = rest[idx+3:]
	}
}

// isBlankToEOL reports whether s contains only whitespace up to the next
// line terminator (or end of input).
func isBlankToEOL(s []byte) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || c == '\r' {
			return true
		}
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}
