package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/BurntSushi/toml"
	"github.com/pgavlin/markdown-kit/diagram"
	"github.com/pgavlin/markdown-kit/docsearch"
	mdk "github.com/pgavlin/markdown-kit/view"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
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
			{
				Name:  "index",
				Usage: "manage the background indexing daemon",
				Commands: []*cli.Command{
					{
						Name:  "start",
						Usage: "start the background indexing daemon",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							return runIndexDaemon(ctx, false, logger)
						},
					},
					{
						Name:  "stop",
						Usage: "stop the background indexing daemon",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							running, pid := isDaemonRunning()
							if !running {
								fmt.Println("Daemon is not running.")
								return nil
							}
							p, err := os.FindProcess(pid)
							if err != nil {
								return fmt.Errorf("finding process %d: %w", pid, err)
							}
							if err := p.Signal(syscall.SIGTERM); err != nil {
								return fmt.Errorf("stopping daemon (pid %d): %w", pid, err)
							}
							fmt.Printf("Sent stop signal to daemon (pid %d).\n", pid)
							return nil
						},
					},
					{
						Name:  "status",
						Usage: "show the status of the indexing daemon",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							running, pid := isDaemonRunning()
							if running {
								fmt.Printf("Daemon is running (pid %d).\n", pid)
							} else {
								fmt.Println("Daemon is not running.")
							}
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

			var viewOpts []mdk.Option
			viewOpts = append(viewOpts, mdk.WithDiagramRenderer(diagram.MermaidRenderer()))
			// Interactive tables are disabled by default; the user can toggle them with "I".
			if cfg.stripDataURIs() {
				viewOpts = append(viewOpts, mdk.WithDocumentTransformer(mdk.StripDataURIs))
			}

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

			// Check if stdin has piped data.
			stdinPiped := !term.IsTerminal(int(os.Stdin.Fd()))

			var model markdownReader
			if cmd.Args().Len() == 0 && stdinPiped {
				// Read markdown from stdin.
				source, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("error reading stdin: %w", err)
				}
				model = newMarkdownReader("", string(source), "", theme, viewOpts, conv, registry, cache, httpCl, fsys, searchIndex, logger)
			} else if cmd.Args().Len() == 0 {
				// No args — start with file picker.
				model = newMarkdownReader("", "", "", theme, viewOpts, conv, registry, cache, httpCl, fsys, searchIndex, logger)
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
					model = newMarkdownReader("", result.markdown, result.source, theme, viewOpts, conv, registry, cache, httpCl, fsys, searchIndex, logger)
					// Index the initial document in the background during Init.
					if searchIndex != nil {
						model.pendingIndex = &pendingIndexEntry{
							path:     result.source,
							title:    model.active().view.GetName(),
							markdown: result.markdown,
						}
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
					model = newMarkdownReader("", cr.markdown, absPath, theme, viewOpts, conv, registry, cache, httpCl, fsys, searchIndex, logger)
					// Index the initial document in the background during Init.
					if searchIndex != nil {
						model.pendingIndex = &pendingIndexEntry{
							path:     absPath,
							title:    model.active().view.GetName(),
							markdown: cr.markdown,
						}
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
					model = newMarkdownReader("", string(source), absPath, theme, viewOpts, conv, registry, cache, httpCl, fsys, searchIndex, logger)
					// Index the initial document in the background during Init.
					if searchIndex != nil {
						model.pendingIndex = &pendingIndexEntry{
							path:     absPath,
							title:    model.active().view.GetName(),
							markdown: string(source),
						}
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

			// Auto-start the indexing daemon if roots are configured and it isn't already running.
			if len(cfg.Index.Roots) > 0 {
				if running, _ := isDaemonRunning(); !running {
					startDaemonBackground(logger)
				}
			}

			var progOpts []tea.ProgramOption
			if stdinPiped {
				tty, err := openTTY()
				if err != nil {
					return fmt.Errorf("error opening terminal for keyboard input: %w", err)
				}
				defer tty.Close()
				progOpts = append(progOpts, tea.WithInput(tty))
			}

			p := tea.NewProgram(model, progOpts...)

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

// runIndexDaemon runs the indexing daemon in the foreground. If background is
// true, the function is being called from a detached child process.
func runIndexDaemon(ctx context.Context, background bool, logger *slog.Logger) error {
	cfgPath, err := configPath()
	if err != nil {
		return fmt.Errorf("determining config path: %w", err)
	}
	cfg, err := loadConfig(cfgPath, osFileSystem{}, logger)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if len(cfg.Index.Roots) == 0 {
		return fmt.Errorf("no index roots configured in %s", cfgPath)
	}

	// Check if another daemon is already running.
	if running, pid := isDaemonRunning(); running {
		if background {
			return nil // Silently exit — another daemon won.
		}
		return fmt.Errorf("daemon already running (pid %d)", pid)
	}

	// Write PID file.
	pidPath, err := pidFilePath()
	if err != nil {
		return fmt.Errorf("determining PID file path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}
	if err := writePIDFile(pidPath); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer removePIDFile(pidPath)

	// Open the search index.
	dd, err := dataDir()
	if err != nil {
		return fmt.Errorf("determining data directory: %w", err)
	}
	dbPath := filepath.Join(dd, "index.db")
	embedder := cfg.Search.newEmbedder()
	index, err := docsearch.Open(dbPath, embedder)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer index.Close()

	// Set up signal handling for clean shutdown.
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	roots := cfg.Index.expandedRoots()
	exclude := cfg.Index.excludeSet()
	interval := cfg.Index.pollInterval()

	if !background {
		fmt.Printf("Indexing daemon started (pid %d), scanning %v\n", os.Getpid(), roots)
	}
	logger.Info("daemon_start", "pid", os.Getpid(), "roots", roots, "interval", interval)

	err = runScanner(ctx, index, roots, exclude, interval, logger)
	if err == context.Canceled {
		logger.Info("daemon_stopped")
		return nil
	}
	return err
}

// startDaemonBackground launches the indexing daemon as a detached child process.
func startDaemonBackground(logger *slog.Logger) {
	exe, err := os.Executable()
	if err != nil {
		logger.Warn("daemon_autostart_failed", "error", err)
		return
	}

	cmd := exec.Command(exe, "index", "start")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	// Detach the child process so it survives after the TUI exits.
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		logger.Warn("daemon_autostart_failed", "error", err)
		return
	}

	// Release so we don't wait for it.
	cmd.Process.Release()
	logger.Info("daemon_autostarted", "pid", cmd.Process.Pid)
}
