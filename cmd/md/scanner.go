package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pgavlin/markdown-kit/docsearch"
)

// scanner manages file discovery and indexing as a long-running daemon.
type scanner struct {
	index    *docsearch.Index
	roots    []string
	exclude  map[string]bool
	interval time.Duration
	logger   *slog.Logger
	watcher  *fsnotify.Watcher
}

// runScanner is the daemon main loop. It performs an initial full scan, then
// watches for changes via fsnotify with a periodic re-walk fallback.
func runScanner(ctx context.Context, index *docsearch.Index, roots []string, exclude map[string]bool, interval time.Duration, logger *slog.Logger) error {
	s := &scanner{
		index:    index,
		roots:    roots,
		exclude:  exclude,
		interval: interval,
		logger:   logger,
	}

	// Phase 1: Full initial scan — reindex and re-embed changed files.
	logger.Info("scan_initial_start", "roots", roots)
	indexed, removed, reembedded := s.fullScan(ctx)
	logger.Info("scan_initial_done", "indexed", indexed, "removed", removed, "reembedded", reembedded)

	// Phase 2: Set up fsnotify watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Warn("fsnotify_init_failed", "error", err)
	} else {
		s.watcher = watcher
		defer watcher.Close()
		s.addWatches()
	}

	// Phase 3: Event loop with periodic re-walk fallback.
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var debounceTimer *time.Timer
	pendingPaths := make(map[string]fsnotify.Op)

	var watchCh <-chan fsnotify.Event
	var watchErrCh <-chan error
	if s.watcher != nil {
		watchCh = s.watcher.Events
		watchErrCh = s.watcher.Errors
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-watchCh:
			if !ok {
				watchCh = nil
				continue
			}
			pendingPaths[event.Name] = event.Op
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(500 * time.Millisecond)
			} else {
				debounceTimer.Reset(500 * time.Millisecond)
			}

		case <-timerChan(debounceTimer):
			debounceTimer = nil
			n := s.processPendingEvents(ctx, pendingPaths)
			pendingPaths = make(map[string]fsnotify.Op)
			if n > 0 {
				logger.Info("watch_indexed", "count", n)
			}

		case err, ok := <-watchErrCh:
			if !ok {
				watchErrCh = nil
				continue
			}
			logger.Warn("fsnotify_error", "error", err)

		case <-ticker.C:
			logger.Info("scan_periodic_start")
			indexed, removed, reembedded := s.fullScan(ctx)
			logger.Info("scan_periodic_done", "indexed", indexed, "removed", removed, "reembedded", reembedded)
		}
	}
}

// timerChan returns a nil channel if the timer is nil.
func timerChan(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

// fullScan walks all roots, indexes markdown files, re-embeds changed files,
// and removes stale entries.
func (s *scanner) fullScan(ctx context.Context) (indexed, removed, reembedded int) {
	found := make(map[string]bool)

	for _, root := range s.roots {
		if ctx.Err() != nil {
			return
		}
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return fs.SkipAll
			}
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if s.exclude[d.Name()] {
					return fs.SkipDir
				}
				return nil
			}
			if !isMarkdownFile(path) {
				return nil
			}
			absPath, err := filepath.Abs(path)
			if err != nil {
				return nil
			}
			found[absPath] = true

			if err := s.indexFile(ctx, absPath); err != nil {
				s.logger.Warn("scan_index_error", "path", absPath, "error", err)
			} else {
				indexed++
			}

			return nil
		})
	}

	// Remove stale entries (files under our roots that no longer exist).
	existingPaths, err := s.index.ListPaths(ctx)
	if err != nil {
		s.logger.Warn("scan_list_paths_error", "error", err)
		return indexed, 0, 0
	}
	for _, p := range existingPaths {
		if !found[p] && s.isUnderRoots(p) {
			if err := s.index.Remove(p); err != nil {
				s.logger.Warn("scan_remove_error", "path", p, "error", err)
			} else {
				removed++
			}
		}
	}

	// Re-embed documents whose content has changed since last embedding.
	if s.index.HasEmbedder() {
		docs, err := s.index.DocumentsNeedingEmbeddings(ctx, 0)
		if err != nil {
			s.logger.Warn("scan_embed_query_error", "error", err)
		} else {
			for _, doc := range docs {
				if ctx.Err() != nil {
					break
				}
				if err := s.index.Add(ctx, doc.Path, "", doc.Markdown); err != nil {
					s.logger.Warn("scan_embed_error", "path", doc.Path, "error", err)
				} else {
					reembedded++
				}
			}
		}
	}

	return indexed, removed, reembedded
}

// indexFile reads a markdown file and indexes it using the full Add method,
// which handles both FTS and embeddings (if an embedder is configured).
func (s *scanner) indexFile(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	title := titleFromMarkdown(string(data))
	return s.index.Add(ctx, path, title, string(data))
}

// titleFromMarkdown extracts the first heading from markdown content.
func titleFromMarkdown(markdown string) string {
	for _, line := range strings.SplitN(markdown, "\n", 20) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

// addWatches adds fsnotify watches on all directories under the configured roots.
func (s *scanner) addWatches() {
	var watchErrors int
	for _, root := range s.roots {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if s.exclude[d.Name()] {
				return fs.SkipDir
			}
			if err := s.watcher.Add(path); err != nil {
				watchErrors++
				if watchErrors == 1 {
					s.logger.Warn("fsnotify_watch_limit", "path", path, "error", err)
				}
			}
			return nil
		})
	}
	if watchErrors > 0 {
		s.logger.Warn("fsnotify_watch_limit_total", "failed_watches", watchErrors)
	}
}

// processPendingEvents handles debounced fsnotify events.
func (s *scanner) processPendingEvents(ctx context.Context, pending map[string]fsnotify.Op) int {
	var indexed int
	for path, op := range pending {
		if ctx.Err() != nil {
			return indexed
		}

		if op.Has(fsnotify.Remove) || op.Has(fsnotify.Rename) {
			absPath, _ := filepath.Abs(path)
			if absPath != "" {
				s.index.Remove(absPath)
			}
			continue
		}

		if isMarkdownFile(path) {
			absPath, err := filepath.Abs(path)
			if err != nil {
				continue
			}
			if err := s.indexFile(ctx, absPath); err != nil {
				s.logger.Warn("watch_index_error", "path", absPath, "error", err)
			} else {
				indexed++
			}
		}

		// New directories: add watch and walk contents.
		if op.Has(fsnotify.Create) {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				if !s.exclude[filepath.Base(path)] && s.watcher != nil {
					s.watcher.Add(path)
					filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
						if err != nil {
							return nil
						}
						if d.IsDir() {
							if s.exclude[d.Name()] {
								return fs.SkipDir
							}
							s.watcher.Add(p)
							return nil
						}
						if !isMarkdownFile(p) {
							return nil
						}
						absP, _ := filepath.Abs(p)
						if absP != "" {
							if err := s.indexFile(ctx, absP); err == nil {
								indexed++
							}
						}
						return nil
					})
				}
			}
		}
	}
	return indexed
}

// isUnderRoots checks if a path is under one of the configured roots.
func (s *scanner) isUnderRoots(path string) bool {
	for _, root := range s.roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if strings.HasPrefix(path, absRoot+string(filepath.Separator)) || path == absRoot {
			return true
		}
	}
	return false
}

// pidFilePath returns the path to the daemon PID file.
func pidFilePath() (string, error) {
	dd, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dd, "scanner.pid"), nil
}

// writePIDFile writes the current process PID to the PID file.
func writePIDFile(path string) error {
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
}

// readPIDFile reads the PID from the PID file. Returns 0 if the file
// doesn't exist or can't be parsed.
func readPIDFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	return pid
}

// removePIDFile removes the PID file.
func removePIDFile(path string) {
	os.Remove(path)
}

// isDaemonRunning checks if the daemon process is still alive.
func isDaemonRunning() (bool, int) {
	pidPath, err := pidFilePath()
	if err != nil {
		return false, 0
	}
	pid := readPIDFile(pidPath)
	if pid == 0 {
		return false, 0
	}
	return isProcessAlive(pid), pid
}

// isProcessAlive checks if a process with the given PID exists.
func isProcessAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return signalProcess(p) == nil
}
