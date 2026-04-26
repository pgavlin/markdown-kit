package main

import (
	"flag"
	"fmt"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"io"
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
	"github.com/pgavlin/markdown-kit/diagram"
	"github.com/pgavlin/markdown-kit/frontmatter"
	"github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	_ "github.com/pgavlin/svg2"
	"golang.org/x/term"
)

// renderOptions captures the configuration `render` needs to turn a source
// document into terminal output. Everything that main() gathers from flags,
// environment, and terminal queries gets funneled through this struct so
// tests can drive rendering deterministically.
type renderOptions struct {
	width      uint
	images     bool
	hyperlinks bool
	theme      *chroma.Style

	sourceDir      string
	supportsImages bool

	hasGeometry                       bool
	cols, rows, termWidth, termHeight int
}

// render parses source as Markdown and writes the ANSI-rendered form to w.
func render(w io.Writer, source []byte, opts renderOptions) error {
	parser := goldmark.DefaultParser()
	parser.AddOptions(
		goldmark_parser.WithParagraphTransformers(
			util.Prioritized(extension.NewTableParagraphTransformer(), 200),
		),
		goldmark_parser.WithBlockParsers(
			util.Prioritized(frontmatter.NewParser(), 0),
		),
	)
	document := parser.Parse(text.NewReader(source))

	imageEncoder := renderer.KittyGraphicsEncoder()
	if opts.images && !opts.supportsImages {
		imageEncoder = renderer.ANSIGraphicsEncoder(color.Transparent, ansimage.DitheringWithChars)
	}

	options := []renderer.RendererOption{
		renderer.WithTheme(opts.theme),
		renderer.WithWordWrap(int(opts.width)),
		renderer.WithSoftBreak(opts.width != 0),
		renderer.WithPad(true),
		renderer.WithHyperlinks(opts.hyperlinks),
		renderer.WithImages(opts.images, opts.termWidth, opts.sourceDir),
		renderer.WithImageEncoder(imageEncoder),
		renderer.WithDiagramRenderer(diagram.MermaidRenderer()),
	}
	if opts.hasGeometry {
		options = append(options, renderer.WithGeometry(opts.cols, opts.rows, opts.termWidth, opts.termHeight))
	}

	r := renderer.New(options...)
	rr := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	return rr.Render(w, source, document)
}

func main() {
	supportsImages := canDisplayImages()
	cols, rows, termWidth, termHeight, hasGeometry := terminalGeometry()

	width := flag.Uint("w", 0, "the maximum line width for wrappable content")
	images := flag.Bool("i", true, "display images")
	hyperlinks := flag.Bool("l", true, "display hyperlinks (use -l=false to show raw link syntax)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: %v [path to Markdown file]\n", filepath.Base(os.Args[0]))
		os.Exit(-1)
	}
	path := flag.Arg(0)

	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %v: %v\n", path, err)
		os.Exit(-1)
	}

	var theme *chroma.Style
	if term.IsTerminal(int(os.Stdout.Fd())) {
		theme = styles.AutoTheme()

		if *width == 0 {
			w, _, err := term.GetSize(int(os.Stdout.Fd()))
			if err == nil {
				*width = uint(w)
			}
		}
	}

	opts := renderOptions{
		width:          *width,
		images:         *images,
		hyperlinks:     *hyperlinks,
		theme:          theme,
		sourceDir:      filepath.Dir(path),
		supportsImages: supportsImages,
		hasGeometry:    hasGeometry,
		cols:           cols,
		rows:           rows,
		termWidth:      termWidth,
		termHeight:     termHeight,
	}

	if err := render(os.Stdout, source, opts); err != nil {
		fmt.Fprintf(os.Stderr, "error rendering %v: %v\n", path, err)
		os.Exit(-1)
	}
}
