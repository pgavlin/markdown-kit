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
		ArgsUsage: "[path or URL]",
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
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				cli.ShowAppHelp(cmd)
				return fmt.Errorf("expected exactly one argument")
			}

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

			arg := cmd.Args().Get(0)
			theme := cfg.theme()
			conv := cfg.Converter.newConverter()
			cache := openCache()
			httpCl := http.DefaultClient

			var model markdownReader
			if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
				result, err := fetchURL(arg, conv, cache, httpCl, logger)
				if err != nil {
					return fmt.Errorf("error fetching %v: %w", arg, err)
				}
				model = newMarkdownReader(result.name, result.markdown, result.source, theme, conv, cache, httpCl, fsys, logger)
				model.currentOriginalHTML = result.originalHTML
				model.currentReadabilityHTML = result.readabilityHTML
				model.updateHTMLKeyBindings()
			} else {
				source, err := fsys.ReadFile(arg)
				if err != nil {
					return fmt.Errorf("error opening %v: %w", arg, err)
				}
				absPath, err := filepath.Abs(arg)
				if err != nil {
					absPath = arg
				}
				model = newMarkdownReader(filepath.Base(absPath), string(source), absPath, theme, conv, cache, httpCl, fsys, logger)
			}

			cfg.applyKeys(&model.keys)
			model.view.KeyMap = model.keys.KeyMap

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
