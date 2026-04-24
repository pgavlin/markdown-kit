package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pgavlin/markdown-kit/odt"
)

// run is the testable entry point: it takes its own stdout/stderr and argv,
// returns an exit code. main() wires the process to it.
func run(stdout, stderr io.Writer, args []string) int {
	if len(args) != 2 {
		fmt.Fprintf(stderr, "usage: %v [path to markdown file]\n", filepath.Base(args[0]))
		return -1
	}

	doc, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "failed to read %v: %v\n", args[1], err)
		return -1
	}

	if err := odt.FromMarkdown(stdout, doc); err != nil {
		fmt.Fprintf(stderr, "failed to convert markdown: %v\n", err)
		return -1
	}
	return 0
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Args))
}
