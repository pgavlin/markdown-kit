package odt

import (
	"archive/zip"
	"fmt"
	"io"

	"github.com/pgavlin/goldmark"
	mdtext "github.com/pgavlin/goldmark/text"
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

func writeManifest(zw *zip.Writer) error {
	const manifest = `<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.3" xmlns:loext="urn:org:documentfoundation:names:experimental:office:xmlns:loext:1.0">
	<manifest:file-entry manifest:full-path="/" manifest:version="1.3" manifest:media-type="application/vnd.oasis.opendocument.text"/>
	<!--<manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>-->
	<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
</manifest:manifest>
`

	f, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "META-INF/manifest.xml",
		Method: zip.Deflate,
	})
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(manifest))
	return err
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
	if err := writeManifest(zw); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	content, err := zw.Create("content.xml")
	if err != nil {
		return fmt.Errorf("creating content.xml: %w", err)
	}

	parser := goldmark.DefaultParser()
	renderer := NewRenderer(opts.proportionalFamily, opts.monospaceFamily)
	if err = renderer.Render(content, markdown, parser.Parse(mdtext.NewReader(markdown))); err != nil {
		return fmt.Errorf("rendering content: %w", err)
	}

	return nil
}
