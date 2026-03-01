package main

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// memFS is an in-memory fileSystem for testing.
type memFS struct {
	wd    string
	files map[string][]byte
}

func newMemFS() *memFS {
	return &memFS{wd: ".", files: make(map[string][]byte)}
}

func (m *memFS) Getwd() (string, error) {
	return m.wd, nil
}

func (m *memFS) ReadFile(name string) ([]byte, error) {
	data, ok := m.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (m *memFS) WriteFile(name string, data []byte, _ fs.FileMode) error {
	m.files[name] = append([]byte(nil), data...)
	return nil
}

func (m *memFS) MkdirAll(_ string, _ fs.FileMode) error {
	return nil
}

// ReadDir synthesizes directory entries from the in-memory file map.
// It finds all direct children of the given directory path, synthesizing
// directory entries from path prefixes.
func (m *memFS) ReadDir(name string) ([]os.DirEntry, error) {
	prefix := strings.TrimSuffix(name, "/") + "/"

	seen := map[string]bool{}
	var entries []os.DirEntry
	for p, data := range m.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		if rest == "" {
			continue
		}
		// Direct child: either a file (no more slashes) or a directory (has slashes).
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			dirName := rest[:i]
			if !seen[dirName] {
				seen[dirName] = true
				entries = append(entries, &memDirEntry{name: dirName, isDir: true})
			}
		} else {
			if !seen[rest] {
				seen[rest] = true
				entries = append(entries, &memDirEntry{name: rest, size: int64(len(data))})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	return entries, nil
}

// memDirEntry implements os.DirEntry for in-memory testing.
type memDirEntry struct {
	name  string
	isDir bool
	size  int64
}

func (e *memDirEntry) Name() string               { return e.name }
func (e *memDirEntry) IsDir() bool                 { return e.isDir }
func (e *memDirEntry) Type() fs.FileMode           { if e.isDir { return fs.ModeDir }; return 0 }
func (e *memDirEntry) Info() (fs.FileInfo, error)   { return &memFileInfo{name: e.name, isDir: e.isDir, size: e.size}, nil }

// memFileInfo implements fs.FileInfo for in-memory testing.
type memFileInfo struct {
	name  string
	isDir bool
	size  int64
}

func (fi *memFileInfo) Name() string      { return path.Base(fi.name) }
func (fi *memFileInfo) Size() int64       { return fi.size }
func (fi *memFileInfo) Mode() fs.FileMode { if fi.isDir { return fs.ModeDir | 0o755 }; return 0o644 }
func (fi *memFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *memFileInfo) IsDir() bool       { return fi.isDir }
func (fi *memFileInfo) Sys() any          { return nil }

// fakeHTTPClient wraps a handler function as an httpClient.
type fakeHTTPClient struct {
	handler func(req *http.Request) (*http.Response, error)
}

func (f *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f.handler(req)
}

// fakeConverter returns canned convertResult / error.
type fakeConverter struct {
	result convertResult
	err    error
}

func (f *fakeConverter) convert(_ []byte, _ *url.URL, _ *slog.Logger) (convertResult, error) {
	return f.result, f.err
}

// fakeShellRunner wraps a handler function as a shellRunner.
type fakeShellRunner struct {
	handler func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error)
}

func (f *fakeShellRunner) Run(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
	return f.handler(command, inputData, inputEnvVar, outputEnvVar, stderr)
}

// discardLogger returns a logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(discardHandler{})
}

// Ensure discardHandler is already defined in logging.go; this uses it.
var _ slog.Handler = discardHandler{}

// Verify interfaces are satisfied at compile time.
var (
	_ fileSystem  = (*memFS)(nil)
	_ httpClient  = (*fakeHTTPClient)(nil)
	_ converter   = (*fakeConverter)(nil)
	_ shellRunner = (*fakeShellRunner)(nil)
)
