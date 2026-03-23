package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"github.com/BurntSushi/toml"
	"github.com/alecthomas/chroma"
	chromaStyles "github.com/alecthomas/chroma/styles"
	"github.com/pgavlin/markdown-kit/docsearch"
	"github.com/pgavlin/markdown-kit/styles"
)

type converterConfig struct {
	Command string `toml:"command"` // shell command for HTML-to-Markdown conversion
}

func (c converterConfig) validate() error {
	return nil
}

func (c converterConfig) newConverter() converter {
	if c.Command != "" {
		return &externalConverter{command: c.Command, shell: osShellRunner{}}
	}
	return nil
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

type apiEmbedderConfig struct {
	URL        string `toml:"url"`         // default: "https://api.openai.com/v1/embeddings"
	Model      string `toml:"model"`       // default: "text-embedding-3-small"
	APIKeyEnv  string `toml:"api_key_env"` // env var name, default: "OPENAI_API_KEY"
	Dimensions int    `toml:"dimensions"`  // required
}

type ollamaEmbedderConfig struct {
	URL        string `toml:"url"`        // default: "http://localhost:11434"
	Model      string `toml:"model"`      // default: "nomic-embed-text"
	Dimensions int    `toml:"dimensions"` // required
}

type commandEmbedderConfig struct {
	Command    string `toml:"command"`    // shell command
	Dimensions int    `toml:"dimensions"` // required
}

type searchConfig struct {
	Embedder string                `toml:"embedder"` // "api", "ollama", "command", or "" (FTS-only)
	API      apiEmbedderConfig     `toml:"api"`
	Ollama   ollamaEmbedderConfig  `toml:"ollama"`
	Command  commandEmbedderConfig `toml:"command"`
}

func (c searchConfig) newEmbedder() docsearch.Embedder {
	switch c.Embedder {
	case "api":
		url := c.API.URL
		if url == "" {
			url = "https://api.openai.com/v1/embeddings"
		}
		model := c.API.Model
		if model == "" {
			model = "text-embedding-3-small"
		}
		keyEnv := c.API.APIKeyEnv
		if keyEnv == "" {
			keyEnv = "OPENAI_API_KEY"
		}
		apiKey := os.Getenv(keyEnv)
		return docsearch.NewAPIEmbedder(url, model, apiKey, c.API.Dimensions)
	case "ollama":
		url := c.Ollama.URL
		if url == "" {
			url = "http://localhost:11434"
		}
		model := c.Ollama.Model
		if model == "" {
			model = "nomic-embed-text"
		}
		return docsearch.NewOllamaEmbedder(url, model, c.Ollama.Dimensions)
	case "command":
		return docsearch.NewCommandEmbedder(c.Command.Command, c.Command.Dimensions)
	default:
		return nil
	}
}

type indexConfig struct {
	// Roots lists directory paths to scan for markdown files.
	// Supports ~ for home directory (e.g. "~/Documents").
	Roots []string `toml:"roots"`

	// Exclude lists directory base names to skip during scanning
	// (e.g. "node_modules", ".git"). These are matched against the
	// final component of each directory path, not as glob patterns.
	Exclude []string `toml:"exclude"`

	// PollInterval controls how often the daemon re-walks roots after
	// the initial scan. Parsed as a Go duration (e.g. "10m", "1h").
	PollInterval string `toml:"poll_interval"`
}

func (c indexConfig) pollInterval() time.Duration {
	if c.PollInterval == "" {
		return 10 * time.Minute
	}
	d, err := time.ParseDuration(c.PollInterval)
	if err != nil {
		return 10 * time.Minute
	}
	return d
}

func (c indexConfig) excludeSet() map[string]bool {
	defaults := map[string]bool{
		"node_modules": true,
		".git":         true,
		"vendor":       true,
		"target":       true,
		"build":        true,
	}
	for _, e := range c.Exclude {
		defaults[e] = true
	}
	return defaults
}

// expandedRoots returns the configured roots with ~ and environment
// variables expanded.
func (c indexConfig) expandedRoots() []string {
	var roots []string
	for _, r := range c.Roots {
		r = os.ExpandEnv(r)
		r = expandTilde(r)
		roots = append(roots, r)
	}
	return roots
}

// expandTilde replaces a leading "~/" with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

type config struct {
	Theme         string                  `toml:"theme"`
	StripDataURIs *bool                   `toml:"strip_data_uris"`
	Keys          map[string]any          `toml:"keys"`
	Converter     converterConfig         `toml:"converter"`
	Converters    []formatConverterConfig `toml:"converters"`
	Search        searchConfig            `toml:"search"`
	Index         indexConfig             `toml:"index"`
}

func (c config) stripDataURIs() bool {
	return c.StripDataURIs == nil || *c.StripDataURIs
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

# Strip inline HTML tags containing data: URIs (e.g. base64 images).
# Enabled by default. Set to false to preserve them.
# strip_data_uris = true

# Content converter for HTML-to-Markdown when opening URLs.
# [converter]
# command = "pandoc -f html -t markdown"  # shell command to convert HTML to Markdown

# Format converters for non-markdown files. Each entry specifies a shell
# command and the file extensions / MIME types it handles.
# [[converters]]
# extensions = [".rst"]
# mime_types = ["text/x-rst"]
# command = "pandoc -f rst -t markdown $MD_INPUT -o $MD_OUTPUT"

# Document search. Opened documents are indexed for full-text search.
# To enable semantic (vector) search, configure an embedder.
# [search]
# embedder = "ollama"   # "api", "ollama", "command", or "" (FTS-only)
#
# [search.api]
# url = "https://api.openai.com/v1/embeddings"
# model = "text-embedding-3-small"
# api_key_env = "OPENAI_API_KEY"
# dimensions = 1536
#
# [search.ollama]
# url = "http://localhost:11434"
# model = "nomic-embed-text"
# dimensions = 768
#
# [search.command]
# command = "my-embed-tool"
# dimensions = 384

# Background document indexing. Configure root directory paths to scan
# for markdown files. A background daemon discovers and indexes files
# for full-text (and optionally semantic) search. The daemon starts
# automatically when the TUI launches.
# [index]
# roots = ["~/Documents", "~/code"]           # directory paths to scan
# exclude = ["node_modules", ".git", "vendor"] # directory names to skip
# poll_interval = "10m"                         # re-scan interval

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
	if c.Theme == "" || c.Theme == "auto" {
		return styles.AutoTheme()
	}
	s := chromaStyles.Get(c.Theme)
	if s == chromaStyles.Fallback {
		return styles.AutoTheme()
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
		"next_item":    &km.NextItem,
		"prev_item":    &km.PrevItem,
		"next_heading": &km.NextHeading,
		"prev_heading": &km.PrevHeading,
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
		"toggle_source":     &km.ToggleSource,
		"toggle_raw":        &km.ToggleSource, // backwards compat
		"open_url":          &km.OpenURL,
		"open_browser":      &km.OpenBrowser,
		"open_file_new_tab": &km.OpenFileNewTab,
		"next_tab":          &km.NextTab,
		"prev_tab":          &km.PrevTab,
		"close_tab":         &km.CloseTab,
		"close_all_tabs":    &km.CloseAllTabs,
		"new_tab":           &km.NewTab,
		"reload":            &km.Reload,
		"history":           &km.History,
		"search_documents":  &km.SearchDocuments,
		"find_similar":      &km.FindSimilar,
		"user_guide":        &km.UserGuide,
		"bug_report":        &km.BugReport,
		"export_gist":       &km.ExportGist,
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
