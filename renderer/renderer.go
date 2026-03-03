package renderer

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
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
	"github.com/charmbracelet/x/ansi"
	"github.com/eliukblau/pixterm/pkg/ansimage"
	"github.com/nfnt/resize"
	"github.com/pgavlin/goldmark/ast"
	xast "github.com/pgavlin/goldmark/extension/ast"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/internal/kitty"
	"github.com/pgavlin/markdown-kit/styles"
	svg "github.com/pgavlin/svg2"
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

// An ImageEncoder converts an image to a binary representation that can be displayed by the target output device.
type ImageEncoder func(w io.Writer, image image.Image, r *Renderer) (int, error)

// A KittyGraphicsEncoder encodes image data to a Writer using the kitty graphics protocol.
func KittyGraphicsEncoder() ImageEncoder {
	return func(w io.Writer, image image.Image, r *Renderer) (int, error) {
		return kitty.Encode(w, image)
	}
}

// An ANSIGraphicsEncoder encodes images to a Writer using ANSI or ASCII characters.
func ANSIGraphicsEncoder(bg color.Color, ditherMode ansimage.DitheringMode) ImageEncoder {
	return func(w io.Writer, image image.Image, r *Renderer) (int, error) {
		if r.WordWrap() && r.cols != 0 {
			cellWidth := r.width / r.cols

			// compute the image's width in cells
			imageWidthInCells := int(math.Ceil(float64(image.Bounds().Dx()) / float64(cellWidth)))

			// if the encoder will use more cells than expected, scale the image down
			maxWidthInCells := r.wordWrap
			if imageWidthInCells < maxWidthInCells {
				maxWidthInCells = imageWidthInCells
			}

			image = resize.Thumbnail(uint(maxWidthInCells*ansimage.BlockSizeX), uint(image.Bounds().Dy()), image, resize.Bicubic)
		}
		ansi, err := ansimage.NewFromImage(image, bg, ditherMode)
		if err != nil {
			return 0, err
		}
		return w.Write([]byte(ansi.Render()))
	}
}

// Renderer is a goldmark renderer that produces Markdown output. Due to information loss in goldmark, its output may
// not be textually identical to the source that produced the AST to be rendered, but the structure should match.
//
// NodeRenderers that want to override rendering of particular node types should write through the Write* functions
// provided by Renderer in order to retain proper indentation and prefices inside of lists and block quotes.
type Renderer struct {
	theme         *chroma.Style
	cols          int
	rows          int
	width         int
	height        int
	wordWrap      int
	hyperlinks    bool
	images        bool
	maxImageWidth int
	contentRoot   string
	imageEncoder  ImageEncoder
	softBreak     bool
	padToWrap     []int
	noBreak       int // nesting counter; when > 0, spaces don't break words

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

// WithPad enables padding each line of output to the wrap width. This option has no effect when word wrapping is
// disabled.
func WithPad(enabled bool) RendererOption {
	return func(r *Renderer) {
		if enabled {
			r.padToWrap = []int{r.wordWrap}
		}
	}
}

// PushPad enables line padding to the given width in the current rendering context.
// A width of 0 disables padding.
func (r *Renderer) PushPad(width int) {
	r.padToWrap = append(r.padToWrap, width)
}

// PopPad restores the previous padding state.
func (r *Renderer) PopPad() {
	if len(r.padToWrap) > 0 {
		r.padToWrap = r.padToWrap[:len(r.padToWrap)-1]
	}
}

// WithImages enables or disables image rendering. When image rendering is enabled, image links will be omitted
// and image data will be sent inline using the renderer's image encoder. The default image encoder encodes
// image data using the kitty graphics protocol; the image encoder can be changed using the WithImageEncoder
// option. Image rendering is disabled by default.
func WithImages(on bool, maxWidth int, contentRoot string) RendererOption {
	return func(r *Renderer) {
		r.images = on
		r.maxImageWidth = maxWidth
		r.contentRoot = contentRoot
	}
}

// WithGeometry sets the geometry of the output.
func WithGeometry(cols, rows, width, height int) RendererOption {
	return func(r *Renderer) {
		r.cols = cols
		r.rows = rows
		r.width = width
		r.height = height
	}
}

// WithImageEncoder sets the image encoder used by the renderer. The default image encoder encodes image
// data using the kitty graphics protocol.
func WithImageEncoder(encoder ImageEncoder) RendererOption {
	return func(r *Renderer) {
		r.imageEncoder = encoder
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
	return ansi.StringWidth(string(buf))
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

		// measure the text we just wrote
		writtenWidth := r.measureText(buf[:n])

		if err == nil && hasNewline && n == newline {
			padWidth := 0
			if len(r.padToWrap) > 0 {
				padWidth = r.padToWrap[len(r.padToWrap)-1]
			}
			if padWidth > 0 {
				// pad out to the target width if necessary
				remaining := padWidth - (r.lineWidth + writtenWidth)
				if remaining > 0 {
					remaining, err = w.Write(bytes.Repeat([]byte{' '}, remaining))
				}
				r.byteOffset += remaining
			}

			if err == nil {
				// write the newline
				if _, err = w.Write([]byte{'\n'}); err == nil {
					n++
				}
			}
		}

		// Account for the written bytes
		written += n

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
		if _, err := r.write(w, []byte{'\n'}); err != nil {
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
		if unicode.IsSpace(c) && r.noBreak == 0 {
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

// writeByte writes a byte to an io.Writer, ensuring that appropriate indentation and prefices are added at the beginning
// of each line.
func (r *Renderer) writeByte(w io.Writer, c byte) error {
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

// insertSpan creates a finalized span (with known Start/End) and inserts it
// into the span tree as a child of the current parent span without pushing it
// onto the span stack. This is used for spans whose content has already been
// written (e.g. pre-rendered table cell content in the wrapping path).
func (r *Renderer) insertSpan(node ast.Node, start, end int) {
	span := &NodeSpan{
		Start: start,
		End:   end,
		Node:  node,
	}

	if len(r.spanStack) != 0 {
		span.Parent = r.spanStack[len(r.spanStack)-1]
		span.Parent.Children = append(span.Parent.Children, span)
	} else if r.rootSpan == nil {
		r.rootSpan = span
	}

	if r.lastSpan != nil {
		span.Prev = r.lastSpan
		span.Prev.Next = span
	}
	r.lastSpan = span
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
		if err := r.writeByte(w, '\n'); err != nil {
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
	// Check both atNewline and the word buffer: when word wrapping is active,
	// short content (e.g. bold text with no spaces) can remain entirely in
	// the word buffer without ever being flushed to output. In that case
	// atNewline is stale (still true from a prior newline). WriteByte('\n')
	// will flush the buffer as part of its normal Write path.
	if !r.atNewline || r.wordBuffer.Len() > 0 {
		if err := r.writeByte(w, '\n'); err != nil {
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
			if err := r.writeByte(w, ' '); err != nil {
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
				if err := r.writeByte(w, '\n'); err != nil {
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

		if err := r.writeByte(w, '\n'); err != nil {
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
		r.PopPad()
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

	// Measure the widest line to determine the code block padding width.
	// Indented code blocks add 4 spaces of indent.
	lines := node.Lines()
	maxWidth := 0
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		v := line.Value(source)
		if len(v) > 0 && v[len(v)-1] == '\n' {
			v = v[:len(v)-1]
		}
		if w := ansi.StringWidth(string(v)); w > maxWidth {
			maxWidth = w
		}
	}
	r.PushPad(maxWidth + 4) // +4 for the indent

	// Each line of a code block needs to be aligned at the same offset, and a code block must start with at least four
	// spaces. To achieve this, we unconditionally add four spaces to the first line of the code block and indent the
	// rest as necessary.
	if _, err := r.WriteString(w, "    "); err != nil {
		return ast.WalkStop, err
	}

	r.PushIndent(4)
	defer r.PopPrefix()

	if err := r.writeCodeLines(w, "", source, lines); err != nil {
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

		r.PopPad()
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

	code := node.(*ast.FencedCodeBlock)
	fence := code.Fence
	language := code.Language(source)

	// Measure the widest line to determine the code block padding width.
	fenceLineWidth := ansi.StringWidth(string(fence)) + ansi.StringWidth(string(language))
	closingFenceWidth := ansi.StringWidth(string(fence))
	maxWidth := fenceLineWidth
	if closingFenceWidth > maxWidth {
		maxWidth = closingFenceWidth
	}
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		v := line.Value(source)
		// Trim trailing newline for width measurement.
		if len(v) > 0 && v[len(v)-1] == '\n' {
			v = v[:len(v)-1]
		}
		if w := ansi.StringWidth(string(v)); w > maxWidth {
			maxWidth = w
		}
	}
	r.PushPad(maxWidth)

	if err := r.PushStyle(w, chroma.LiteralStringHeredoc); err != nil {
		return ast.WalkStop, err
	}

	// Write the start of the fenced code block.
	if _, err := r.Write(w, fence); err != nil {
		return ast.WalkStop, err
	}
	if _, err := r.Write(w, language); err != nil {
		return ast.WalkStop, err
	}
	if err := r.writeByte(w, '\n'); err != nil {
		return ast.WalkStop, nil
	}

	// Write the contents of the fenced code block.
	if err := r.writeCodeLines(w, string(language), source, lines); err != nil {
		return ast.WalkStop, err
	}

	// Write the end of the fenced code block.
	if err := r.beginLine(w); err != nil {
		return ast.WalkStop, err
	}
	if _, err := r.Write(w, fence); err != nil {
		return ast.WalkStop, err
	}
	if err := r.writeByte(w, '\n'); err != nil {
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
				if err := r.writeByte(w, '\n'); err != nil {
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

	if r.theme == nil {
		if _, err := r.WriteString(w, "***\n"); err != nil {
			return ast.WalkStop, err
		}
	} else {
		width := r.wordWrap
		if width <= 0 {
			width = 80
		}
		if err := r.writeSGR(w, "2"); err != nil {
			return ast.WalkStop, err
		}
		if _, err := r.WriteString(w, strings.Repeat("─", width)); err != nil {
			return ast.WalkStop, err
		}
		if err := r.writeSGR(w, "22"); err != nil {
			return ast.WalkStop, err
		}
		if _, err := r.WriteString(w, "\n"); err != nil {
			return ast.WalkStop, err
		}
	}

	return ast.WalkContinue, nil
}

// RenderAutoLink renders an *ast.AutoLink node to the given BufWriter.
func (r *Renderer) RenderAutoLink(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		return ast.WalkContinue, nil
	}

	if err := r.writeByte(w, '<'); err != nil {
		return ast.WalkStop, err
	}
	if _, err := r.Write(w, node.(*ast.AutoLink).Label(source)); err != nil {
		return ast.WalkStop, err
	}
	if err := r.writeByte(w, '>'); err != nil {
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
		if err := r.PopStyle(w); err != nil {
			return ast.WalkStop, err
		}
		r.noBreak--
		r.CloseSpan()
		return ast.WalkContinue, nil
	}

	r.OpenSpan(node)
	r.noBreak++
	if err := r.PushStyle(w, styles.CodeSpan); err != nil {
		return ast.WalkStop, err
	}

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
		if err := r.writeByte(w, ' '); err != nil {
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
		if err := r.writeByte(w, ' '); err != nil {
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
	em := node.(*ast.Emphasis)

	if r.theme == nil {
		// No theme: write raw markers for round-tripping.
		if enter {
			r.OpenSpan(node)
		} else {
			r.CloseSpan()
		}
		if _, err := r.WriteString(w, strings.Repeat(string([]byte{em.Marker}), em.Level)); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	}

	if enter {
		r.OpenSpan(node)
		var token chroma.TokenType
		switch {
		case em.Level >= 3:
			token = styles.StrongEmph
		case em.Level >= 2:
			token = chroma.GenericStrong
		default:
			token = chroma.GenericEmph
		}
		if err := r.PushStyle(w, token); err != nil {
			return ast.WalkStop, err
		}
	} else {
		if err := r.PopStyle(w); err != nil {
			return ast.WalkStop, err
		}
		r.CloseSpan()
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
			if err := r.writeByte(w, ']'); err != nil {
				return err
			}
		case ast.LinkCollapsedReference:
			if _, err := r.WriteString(w, "][]"); err != nil {
				return err
			}
		case ast.LinkShortcutReference:
			if err := r.writeByte(w, ']'); err != nil {
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

			if err := r.writeByte(w, ')'); err != nil {
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

	if svgImage, ok := image.(*svg.SVGImage); ok && r.cols != 0 && r.rows != 0 {
		cellHeight := r.height / r.rows
		heightInCells := float64(image.Bounds().Dy()) / float64(cellHeight)
		if heightInCells < 1 {
			if image, err = svgImage.Scale(1.0 / heightInCells); err != nil {
				return err
			}
		}
	}

	if r.maxImageWidth != 0 {
		image = resize.Thumbnail(uint(r.maxImageWidth), uint(image.Bounds().Dy()), image, resize.Bicubic)
	}

	// Encoders write directly to the destination, so we need to do a little bit of extra accounting here.
	//
	// First, flush the current word and deal with line starts.
	if err := r.flushWordBuffer(w); err != nil {
		return err
	}
	if r.atNewline && image.Bounds().Dx() > 0 {
		if err := r.beginLine(w); err != nil {
			return err
		}
	}

	// Next, write the image itself and account for its bytes.
	encoder := r.imageEncoder
	if encoder == nil {
		encoder = KittyGraphicsEncoder()
		r.imageEncoder = encoder
	}
	written, err := encoder(w, image, r)
	if err != nil {
		return err
	}
	r.byteOffset += written

	// Finally, adjust the line width per the image's width and height in cells.
	if r.WordWrap() && r.cols != 0 && r.rows != 0 {
		cellWidth, cellHeight := r.width/r.cols, r.height/r.rows

		heightInCells := int(math.Ceil(float64(image.Bounds().Dy()) / float64(cellHeight)))
		if heightInCells > 1 {
			r.lineWidth = 0
		}

		widthInCells := int(math.Ceil(float64(image.Bounds().Dx()) / float64(cellWidth)))
		r.lineWidth += widthInCells

		if widthInCells > 0 {
			r.atNewline = false
		}
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

		if err := r.writeByte(w, byte(c)); err != nil {
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

// collectLinks walks an AST subtree and returns all Link and AutoLink nodes.
func collectLinks(node ast.Node) []ast.Node {
	var links []ast.Node
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n.Kind() {
			case ast.KindLink, ast.KindAutoLink:
				links = append(links, n)
			}
		}
		return ast.WalkContinue, nil
	})
	return links
}

// sanitizeWrappedLines post-processes lines produced by ansi.Wrap so that
// each line is self-contained with respect to ANSI SGR state. ansi.Wrap
// inserts line breaks without resetting or re-applying active styles, so a
// styled region that spans a break leaks into subsequent lines (or is lost
// if something else resets it). This function appends a reset (\033[0m) to
// any line that ends with active styles and prepends the carried-over style
// sequences to the following line.
func sanitizeWrappedLines(lines []string) []string {
	if len(lines) <= 1 {
		return lines
	}

	out := make([]string, len(lines))

	// Track which SGR sequences are "active" at the end of each line by
	// collecting the raw escape sequences and replaying them to determine
	// the terminal state.
	var carry string // SGR sequences to prepend to the next line
	for i, line := range lines {
		// Prepend carried-over style to this line.
		out[i] = carry + line

		// Scan the line (with carry applied) to find the active SGR state
		// at the end. We collect all SGR sequences and track a simplified
		// attribute set: bold, italic, underline, fg color, bg color.
		var (
			bold, italic, underline bool
			fg, bg                  string // raw SGR parameter strings
		)

		src := out[i]
		j := 0
		for j < len(src) {
			if src[j] == '\033' && j+1 < len(src) && src[j+1] == '[' {
				end := strings.IndexByte(src[j:], 'm')
				if end < 0 {
					break
				}
				params := src[j+2 : j+end]
				j += end + 1

				// Parse semicolon-separated SGR parameters.
				parts := strings.Split(params, ";")
				for k := 0; k < len(parts); k++ {
					switch parts[k] {
					case "0":
						bold, italic, underline = false, false, false
						fg, bg = "", ""
					case "1":
						bold = true
					case "22":
						bold = false
					case "3":
						italic = true
					case "23":
						italic = false
					case "4":
						underline = true
					case "24":
						underline = false
					case "38":
						// Foreground color: consume the rest as the full sequence.
						fg = "38;" + strings.Join(parts[k+1:], ";")
						k = len(parts) // consumed all remaining
					case "39":
						fg = ""
					case "48":
						// Background color: consume the rest as the full sequence.
						bg = "48;" + strings.Join(parts[k+1:], ";")
						k = len(parts) // consumed all remaining
					case "49":
						bg = ""
					}
				}
			} else {
				j++
			}
		}

		// Build the carry string for the next line.
		hasState := bold || italic || underline || fg != "" || bg != ""
		if hasState {
			// Append reset to the current line.
			out[i] += "\033[0m"

			// Build the re-application sequence.
			var sb strings.Builder
			if bg != "" {
				sb.WriteString("\033[")
				sb.WriteString(bg)
				sb.WriteString("m")
			}
			if fg != "" {
				sb.WriteString("\033[")
				sb.WriteString(fg)
				sb.WriteString("m")
			}
			if bold {
				sb.WriteString("\033[1m")
			}
			if underline {
				sb.WriteString("\033[4m")
			}
			if italic {
				sb.WriteString("\033[3m")
			}
			carry = sb.String()
		} else {
			carry = ""
		}
	}

	return out
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
	return r.writeByte(w, '\n')
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
				styles:        r.styles,
			}
			cellRenderer := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(cr, 100)))
			if err := cellRenderer.Render(io.Discard, source, cell); err != nil {
				return ast.WalkStop, err
			}

			for col >= len(columnWidths) {
				columnWidths = append(columnWidths, 0)
			}
			if columnWidths[col] < cr.lineWidth {
				columnWidths[col] = cr.lineWidth
			}
			cellWidths = append(cellWidths, cr.lineWidth)
		}
	}

	// Check if the table exceeds the available width and needs constraining.
	borderWidth := len(columnWidths) + 1 // one │ per column + trailing │
	naturalTotal := 0
	for _, w := range columnWidths {
		naturalTotal += w
	}
	totalTableWidth := borderWidth + naturalTotal

	if r.wordWrap > 0 && totalTableWidth >= r.wordWrap {
		// Proportionally shrink columns to fit within the wrap width.
		available := r.wordWrap - borderWidth
		if available < len(columnWidths) {
			available = len(columnWidths) // minimum 1 char per column
		}

		constrainedWidths := make([]int, len(columnWidths))
		frozen := make([]bool, len(columnWidths))
		remaining := available
		numUnfrozen := len(columnWidths)

		// Iteratively freeze columns whose natural width fits within a fair share.
		for numUnfrozen > 0 {
			fairShare := remaining / numUnfrozen
			newlyFrozen := 0
			for i, w := range columnWidths {
				if !frozen[i] && w <= fairShare {
					frozen[i] = true
					constrainedWidths[i] = w
					remaining -= w
					numUnfrozen--
					newlyFrozen++
				}
			}
			if newlyFrozen == 0 {
				break
			}
		}

		// Distribute remaining space among unfrozen columns proportionally.
		unfrozenTotal := 0
		for i, w := range columnWidths {
			if !frozen[i] {
				unfrozenTotal += w
			}
		}
		distributed := 0
		for i, w := range columnWidths {
			if !frozen[i] {
				constrainedWidths[i] = w * remaining / unfrozenTotal
				if constrainedWidths[i] < 1 {
					constrainedWidths[i] = 1
				}
				distributed += constrainedWidths[i]
			}
		}

		// Distribute rounding remainder to the naturally widest unfrozen columns.
		leftover := remaining - distributed
		for leftover > 0 {
			widest := -1
			for i := range columnWidths {
				if !frozen[i] && (widest == -1 || columnWidths[i] > columnWidths[widest]) {
					widest = i
				}
			}
			if widest == -1 {
				break
			}
			constrainedWidths[widest]++
			leftover--
			// Mark as used so next iteration picks another column if needed.
			columnWidths[widest] = 0
		}

		// Pre-render all cells with word wrapping applied at the constrained widths.
		numCols := len(constrainedWidths)
		type cellData struct {
			lines []string   // rendered lines for this cell
			links []ast.Node // Link/AutoLink nodes in this cell
		}
		var rows [][]cellData // rows[rowIdx][colIdx]

		for rowIdx, row := 0, table.FirstChild(); row != nil; rowIdx, row = rowIdx+1, row.NextSibling() {
			// Determine the row style so we can seed the cell renderer's
			// style stack. This ensures that nested PopStyle calls (e.g.
			// after an inline code span) restore to the row's background
			// color rather than resetting it.
			var rowStyleToken chroma.TokenType
			switch {
			case rowIdx == 0:
				rowStyleToken = styles.TableHeader
			case rowIdx%2 == 0:
				rowStyleToken = styles.TableRowAlt
			default:
				rowStyleToken = styles.TableRow
			}

			var rowCells []cellData
			for col, cell := 0, row.FirstChild(); cell != nil; col, cell = col+1, cell.NextSibling() {
				colWidth := constrainedWidths[col]

				// Render cell content with no word wrap (same as measurement), then
				// use ansi.Wrap to word-wrap + hard-wrap to the column width.
				cr := &Renderer{
					theme:         r.theme,
					wordWrap:      0,
					hyperlinks:    r.hyperlinks,
					images:        r.images,
					maxImageWidth: r.maxImageWidth,
					contentRoot:   r.contentRoot,
					softBreak:     r.softBreak,
					tableStack:    []tableState{{measuring: true}},
					styles:        r.styles,
				}
				// Silently push the row style so that style pops during
				// pre-rendering restore to the row background, not the
				// terminal default.
				if resolved, ok := cr.resolveStyle(rowStyleToken); ok {
					cr.styles = append(cr.styles[:len(cr.styles):len(cr.styles)], resolved)
				}
				var buf bytes.Buffer
				cellRenderer := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(cr, 100)))
				if err := cellRenderer.Render(&buf, source, cell); err != nil {
					return ast.WalkStop, err
				}

				content := strings.TrimRight(buf.String(), "\n")
				// Word-wrap and hard-wrap the cell content to the column width.
				content = ansi.Wrap(content, colWidth, "")
				var lines []string
				if content == "" {
					lines = []string{""}
				} else {
					lines = sanitizeWrappedLines(strings.Split(content, "\n"))
				}
				rowCells = append(rowCells, cellData{lines: lines, links: collectLinks(cell)})
			}
			// Pad with empty cells if row has fewer columns.
			for len(rowCells) < numCols {
				rowCells = append(rowCells, cellData{lines: []string{""}})
			}
			rows = append(rows, rowCells)
		}

		// Push a table state so renderTableBorder works.
		r.tableStack = append(r.tableStack, tableState{
			columnWidths: constrainedWidths,
			alignments:   table.Alignments,
		})

		// Disable word wrapping while assembling the table — the cell content
		// has already been wrapped to fit within the constrained column widths.
		r.PushWordWrap(false)

		// Assemble the table output.
		// Top border.
		if err := r.renderTableBorder(w, borders.topLeft(), borders.topJoin(), borders.topRight()); err != nil {
			return ast.WalkStop, err
		}

		for rowIdx, rowCells := range rows {
			// Determine the max number of sub-lines in this row.
			maxLines := 0
			for _, cell := range rowCells {
				if len(cell.lines) > maxLines {
					maxLines = len(cell.lines)
				}
			}

			// Determine the style for this row.
			var style chroma.TokenType
			switch {
			case rowIdx == 0:
				style = styles.TableHeader
			case rowIdx%2 == 0:
				style = styles.TableRowAlt
			default:
				style = styles.TableRow
			}

			// Write each sub-line of the row.
			for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
				if _, err := r.WriteRune(w, borders.vertical()); err != nil {
					return ast.WalkStop, err
				}
				for colIdx, cell := range rowCells {
					if err := r.PushStyle(w, style); err != nil {
						return ast.WalkStop, err
					}

					var cellLine string
					if lineIdx < len(cell.lines) {
						cellLine = cell.lines[lineIdx]
					}
					contentStart := r.byteOffset
					lineWidth := ansi.StringWidth(cellLine)
					if _, err := r.WriteString(w, cellLine); err != nil {
						return ast.WalkStop, err
					}
					// Insert spans for links in this cell so they
					// appear in the span tree for navigation. The
					// wrapping path skips the normal AST walk, so
					// link spans must be created manually here.
					if lineIdx == 0 {
						contentEnd := r.byteOffset
						for _, link := range cell.links {
							r.insertSpan(link, contentStart, contentEnd)
						}
					}
					// The pre-rendered cell content may contain raw ANSI
					// sequences that leave the terminal in an unknown SGR
					// state. Reset and re-apply the row style so that
					// padding and PopStyle work correctly.
					if err := r.writeSGR(w, "0"); err != nil {
						return ast.WalkStop, err
					}
					if err := r.reapplyStyle(w); err != nil {
						return ast.WalkStop, err
					}
					// Pad to column width.
					pad := constrainedWidths[colIdx] - lineWidth
					if pad > 0 {
						if _, err := r.WriteString(w, strings.Repeat(" ", pad)); err != nil {
							return ast.WalkStop, err
						}
					}

					if err := r.PopStyle(w); err != nil {
						return ast.WalkStop, err
					}
					if _, err := r.WriteRune(w, borders.vertical()); err != nil {
						return ast.WalkStop, err
					}
				}
				if err := r.writeByte(w, '\n'); err != nil {
					return ast.WalkStop, err
				}
			}

			// After header row, emit middle border.
			if rowIdx == 0 {
				if err := r.renderTableBorder(w, borders.middleLeft(), borders.middleJoin(), borders.middleRight()); err != nil {
					return ast.WalkStop, err
				}
			}
		}

		r.PopWordWrap()

		// Bottom border and cleanup are handled by the !enter path of RenderTable,
		// which runs even with WalkSkipChildren. The tableStack entry remains pushed
		// so renderTableBorder can access constrainedWidths.
		return ast.WalkSkipChildren, nil
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
		if err := r.writeByte(w, '\n'); err != nil {
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
