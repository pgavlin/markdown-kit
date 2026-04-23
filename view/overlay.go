package view

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	"github.com/charmbracelet/x/ansi"
)

// placeOverlay pastes dialog centered over base, ANSI-aware. Returns the
// composed string with `height` lines, each padded to `width` visible columns
// where the dialog appears.
//
// Lines of base outside the dialog's bounding rectangle are preserved
// verbatim (including ANSI styling). Lines intersecting the dialog are
// rebuilt as `ansiCut(base, 0, startX) + dialogLine + ansiCut(base, endX, width)`.
// If the dialog is wider than the viewport, it is clipped.
// If the dialog is taller than the viewport, trailing dialog lines are dropped.
func placeOverlay(width, height int, dialog, base string) string {
	if width <= 0 || height <= 0 {
		return base
	}

	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}

	if dialog == "" {
		return strings.Join(baseLines, "\n")
	}

	dialogLines := strings.Split(dialog, "\n")
	dh := len(dialogLines)
	dw := 0
	for _, dl := range dialogLines {
		w := ansi.StringWidth(dl)
		if w > dw {
			dw = w
		}
	}

	// Clip dialog to viewport.
	if dw > width {
		dw = width
	}
	if dh > height {
		dh = height
		dialogLines = dialogLines[:dh]
	}

	startX := (width - dw) / 2
	if startX < 0 {
		startX = 0
	}
	startY := (height - dh) / 2
	if startY < 0 {
		startY = 0
	}
	endX := startX + dw

	out := make([]string, height)
	for i := 0; i < height; i++ {
		baseLine := baseLines[i]
		baseW := ansi.StringWidth(baseLine)
		// Pad base line to full width so ansiCut math works.
		if baseW < width {
			baseLine = baseLine + strings.Repeat(" ", width-baseW)
		}

		if i < startY || i >= startY+dh {
			// Outside dialog band: pass through unchanged.
			out[i] = baseLine
			continue
		}

		dl := dialogLines[i-startY]
		// Clip dialog line to dw columns.
		if ansi.StringWidth(dl) > dw {
			dl = ansiTruncate(dl, dw)
		}
		// Pad dialog line to dw with spaces so the right edge is aligned.
		if w := ansi.StringWidth(dl); w < dw {
			dl = dl + strings.Repeat(" ", dw-w)
		}

		left := ansiCut(baseLine, 0, startX)
		right := ansiCut(baseLine, endX, width)
		out[i] = left + dl + right
	}

	return strings.Join(out, "\n")
}

// renderDialog renders body in a rounded-border box with an optional title
// overlaid on the top border line. innerWidth is the visible content width
// (excluding border + padding). Border color is drawn from chroma.Comment in
// the current theme; defaults to no color if theme is nil.
func (m *Model) renderDialog(title, body string, innerWidth int) string {
	if innerWidth < 2 {
		innerWidth = 2
	}

	// Derive border color from theme. Falls back to default when theme is nil.
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(innerWidth)
	if m.theme != nil {
		if c := m.theme.Get(chroma.Comment).Colour; c.IsSet() {
			border = border.BorderForeground(lipgloss.Color(
				fmt.Sprintf("#%02x%02x%02x", c.Red(), c.Green(), c.Blue())))
		}
	}

	dialog := border.Render(body)
	if title == "" {
		return dialog
	}

	// Overlay the title on the top border: replace a portion of the ─'s with
	// " title " starting at column 2 (just past the ╭ corner).
	lines := strings.Split(dialog, "\n")
	if len(lines) == 0 {
		return dialog
	}
	top := lines[0]
	prefix := ansiCut(top, 0, 2)
	titleText := " " + title + " "
	tw := ansi.StringWidth(titleText)
	topW := ansi.StringWidth(top)
	if tw+2 > topW {
		// Title wider than border; skip overlay.
		return dialog
	}
	remainder := ansiCut(top, 2+tw, topW)
	// Render title in muted style if available.
	titleStyle := lipgloss.NewStyle()
	if m.theme != nil {
		if c := m.theme.Get(chroma.Comment).Colour; c.IsSet() {
			titleStyle = titleStyle.Foreground(lipgloss.Color(
				fmt.Sprintf("#%02x%02x%02x", c.Red(), c.Green(), c.Blue())))
		}
	}
	lines[0] = prefix + titleStyle.Render(titleText) + remainder
	return strings.Join(lines, "\n")
}
