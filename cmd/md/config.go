package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/BurntSushi/toml"
	"github.com/alecthomas/chroma"
	chromaStyles "github.com/alecthomas/chroma/styles"
	"github.com/pgavlin/markdown-kit/styles"
)

type converterConfig struct {
	Method  string `toml:"method"`  // "builtin" (default) or "external"
	Command string `toml:"command"` // required when method = "external"; run via system shell
}

func (c converterConfig) validate() error {
	switch c.Method {
	case "", "builtin":
		return nil
	case "external":
		if c.Command == "" {
			return fmt.Errorf("converter: method \"external\" requires a non-empty command")
		}
		return nil
	default:
		return fmt.Errorf("converter: unknown method %q (expected \"builtin\" or \"external\")", c.Method)
	}
}

func (c converterConfig) newConverter() converter {
	if c.Method == "external" {
		return &externalConverter{command: c.Command}
	}
	return &builtinConverter{}
}

type config struct {
	Theme     string          `toml:"theme"`
	Keys      map[string]any  `toml:"keys"`
	Converter converterConfig `toml:"converter"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "md", "config.toml"), nil
}

func loadConfig() (config, error) {
	path, err := configPath()
	if err != nil {
		return config{}, nil
	}

	var cfg config
	_, err = toml.DecodeFile(path, &cfg)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if err := createDefaultConfig(path); err != nil {
				return config{}, fmt.Errorf("creating default config: %w", err)
			}
			return config{}, nil
		}
		return config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

const defaultConfig = `# md configuration file
# See https://github.com/pgavlin/markdown-kit for documentation.

# Color theme (any Chroma style name, e.g. "monokai", "dracula").
# Defaults to a built-in dark theme when empty.
# theme = ""

# Content converter for HTML-to-Markdown when opening URLs.
# [converter]
# method = "builtin"              # "builtin" (default) or "external"
# command = "pandoc -f html -t markdown"  # required when method = "external"; run via system shell

# Custom key bindings. Each key accepts a string or array of strings.
# [keys]
# quit = ["q", "ctrl+c"]
# help = "?"
`

func createDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfig), 0o644)
}

func (c config) theme() *chroma.Style {
	if c.Theme == "" {
		return styles.GlamourDark
	}
	s := chromaStyles.Get(c.Theme)
	if s == chromaStyles.Fallback {
		return styles.GlamourDark
	}
	return s
}

func (c config) applyKeys(km *readerKeyMap) {
	// Map snake_case config names to binding pointers.
	nameToBinding := map[string]*key.Binding{
		// View keys
		"up":              &km.Up,
		"down":            &km.Down,
		"page_up":         &km.PageUp,
		"page_down":       &km.PageDown,
		"goto_top":        &km.GotoTop,
		"goto_end":        &km.GotoEnd,
		"home":            &km.Home,
		"end":             &km.End,
		"left":            &km.Left,
		"right":           &km.Right,
		"next_link":       &km.NextLink,
		"prev_link":       &km.PrevLink,
		"next_code_block": &km.NextCodeBlock,
		"prev_code_block": &km.PrevCodeBlock,
		"next_heading":    &km.NextHeading,
		"prev_heading":    &km.PrevHeading,
		"decrease_width":  &km.DecreaseWidth,
		"increase_width":  &km.IncreaseWidth,
		"follow_link":     &km.FollowLink,
		"go_back":         &km.GoBack,
		"copy_selection":  &km.CopySelection,
		"search":          &km.Search,
		"next_match":      &km.NextMatch,
		"prev_match":      &km.PrevMatch,
		"clear_search":    &km.ClearSearch,
		// Reader keys
		"toggle_raw":              &km.ToggleRaw,
		"toggle_original_html":    &km.ToggleOriginalHTML,
		"toggle_readability_html": &km.ToggleReadabilityHTML,
		"open_browser":            &km.OpenBrowser,
		"help":                    &km.Help,
		"quit":                    &km.Quit,
	}

	for name, val := range c.Keys {
		b, ok := nameToBinding[name]
		if !ok {
			continue
		}

		var keys []string
		switch v := val.(type) {
		case string:
			keys = []string{v}
		case []any:
			for _, elem := range v {
				if s, ok := elem.(string); ok {
					keys = append(keys, s)
				}
			}
		}
		if len(keys) == 0 {
			continue
		}

		// Preserve the existing help text.
		help := b.Help()
		helpKey := strings.Join(keys, "/")
		*b = key.NewBinding(
			key.WithKeys(keys...),
			key.WithHelp(helpKey, help.Desc),
		)
	}
}
