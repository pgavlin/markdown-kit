package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/pgavlin/markdown-kit/styles"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %v [path to Markdown file]\n", filepath.Base(os.Args[0]))
		os.Exit(-1)
	}

	source, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %v: %v\n", os.Args[1], err)
		os.Exit(-1)
	}

	model := newMarkdownReader(filepath.Base(os.Args[1]), string(source), styles.GlamourDark)
	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running app: %v\n", err)
		os.Exit(-1)
	}
}
