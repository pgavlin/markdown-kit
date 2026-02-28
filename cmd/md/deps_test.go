package main

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
)

// memFS is an in-memory fileSystem for testing.
type memFS struct {
	files map[string][]byte
}

func newMemFS() *memFS {
	return &memFS{files: make(map[string][]byte)}
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
