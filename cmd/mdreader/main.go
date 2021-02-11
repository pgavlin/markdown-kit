package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gdamore/tcell/terminfo"
	"github.com/gdamore/tcell/terminfo/dynamic"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/rivo/tview"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %v [path to Markdown file]\n", filepath.Base(os.Args[0]))
		os.Exit(-1)
	}

	source, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %v: %v\n", os.Args[1], err)
		os.Exit(-1)
	}

	ti, _, err := dynamic.LoadTerminfo(os.Getenv("TERM"))
	if err == nil {
		terminfo.AddTerminfo(ti)
	}

	app := tview.NewApplication()
	reader := newMarkdownReader(filepath.Base(os.Args[1]), string(source), styles.Pulumi, app)
	app.SetRoot(reader, true)
	app.SetFocus(reader)

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running app: %v\n", err)
		os.Exit(-1)
	}
}
