package renderer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/lexers"
	"github.com/nfnt/resize"
	"github.com/pgavlin/ansicsi"
	"github.com/pgavlin/goldmark/ast"
	xast "github.com/pgavlin/goldmark/extension/ast"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/rivo/uniseg"
)

type blockState struct {
	node  ast.Node
	fresh bool
}

type listState struct {
	marker  byte
	ordered bool
	index   int
}

type tableBorders []rune

func (b tableBorders) topLeft() rune {
	return b[0]
}

func (b tableBorders) topJoin() rune {
	return b[1]
}

func (b tableBorders) topRight() rune {
	return b[2]
}

func (b tableBorders) middleLeft() rune {
	return b[3]
}

func (b tableBorders) middleJoin() rune {
	return b[4]
}

func (b tableBorders) middleRight() rune {
	return b[5]
}

func (b tableBorders) bottomLeft() rune {
	return b[6]
}

func (b tableBorders) bottomJoin() rune {
	return b[7]
}

func (b tableBorders) bottomRight() rune {
	return b[8]
}

func (b tableBorders) vertical() rune {
	return b[9]
}

func (b tableBorders) horizontal() string {
	return string(b[10:11])
}

var borders = tableBorders("╭┬╮├┼┤╰┴╯│─")

type tableState struct {
	columnWidths []int
	cellWidths   []int
	alignments   []xast.Alignment

	rowIndex    int
	columnIndex int
	cellIndex   int

	measuring bool
}

type countingWriter struct {
	n int
}

func (w *countingWriter) Write(b []byte) (int, error) {
	w.n += len(b)
	return len(b), nil
}

// A NodeSpan maps from an AST node to its representative span in a rendered document. The NodeSpans for an AST form
// a tree; the root of the span tree for a rendered document can be accessed using Renderer.SpanTree.
type NodeSpan struct {
	// The byte offset of the start of the span. Inclusive.
	Start int
	// The byte offset of the end of the span. Exclusive.
	End int
	// The node that this span represents.
	Node ast.Node

	// The parent node in the span tree.
	Parent *NodeSpan
	// The next node in a preorder traversal of the span tree.
	Next *NodeSpan
	// The previous node in a preorder traversal of the span tree.
	Prev *NodeSpan
	// The children of this node.
	Children []*NodeSpan
}

// Contains returns true if the given byte offset is contained within this node's span.
func (s *NodeSpan) Contains(offset int) bool {
	return s.Start <= offset && offset < s.End
}

// Renderer is a goldmark renderer that produces Markdown output. Due to information loss in goldmark, its output may
// not be textually identical to the source that produced the AST to be rendered, but the structure should match.
//
// NodeRenderers that want to override rendering of particular node types should write through the Write* functions
// provided by Renderer in order to retain proper indentation and prefices inside of lists and block quotes.
type Renderer struct {
	theme         *chroma.Style
	wordWrap      int
	hyperlinks    bool
	images        bool
	maxImageWidth int
	contentRoot   string
	softBreak     bool

	listStack  []listState
	tableStack []tableState
	openBlocks []blockState
	wrapping   []bool
	spanStack  []*NodeSpan
	lastSpan   *NodeSpan

	rootSpan *NodeSpan

	styles      []chroma.StyleEntry
	prefixStack []string
	prefix      []byte
	wordBuffer  bytes.Buffer
	lineWidth   int
	atNewline   bool
	byteOffset  int
	inImage     bool
}

// A RendererOption represents a configuration option for a Renderer.
type RendererOption func(r *Renderer)

// WithTheme sets the theme used for colorization during rendering. If the theme is nil, output will not be
// colorized.
func WithTheme(theme *chroma.Style) RendererOption {
	return func(r *Renderer) {
		r.theme = theme
	}
}

// WithHyperlinks enables or disables hyperlink rendering. When hyperlink rendering is enabled, links will be
// underlined and link destinations will be omitted. The destination of a link can be accessed by looking up the
// link's node in the span tree using the offset of the link text. Hyperlink rendering is disabled by default.
func WithHyperlinks(on bool) RendererOption {
	return func(r *Renderer) {
		r.hyperlinks = on
	}
}

// WithWordWrap enables word wrapping at the desired width. A width of zero disables wrapping. Words inside code spans
// or code blocks will not be subject to wrapping. Word wrapping is disabled by default.
func WithWordWrap(width int) RendererOption {
	return func(r *Renderer) {
		r.wordWrap = width
	}
}

// WithImages enables or disables image rendering. When image rendering is enabled, image links will be omitted
// and iamge data will be sent inline using the kitty graphics protocol. A line break will be inserted before
// and after each image. Image rendering is disabled by default.
func WithImages(on bool, maxWidth int, contentRoot string) RendererOption {
	return func(r *Renderer) {
		r.images = on
		r.maxImageWidth = maxWidth
		r.contentRoot = contentRoot
	}
}

// WithSoftBreak enables or disables soft line breaks. When soft line breaks are enabled, a soft line break in the
// input will _not_ be rendered as a newline in the output. When soft line breaks are disabled, a soft line break in
// the input _will_ be rendered as a newline. In general, soft line breaks should be enabled if word wrapping is
// enabled. Soft line breaks are dsiabled by default.
func WithSoftBreak(on bool) RendererOption {
	return func(r *Renderer) {
		r.softBreak = on
	}
}

// New creates a new Renderer with the given options.
func New(options ...RendererOption) *Renderer {
	var r Renderer
	for _, o := range options {
		o(&r)
	}
	return &r
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// blocks
	reg.Register(ast.KindDocument, r.RenderDocument)
	reg.Register(ast.KindHeading, r.RenderHeading)
	reg.Register(ast.KindBlockquote, r.RenderBlockquote)
	reg.Register(ast.KindCodeBlock, r.RenderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.RenderFencedCodeBlock)
	reg.Register(ast.KindHTMLBlock, r.RenderHTMLBlock)
	reg.Register(ast.KindLinkReferenceDefinition, r.RenderLinkReferenceDefinition)
	reg.Register(ast.KindList, r.RenderList)
	reg.Register(ast.KindListItem, r.RenderListItem)
	reg.Register(ast.KindParagraph, r.RenderParagraph)
	reg.Register(ast.KindTextBlock, r.RenderTextBlock)
	reg.Register(ast.KindThematicBreak, r.RenderThematicBreak)

	// extension blocks
	reg.Register(xast.KindTable, r.RenderTable)
	reg.Register(xast.KindTableHeader, r.RenderTableHeader)
	reg.Register(xast.KindTableRow, r.RenderTableRow)
	reg.Register(xast.KindTableCell, r.RenderTableCell)

	// inlines
	reg.Register(ast.KindAutoLink, r.RenderAutoLink)
	reg.Register(ast.KindCodeSpan, r.RenderCodeSpan)
	reg.Register(ast.KindEmphasis, r.RenderEmphasis)
	reg.Register(ast.KindImage, r.RenderImage)
	reg.Register(ast.KindLink, r.RenderLink)
	reg.Register(ast.KindRawHTML, r.RenderRawHTML)
	reg.Register(ast.KindText, r.RenderText)
	reg.Register(ast.KindString, r.RenderString)
	reg.Register(ast.KindWhitespace, r.RenderWhitespace)
}

// SpanTree returns the root of the rendered document's span tree. This tree maps AST nodes to their representative
// text spans in the renderer's output. This method must only be called after a call to RenderDocument.
func (r *Renderer) SpanTree() *NodeSpan {
	return r.rootSpan
}

func (r *Renderer) beginLine(w io.Writer) error {
	if len(r.openBlocks) != 0 {
		current := r.openBlocks[len(r.openBlocks)-1]
		if current.node.Kind() == ast.KindParagraph && !current.fresh {
			return nil
		}
	}

	n, err := w.Write(r.prefix)
	if n != 0 {
		r.atNewline = r.prefix[len(r.prefix)-1] == '\n'
		if !r.atNewline {
			r.lineWidth = n
		}
		r.byteOffset += n
	}
	return err
}

func (r *Renderer) writeLines(w util.BufWriter, source []byte, lines *text.Segments) error {
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		if _, err := r.Write(w, line.Value(source)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) writeCodeLines(w util.BufWriter, language string, source []byte, lines *text.Segments) error {
	if r.theme == nil {
		return r.writeLines(w, source, lines)
	}

	var buf strings.Builder
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}

	return r.writeCode(w, language, buf.String())
}

func (r *Renderer) writeCode(w util.BufWriter, language, code string) error {
	if r.theme == nil {
		_, err := r.WriteString(w, code)
		return err
	}

	var lexer chroma.Lexer
	if language == "" {
		lexer = lexers.Analyse(code)
	} else {
		lexer = lexers.Get(language)
	}
	if lexer == nil {
		_, err := r.WriteString(w, code)
		return err
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return err
	}

	for token := iterator(); token != chroma.EOF; token = iterator() {
		if err := r.PushStyle(w, token.Type); err != nil {
			return err
		}
		if _, err := r.WriteString(w, token.Value); err != nil {
			return err
		}
		if err := r.PopStyle(w); err != nil {
			return err
		}
	}

	return nil
}

// WordWrap returns true if word wrapping is enabled within the current rendering context.
func (r *Renderer) WordWrap() bool {
	return r.wordWrap > 0 && len(r.wrapping) > 0 && r.wrapping[len(r.wrapping)-1]
}

// PushWordWrap enables or disables word wrapping in the current rendering context.
func (r *Renderer) PushWordWrap(wrap bool) {
	r.wrapping = append(r.wrapping, wrap)
}

// PopWordWrap restores the word wrap setting prior to the last call to PushWordWrap.
func (r *Renderer) PopWordWrap() {
	r.wrapping = r.wrapping[:len(r.wrapping)-1]
}

type writer struct {
	r *Renderer
	w io.Writer
}

func (w *writer) Write(b []byte) (int, error) {
	return w.r.Write(w.w, b)
}

// Writer returns an io.Writer that uses the Renderer's Write method to ensure appropriate indentation and prefices
// are added at the beginning of each line.
func (r *Renderer) Writer(w io.Writer) io.Writer {
	return &writer{r: r, w: w}
}

func (r *Renderer) measureText(buf []byte) int {
	// Measure each segment of the word that is bounded by control codes.
	width := 0
	for start, end := 0, 0; start < len(buf); {
		if _, sz := ansicsi.Decode(buf[end:]); sz != 0 || end == len(buf) {
			width += uniseg.GraphemeClusterCount(string(buf[start:end]))
			start = end + sz
			end = start
		} else {
			end++
		}
	}
	return width
}

// write writes a slice of bytes to an io.Writer, ensuring that appropriate indentation and prefices
// are added at the beginning of each line.
func (r *Renderer) write(w io.Writer, buf []byte) (int, error) {
	written := 0
	for len(buf) > 0 {
		hasNewline := false
		newline := bytes.IndexByte(buf, '\n')
		if newline == -1 {
			newline = len(buf)
		} else {
			hasNewline = true
		}

		if r.atNewline && r.measureText(buf[:newline]) != 0 {
			if err := r.beginLine(w); err != nil {
				return written, err
			}
		}

		// write up to the newline
		n, err := w.Write(buf[:newline])
		written += n

		// measure the text we just wrote
		writtenWidth := r.measureText(buf[:n])

		if err == nil && hasNewline && n == newline {
			// pad out to the wrap width if necessary
			remaining := r.wordWrap - (r.lineWidth + writtenWidth)
			switch {
			case remaining < 0:
				_, err = w.Write(bytes.Repeat([]byte{' '}, r.wordWrap-(-remaining%r.wordWrap)))
			case remaining > 0:
				_, err = w.Write(bytes.Repeat([]byte{' '}, remaining))
			}

			if err == nil {
				// write the newline
				if _, err = w.Write([]byte{'\n'}); err == nil {
					n++
				}
			}
		}

		r.atNewline = r.atNewline && writtenWidth == 0 || hasNewline && n == newline+1
		if r.atNewline {
			r.lineWidth = 0
		} else {
			// NOTE: the count will be off if we have a partial code point or control code at the end of the write.
			r.lineWidth += writtenWidth
		}
		if len(r.openBlocks) != 0 {
			r.openBlocks[len(r.openBlocks)-1].fresh = false
		}
		if err != nil {
			return written, err
		}
		buf = buf[n:]
	}
	return written, nil
}

func (r *Renderer) flushWordBuffer(w io.Writer) error {
	buf := r.wordBuffer.Bytes()
	wordWidth := r.measureText(buf)

	// Flush the buffer. If the currently buffered word would exceed the wrap point, write a newline.
	if r.lineWidth > 0 && r.lineWidth+wordWidth >= r.wordWrap {
		_, err := r.write(w, []byte{'\n'})
		if err != nil {
			return err
		}
		r.byteOffset++
	}
	_, err := r.write(w, buf)
	r.wordBuffer.Reset()
	return err
}

// Write writes a slice of bytes to an io.Writer, ensuring that appropriate indentation and prefices
// are added at the beginning of each line.
func (r *Renderer) Write(w io.Writer, buf []byte) (int, error) {
	if !r.WordWrap() {
		// If there is data in the word buffer, then we've hit a transition between wrapping and no wrapping.
		// Empty the buffer here, wrapping as usual, then write the current bytes out.
		if r.wordBuffer.Len() > 0 {
			if err := r.flushWordBuffer(w); err != nil {
				return 0, err
			}
		}
		n, err := r.write(w, buf)
		r.byteOffset += n
		return n, err
	}

	written := 0
	for len(buf) > 0 {
		c, sz := utf8.DecodeRune(buf)
		if unicode.IsSpace(c) {
			// Flush the word buffer, then write out the whitespace.
			if err := r.flushWordBuffer(w); err != nil {
				return written, err
			}
			if _, err := r.write(w, buf[:sz]); err != nil {
				return written, err
			}
		} else {
			r.wordBuffer.Write(buf[:sz])
		}
		r.byteOffset += sz
		buf, written = buf[sz:], written+sz
	}
	return written, nil
}

// WriteByte writes a byte to an io.Writer, ensuring that appropriate indentation and prefices are added at the beginning
// of each line.
func (r *Renderer) WriteByte(w io.Writer, c byte) error {
	_, err := r.Write(w, []byte{c})
	return err
}

// WriteRune writes a rune to an io.Writer, ensuring that appropriate indentation and prefices are added at the beginning
// of each line.
func (r *Renderer) WriteRune(w io.Writer, c rune) (int, error) {
	buf := make([]byte, utf8.UTFMax)
	sz := utf8.EncodeRune(buf, c)
	return r.Write(w, buf[:sz])
}

// WriteString writes a string to an io.Writer, ensuring that appropriate indentation and prefices are added at the
// beginning of each line.
func (r *Renderer) WriteString(w io.Writer, s string) (int, error) {
	return r.Write(w, []byte(s))
}

// Prefix returns the prefix for the current line, if any.
func (r *Renderer) Prefix() string {
	return string(r.prefix)
}

// PushIndent adds the specified amount of indentation to the current line prefix.
func (r *Renderer) PushIndent(amount int) {
	r.PushPrefix(strings.Repeat(" ", amount))
}

// PushPrefix adds the specified string to the current line prefix.
func (r *Renderer) PushPrefix(prefix string) {
	r.prefixStack = append(r.prefixStack, prefix)
	r.prefix = append(r.prefix, []byte(prefix)...)
}

// PopPrefix removes the last piece added by a call to PushIndent or PushPrefix from the current line prefix.
func (r *Renderer) PopPrefix() {
	r.prefix = r.prefix[:len(r.prefix)-len(r.prefixStack[len(r.prefixStack)-1])]
	r.prefixStack = r.prefixStack[:len(r.prefixStack)-1]
}

// OpenSpan begins a new span associated with the given node.
func (r *Renderer) OpenSpan(node ast.Node) {
	span := &NodeSpan{
		Start: r.byteOffset,
		Node:  node,
	}

	if len(r.spanStack) != 0 {
		span.Parent = r.spanStack[len(r.spanStack)-1]
		span.Parent.Children = append(span.Parent.Children, span)
	} else {
		r.rootSpan = span
	}

	if r.lastSpan != nil {
		span.Prev = r.lastSpan
		span.Prev.Next = span
	}
	r.lastSpan = span

	r.spanStack = append(r.spanStack, span)
}

// CloseSpan closes the current span.
func (r *Renderer) CloseSpan() {
	span := r.spanStack[len(r.spanStack)-1]
	r.spanStack = r.spanStack[:len(r.spanStack)-1]
	span.End = r.byteOffset
}

// OpenBlock ensures that each block begins on a new line, and that blank lines are inserted before blocks as
// indicated by node.HasPreviousBlankLines.
func (r *Renderer) OpenBlock(w util.BufWriter, source []byte, node ast.Node) error {
	r.OpenSpan(node)

	r.openBlocks = append(r.openBlocks, blockState{
		node:  node,
		fresh: true,
	})

	// Work around the fact that the first child of a node notices the same set of preceding blank lines as its parent.
	hasBlankPreviousLines := node.HasBlankPreviousLines()
	if p := node.Parent(); p != nil && p.FirstChild() == node {
		if p.Kind() == ast.KindDocument || p.Kind() == ast.KindListItem || p.HasBlankPreviousLines() {
			hasBlankPreviousLines = false
		}
	}

	if hasBlankPreviousLines {
		if err := r.WriteByte(w, '\n'); err != nil {
			return err
		}
	}

	if ws := node.LeadingWhitespace(); ws.Len() != 0 {
		if _, err := r.Write(w, ws.Value(source)); err != nil {
			return err
		}
	}

	r.openBlocks[len(r.openBlocks)-1].fresh = true

	return nil
}

// CloseBlock marks the current block as closed.
func (r *Renderer) CloseBlock(w io.Writer) error {
	if !r.atNewline {
		if err := r.WriteByte(w, '\n'); err != nil {
			return err
		}
	}

	r.openBlocks = r.openBlocks[:len(r.openBlocks)-1]
	r.CloseSpan()
	return nil
}

// RenderDocument renders an *ast.Document node to the given BufWriter.
func (r *Renderer) RenderDocument(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	r.listStack, r.prefixStack, r.prefix, r.wrapping, r.atNewline = nil, nil, nil, []bool{true}, false

	if enter {
		r.OpenSpan(node)

		r.styles = nil
		if err := r.PushStyle(w, chroma.Generic); err != nil {
			return ast.WalkStop, err
		}
	} else {
		if err := r.PopStyle(w); err != nil {
			return ast.WalkStop, err
		}
		r.styles = nil

		r.CloseSpan()
	}

	return ast.WalkContinue, nil
}

// RenderHeading renders an *ast.Heading node to the given BufWriter.
func (r *Renderer) RenderHeading(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := r.OpenBlock(w, source, node); err != nil {
			return ast.WalkStop, err
		}

		r.PushWordWrap(false)

		style := chroma.GenericHeading
		if node.(*ast.Heading).Level > 2 {
			style = chroma.GenericSubheading
		}
		if err := r.PushStyle(w, style); err != nil {
			return ast.WalkStop, err
		}

		if !node.(*ast.Heading).IsSetext {
			if _, err := r.WriteString(w, strings.Repeat("#", node.(*ast.Heading).Level)); err != nil {
				return ast.WalkStop, err
			}
			if err := r.WriteByte(w, ' '); err != nil {
				return ast.WalkStop, err
			}
		}
	} else {
		if node.(*ast.Heading).IsSetext {
			s := "==="
			if node.(*ast.Heading).Level == 2 {
				s = "---"
			}
			if !r.atNewline {
				if err := r.WriteByte(w, '\n'); err != nil {
					return ast.WalkStop, err
				}
			}
			if _, err := r.WriteString(w, s); err != nil {
				return ast.WalkStop, err
			}
		}

		if err := r.PopStyle(w); err != nil {
			return ast.WalkStop, err
		}

		if err := r.WriteByte(w, '\n'); err != nil {
			return ast.WalkStop, err
		}

		r.PopWordWrap()

		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderBlockquote renders an *ast.Blockquote node to the given BufWriter.
func (r *Renderer) RenderBlockquote(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := r.OpenBlock(w, source, node); err != nil {
			return ast.WalkStop, err
		}

		// TODO:
		// - case 63, an setext heading in a lazy blockquote
		// - case 208, a list item in a lazy blockquote
		// - cases 262 and 263, a blockquote in a list item

		if err := r.PushStyle(w, chroma.GenericEmph); err != nil {
			return ast.WalkStop, err
		}

		if _, err := r.WriteString(w, "> "); err != nil {
			return ast.WalkStop, err
		}
		r.PushPrefix("> ")
	} else {
		r.PopPrefix()

		if err := r.PopStyle(w); err != nil {
			return ast.WalkStop, err
		}

		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderCodeBlock renders an *ast.CodeBlock node to the given BufWriter.
func (r *Renderer) RenderCodeBlock(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		r.PopWordWrap()

		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	if err := r.OpenBlock(w, source, node); err != nil {
		return ast.WalkStop, err
	}

	r.PushWordWrap(false)

	// Each line of a code block needs to be aligned at the same offset, and a code block must start with at least four
	// spaces. To achieve this, we unconditionally add four spaces to the first line of the code block and indent the
	// rest as necessary.
	if _, err := r.WriteString(w, "    "); err != nil {
		return ast.WalkStop, err
	}

	r.PushIndent(4)
	defer r.PopPrefix()

	if err := r.writeCodeLines(w, "", source, node.Lines()); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkContinue, nil
}

// RenderFencedCodeBlock renders an *ast.FencedCodeBlock node to the given BufWriter.
func (r *Renderer) RenderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		if err := r.PopStyle(w); err != nil {
			return ast.WalkStop, err
		}

		r.PopWordWrap()

		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	if err := r.OpenBlock(w, source, node); err != nil {
		return ast.WalkStop, err
	}

	r.PushWordWrap(false)

	if err := r.PushStyle(w, chroma.LiteralStringHeredoc); err != nil {
		return ast.WalkStop, err
	}

	code := node.(*ast.FencedCodeBlock)

	// Write the start of the fenced code block.
	fence := code.Fence
	if _, err := r.Write(w, fence); err != nil {
		return ast.WalkStop, err
	}
	language := code.Language(source)
	if _, err := r.Write(w, language); err != nil {
		return ast.WalkStop, err
	}
	if err := r.WriteByte(w, '\n'); err != nil {
		return ast.WalkStop, nil
	}

	// Write the contents of the fenced code block.
	if err := r.writeCodeLines(w, string(language), source, node.Lines()); err != nil {
		return ast.WalkStop, err
	}

	// Write the end of the fenced code block.
	if err := r.beginLine(w); err != nil {
		return ast.WalkStop, err
	}
	if _, err := r.Write(w, fence); err != nil {
		return ast.WalkStop, err
	}
	if err := r.WriteByte(w, '\n'); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkContinue, nil
}

// RenderHTMLBlock renders an *ast.HTMLBlock node to the given BufWriter.
func (r *Renderer) RenderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		r.PopWordWrap()

		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	if err := r.OpenBlock(w, source, node); err != nil {
		return ast.WalkStop, err
	}

	r.PushWordWrap(false)

	// Write the contents of the HTML block.
	if err := r.writeLines(w, source, node.Lines()); err != nil {
		return ast.WalkStop, err
	}

	// Write the closure line, if any.
	html := node.(*ast.HTMLBlock)
	if html.HasClosure() {
		if _, err := r.Write(w, html.ClosureLine.Value(source)); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderLinkReferenceDefinition renders an *ast.LinkReferenceDefinition node to the given BufWriter.
func (r *Renderer) RenderLinkReferenceDefinition(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		r.PopWordWrap()

		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	if err := r.OpenBlock(w, source, node); err != nil {
		return ast.WalkStop, err
	}

	r.PushWordWrap(true)

	// Write the contents of the link reference definition.
	if err := r.writeLines(w, source, node.Lines()); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkContinue, nil
}

// RenderList renders an *ast.List node to the given BufWriter.
func (r *Renderer) RenderList(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := r.OpenBlock(w, source, node); err != nil {
			return ast.WalkStop, err
		}

		list := node.(*ast.List)
		r.listStack = append(r.listStack, listState{
			marker:  list.Marker,
			ordered: list.IsOrdered(),
			index:   list.Start,
		})
	} else {
		r.listStack = r.listStack[:len(r.listStack)-1]
		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderListItem renders an *ast.ListItem node to the given BufWriter.
func (r *Renderer) RenderListItem(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := r.OpenBlock(w, source, node); err != nil {
			return ast.WalkStop, err
		}

		// TODO:
		// - case 227, a code block following a list item

		markerWidth := 2
		state := &r.listStack[len(r.listStack)-1]
		if state.ordered {
			width, err := r.WriteString(w, strconv.FormatInt(int64(state.index), 10))
			if err != nil {
				return ast.WalkStop, err
			}
			state.index++
			markerWidth += width
		}
		if _, err := r.Write(w, []byte{state.marker, ' '}); err != nil {
			return ast.WalkStop, err
		}

		ws := node.LeadingWhitespace()
		offset := markerWidth + ws.Len()
		if o := node.(*ast.ListItem).Offset; offset < o {
			if _, err := r.Write(w, bytes.Repeat([]byte{' '}, o-offset)); err != nil {
				return ast.WalkStop, err
			}
			offset = o
		}
		r.PushIndent(offset)
	} else {
		r.PopPrefix()
		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderParagraph renders an *ast.Paragraph node to the given BufWriter.
func (r *Renderer) RenderParagraph(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if enter {
		// A paragraph that follows another paragraph or a blockquote must be preceded by a blank line.
		if !node.HasBlankPreviousLines() {
			if prev := node.PreviousSibling(); prev != nil && (prev.Kind() == ast.KindParagraph || prev.Kind() == ast.KindBlockquote) {
				if err := r.WriteByte(w, '\n'); err != nil {
					return ast.WalkStop, err
				}
			}
		}

		if err := r.OpenBlock(w, source, node); err != nil {
			return ast.WalkStop, err
		}
		r.PushWordWrap(true)
	} else {
		r.PopWordWrap()
		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderTextBlock renders an *ast.TextBlock node to the given BufWriter.
func (r *Renderer) RenderTextBlock(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := r.OpenBlock(w, source, node); err != nil {
			return ast.WalkStop, err
		}
		r.PushWordWrap(true)
	} else {
		r.PopWordWrap()
		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderThematicBreak renders an *ast.ThematicBreak node to the given BufWriter.
func (r *Renderer) RenderThematicBreak(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	if err := r.OpenBlock(w, source, node); err != nil {
		return ast.WalkStop, err
	}

	if _, err := r.WriteString(w, "***\n"); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkContinue, nil
}

// RenderAutoLink renders an *ast.AutoLink node to the given BufWriter.
func (r *Renderer) RenderAutoLink(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		return ast.WalkContinue, nil
	}

	if err := r.WriteByte(w, '<'); err != nil {
		return ast.WalkStop, err
	}
	if _, err := r.Write(w, node.(*ast.AutoLink).Label(source)); err != nil {
		return ast.WalkStop, err
	}
	if err := r.WriteByte(w, '>'); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkContinue, nil
}

func (r *Renderer) shouldPadCodeSpan(source []byte, node *ast.CodeSpan) bool {
	c := node.FirstChild()
	if c == nil {
		return false
	}

	segment := c.(*ast.Text).Segment
	text := segment.Value(source)

	var firstChar byte
	if len(text) > 0 {
		firstChar = text[0]
	}

	allWhitespace := true
	for {
		if util.FirstNonSpacePosition(text) != -1 {
			allWhitespace = false
			break
		}
		c = c.NextSibling()
		if c == nil {
			break
		}
		segment = c.(*ast.Text).Segment
		text = segment.Value(source)
	}
	if allWhitespace {
		return false
	}

	var lastChar byte
	if len(text) > 0 {
		lastChar = text[len(text)-1]
	}

	return firstChar == '`' || firstChar == ' ' || lastChar == '`' || lastChar == ' '
}

// RenderCodeSpan renders an *ast.CodeSpan node to the given BufWriter.
func (r *Renderer) RenderCodeSpan(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		r.PopWordWrap()
		r.CloseSpan()
		return ast.WalkContinue, nil
	}

	r.OpenSpan(node)
	r.PushWordWrap(false)

	// TODO:
	// - case 330, 331, single space stripping -> contents need an additional leading and trailing space
	// - case 339, backtick inside text -> start/end need additional backtick

	code := node.(*ast.CodeSpan)
	delimiter := bytes.Repeat([]byte{'`'}, code.Backticks)
	pad := r.shouldPadCodeSpan(source, code)

	if _, err := r.Write(w, delimiter); err != nil {
		return ast.WalkStop, err
	}
	if pad {
		if err := r.WriteByte(w, ' '); err != nil {
			return ast.WalkStop, err
		}
	}

	var buf strings.Builder
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		text := c.(*ast.Text).Segment
		buf.Write(text.Value(source))
	}
	if err := r.writeCode(w, "", buf.String()); err != nil {
		return ast.WalkStop, err
	}

	if pad {
		if err := r.WriteByte(w, ' '); err != nil {
			return ast.WalkStop, err
		}
	}
	if _, err := r.Write(w, delimiter); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkSkipChildren, nil
}

// RenderEmphasis renders an *ast.Emphasis node to the given BufWriter.
func (r *Renderer) RenderEmphasis(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if enter {
		r.OpenSpan(node)
	} else {
		r.CloseSpan()
	}

	em := node.(*ast.Emphasis)
	if _, err := r.WriteString(w, strings.Repeat(string([]byte{em.Marker}), em.Level)); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) escapeLinkDest(dest []byte) []byte {
	requiresEscaping := false
	for _, c := range dest {
		if c <= 32 || c == '(' || c == ')' || c == 127 {
			requiresEscaping = true
			break
		}
	}
	if !requiresEscaping {
		return dest
	}

	escaped := make([]byte, 0, len(dest)+2)
	escaped = append(escaped, '<')
	for _, c := range dest {
		if c == '<' || c == '>' {
			escaped = append(escaped, '\\')
		}
		escaped = append(escaped, c)
	}
	escaped = append(escaped, '>')
	return escaped
}

func (r *Renderer) linkTitleDelimiter(title []byte) byte {
	for i, c := range title {
		if c == '"' && (i == 0 || title[i-1] != '\\') {
			return '\''
		}
	}
	return '"'
}

func (r *Renderer) renderHyperlink(w util.BufWriter, node ast.Node, open string, refType ast.LinkReferenceType, label, dest, title []byte, enter bool) error {
	if enter {
		if err := r.PushStyle(w, chroma.GenericUnderline); err != nil {
			return err
		}
	} else {
		if err := r.PopStyle(w); err != nil {
			return err
		}
	}

	return nil
}

func (r *Renderer) renderLinkOrImage(w util.BufWriter, node ast.Node, open string, refType ast.LinkReferenceType, label, dest, title []byte, enter bool) error {
	if enter {
		r.OpenSpan(node)
	} else {
		r.CloseSpan()
	}

	if r.hyperlinks {
		return r.renderHyperlink(w, node, open, refType, label, dest, title, enter)
	}

	if enter {
		if _, err := r.WriteString(w, open); err != nil {
			return err
		}
	} else {
		switch refType {
		case ast.LinkFullReference:
			if _, err := r.WriteString(w, "]["); err != nil {
				return err
			}
			if _, err := r.Write(w, label); err != nil {
				return err
			}
			if err := r.WriteByte(w, ']'); err != nil {
				return err
			}
		case ast.LinkCollapsedReference:
			if _, err := r.WriteString(w, "][]"); err != nil {
				return err
			}
		case ast.LinkShortcutReference:
			if err := r.WriteByte(w, ']'); err != nil {
				return err
			}
		default:
			if _, err := r.WriteString(w, "]("); err != nil {
				return err
			}

			if _, err := r.Write(w, r.escapeLinkDest(dest)); err != nil {
				return err
			}
			if len(title) != 0 {
				delimiter := r.linkTitleDelimiter(title)
				if _, err := fmt.Fprintf(w, ` %c%s%c`, delimiter, string(title), delimiter); err != nil {
					return err
				}
			}

			if err := r.WriteByte(w, ')'); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Renderer) openImage(location string) (io.ReadCloser, error) {
	parsedLocation, err := url.Parse(location)
	if err != nil {
		return nil, err
	}

	// If this is a relative URL, append it to the content root and re-parse.
	if !parsedLocation.IsAbs() {
		parsedLocation, err = url.Parse(path.Join(r.contentRoot, location))
		if err != nil {
			return nil, err
		}

		// If we still have a relative URL, treat it as relative to the current directory.
		if !parsedLocation.IsAbs() {
			parsedLocation.Path = "./" + parsedLocation.Path
			parsedLocation.RawPath = ""
		}
	}

	switch parsedLocation.Scheme {
	case "", "file":
		return os.Open(parsedLocation.Path)
	case "http", "https":
		resp, err := http.DefaultClient.Do(&http.Request{URL: parsedLocation, Method: http.MethodGet})
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	default:
		return nil, fmt.Errorf("unsupported scheme %v", parsedLocation.Scheme)
	}
}

func (r *Renderer) renderImage(w util.BufWriter, source []byte, img *ast.Image, enter bool) error {
	reader, err := r.openImage(string(img.Destination))
	if err != nil {
		return err
	}
	defer reader.Close()

	image, _, err := image.Decode(reader)
	if err != nil {
		return err
	}

	image = resize.Thumbnail(uint(r.maxImageWidth), uint(image.Bounds().Dy()), image, resize.Bicubic)

	var buf bytes.Buffer
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	if err := png.Encode(enc, image); err != nil {
		return err
	}
	enc.Close()
	data := buf.Bytes()

	if _, err = fmt.Fprint(w, "\n"); err != nil {
		return err
	}

	first := true
	for len(data) > 0 {
		if first {
			if _, err = fmt.Fprintf(w, "\x1b_Gf=100,a=T,"); err != nil {
				return err
			}
			first = false
		} else {
			if _, err = fmt.Fprint(w, "\x1b_G"); err != nil {
				return err
			}
		}

		more, b := 0, data
		if len(data) > 4096 {
			more, b = 1, data[:4096]
		}
		if _, err = fmt.Fprintf(w, "m=%d;", more); err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if _, err = fmt.Fprint(w, "\x1b\\"); err != nil {
			return err
		}

		data = data[len(b):]
	}

	if _, err = fmt.Fprint(w, "\n"); err != nil {
		return err
	}

	return nil
}

// RenderImage renders an *ast.Image node to the given BufWriter.
func (r *Renderer) RenderImage(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	img := node.(*ast.Image)

	if r.images {
		if enter {
			if err := r.renderImage(w, source, img, enter); err == nil {
				r.inImage = true
				return ast.WalkSkipChildren, nil
			}
		} else if r.inImage {
			r.inImage = false
			return ast.WalkContinue, nil
		}
	}

	if err := r.renderLinkOrImage(w, node, "![", img.ReferenceType, img.Label, img.Destination, img.Title, enter); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkContinue, nil
}

// RenderLink renders an *ast.Link node to the given BufWriter.
func (r *Renderer) RenderLink(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	link := node.(*ast.Link)
	if err := r.renderLinkOrImage(w, node, "[", link.ReferenceType, link.Label, link.Destination, link.Title, enter); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkContinue, nil
}

// RenderRawHTML renders an *ast.RawHTML node to the given BufWriter.
func (r *Renderer) RenderRawHTML(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		r.PopWordWrap()
		r.CloseSpan()
		return ast.WalkSkipChildren, nil
	}

	r.OpenSpan(node)
	r.PushWordWrap(false)

	raw := node.(*ast.RawHTML)
	for i := 0; i < raw.Segments.Len(); i++ {
		segment := raw.Segments.At(i)
		if _, err := r.Write(w, segment.Value(source)); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkSkipChildren, nil
}

func isBlank(bytes []byte) bool {
	for _, b := range bytes {
		if b != ' ' {
			return false
		}
	}
	return true
}

// RenderText renders an *ast.Text node to the given BufWriter.
func (r *Renderer) RenderText(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		r.CloseSpan()
		return ast.WalkContinue, nil
	}

	r.OpenSpan(node)

	text := node.(*ast.Text)
	value := text.Segment.Value(source)

	if _, err := r.Write(w, value); err != nil {
		return ast.WalkStop, err
	}
	switch {
	case text.HardLineBreak():
		if _, err := r.WriteString(w, "\\\n"); err != nil {
			return ast.WalkStop, err
		}
	case text.SoftLineBreak():
		c := '\n'
		if r.softBreak {
			c = ' '
		}

		if err := r.WriteByte(w, byte(c)); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderString renders an *ast.String node to the given BufWriter.
func (r *Renderer) RenderString(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		r.CloseSpan()
		return ast.WalkContinue, nil
	}

	r.OpenSpan(node)

	str := node.(*ast.String)
	if _, err := r.Write(w, str.Value); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkContinue, nil
}

// RenderWhitespace renders an *ast.Text node to the given BufWriter.
func (r *Renderer) RenderWhitespace(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		r.CloseSpan()
		return ast.WalkContinue, nil
	}

	r.OpenSpan(node)

	if _, err := r.Write(w, node.(*ast.Whitespace).Segment.Value(source)); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkContinue, nil
}

func (r *Renderer) renderTableBorder(w util.BufWriter, left, join, right rune) error {
	state := &r.tableStack[len(r.tableStack)-1]
	horizontal := borders.horizontal()

	if _, err := r.WriteRune(w, left); err != nil {
		return err
	}
	for i, width := range state.columnWidths {
		if i > 0 {
			if _, err := r.WriteRune(w, join); err != nil {
				return err
			}
		}
		if _, err := r.WriteString(w, strings.Repeat(horizontal, width)); err != nil {
			return err
		}
	}
	if _, err := r.WriteRune(w, right); err != nil {
		return err
	}
	return r.WriteByte(w, '\n')
}

// RenderTable renders an *xast.Table to the given BufWriter.
func (r *Renderer) RenderTable(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		if err := r.renderTableBorder(w, borders.bottomLeft(), borders.bottomJoin(), borders.bottomRight()); err != nil {
			return ast.WalkStop, err
		}

		r.tableStack = r.tableStack[:len(r.tableStack)-1]
		if err := r.CloseBlock(w); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	if err := r.OpenBlock(w, source, node); err != nil {
		return ast.WalkStop, err
	}

	// A table is structured like so:
	// table/
	//   TableHeader/
	//     TableCell
	//     ...
	//     TableCell
	//   TableRow/
	//     TableCell
	//     ...
	//     TableCell
	//   ...
	//   TableRow/
	//     TableCell
	//     ...
	//     TableCell
	table := node.(*xast.Table)

	// First, measure the width of each column by rendering each cell in each column's contents into an infinitely-wide
	// buffer and finding the maximum. This also allows us to count the columns.
	var columnWidths []int
	var cellWidths []int
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		for col, cell := 0, row.FirstChild(); cell != nil; col, cell = col+1, cell.NextSibling() {
			cr := &Renderer{
				theme:         r.theme,
				wordWrap:      0,
				hyperlinks:    r.hyperlinks,
				images:        r.images,
				maxImageWidth: r.maxImageWidth,
				contentRoot:   r.contentRoot,
				softBreak:     r.softBreak,
				tableStack:    []tableState{{measuring: true}},
			}
			cellRenderer := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(cr, 100)))
			dest := &countingWriter{}
			if err := cellRenderer.Render(dest, source, cell); err != nil {
				return ast.WalkStop, err
			}

			for col >= len(columnWidths) {
				columnWidths = append(columnWidths, 0)
			}
			if columnWidths[col] < dest.n {
				columnWidths[col] = dest.n
			}
			cellWidths = append(cellWidths, dest.n)
		}
	}

	r.tableStack = append(r.tableStack, tableState{
		columnWidths: columnWidths,
		cellWidths:   cellWidths,
		alignments:   table.Alignments,
	})

	return ast.WalkContinue, nil
}

func (r *Renderer) RenderTableHeader(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if enter {
		left, join, right := borders.topLeft(), borders.topJoin(), borders.topRight()
		if err := r.renderTableBorder(w, left, join, right); err != nil {
			return ast.WalkStop, err
		}
		if _, err := r.WriteRune(w, borders.vertical()); err != nil {
			return ast.WalkStop, err
		}
	} else {
		if _, err := r.WriteRune(w, borders.vertical()); err != nil {
			return ast.WalkStop, err
		}
		if err := r.WriteByte(w, '\n'); err != nil {
			return ast.WalkStop, err
		}

		left, join, right := borders.middleLeft(), borders.middleJoin(), borders.middleRight()
		if err := r.renderTableBorder(w, left, join, right); err != nil {
			return ast.WalkStop, err
		}

		state := &r.tableStack[len(r.tableStack)-1]
		state.columnIndex = 0
		state.rowIndex++
	}

	return ast.WalkContinue, nil
}

func (r *Renderer) RenderTableRow(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	state := &r.tableStack[len(r.tableStack)-1]

	if _, err := r.WriteRune(w, borders.vertical()); err != nil {
		return ast.WalkStop, err
	}
	if !enter {
		if _, err := r.WriteRune(w, '\n'); err != nil {
			return ast.WalkStop, err
		}

		state.columnIndex = 0
		state.rowIndex++
	}

	return ast.WalkContinue, nil
}

func (r *Renderer) RenderTableCell(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	state := &r.tableStack[len(r.tableStack)-1]
	if !state.measuring {
		if enter {
			if state.rowIndex == 0 && state.columnIndex > 0 {
				if _, err := r.WriteRune(w, borders.vertical()); err != nil {
					return ast.WalkStop, err
				}
			}

			var style chroma.TokenType
			switch {
			case state.rowIndex == 0:
				style = styles.TableHeader
			case state.rowIndex%2 == 0:
				style = styles.TableRowAlt
			default:
				style = styles.TableRow
			}
			if err := r.PushStyle(w, style); err != nil {
				return ast.WalkStop, err
			}

			if state.rowIndex != 0 && state.columnIndex > 0 {
				if _, err := r.WriteRune(w, borders.vertical()); err != nil {
					return ast.WalkStop, err
				}
			}
		} else {
			columnWidth := state.columnWidths[state.columnIndex]
			cellWidth := state.cellWidths[state.cellIndex]

			if _, err := r.WriteString(w, strings.Repeat(" ", columnWidth-cellWidth)); err != nil {
				return ast.WalkStop, err
			}

			if err := r.PopStyle(w); err != nil {
				return ast.WalkStop, err
			}

			state.columnIndex++
			state.cellIndex++
		}
	}

	return ast.WalkContinue, nil
}
