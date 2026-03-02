package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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
		return &externalConverter{command: c.Command, shell: osShellRunner{}}
	}
	return &builtinConverter{}
}

type formatConverterConfig struct {
	Extensions []string `toml:"extensions"` // e.g. [".rst", ".adoc"]
	MIMETypes  []string `toml:"mime_types"` // e.g. ["text/x-rst"]
	Command    string   `toml:"command"`    // required
}

func (c formatConverterConfig) validate() error {
	if c.Command == "" {
		return fmt.Errorf("format converter: command is required")
	}
	if len(c.Extensions) == 0 && len(c.MIMETypes) == 0 {
		return fmt.Errorf("format converter: at least one extension or MIME type is required")
	}
	for _, ext := range c.Extensions {
		if !strings.HasPrefix(ext, ".") {
			return fmt.Errorf("format converter: extension %q must start with \".\"", ext)
		}
	}
	return nil
}

type config struct {
	Theme      string                  `toml:"theme"`
	Keys       map[string]any          `toml:"keys"`
	Converter  converterConfig         `toml:"converter"`
	Converters []formatConverterConfig `toml:"converters"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "md", "config.toml"), nil
}

func loadConfig(path string, fsys fileSystem, logger *slog.Logger) (config, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if err := createDefaultConfig(path, fsys); err != nil {
				logger.Error("config_load_error", "path", path, "error", err)
				return config{}, fmt.Errorf("creating default config: %w", err)
			}
			logger.Info("config_created", "path", path)
			return config{}, nil
		}
		logger.Error("config_load_error", "path", path, "error", err)
		return config{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		logger.Error("config_load_error", "path", path, "error", err)
		return config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	logger.Info("config_loaded", "path", path)
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

# Format converters for non-markdown files. Each entry specifies a shell
# command and the file extensions / MIME types it handles.
# [[converters]]
# extensions = [".rst"]
# mime_types = ["text/x-rst"]
# command = "pandoc -f rst -t markdown $MD_INPUT -o $MD_OUTPUT"

# Custom key bindings. Each key accepts a string or array of strings.
# [keys]
# quit = ["q", "ctrl+c"]
# help = "?"
`

func createDefaultConfig(path string, fsys fileSystem) error {
	if err := fsys.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return fsys.WriteFile(path, []byte(defaultConfig), 0o644)
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
		"toggle_raw":        &km.ToggleRaw,
		"open_url":          &km.OpenURL,
		"open_browser":      &km.OpenBrowser,
		"open_file_new_tab": &km.OpenFileNewTab,
		"next_tab":          &km.NextTab,
		"prev_tab":          &km.PrevTab,
		"close_tab":         &km.CloseTab,
		"close_all_tabs":    &km.CloseAllTabs,
		"new_tab":           &km.NewTab,
		"history":           &km.History,
		"help":              &km.Help,
		"quit":              &km.Quit,
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
