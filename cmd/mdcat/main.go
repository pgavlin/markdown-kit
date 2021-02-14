package main

import (
	"flag"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/alecthomas/chroma"
	"github.com/pgavlin/goldmark"
	goldmark_renderer "github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	"golang.org/x/term"
)

func main() {
	showImages, termWidth, _ := canDisplayImages()

	width := flag.Uint("w", 0, "the maximum line width for wrappable content")
	images := flag.Bool("i", showImages, "display images")
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
	document := parser.Parse(text.NewReader(source))

	r := renderer.New(
		renderer.WithTheme(theme),
		renderer.WithWordWrap(int(*width)),
		renderer.WithSoftBreak(*width != 0),
		renderer.WithImages(*images, termWidth, filepath.Dir(path)))
	renderer := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	if err := renderer.Render(os.Stdout, source, document); err != nil {
		fmt.Fprintf(os.Stderr, "error rendering %v: %v\n", path, err)
		os.Exit(-1)
	}
}
