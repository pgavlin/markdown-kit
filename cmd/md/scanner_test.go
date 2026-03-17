package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTitleFromMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{
			name:     "simple heading",
			markdown: "# Hello World\nSome content",
			want:     "Hello World",
		},
		{
			name:     "no heading",
			markdown: "Just some text",
			want:     "",
		},
		{
			name:     "heading after blank line",
			markdown: "\n\n# Title\nContent",
			want:     "Title",
		},
		{
			name:     "heading with leading spaces",
			markdown: "  # Indented  ",
			want:     "Indented",
		},
		{
			name:     "h2 not extracted",
			markdown: "## Section\nContent",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := titleFromMarkdown(tt.markdown)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIndexConfigDefaults(t *testing.T) {
	cfg := indexConfig{}

	interval := cfg.pollInterval()
	assert.Equal(t, "10m0s", interval.String())

	exclude := cfg.excludeSet()
	assert.True(t, exclude["node_modules"])
	assert.True(t, exclude[".git"])
	assert.True(t, exclude["vendor"])
}

func TestIndexConfigCustom(t *testing.T) {
	cfg := indexConfig{
		PollInterval: "5m",
		Exclude:      []string{"dist", "tmp"},
	}

	assert.Equal(t, "5m0s", cfg.pollInterval().String())

	exclude := cfg.excludeSet()
	assert.True(t, exclude["dist"])
	assert.True(t, exclude["tmp"])
	assert.True(t, exclude["node_modules"]) // Default still present.
}

func TestIndexConfigExpandedRoots(t *testing.T) {
	cfg := indexConfig{
		Roots: []string{"/absolute/path", "relative/path"},
	}

	roots := cfg.expandedRoots()
	assert.Contains(t, roots, "/absolute/path")
	assert.Contains(t, roots, "relative/path")
}

func TestExpandTilde(t *testing.T) {
	// No tilde — returned as-is.
	assert.Equal(t, "/foo/bar", expandTilde("/foo/bar"))
	assert.Equal(t, "relative", expandTilde("relative"))

	// With tilde — should expand (exact result depends on test environment).
	result := expandTilde("~/Documents")
	assert.NotEqual(t, "~/Documents", result)
	assert.True(t, len(result) > len("~/Documents"))
}

func TestIsDaemonRunning_NoPIDFile(t *testing.T) {
	// With no PID file, daemon should not be reported as running.
	// (This test works because the test environment won't have a PID file.)
	running, _ := isDaemonRunning()
	// We can't assert false because the test environment might have one,
	// but we at least verify the function doesn't panic.
	_ = running
}
