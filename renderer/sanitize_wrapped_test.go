package renderer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sanitizeWrappedLines post-processes a slice of lines produced by a
// word-wrap pass and arranges each line to be self-contained with respect
// to SGR state. These tests exercise the state-tracking paths for each
// attribute (bold, italic, underline, foreground, background, reset) and
// their re-application on subsequent lines.

func TestSanitizeWrappedLines_NoChangeForSingleLine(t *testing.T) {
	in := []string{"just one line"}
	out := sanitizeWrappedLines(in)
	assert.Equal(t, in, out, "single-line input is returned unchanged")
}

func TestSanitizeWrappedLines_EmptyInput(t *testing.T) {
	out := sanitizeWrappedLines(nil)
	assert.Nil(t, out)
}

func TestSanitizeWrappedLines_NoSGRPassesThrough(t *testing.T) {
	in := []string{"line one", "line two", "line three"}
	out := sanitizeWrappedLines(in)
	assert.Equal(t, in, out, "lines without SGR are returned unchanged")
}

// For each attribute, when it's turned on mid-line, the function should
// close it on that line and re-open it on the next line so the next line
// is independently renderable (no ANSI state leak between lines).
func TestSanitizeWrappedLines_BoldCarry(t *testing.T) {
	in := []string{"before \x1b[1mbold text", "continuation"}
	out := sanitizeWrappedLines(in)
	require.Len(t, out, 2)
	// First line should end with a reset.
	assert.True(t, strings.HasSuffix(out[0], "\x1b[0m"),
		"line 0 should end with reset, got %q", out[0])
	// Second line should begin with the bold re-application.
	assert.True(t, strings.HasPrefix(out[1], "\x1b[1m"),
		"line 1 should start with bold re-enable, got %q", out[1])
}

func TestSanitizeWrappedLines_ItalicCarry(t *testing.T) {
	in := []string{"a \x1b[3mitalic", "next"}
	out := sanitizeWrappedLines(in)
	assert.True(t, strings.HasSuffix(out[0], "\x1b[0m"))
	assert.True(t, strings.HasPrefix(out[1], "\x1b[3m"),
		"line 1 should re-enable italic, got %q", out[1])
}

func TestSanitizeWrappedLines_UnderlineCarry(t *testing.T) {
	in := []string{"a \x1b[4munderlined", "next"}
	out := sanitizeWrappedLines(in)
	assert.True(t, strings.HasSuffix(out[0], "\x1b[0m"))
	assert.True(t, strings.HasPrefix(out[1], "\x1b[4m"))
}

func TestSanitizeWrappedLines_CombinedAttributes(t *testing.T) {
	in := []string{"x \x1b[1m\x1b[3m\x1b[4mall-on", "next"}
	out := sanitizeWrappedLines(in)
	require.Len(t, out, 2)
	// Line 1 must re-enable each of bold/italic/underline. Order isn't
	// strictly specified by the function so just check all three are
	// present before the content.
	assert.Contains(t, out[1], "\x1b[1m")
	assert.Contains(t, out[1], "\x1b[3m")
	assert.Contains(t, out[1], "\x1b[4m")
	assert.Contains(t, out[1], "next")
}

func TestSanitizeWrappedLines_OffCodesClearAttributes(t *testing.T) {
	// Bold is turned on, then off — second line should NOT re-enable bold.
	in := []string{"\x1b[1mon\x1b[22moff", "next"}
	out := sanitizeWrappedLines(in)
	assert.False(t, strings.Contains(out[1], "\x1b[1m"),
		"bold was turned off; line 1 should not re-enable it, got %q", out[1])
}

func TestSanitizeWrappedLines_ResetCodeClearsAllState(t *testing.T) {
	in := []string{"\x1b[1m\x1b[3mboth\x1b[0mcleared", "plain next"}
	out := sanitizeWrappedLines(in)
	assert.Equal(t, "plain next", out[1],
		"reset on line 0 should cause line 1 to carry nothing")
}

func TestSanitizeWrappedLines_Foreground16ColorCarry(t *testing.T) {
	// 38;5;n is a 256-color foreground.
	in := []string{"\x1b[38;5;45mcolored text", "next"}
	out := sanitizeWrappedLines(in)
	require.Len(t, out, 2)
	assert.Contains(t, out[1], "\x1b[38;5;45m",
		"256-color foreground should be re-applied on next line, got %q", out[1])
}

func TestSanitizeWrappedLines_ForegroundTrueColorCarry(t *testing.T) {
	// 38;2;r;g;b is a truecolor foreground.
	in := []string{"\x1b[38;2;255;128;0morange", "next"}
	out := sanitizeWrappedLines(in)
	assert.Contains(t, out[1], "\x1b[38;2;255;128;0m",
		"truecolor foreground should be re-applied, got %q", out[1])
}

func TestSanitizeWrappedLines_Foreground39ClearsColor(t *testing.T) {
	in := []string{"\x1b[38;5;45mcolored\x1b[39mplain", "next"}
	out := sanitizeWrappedLines(in)
	assert.NotContains(t, out[1], "\x1b[38",
		"\\x1b[39m should clear fg state; line 1 should not carry it, got %q", out[1])
}

func TestSanitizeWrappedLines_BackgroundCarry(t *testing.T) {
	in := []string{"\x1b[48;5;100mbg-colored", "next"}
	out := sanitizeWrappedLines(in)
	assert.Contains(t, out[1], "\x1b[48;5;100m",
		"background color should be re-applied, got %q", out[1])
}

func TestSanitizeWrappedLines_Background49ClearsColor(t *testing.T) {
	in := []string{"\x1b[48;5;100mbg\x1b[49mplain", "next"}
	out := sanitizeWrappedLines(in)
	assert.NotContains(t, out[1], "\x1b[48",
		"\\x1b[49m should clear bg state, got %q", out[1])
}

func TestSanitizeWrappedLines_MalformedSGRIsIgnored(t *testing.T) {
	// An escape sequence without a closing 'm' should not hang or carry.
	in := []string{"\x1b[1", "next"}
	out := sanitizeWrappedLines(in)
	// Malformed input produces no carry, but the function must not panic
	// and must return both lines.
	assert.Len(t, out, 2)
	assert.Equal(t, "next", out[1])
}

func TestSanitizeWrappedLines_CarryChainsAcrossMultipleLines(t *testing.T) {
	// State carries across more than one subsequent line.
	in := []string{"\x1b[1mbold", "middle", "last"}
	out := sanitizeWrappedLines(in)
	require.Len(t, out, 3)
	assert.True(t, strings.HasPrefix(out[1], "\x1b[1m"), "line 1: %q", out[1])
	assert.True(t, strings.HasSuffix(out[1], "\x1b[0m"), "line 1: %q", out[1])
	assert.True(t, strings.HasPrefix(out[2], "\x1b[1m"), "line 2: %q", out[2])
}
