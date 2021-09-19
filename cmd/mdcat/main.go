package main

import (
	"flag"
	"fmt"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/alecthomas/chroma"
	"github.com/eliukblau/pixterm/pkg/ansimage"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/extension"
	goldmark_parser "github.com/pgavlin/goldmark/parser"
	goldmark_renderer "github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	_ "github.com/pgavlin/svg2"
	"golang.org/x/term"
)

func main() {
	supportsImages := canDisplayImages()
	cols, rows, termWidth, termHeight, hasGeometry := terminalGeometry()

	width := flag.Uint("w", 0, "the maximum line width for wrappable content")
	images := flag.Bool("i", true, "display images")
	hyperlinks := flag.Bool("h", false, "display hyperlinks instead of link text")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: %v [path to Markdown file]\n", filepath.Base(os.Args[0]))
		os.Exit(-1)
	}
	path := flag.Arg(0)

	source, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %v: %v\n", path, err)
		os.Exit(-1)
	}

	var theme *chroma.Style
	if term.IsTerminal(int(os.Stdout.Fd())) {
		theme = styles.Pulumi

		if *width == 0 {
			w, _, err := term.GetSize(int(os.Stdout.Fd()))
			if err == nil {
				*width = uint(w)
			}
		}
	}

	parser := goldmark.DefaultParser()
	parser.AddOptions(goldmark_parser.WithParagraphTransformers(
		util.Prioritized(extension.NewTableParagraphTransformer(), 200),
	))
	document := parser.Parse(text.NewReader(source))

	imageEncoder := renderer.KittyGraphicsEncoder()
	if *images && !supportsImages {
		imageEncoder = renderer.ANSIGraphicsEncoder(color.Transparent, ansimage.DitheringWithChars)
	}

	options := []renderer.RendererOption{renderer.WithTheme(theme),
		renderer.WithWordWrap(int(*width)),
		renderer.WithSoftBreak(*width != 0),
		renderer.WithPad(true),
		renderer.WithHyperlinks(*hyperlinks),
		renderer.WithImages(*images, termWidth, filepath.Dir(path)),
		renderer.WithImageEncoder(imageEncoder)}
	if hasGeometry {
		options = append(options, renderer.WithGeometry(cols, rows, termWidth, termHeight))
	}

	r := renderer.New(options...)
	renderer := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	if err := renderer.Render(os.Stdout, source, document); err != nil {
		fmt.Fprintf(os.Stderr, "error rendering %v: %v\n", path, err)
		os.Exit(-1)
	}
}
