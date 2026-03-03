package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/BurntSushi/toml"
	"github.com/pgavlin/markdown-kit/docsearch"
	"github.com/urfave/cli/v3"
)

func main() {
	logger, logFile, _ := openLogger()
	if logFile != nil {
		defer logFile.Close()
	}
	defer handlePanic(logger, logFile)

	cmd := &cli.Command{
		Name:      "md",
		Usage:     "interactive terminal-based Markdown reader",
		ArgsUsage: "[path or URL ...]",
		Commands: []*cli.Command{
			{
				Name:  "config",
				Usage: "show the current configuration",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					cfgPath, err := configPath()
					if err != nil {
						fmt.Println("# config file path unknown")
					} else {
						fmt.Printf("# %s\n", cfgPath)
					}

					cfg, err := loadConfig(cfgPath, osFileSystem{}, logger)
					if err != nil {
						return err
					}

					return toml.NewEncoder(os.Stdout).Encode(cfg)
				},
			},
			{
				Name:  "system",
				Usage: "manage system data (cache, index)",
				Commands: []*cli.Command{
					{
						Name:  "clear-cache",
						Usage: "remove all cached conversion results",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							userCache, err := os.UserCacheDir()
							if err != nil {
								return fmt.Errorf("determining cache directory: %w", err)
							}
							dir := filepath.Join(userCache, "md")
							if err := os.RemoveAll(dir); err != nil {
								return fmt.Errorf("removing cache: %w", err)
							}
							fmt.Printf("Cleared cache: %s\n", dir)
							return nil
						},
					},
					{
						Name:  "clear-index",
						Usage: "remove the document search index",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							dd, err := dataDir()
							if err != nil {
								return fmt.Errorf("determining data directory: %w", err)
							}
							dbPath := filepath.Join(dd, "index.db")
							// Remove the main db and any WAL/SHM files.
							for _, suffix := range []string{"", "-wal", "-shm"} {
								os.Remove(dbPath + suffix)
							}
							fmt.Printf("Cleared search index: %s\n", dbPath)
							return nil
						},
					},
				},
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfgPath, err := configPath()
			if err != nil {
				return fmt.Errorf("error determining config path: %w", err)
			}

			fsys := osFileSystem{}
			cfg, err := loadConfig(cfgPath, fsys, logger)
			if err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}

			if err := cfg.Converter.validate(); err != nil {
				return fmt.Errorf("error in config: %w", err)
			}
			for i, fc := range cfg.Converters {
				if err := fc.validate(); err != nil {
					return fmt.Errorf("error in config: converters[%d]: %w", i, err)
				}
			}

			theme := cfg.theme()
			conv := cfg.Converter.newConverter()
			registry := newConverterRegistry(cfg.Converters, osShellRunner{})
			cache := openCache()
			httpCl := http.DefaultClient

			// Open the document search index.
			var searchIndex *docsearch.Index
			if dd, err := dataDir(); err == nil {
				if err := os.MkdirAll(dd, 0o755); err == nil {
					dbPath := filepath.Join(dd, "index.db")
					embedder := cfg.Search.newEmbedder()
					if idx, err := docsearch.Open(dbPath, embedder); err == nil {
						searchIndex = idx
						defer idx.Close()
					} else {
						logger.Error("search_index_open_error", "path", dbPath, "error", err)
					}
				}
			}

			var model markdownReader
			if cmd.Args().Len() == 0 {
				// No args — start with file picker.
				model = newMarkdownReader("", "", "", theme, conv, registry, cache, httpCl, fsys, searchIndex, logger)
				model.showPicker = true
				model.pickerStartup = true
			} else {
				// Load the first argument into the initial tab.
				arg := cmd.Args().Get(0)
				if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
					result, err := fetchURL(arg, conv, registry, cache, httpCl, logger)
					if err != nil {
						return fmt.Errorf("error fetching %v: %w", arg, err)
					}
					model = newMarkdownReader("", result.markdown, result.source, theme, conv, registry, cache, httpCl, fsys, searchIndex, logger)
					// Index the initial document.
					if searchIndex != nil {
						searchIndex.Add(ctx, result.source, model.active().view.GetName(), result.markdown)
					}
				} else if isConvertibleFile(arg, registry) {
					source, err := fsys.ReadFile(arg)
					if err != nil {
						return fmt.Errorf("error opening %v: %w", arg, err)
					}
					absPath, err := filepath.Abs(arg)
					if err != nil {
						absPath = arg
					}
					ext := strings.ToLower(filepath.Ext(absPath))
					fc := registry.forExtension(ext)
					cr, err := fc.convert(source, nil, logger)
					if err != nil {
						return fmt.Errorf("error converting %v: %w", arg, err)
					}
					model = newMarkdownReader("", cr.markdown, absPath, theme, conv, registry, cache, httpCl, fsys, searchIndex, logger)
					// Index the initial document.
					if searchIndex != nil {
						searchIndex.Add(ctx, absPath, model.active().view.GetName(), cr.markdown)
					}
				} else {
					source, err := fsys.ReadFile(arg)
					if err != nil {
						return fmt.Errorf("error opening %v: %w", arg, err)
					}
					absPath, err := filepath.Abs(arg)
					if err != nil {
						absPath = arg
					}
					model = newMarkdownReader("", string(source), absPath, theme, conv, registry, cache, httpCl, fsys, searchIndex, logger)
					// Index the initial document.
					if searchIndex != nil {
						searchIndex.Add(ctx, absPath, model.active().view.GetName(), string(source))
					}
				}
			}

			cfg.applyKeys(&model.keys)
			model.active().view.KeyMap = model.keys.KeyMap

			// Open remaining arguments in additional tabs.
			for i := 1; i < cmd.Args().Len(); i++ {
				arg := cmd.Args().Get(i)
				if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
					result, err := fetchURL(arg, conv, registry, cache, httpCl, logger)
					if err != nil {
						return fmt.Errorf("error fetching %v: %w", arg, err)
					}
					model.openNewTab("", result.markdown, result.source)
				} else if isConvertibleFile(arg, registry) {
					source, err := fsys.ReadFile(arg)
					if err != nil {
						return fmt.Errorf("error opening %v: %w", arg, err)
					}
					absPath, err := filepath.Abs(arg)
					if err != nil {
						absPath = arg
					}
					ext := strings.ToLower(filepath.Ext(absPath))
					fc := registry.forExtension(ext)
					cr, err := fc.convert(source, nil, logger)
					if err != nil {
						return fmt.Errorf("error converting %v: %w", arg, err)
					}
					model.openNewTab(cr.name, cr.markdown, absPath)
				} else {
					source, err := fsys.ReadFile(arg)
					if err != nil {
						return fmt.Errorf("error opening %v: %w", arg, err)
					}
					absPath, err := filepath.Abs(arg)
					if err != nil {
						absPath = arg
					}
					model.openNewTab("", string(source), absPath)
				}
			}

			// Activate the first tab when multiple were opened.
			if cmd.Args().Len() > 1 {
				model.activeTab = 0
			}

			p := tea.NewProgram(model)

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("error running app: %w", err)
			}

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
