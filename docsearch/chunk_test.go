package docsearch

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		maxLen   int
		expected []chunk
	}{
		{
			name:     "no headings",
			markdown: "Just some plain text content.",
			maxLen:   2000,
			expected: []chunk{
				{heading: "", text: "Just some plain text content."},
			},
		},
		{
			name:     "single heading",
			markdown: "# Introduction\n\nThis is the intro.",
			maxLen:   2000,
			expected: []chunk{
				{heading: "Introduction", text: "Introduction:\nThis is the intro."},
			},
		},
		{
			name:     "multiple headings",
			markdown: "# Section A\n\nContent A.\n\n# Section B\n\nContent B.",
			maxLen:   2000,
			expected: []chunk{
				{heading: "Section A", text: "Section A:\nContent A."},
				{heading: "Section B", text: "Section B:\nContent B."},
			},
		},
		{
			name:     "nested headings",
			markdown: "# Top\n\nTop content.\n\n## Sub\n\nSub content.\n\n### Deep\n\nDeep content.",
			maxLen:   2000,
			expected: []chunk{
				{heading: "Top", text: "Top:\nTop content."},
				{heading: "Top > Sub", text: "Top > Sub:\nSub content."},
				{heading: "Top > Sub > Deep", text: "Top > Sub > Deep:\nDeep content."},
			},
		},
		{
			name:     "heading hierarchy reset",
			markdown: "# A\n\n## A1\n\nA1 content.\n\n# B\n\n## B1\n\nB1 content.",
			maxLen:   2000,
			expected: []chunk{
				{heading: "A > A1", text: "A > A1:\nA1 content."},
				{heading: "B > B1", text: "B > B1:\nB1 content."},
			},
		},
		{
			name:     "empty sections skipped",
			markdown: "# Heading Only\n\n# Content Here\n\nSome text.",
			maxLen:   2000,
			expected: []chunk{
				{heading: "Content Here", text: "Content Here:\nSome text."},
			},
		},
		{
			name:     "content before first heading",
			markdown: "Some preamble text.\n\n# First Section\n\nSection content.",
			maxLen:   2000,
			expected: []chunk{
				{heading: "", text: "Some preamble text."},
				{heading: "First Section", text: "First Section:\nSection content."},
			},
		},
		{
			name:     "markdown stripped in chunks",
			markdown: "# **Bold** Title\n\nSome [link](url) and `code`.",
			maxLen:   2000,
			expected: []chunk{
				{heading: "Bold Title", text: "Bold Title:\nSome link and code."},
			},
		},
		{
			name:     "empty document",
			markdown: "",
			maxLen:   2000,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := chunkMarkdown(tt.markdown, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChunkMarkdownLongSection(t *testing.T) {
	// Create a section longer than maxLen.
	long := strings.Repeat("word ", 100) // 500 chars
	markdown := "# Title\n\n" + long
	chunks := chunkMarkdown(markdown, 100)

	require.True(t, len(chunks) > 1, "should split into multiple chunks")

	// All chunks should have the heading context.
	for _, c := range chunks {
		assert.Equal(t, "Title", c.heading)
	}

	// No chunk should exceed maxLen.
	for _, c := range chunks {
		assert.LessOrEqual(t, len(c.text), 100)
	}
}
