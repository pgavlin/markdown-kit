package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %v [path or URL]\n", filepath.Base(os.Args[0]))
		os.Exit(-1)
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(-1)
	}

	if err := cfg.Converter.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error in config: %v\n", err)
		os.Exit(-1)
	}

	arg := os.Args[1]
	theme := cfg.theme()
	conv := cfg.Converter.newConverter()

	var model markdownReader
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
		result, err := fetchURL(arg, conv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error fetching %v: %v\n", arg, err)
			os.Exit(-1)
		}
		model = newMarkdownReader(result.name, result.markdown, result.source, theme, conv)
		model.currentOriginalHTML = result.originalHTML
		model.currentReadabilityHTML = result.readabilityHTML
		model.updateHTMLKeyBindings()
	} else {
		source, err := os.ReadFile(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening %v: %v\n", arg, err)
			os.Exit(-1)
		}
		absPath, err := filepath.Abs(arg)
		if err != nil {
			absPath = arg
		}
		model = newMarkdownReader(filepath.Base(absPath), string(source), absPath, theme, conv)
	}

	cfg.applyKeys(&model.keys)
	model.view.KeyMap = model.keys.KeyMap

	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running app: %v\n", err)
		os.Exit(-1)
	}
}
