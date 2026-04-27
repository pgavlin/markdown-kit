package odt

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/parser"
	mdtext "github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/frontmatter"
)

func writeMimetype(zw *zip.Writer) error {
	f, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(f, "application/vnd.oasis.opendocument.text")
	return err
}

func writeManifest(zw *zip.Writer, hasMeta bool) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.3" xmlns:loext="urn:org:documentfoundation:names:experimental:office:xmlns:loext:1.0">
	<manifest:file-entry manifest:full-path="/" manifest:version="1.3" manifest:media-type="application/vnd.oasis.opendocument.text"/>
	<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
`)
	if hasMeta {
		b.WriteString(`	<manifest:file-entry manifest:full-path="meta.xml" manifest:media-type="text/xml"/>` + "\n")
	}
	b.WriteString("</manifest:manifest>\n")

	f, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "META-INF/manifest.xml",
		Method: zip.Deflate,
	})
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(b.String()))
	return err
}

// writeMeta emits a meta.xml part populated from frontmatter. Returns
// true if the part was written (frontmatter contained at least one
// mappable field), false if nothing was written. The fields supported
// today are dc:title, dc:creator (author), dc:date (creation-date),
// dc:description, dc:subject, and meta:keyword (one per tag).
func writeMeta(zw *zip.Writer, fm map[string]any) (bool, error) {
	if len(fm) == 0 {
		return false, nil
	}

	type metaField struct {
		tag, value string
	}
	var fields []metaField
	addString := func(tag, key string) {
		if v, ok := fm[key].(string); ok && v != "" {
			fields = append(fields, metaField{tag, v})
		}
	}
	addString("dc:title", "title")
	if creator, ok := fm["author"].(string); ok && creator != "" {
		fields = append(fields, metaField{"dc:creator", creator})
		fields = append(fields, metaField{"meta:initial-creator", creator})
	} else {
		addString("dc:creator", "creator")
	}
	// `date` may decode as either a YAML-typed timestamp (time.Time) or
	// a free-form string. ODF expects ISO 8601 — render time.Time via
	// RFC3339 and pass strings through unchanged.
	switch d := fm["date"].(type) {
	case time.Time:
		fields = append(fields, metaField{"meta:creation-date", d.Format(time.RFC3339)})
	case string:
		if d != "" {
			fields = append(fields, metaField{"meta:creation-date", d})
		}
	}
	addString("dc:description", "description")
	addString("dc:subject", "subject")

	var keywords []string
	switch ks := fm["tags"].(type) {
	case []any:
		for _, k := range ks {
			if s, ok := k.(string); ok && s != "" {
				keywords = append(keywords, s)
			}
		}
	}

	if len(fields) == 0 && len(keywords) == 0 {
		return false, nil
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<office:document-meta office:version="1.3" `)
	b.WriteString(`xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" `)
	b.WriteString(`xmlns:meta="urn:oasis:names:tc:opendocument:xmlns:meta:1.0" `)
	b.WriteString(`xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
	b.WriteString("\t<office:meta>\n")
	for _, f := range fields {
		fmt.Fprintf(&b, "\t\t<%s>%s</%s>\n", f.tag, xmlEscape(f.value), f.tag)
	}
	for _, kw := range keywords {
		fmt.Fprintf(&b, "\t\t<meta:keyword>%s</meta:keyword>\n", xmlEscape(kw))
	}
	b.WriteString("\t</office:meta>\n")
	b.WriteString("</office:document-meta>\n")

	f, err := zw.Create("meta.xml")
	if err != nil {
		return false, err
	}
	if _, err := f.Write([]byte(b.String())); err != nil {
		return false, err
	}
	return true, nil
}

// xmlEscape minimally escapes s for placement inside an XML element body.
func xmlEscape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}

type options struct {
	proportionalFamily string
	monospaceFamily    string
}

type RenderOption func(opts *options)

func WithProportionalFamily(fontFamily string) RenderOption {
	return func(opts *options) {
		opts.proportionalFamily = fontFamily
	}
}

func WithMonospaceFamily(fontFamily string) RenderOption {
	return func(opts *options) {
		opts.monospaceFamily = fontFamily
	}
}

func FromMarkdown(w io.Writer, markdown []byte, renderOptions ...RenderOption) error {
	var opts options
	for _, o := range renderOptions {
		o(&opts)
	}

	zw := zip.NewWriter(w)
	defer zw.Close()

	if err := writeMimetype(zw); err != nil {
		return fmt.Errorf("writing mimetype: %w", err)
	}

	// Parse the document with the frontmatter extension so the YAML
	// block becomes a KindFrontmatter node (which the renderer skips
	// by virtue of having no node renderer registered for it) instead
	// of being treated as paragraph content.
	p := goldmark.DefaultParser()
	p.AddOptions(parser.WithBlockParsers(
		util.Prioritized(frontmatter.NewParser(), 0),
	))
	doc := p.Parse(mdtext.NewReader(markdown))

	// If frontmatter is present, emit meta.xml so authoring metadata
	// (title, author, date, ...) round-trips into the ODT.
	hasMeta := false
	if fm := frontmatter.Find(doc); fm != nil {
		var meta map[string]any
		if err := frontmatter.Decode(fm, markdown, &meta); err == nil {
			if ok, err := writeMeta(zw, meta); err != nil {
				return fmt.Errorf("writing meta.xml: %w", err)
			} else {
				hasMeta = ok
			}
		}
	}

	if err := writeManifest(zw, hasMeta); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	content, err := zw.Create("content.xml")
	if err != nil {
		return fmt.Errorf("creating content.xml: %w", err)
	}

	renderer := NewRenderer(opts.proportionalFamily, opts.monospaceFamily)
	if err = renderer.Render(content, markdown, doc); err != nil {
		return fmt.Errorf("rendering content: %w", err)
	}

	return nil
}
