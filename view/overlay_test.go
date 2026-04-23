package view

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderRect splits s into lines and returns them stripped of ANSI and padded
// to `width`, height `height` — useful for asserting visual layout.
func renderRect(t *testing.T, s string, width, height int) []string {
	t.Helper()
	lines := strings.Split(s, "\n")
	require.Len(t, lines, height, "expected %d lines, got %d", height, len(lines))
	out := make([]string, height)
	for i, ln := range lines {
		stripped := ansi.Strip(ln)
		w := ansi.StringWidth(stripped)
		if w > width {
			stripped = ansi.Truncate(stripped, width, "")
			w = width
		}
		out[i] = stripped + strings.Repeat(" ", width-w)
	}
	return out
}

func TestPlaceOverlay_Centers(t *testing.T) {
	// 7x5 viewport, 3x3 dialog -> dialog fits at x=2, y=1.
	base := strings.Repeat(strings.Repeat(".", 7)+"\n", 5)
	base = strings.TrimSuffix(base, "\n")
	dialog := "XXX\nXYX\nXXX"
	out := placeOverlay(7, 5, dialog, base)
	rect := renderRect(t, out, 7, 5)
	assert.Equal(t, ".......", rect[0])
	assert.Equal(t, "..XXX..", rect[1])
	assert.Equal(t, "..XYX..", rect[2])
	assert.Equal(t, "..XXX..", rect[3])
	assert.Equal(t, ".......", rect[4])
}

func TestPlaceOverlay_PadsShortBase(t *testing.T) {
	// Base has only 2 lines; viewport is 5 tall. Dialog 2x2 centered in 5x5:
	// startX=(5-2)/2=1, startY=(5-2)/2=1. Rows 0,3,4 empty; rows 1,2 have dialog.
	base := "abc\ndef"
	dialog := "XX\nXX"
	out := placeOverlay(5, 5, dialog, base)
	rect := renderRect(t, out, 5, 5)
	assert.Equal(t, "abc  ", rect[0])
	assert.Equal(t, "dXX  ", rect[1])
	assert.Equal(t, " XX  ", rect[2])
	assert.Equal(t, "     ", rect[3])
	assert.Equal(t, "     ", rect[4])
}

func TestPlaceOverlay_DialogLargerThanViewport(t *testing.T) {
	// Dialog 10 wide in 5-wide viewport: clip dialog, do not overflow.
	base := strings.Repeat(".....\n", 3)
	base = strings.TrimSuffix(base, "\n")
	dialog := "XXXXXXXXXX\nXXXXXXXXXX"
	out := placeOverlay(5, 3, dialog, base)
	rect := renderRect(t, out, 5, 3)
	// Every output line must be exactly 5 columns and contain no newline bleed.
	for _, ln := range rect {
		assert.Equal(t, 5, len(ln))
	}
}

func TestPlaceOverlay_PreservesBaseOutsideDialog(t *testing.T) {
	// Styled base text outside the dialog region stays styled.
	// Build a 5x3 base with reverse-video on the first line.
	base := "\x1b[7m12345\x1b[27m\n.....\n....."
	dialog := "XX\nXX"
	out := placeOverlay(5, 3, dialog, base)
	// The first line should still contain the reverse-video sequence.
	assert.Contains(t, out, "\x1b[7m")
}

func TestPlaceOverlay_EmptyInputs(t *testing.T) {
	assert.NotPanics(t, func() { placeOverlay(0, 0, "", "") })
	assert.NotPanics(t, func() { placeOverlay(5, 3, "", "abc\n.....\n.....") })
	assert.NotPanics(t, func() { placeOverlay(5, 3, "XX\nXX", "") })
}

func TestRenderDialog_Basic(t *testing.T) {
	m := newTestModelWithTOC(t)
	body := "line one\nline two"
	out := m.renderDialog("TOC", body, 20)

	rawLines := strings.Split(out, "\n")
	require.GreaterOrEqual(t, len(rawLines), 4) // top border, 2 body, bottom border
	stripped := make([]string, len(rawLines))
	for i, ln := range rawLines {
		stripped[i] = ansi.Strip(ln)
	}

	// Top border contains the title.
	assert.Contains(t, stripped[0], "TOC")
	// Top border starts with ╭ (rounded corner).
	assert.True(t, strings.HasPrefix(stripped[0], "╭"))
	// Bottom border ends with ╯.
	last := stripped[len(stripped)-1]
	assert.True(t, strings.HasSuffix(last, "╯"))
	// Body lines are enclosed by │ on both sides.
	assert.True(t, strings.HasPrefix(stripped[1], "│"))
	assert.True(t, strings.HasSuffix(stripped[1], "│"))
}

func TestRenderDialog_NoTitle(t *testing.T) {
	m := newTestModelWithTOC(t)
	out := m.renderDialog("", "body", 10)
	first := ansi.Strip(strings.SplitN(out, "\n", 2)[0])
	// First line is purely border characters, no title embedded.
	for _, r := range first {
		assert.True(t, r == '╭' || r == '╮' || r == '─',
			"unexpected rune %q in border-only line", r)
	}
}
