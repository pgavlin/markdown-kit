package view

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// metadataState tracks the document-properties overlay shown by ToggleMetadata.
type metadataState struct {
	active bool
}

// MetadataActive reports whether the metadata overlay is open. Embedders
// should gate their own key handling the same way they gate on
// Searching() — defer input to the view while the overlay is up.
func (m *Model) MetadataActive() bool {
	return m.metadata.active
}

// openMetadata activates the overlay if there's anything to show. The
// overlay is purely informational: there's always at least the document
// name and (optionally) the source path; if neither plus frontmatter is
// present the overlay would be empty, so we still open it but render an
// "empty" placeholder so the user gets feedback that the key was
// received.
func (m *Model) openMetadata() {
	if m.width < tocMinWidth || m.height < tocMinHeight {
		m.SetStatusMessage("Terminal too small for metadata")
		return
	}
	m.metadata.active = true
}

// handleMetadataKey routes keys while the metadata overlay is up. The
// overlay is read-only, so only dismissal keys are handled.
func (m *Model) handleMetadataKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "m", "ctrl+c":
		m.metadata = metadataState{}
	}
	return nil
}

// renderMetadataOverlay composes the metadata dialog on top of the base
// content.
func (m *Model) renderMetadataOverlay(base string) string {
	body := m.renderMetadataBody()
	innerWidth := m.metadataInnerWidth(body)
	dialog := m.renderDialog("Metadata", body, innerWidth)
	return placeOverlay(m.width, m.height, dialog, base)
}

// renderMetadataBody produces the inner body of the metadata dialog:
// a labeled "Path" line (if any), a labeled "Name" line (if any), then
// each frontmatter key/value formatted as a two-column table.
func (m *Model) renderMetadataBody() string {
	_, muted := m.tocStyles()
	keyStyle := muted

	type row struct{ key, value string }
	var rows []row

	if p := m.SourcePath(); p != "" {
		rows = append(rows, row{"Path", p})
	}
	if n := m.GetName(); n != "" {
		rows = append(rows, row{"Name", n})
	}

	// Sort frontmatter keys deterministically so the overlay layout is
	// stable across opens.
	if fm := m.Frontmatter(); len(fm) > 0 {
		keys := make([]string, 0, len(fm))
		for k := range fm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			rows = append(rows, row{k, formatMetadataValue(fm[k])})
		}
	}

	if len(rows) == 0 {
		return muted.Render("  No metadata available")
	}

	// Determine column widths.
	keyW := 0
	for _, r := range rows {
		if w := ansi.StringWidth(r.key); w > keyW {
			keyW = w
		}
	}

	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		pad := strings.Repeat(" ", keyW-ansi.StringWidth(r.key))
		b.WriteString(keyStyle.Render(r.key + pad + "  "))
		b.WriteString(r.value)
	}
	return b.String()
}

// metadataInnerWidth returns the inner content width of the dialog,
// sized to body content but clamped within the same bounds the TOC
// overlay uses.
func (m *Model) metadataInnerWidth(body string) int {
	widest := 0
	for _, line := range strings.Split(body, "\n") {
		if w := ansi.StringWidth(line); w > widest {
			widest = w
		}
	}
	maxOuter := m.width * 3 / 4
	if maxOuter < tocMinWidth {
		maxOuter = tocMinWidth
	}
	inner := widest
	if inner+4 > maxOuter {
		inner = maxOuter - 4
	}
	if inner < tocMinWidth-4 {
		inner = tocMinWidth - 4
	}
	return inner
}

// formatMetadataValue renders a single frontmatter value in a single
// line. Slices and maps get a compact summary; deeper nesting is
// truncated rather than expanded so the overlay stays readable.
func formatMetadataValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int, int64, uint64, float64:
		return fmt.Sprintf("%v", t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, formatMetadataValue(e))
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		// Map → "key1=val1, key2=val2" sorted by key for determinism.
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(t))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, formatMetadataValue(t[k])))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", t)
	}
}

// Discourage an "unused import" if lipgloss were dropped later from
// renderMetadataBody. We use it indirectly via tocStyles, so this is a
// no-op safeguard during refactors.
var _ = lipgloss.NewStyle
