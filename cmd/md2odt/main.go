package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pgavlin/markdown-kit/odt"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %v [path to markdown file]\n", filepath.Base(os.Args[0]))
		os.Exit(-1)
	}

	doc, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read %v: %v\n", os.Args[1], err)
		os.Exit(-1)
	}

	if err = odt.FromMarkdown(os.Stdout, doc); err != nil {
		fmt.Fprintf(os.Stderr, "failed to convert markdown: %v\n", err)
		os.Exit(-1)
	}
}
