package odt

import (
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/pgavlin/goldmark/ast"
	mdtext "github.com/pgavlin/goldmark/text"
)

type listState struct {
	node  *ast.List
	fresh bool
}

type Renderer struct {
	proportionalFamily string
	monospaceFamily    string

	listStack []listState
}

func NewRenderer(proportionalFamily, monospaceFamily string) *Renderer {
	return &Renderer{
		proportionalFamily: proportionalFamily,
		monospaceFamily:    monospaceFamily,
	}
}

func (r *Renderer) Render(w io.Writer, source []byte, n ast.Node) error {
	return ast.Walk(n, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		switch n := n.(type) {
		case *ast.Document:
			return r.renderDocument(w, source, n, enter)

		// blocks
		case *ast.Heading:
			return r.renderHeading(w, source, n, enter)
		case *ast.Blockquote:
			return r.renderBlockquote(w, source, n, enter)
		case *ast.CodeBlock:
			return r.renderCodeBlock(w, source, n, enter)
		case *ast.FencedCodeBlock:
			return r.renderFencedCodeBlock(w, source, n, enter)
		case *ast.List:
			return r.renderList(w, source, n, enter)
		case *ast.ListItem:
			return r.renderListItem(w, source, n, enter)
		case *ast.Paragraph:
			return r.renderParagraph(w, source, n, enter)
		case *ast.TextBlock:
			return r.renderTextBlock(w, source, n, enter)
		case *ast.ThematicBreak:
			return r.renderThematicBreak(w, source, n, enter)

		// inlines
		case *ast.AutoLink:
			return r.renderAutoLink(w, source, n, enter)
		case *ast.CodeSpan:
			return r.renderCodeSpan(w, source, n, enter)
		case *ast.Emphasis:
			return r.renderEmphasis(w, source, n, enter)
		case *ast.Image:
			return r.renderImage(w, source, n, enter)
		case *ast.Link:
			return r.renderLink(w, source, n, enter)
		case *ast.Text:
			return r.renderText(w, source, n, enter)
		case *ast.String:
			return r.renderString(w, source, n, enter)
		}

		return ast.WalkContinue, nil
	})
}

const prolog = `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content  xmlns:css3t="http://www.w3.org/TR/css3-text/" xmlns:grddl="http://www.w3.org/2003/g/data-view#" xmlns:xhtml="http://www.w3.org/1999/xhtml" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xforms="http://www.w3.org/2002/xforms" xmlns:dom="http://www.w3.org/2001/xml-events" xmlns:script="urn:oasis:names:tc:opendocument:xmlns:script:1.0" xmlns:form="urn:oasis:names:tc:opendocument:xmlns:form:1.0" xmlns:math="http://www.w3.org/1998/Math/MathML" xmlns:number="urn:oasis:names:tc:opendocument:xmlns:datastyle:1.0" xmlns:field="urn:openoffice:names:experimental:ooo-ms-interop:xmlns:field:1.0" xmlns:meta="urn:oasis:names:tc:opendocument:xmlns:meta:1.0" xmlns:loext="urn:org:documentfoundation:names:experimental:office:xmlns:loext:1.0" xmlns:officeooo="http://openoffice.org/2009/office" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:chart="urn:oasis:names:tc:opendocument:xmlns:chart:1.0" xmlns:tableooo="http://openoffice.org/2009/table" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:rpt="http://openoffice.org/2005/report" xmlns:dr3d="urn:oasis:names:tc:opendocument:xmlns:dr3d:1.0" xmlns:of="urn:oasis:names:tc:opendocument:xmlns:of:1.2" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:calcext="urn:org:documentfoundation:names:experimental:calc:xmlns:calcext:1.0" xmlns:oooc="http://openoffice.org/2004/calc" xmlns:drawooo="http://openoffice.org/2010/draw" xmlns:xlink="http://www.w3.org/1999/xlink" xmlns:ooo="http://openoffice.org/2004/office" xmlns:ooow="http://openoffice.org/2004/writer" xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" xmlns:formx="urn:openoffice:names:experimental:ooxml-odf-interop:xmlns:form:1.0" xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" office:version="1.3">
	<office:font-face-decls>
        <style:font-face style:name="Proportional Serif" svg:font-family="&apos;Liberation Serif&apos;, &apos;Times New Roman&apos;, serif" style:font-family-generic="roman" style:font-pitch="variable"/>
        <style:font-face style:name="Proportional Sans" svg:font-family="&apos;Liberation Sans&apos;, Helvetica, Arial, sans-serif" style:font-family-generic="swiss" style:font-pitch="variable"/>
		<style:font-face style:name="Monospace" svg:font-family="&apos;Liberation Mono&apos;, Consolas, monospace" style:font-family-generic="roman" style:font-pitch="fixed"/>
	</office:font-face-decls>

	<office:automatic-styles>
		<!-- Blockquote, code block, and paragraph styles -->

		<!-- Blockquote -->
		<style:style style:family="paragraph" style:name="Blockquote" style:parent-style-name="Paragraph">
		</style:style>

		<!-- Code block -->
		<style:style style:family="paragraph" style:name="Code Block" style:parent-style-name="Paragraph">
			<style:paragraph-properties fo:background-color="#f6f8fA"/>
			<style:text-properties style:font-name="Monospace" fo:color="#000000" fo:font-size="9pt"/>
		</style:style>

		<!-- Paragraph -->
		<style:style style:family="paragraph" style:name="Paragraph">
			<style:paragraph-properties fo:margin-top="4.5pt" fo:margin-bottom="4.5pt"/>
			<style:text-properties style:font-name="Proportional Serif"/>
		</style:style>

		<!-- Thematic break style -->
		<style:style style:family="paragraph" style:name="Thematic Break">
            <style:paragraph-properties fo:margin-top="0in" fo:margin-bottom="0.1965in" style:contextual-spacing="false" style:border-line-width-bottom="0.0008in 0.0016in 0.0008in" fo:padding="0in" fo:border-left="none" fo:border-right="none" fo:border-top="none" fo:border-bottom="0.14pt double #808080" text:number-lines="false" text:line-number="0" style:join-border="false"/>
            <style:text-properties fo:font-size="6pt"/>
		</style:style>

		<!-- List styles -->

		<!-- Unordered lists -->
		<text:list-style style:name="Unordered List">
			<text:list-level-style-bullet>
				<style:list-level-properties text:list-level-position-and-space-mode="label-alignment">
					<style:list-level-label-alignment text:label-followed-by="listtab" text:list-tab-stop-position="0.5in" fo:text-indent="-0.25in" fo:margin-left="0.5in"/>
				</style:list-level-properties>
			</text:list-level-style-bullet>
		</text:list-style>

		<!-- Ordered lists -->
		<text:list-style style:name="Ordered List">
			<text:list-level-style-number text:level="1" style:num-format="1" style:num-suffix=".">
				<style:list-level-properties text:list-level-position-and-space-mode="label-alignment">
					<style:list-level-label-alignment text:label-followed-by="listtab" text:list-tab-stop-position="0.5in" fo:text-indent="-0.25in" fo:margin-left="0.5in"/>
				</style:list-level-properties>
			</text:list-level-style-number>
		</text:list-style>

		<!-- Inline styles -->

		<!-- Emphasis -->
		<style:style style:family="text" style:name="Emphasis">
			<style:text-properties style:font-weight="bold"/>
		</style:style>

		<!-- Strong emphasis -->
		<style:style style:family="text" style:name="Strong Emphasis">
			<style:text-properties style:font-style="italic"/>
		</style:style>

		<!-- Code span -->
		<style:style style:family="text" style:name="Code Span">
			<style:text-properties style:font-name="Monospace" fo:background-color="#f6f8fa" fo:color="#000000"/>
		</style:style>
	</office:automatic-styles>

	<office:body>
		<office:text>`

// renderDocument renders an *ast.Document node to the given io.Writer.
func (r *Renderer) renderDocument(w io.Writer, source []byte, node *ast.Document, enter bool) (ast.WalkStatus, error) {
	if enter {
		r.listStack = nil
		fmt.Fprintln(w, prolog)
	} else {
		fmt.Fprintln(w, `		</office:text>`)
		fmt.Fprintln(w, `	</office:body>`)
		fmt.Fprintln(w, `</office:document-content>`)
	}
	return ast.WalkContinue, nil
}

// renderHeading renders an *ast.Heading node to the given io.Writer.
func (r *Renderer) renderHeading(w io.Writer, source []byte, node *ast.Heading, enter bool) (ast.WalkStatus, error) {
	if enter {
		fmt.Fprintf(w, "\t\t\t<text:h text:outline-level=\"%d\">", node.Level)
	} else {
		fmt.Fprintln(w, "</text:h>")
	}
	return ast.WalkContinue, nil
}

// renderBlockquote renders an *ast.Blockquote node to the given io.Writer.
func (r *Renderer) renderBlockquote(w io.Writer, source []byte, node *ast.Blockquote, enter bool) (ast.WalkStatus, error) {
	if enter {
		fmt.Fprint(w, "\t\t\t<text:p text:style-name=\"Blockquote\">")
	} else {
		fmt.Fprintln(w, "</text:p>")
	}
	return ast.WalkContinue, nil
}

var (
	escQuot = []byte("&#34;") // shorter than "&quot;"
	escApos = []byte("&#39;") // shorter than "&apos;"
	escAmp  = []byte("&amp;")
	escLT   = []byte("&lt;")
	escGT   = []byte("&gt;")
	escTab  = []byte("&#x9;")
	escNL   = []byte("&#xA;")
	escCR   = []byte("&#xD;")
	escFFFD = []byte("\uFFFD") // Unicode replacement character

	textSpace = []byte("<text:s/>")
	textTab   = []byte("<text:s/><text:s/><text:s/><text:s/>")
	textNL    = []byte("<text:line-break/>")
)

// escapeText writes to w the properly escaped XML equivalent of the plain text data s. If preserveWhitespace is true,
// " " will be replaced with "<text:s/>", "\t" with "<text:tab/>", and "\n" with "<text:line-break/>".
func escapeText(w io.Writer, s []byte, preserveWhitespace bool) error {
	var esc []byte
	last := 0
	for i := 0; i < len(s); {
		r, width := utf8.DecodeRune(s[i:])
		i += width
		switch r {
		case '"':
			esc = escQuot
		case '\'':
			esc = escApos
		case '&':
			esc = escAmp
		case '<':
			esc = escLT
		case '>':
			esc = escGT
		case '\t':
			if preserveWhitespace {
				esc = textTab
			} else {
				esc = escTab
			}
		case '\n':
			if preserveWhitespace {
				esc = textNL
			} else {
				esc = escNL
			}
		case '\r':
			esc = escCR
		case ' ':
			if preserveWhitespace {
				esc = textSpace
				break
			}
			continue
		default:
			if !isInCharacterRange(r) || (r == 0xFFFD && width == 1) {
				esc = escFFFD
				break
			}
			continue
		}
		if _, err := w.Write(s[last : i-width]); err != nil {
			return err
		}
		if _, err := w.Write(esc); err != nil {
			return err
		}
		last = i
	}
	_, err := w.Write(s[last:])
	return err
}

// Decide whether the given rune is in the XML Character Range, per the Char production of
// https://www.xml.com/axml/testaxml.htm, Section 2.2 Characters.
func isInCharacterRange(r rune) (inrange bool) {
	return r == 0x09 ||
		r == 0x0A ||
		r == 0x0D ||
		r >= 0x20 && r <= 0xD7FF ||
		r >= 0xE000 && r <= 0xFFFD ||
		r >= 0x10000 && r <= 0x10FFFF
}

func (r *Renderer) renderCode(w io.Writer, source []byte, lines *mdtext.Segments) error {
	fmt.Fprint(w, "\t\t\t<text:p text:style-name=\"Code Block\">")
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		value := line.Value(source)
		if i == lines.Len()-1 {
			value = bytes.TrimRight(value, "\n")
		}
		if err := escapeText(w, value, true); err != nil {
			return err
		}
	}
	fmt.Fprintln(w, "</text:p>")
	return nil
}

// renderCodeBlock renders an *ast.CodeBlock node to the given io.Writer.
func (r *Renderer) renderCodeBlock(w io.Writer, source []byte, node *ast.CodeBlock, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := r.renderCode(w, source, node.Lines()); err != nil {
			return ast.WalkStop, err
		}
	}
	return ast.WalkSkipChildren, nil
}

// renderFencedCodeBlock renders an *ast.FencedCodeBlock node to the given io.Writer.
func (r *Renderer) renderFencedCodeBlock(w io.Writer, source []byte, node *ast.FencedCodeBlock, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := r.renderCode(w, source, node.Lines()); err != nil {
			return ast.WalkStop, err
		}
	}
	return ast.WalkSkipChildren, nil
}

// renderList renders an *ast.List node to the given io.Writer.
func (r *Renderer) renderList(w io.Writer, source []byte, node *ast.List, enter bool) (ast.WalkStatus, error) {
	if enter {
		r.listStack = append(r.listStack, listState{node: node, fresh: true})

		style := "Unordered"
		if node.IsOrdered() {
			style = "Ordered"
		}

		fmt.Fprintf(w, "\t\t\t<text:list text:style-name=\"%s List\">\n", style)
	} else {
		fmt.Fprintln(w, "\t\t\t</text:list>")
		r.listStack = r.listStack[:len(r.listStack)-1]
	}
	return ast.WalkContinue, nil
}

// renderListItem renders an *ast.ListItem node to the given io.Writer.
func (r *Renderer) renderListItem(w io.Writer, source []byte, node *ast.ListItem, enter bool) (ast.WalkStatus, error) {
	if enter {
		state := &r.listStack[len(r.listStack)-1]

		attrs := ""
		if state.fresh && state.node.IsOrdered() {
			attrs = fmt.Sprintf(" text:start-value=\"%d\"", state.node.Start)
			state.fresh = false
		}

		fmt.Fprintf(w, "\t\t\t<text:list-item%s>\n", attrs)
	} else {
		fmt.Fprintln(w, "\t\t\t</text:list-item>")
	}
	return ast.WalkContinue, nil
}

// renderParagraph renders an *ast.Paragraph node to the given io.Writer.
func (r *Renderer) renderParagraph(w io.Writer, source []byte, node *ast.Paragraph, enter bool) (ast.WalkStatus, error) {
	if enter {
		fmt.Fprint(w, "\t\t\t<text:p text:style-name=\"Paragraph\">")
	} else {
		fmt.Fprintln(w, "</text:p>")
	}
	return ast.WalkContinue, nil
}

// renderTextBlock renders an *ast.TextBlock node to the given io.Writer.
func (r *Renderer) renderTextBlock(w io.Writer, source []byte, node *ast.TextBlock, enter bool) (ast.WalkStatus, error) {
	if enter {
		fmt.Fprint(w, "\t\t\t<text:p text:style-name=\"Paragraph\">")
	} else {
		fmt.Fprintln(w, "</text:p>")
	}
	return ast.WalkContinue, nil
}

// renderThematicBreak renders an *ast.ThematicBreak node to the given io.Writer.
func (r *Renderer) renderThematicBreak(w io.Writer, source []byte, node *ast.ThematicBreak, enter bool) (ast.WalkStatus, error) {
	if enter {
		fmt.Fprintln(w, "\t\t\t<text:p text:style-name=\"Thematic Break\"/>")
	}
	return ast.WalkContinue, nil
}

// renderAutoLink renders an *ast.AutoLink node to the given io.Writer.
func (r *Renderer) renderAutoLink(w io.Writer, source []byte, node *ast.AutoLink, enter bool) (ast.WalkStatus, error) {
	if enter {
		fmt.Fprintf(w, "<text:a xlink:href=\"%s\">", string(node.URL(source)))
	} else {
		fmt.Fprint(w, "</text:a>")
	}
	return ast.WalkContinue, nil
}

// renderCodeSpan renders an *ast.CodeSpan node to the given io.Writer.
func (r *Renderer) renderCodeSpan(w io.Writer, source []byte, node *ast.CodeSpan, enter bool) (ast.WalkStatus, error) {
	if enter {
		fmt.Fprint(w, "<text:span text:style-name=\"Code Span\">")
	} else {
		fmt.Fprint(w, "</text:span>")
	}
	return ast.WalkContinue, nil
}

// renderEmphasis renders an *ast.Emphasis node to the given io.Writer.
func (r *Renderer) renderEmphasis(w io.Writer, source []byte, node *ast.Emphasis, enter bool) (ast.WalkStatus, error) {
	if enter {
		style := "Emphasis"
		if node.Level > 1 {
			style = "Strong Emphasis"
		}
		fmt.Fprintf(w, "<text:span text:style-name=\"%s\">", style)
	} else {
		fmt.Fprint(w, "</text:span>")
	}
	return ast.WalkContinue, nil
}

// renderImage renders an *ast.Image node to the given io.Writer.
func (r *Renderer) renderImage(w io.Writer, source []byte, node *ast.Image, enter bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

// renderLink renders an *ast.Link node to the given io.Writer.
func (r *Renderer) renderLink(w io.Writer, source []byte, node *ast.Link, enter bool) (ast.WalkStatus, error) {
	if enter {
		fmt.Fprintf(w, "<text:a xlink:href=\"%s\">", string(node.Destination))
	} else {
		fmt.Fprint(w, "</text:a>")
	}
	return ast.WalkContinue, nil
}

// renderText renders an *ast.Text node to the given io.Writer.
func (r *Renderer) renderText(w io.Writer, source []byte, node *ast.Text, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := escapeText(w, node.Segment.Value(source), false); err != nil {
			return ast.WalkStop, err
		}
		if node.HardLineBreak() {
			fmt.Fprint(w, "<text:line-break/>")
		} else if node.NextSibling() != nil {
			fmt.Fprint(w, " ")
		}
	}
	return ast.WalkContinue, nil
}

// renderString renders an *ast.String node to the given io.Writer.
func (r *Renderer) renderString(w io.Writer, source []byte, node *ast.String, enter bool) (ast.WalkStatus, error) {
	if enter {
		if err := escapeText(w, node.Value, false); err != nil {
			return ast.WalkStop, err
		}
	}
	return ast.WalkContinue, nil
}
