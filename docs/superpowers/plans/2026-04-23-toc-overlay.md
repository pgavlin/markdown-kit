# TOC Overlay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a centered-modal table-of-contents overlay to `view.Model` with tree and filter modes, opened by a new lowercase `t` binding.

**Architecture:** New state `tocState` on `Model`, a pair of rendering helpers (`placeOverlay`, `dialog`) private to `view/`, and a new file `view/toc.go` containing the overlay logic. Rendering layers a bordered centered dialog on top of the existing base `View()` output. Navigation calls the existing `SelectAnchor`/backstack primitives so behavior matches internal-link following.

**Tech Stack:** Go 1.24, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2/key`, `github.com/pgavlin/markdown-kit/indexer`, `github.com/charmbracelet/x/ansi`, `github.com/alecthomas/chroma`.

**Spec reference:** `docs/superpowers/specs/2026-04-23-toc-overlay-design.md`

---

## File plan

**Create:**
- `view/toc.go` — `tocState`, `tocEntry`, `tocMode` types; key handling (`handleTOCKey`); entry building (`buildTOCEntries`); filter (`subsequenceMatchPositions`, `rebuildTOCMatches`); rendering (`renderTOCBody`); public accessor (`TOCActive`).
- `view/toc_test.go` — unit + snapshot tests for the TOC.
- `view/overlay.go` — `placeOverlay`, `renderDialog` helpers.
- `view/overlay_test.go` — layout tests.
- `view/testdata/toc_snapshot.txt` — golden fixture for the snapshot test.

**Modify:**
- `view/keymap.go` — add `ToggleTOC` binding, include in `FullHelp` and `SetEnabled`.
- `view/markdown_view.go` — add `toc tocState` field; route keys when active; clear in `Clear()`; compose overlay in `View()`.

Task order below mirrors the dependency chain: keymap/scaffolding → overlay helpers → open/close path → tree nav → filter nav → edge cases → snapshot.

---

### Task 1: Add `ToggleTOC` KeyMap entry

**Files:**
- Modify: `view/keymap.go`
- Test: `view/options_test.go`

The `KeyMap` struct gets a new binding. Default is lowercase `t`, enabled by default. Must be included in `SetEnabled` and `FullHelp`.

- [ ] **Step 1: Write the failing test**

Append to `view/options_test.go`:

```go
func TestDefaultKeyMap_ToggleTOC(t *testing.T) {
	km := DefaultKeyMap()
	require.True(t, km.ToggleTOC.Enabled())
	assert.Contains(t, km.ToggleTOC.Keys(), "t")
}

func TestKeyMap_SetEnabled_IncludesToggleTOC(t *testing.T) {
	km := DefaultKeyMap()
	km.SetEnabled(false)
	assert.False(t, km.ToggleTOC.Enabled())
	km.SetEnabled(true)
	assert.True(t, km.ToggleTOC.Enabled())
}
```

If `options_test.go` lacks `require`/`assert` imports, verify before adding:

```bash
grep -n "require\|assert" view/options_test.go
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./view/ -run 'TestDefaultKeyMap_ToggleTOC|TestKeyMap_SetEnabled_IncludesToggleTOC' -v
```

Expected: compile error (`km.ToggleTOC undefined`).

- [ ] **Step 3: Add the binding field and default**

Edit `view/keymap.go`:

Add to the `KeyMap` struct, right after the `Search*` group at the bottom:

```go
	ToggleTOC key.Binding
```

Add to `DefaultKeyMap()`, after the `ClearSearch` entry, before the closing `}`:

```go
		ToggleTOC: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "toggle table of contents"),
		),
```

Add to the `SetEnabled` bindings slice, at the end:

```go
		&km.ToggleTOC,
```

Append `km.ToggleTOC` to the last row of `FullHelp`:

```go
		{km.Search, km.NextMatch, km.PrevMatch, km.ClearSearch, km.ToggleTOC},
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./view/ -run 'TestDefaultKeyMap_ToggleTOC|TestKeyMap_SetEnabled_IncludesToggleTOC' -v
```

Expected: PASS. Also run the full view package build to make sure nothing else breaks: `go build ./...`.

- [ ] **Step 5: Commit**

```bash
git add view/keymap.go view/options_test.go
git commit -m "Add ToggleTOC binding to view.KeyMap"
```

---

### Task 2: Add `tocState` field + `TOCActive()` accessor + clear paths

**Files:**
- Create: `view/toc.go`
- Modify: `view/markdown_view.go`
- Test: `view/toc_test.go`

Scaffolding: new `toc.go` file with the state types and public accessor. `Clear()` zeroes the state. No key handling yet; no rendering yet.

- [ ] **Step 1: Write the failing test**

Create `view/toc_test.go`:

```go
package view

import (
	"testing"

	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
)

// newTestModelWithTOC builds a Model with a non-empty TOC-shaped document.
func newTestModelWithTOC(t *testing.T) *Model {
	t.Helper()
	md := `# Top

Intro paragraph.

## Section A

Text.

### Subsection A1

Text.

## Section B

Text.
`
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("test.md", md)
	return &m
}

func TestTOC_InitiallyInactive(t *testing.T) {
	m := newTestModelWithTOC(t)
	assert.False(t, m.TOCActive())
}

func TestTOC_ClearResetsState(t *testing.T) {
	m := newTestModelWithTOC(t)
	m.toc.active = true
	m.Clear()
	assert.False(t, m.TOCActive())
	assert.Nil(t, m.toc.allEntries)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./view/ -run TestTOC_ -v
```

Expected: compile error (`m.toc undefined`, `m.TOCActive undefined`).

- [ ] **Step 3: Create `view/toc.go` with state types**

```go
package view

// tocMode distinguishes the tree vs filter view of the overlay.
type tocMode int

const (
	tocModeTree tocMode = iota
	tocModeFilter
)

// tocEntry is one heading in the overlay.
type tocEntry struct {
	anchor    string   // DocumentIndex anchor used for SelectAnchor
	level     int      // 1..6
	text      string   // heading plain text
	ancestors []string // ancestor heading texts (level 1 ... parent)
	lastChild bool     // true if last sibling at its level (for tree edges)
	matchCols []int    // filter-mode: visible column positions of matched chars
}

// tocState is the overlay's runtime state.
type tocState struct {
	active     bool
	mode       tocMode
	query      string
	allEntries []tocEntry
	matches    []int
	cursor     int
	scroll     int
}

// TOCActive reports whether the table-of-contents overlay is open.
// Embedders should gate their own key handling (see Searching()).
func (m *Model) TOCActive() bool {
	return m.toc.active
}
```

- [ ] **Step 4: Add `toc` field to `Model`**

Edit `view/markdown_view.go`, find the `search searchState` field (around line 407) and add on the next line:

```go
	// Table-of-contents overlay state.
	toc tocState
```

Also edit `Clear()` (around line 453): add `m.toc = tocState{}` before the closing brace (the function ends with `m.search = searchState{}`):

```go
	m.search = searchState{}
	m.toc = tocState{}
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./view/ -run TestTOC_ -v
```

Expected: PASS. Verify build: `go build ./...`.

- [ ] **Step 6: Commit**

```bash
git add view/toc.go view/toc_test.go view/markdown_view.go
git commit -m "Scaffold tocState, TOCActive accessor, and Clear path"
```

---

### Task 3: `placeOverlay` helper

**Files:**
- Create: `view/overlay.go`
- Test: `view/overlay_test.go`

ANSI-aware layering that pastes a pre-rendered dialog onto a base viewport string, centered. Uses the existing `ansiCut`/`ansiSlice` helpers.

- [ ] **Step 1: Write the failing tests**

Create `view/overlay_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestPlaceOverlay_ -v
```

Expected: compile error (`placeOverlay undefined`).

- [ ] **Step 3: Implement `placeOverlay`**

Create `view/overlay.go`:

```go
package view

import (
	"strings"

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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestPlaceOverlay_ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add view/overlay.go view/overlay_test.go
git commit -m "Add placeOverlay helper for centered-modal rendering"
```

---

### Task 4: `renderDialog` helper

**Files:**
- Modify: `view/overlay.go`
- Test: `view/overlay_test.go`

Wraps content in a rounded border with a title; uses the theme's `chroma.Comment` foreground for the border color (same derivation as `renderGutter`).

- [ ] **Step 1: Write the failing test**

Append to `view/overlay_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestRenderDialog_ -v
```

Expected: compile error (`m.renderDialog undefined`).

- [ ] **Step 3: Implement `renderDialog`**

Append to `view/overlay.go`:

```go
import (
	// keep existing imports; add these:
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
)
```

(Merge with the existing import block.)

Then append:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestRenderDialog_ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add view/overlay.go view/overlay_test.go
git commit -m "Add renderDialog helper (bordered box with optional title)"
```

---

### Task 5: Build `tocEntry` list from the document index

**Files:**
- Modify: `view/toc.go`
- Test: `view/toc_test.go`

Walks `indexer.Section` tree, producing a flat ordered slice of `tocEntry`. No overlay opens yet — this is pure data building.

- [ ] **Step 1: Write the failing test**

Append to `view/toc_test.go`:

```go
func TestBuildTOCEntries_Structure(t *testing.T) {
	m := newTestModelWithTOC(t)
	entries := m.buildTOCEntries()

	// Expected sections: Top, Section A, Subsection A1, Section B
	require.Len(t, entries, 4)

	assert.Equal(t, "Top", entries[0].text)
	assert.Equal(t, 1, entries[0].level)
	assert.Empty(t, entries[0].ancestors)
	assert.True(t, entries[0].lastChild) // only root-level heading

	assert.Equal(t, "Section A", entries[1].text)
	assert.Equal(t, 2, entries[1].level)
	assert.Equal(t, []string{"Top"}, entries[1].ancestors)
	assert.False(t, entries[1].lastChild) // Section B follows at same level

	assert.Equal(t, "Subsection A1", entries[2].text)
	assert.Equal(t, 3, entries[2].level)
	assert.Equal(t, []string{"Top", "Section A"}, entries[2].ancestors)
	assert.True(t, entries[2].lastChild) // only level-3 under Section A

	assert.Equal(t, "Section B", entries[3].text)
	assert.Equal(t, 2, entries[3].level)
	assert.Equal(t, []string{"Top"}, entries[3].ancestors)
	assert.True(t, entries[3].lastChild)

	// Every entry has a non-empty anchor sourced from the indexer.
	for _, e := range entries {
		assert.NotEmpty(t, e.anchor)
	}
}

func TestBuildTOCEntries_EmptyDocument(t *testing.T) {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("empty.md", "Just a paragraph, no headings.\n")
	entries := m.buildTOCEntries()
	assert.Empty(t, entries)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestBuildTOCEntries_ -v
```

Expected: compile error (`m.buildTOCEntries undefined`).

- [ ] **Step 3: Implement `buildTOCEntries`**

Append to `view/toc.go`:

```go
import (
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/indexer"
)
```

(Merge with any existing import block — if `view/toc.go` has no imports yet, add this block below the `package view` line.)

Append:

```go
// buildTOCEntries walks the document's section tree and returns a flat,
// document-ordered list of entries suitable for rendering.
func (m *Model) buildTOCEntries() []tocEntry {
	if m.index == nil {
		return nil
	}
	root := m.index.TableOfContents()
	if root == nil {
		return nil
	}
	var entries []tocEntry
	var ancestors []string
	visitTOCSections(m.markdown, root.Subsections, ancestors, &entries)
	return entries
}

// visitTOCSections is a recursive helper that appends entries in document
// order, tracking ancestor heading texts along the recursion path.
func visitTOCSections(source []byte, sections []*indexer.Section, ancestors []string, out *[]tocEntry) {
	for i, s := range sections {
		h, ok := s.Start.(*ast.Heading)
		if !ok {
			continue
		}
		text := string(h.Text(source))
		entry := tocEntry{
			anchor:    s.Anchor,
			level:     s.Level,
			text:      text,
			ancestors: append([]string(nil), ancestors...),
			lastChild: i == len(sections)-1,
		}
		*out = append(*out, entry)

		if len(s.Subsections) > 0 {
			visitTOCSections(source, s.Subsections, append(ancestors, text), out)
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestBuildTOCEntries_ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/toc_test.go
git commit -m "Build flat TOC entry list from indexer section tree"
```

---

### Task 6: Open overlay on `t` key; no-op guards; empty-doc status message

**Files:**
- Modify: `view/toc.go`, `view/markdown_view.go`
- Test: `view/toc_test.go`

Wire the key binding into the dispatch. On `t`:
- if `m.index == nil`: no-op
- if no headings: set `statusMessage = "No headings"`
- if viewport too small: set `statusMessage = "Terminal too small for TOC"`
- otherwise: populate `tocState` and set `active = true`.

- [ ] **Step 1: Write the failing tests**

Append to `view/toc_test.go`:

```go
import (
	tea "charm.land/bubbletea/v2"
)
```

(Merge into the existing import block.)

```go
func pressKey(t *testing.T, m *Model, k string) {
	t.Helper()
	updated, _ := m.Update(tea.KeyPressMsg{Text: k})
	*m = updated
}

func TestTOC_OpensOnToggleKey(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	assert.True(t, m.TOCActive())
	assert.Equal(t, tocModeTree, m.toc.mode)
	assert.NotEmpty(t, m.toc.allEntries)
	assert.Len(t, m.toc.matches, len(m.toc.allEntries))
}

func TestTOC_NoHeadingsShowsStatusMessage(t *testing.T) {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("flat.md", "Just a paragraph.\n")
	pressKey(t, m, "t")
	assert.False(t, m.TOCActive())
	assert.Equal(t, "No headings", m.statusMessage)
}

func TestTOC_TerminalTooSmallShowsStatusMessage(t *testing.T) {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(30),
		WithHeight(5),
	)
	m.SetText("test.md", "# H1\n\n## H2\n")
	pressKey(t, m, "t")
	assert.False(t, m.TOCActive())
	assert.Equal(t, "Terminal too small for TOC", m.statusMessage)
}

func TestTOC_NoIndexIsNoOp(t *testing.T) {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithWidth(80),
		WithHeight(25),
	)
	// No SetText -> m.index is nil.
	pressKey(t, m, "t")
	assert.False(t, m.TOCActive())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestTOC_OpensOnToggleKey -v
```

Expected: FAIL (key binding not wired; `m.TOCActive()` remains false).

- [ ] **Step 3: Wire key dispatch**

Edit `view/markdown_view.go`. In `handleKey`, after the `m.search.active` block (around line 1043) and BEFORE the visual/cursor mode blocks, add:

```go
	if m.toc.active {
		return m.handleTOCKey(msg)
	}
```

Then in the main switch (around line 1098, near the `Search` binding), add a case for the new key:

```go
		case key.Matches(msg, m.KeyMap.ToggleTOC):
			m.openTOC()
			return nil
```

- [ ] **Step 4: Implement `openTOC` and a stub `handleTOCKey`**

Append to `view/toc.go`:

```go
const (
	tocMinWidth  = 40
	tocMinHeight = 6
)

// openTOC initializes and activates the overlay. Guards for missing index,
// empty headings, and undersized viewport set a transient status message
// instead of opening.
func (m *Model) openTOC() {
	if m.index == nil {
		return
	}
	if m.width < tocMinWidth || m.height < tocMinHeight {
		m.SetStatusMessage("Terminal too small for TOC")
		return
	}
	entries := m.buildTOCEntries()
	if len(entries) == 0 {
		m.SetStatusMessage("No headings")
		return
	}
	matches := make([]int, len(entries))
	for i := range entries {
		matches[i] = i
	}
	m.toc = tocState{
		active:     true,
		mode:       tocModeTree,
		allEntries: entries,
		matches:    matches,
		cursor:     0,
		scroll:     0,
	}
}

// handleTOCKey routes keys while the overlay is active. Filled in by later
// tasks; for now it handles nothing and returns nil (falling through swallows
// the key while active).
func (m *Model) handleTOCKey(msg tea.KeyPressMsg) tea.Cmd {
	_ = msg
	return nil
}
```

Add the `tea` import to the import block at the top of `view/toc.go`:

```go
tea "charm.land/bubbletea/v2"
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./view/ -run TestTOC_ -v
```

Expected: all four new tests PASS plus the prior ones still pass.

- [ ] **Step 6: Commit**

```bash
git add view/toc.go view/markdown_view.go view/toc_test.go
git commit -m "Open TOC overlay on 't' with no-headings / too-small guards"
```

---

### Task 7: Pre-select enclosing heading on open

**Files:**
- Modify: `view/toc.go`
- Test: `view/toc_test.go`

When the overlay opens, the cursor should land on the heading that encloses the current scroll position.

- [ ] **Step 1: Write the failing test**

Append to `view/toc_test.go`:

```go
func TestTOC_OpenPreselectsEnclosingHeading(t *testing.T) {
	// Build a doc with enough content to scroll; open at a known line.
	md := "# Top\n\n"
	for i := 0; i < 20; i++ {
		md += "filler line\n"
	}
	md += "\n## Target\n\n"
	for i := 0; i < 20; i++ {
		md += "more filler\n"
	}

	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("test.md", md)

	// Scroll well into the "Target" section.
	m.lineOffset = 25
	m.clampOffsets()

	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	// The cursor should point at the "Target" entry.
	require.Greater(t, len(m.toc.matches), m.toc.cursor)
	current := m.toc.allEntries[m.toc.matches[m.toc.cursor]]
	assert.Equal(t, "Target", current.text)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./view/ -run TestTOC_OpenPreselectsEnclosingHeading -v
```

Expected: FAIL — cursor defaults to 0 (the "Top" heading).

- [ ] **Step 3: Implement preselection in `openTOC`**

Edit `view/toc.go`. In `openTOC`, replace the `cursor: 0,` line with a computed cursor:

```go
	cursor := m.findEnclosingTOCEntry(entries)
	m.toc = tocState{
		active:     true,
		mode:       tocModeTree,
		allEntries: entries,
		matches:    matches,
		cursor:     cursor,
		scroll:     0,
	}
```

Append a helper to `view/toc.go`:

```go
// findEnclosingTOCEntry returns the index of the entry whose section covers
// the current scroll position. Falls back to 0 if no heading precedes
// m.lineOffset.
func (m *Model) findEnclosingTOCEntry(entries []tocEntry) int {
	if len(entries) == 0 || m.spanTree == nil || len(m.lines) == 0 {
		return 0
	}
	lineOffset := m.lineOffset
	if lineOffset >= len(m.lines) {
		lineOffset = len(m.lines) - 1
	}
	if lineOffset < 0 {
		return 0
	}
	topOffset := m.lines[lineOffset].start

	// Walk the span tree in document order; track the most recent heading
	// whose start is at or before topOffset.
	var lastHeadingText string
	found := false
	for s := m.spanTree; s != nil; s = s.Next {
		if s.Start > topOffset {
			break
		}
		if h, ok := s.Node.(*ast.Heading); ok {
			lastHeadingText = string(h.Text(m.markdown))
			found = true
		}
	}
	if !found {
		return 0
	}

	// Map heading text back to entry index. Multiple headings may share
	// text; prefer the last occurrence at or before topOffset, so search
	// from the end.
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].text == lastHeadingText {
			return i
		}
	}
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./view/ -run TestTOC_OpenPreselectsEnclosingHeading -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/toc_test.go
git commit -m "Preselect enclosing heading when opening TOC overlay"
```

---

### Task 8: Tree-mode navigation keys

**Files:**
- Modify: `view/toc.go`
- Test: `view/toc_test.go`

Implement `j/↓`, `k/↑`, `PgDn/PgUp`, `g/G`, `esc`, `t`, `ctrl+c`. `enter` and `/` are in later tasks.

- [ ] **Step 1: Write the failing tests**

Append to `view/toc_test.go`:

```go
func TestTOC_TreeNavigation(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	start := m.toc.cursor

	pressKey(t, m, "j")
	assert.Equal(t, start+1, m.toc.cursor)

	pressKey(t, m, "k")
	assert.Equal(t, start, m.toc.cursor)

	// G jumps to last.
	pressKey(t, m, "G")
	assert.Equal(t, len(m.toc.matches)-1, m.toc.cursor)

	// g jumps to first.
	pressKey(t, m, "g")
	assert.Equal(t, 0, m.toc.cursor)

	// j at last stays at last (no wrap).
	pressKey(t, m, "G")
	pressKey(t, m, "j")
	assert.Equal(t, len(m.toc.matches)-1, m.toc.cursor)

	// k at first stays at first.
	pressKey(t, m, "g")
	pressKey(t, m, "k")
	assert.Equal(t, 0, m.toc.cursor)
}

func TestTOC_EscClosesTreeMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	pressKey(t, m, "esc")
	assert.False(t, m.TOCActive())
}

func TestTOC_ToggleKeyClosesTreeMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	pressKey(t, m, "t")
	assert.False(t, m.TOCActive())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestTOC_TreeNavigation -v
```

Expected: FAIL — `j` does not move the cursor.

- [ ] **Step 3: Implement tree-mode key handling**

Replace the stub `handleTOCKey` in `view/toc.go`:

```go
// handleTOCKey routes keys while the overlay is active.
func (m *Model) handleTOCKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.toc.mode == tocModeFilter {
		return m.handleTOCFilterKey(msg)
	}
	return m.handleTOCTreeKey(msg)
}

// handleTOCTreeKey handles keys in tree mode.
func (m *Model) handleTOCTreeKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "t", "ctrl+c":
		m.toc = tocState{}
		return nil
	case "j", "down":
		m.moveTOCCursor(1)
		return nil
	case "k", "up":
		m.moveTOCCursor(-1)
		return nil
	case "pgdown", "ctrl+f":
		m.moveTOCCursor(10)
		return nil
	case "pgup", "ctrl+b":
		m.moveTOCCursor(-10)
		return nil
	case "g", "home":
		m.toc.cursor = 0
		m.toc.scroll = 0
		return nil
	case "G", "end":
		m.toc.cursor = len(m.toc.matches) - 1
		if m.toc.cursor < 0 {
			m.toc.cursor = 0
		}
		return nil
	}
	return nil
}

// handleTOCFilterKey is implemented in a later task; default to tree keys so
// the filter mode can be added incrementally without breaking tree behavior.
func (m *Model) handleTOCFilterKey(msg tea.KeyPressMsg) tea.Cmd {
	return m.handleTOCTreeKey(msg)
}

// moveTOCCursor shifts the cursor by n (positive = down) with clamping.
func (m *Model) moveTOCCursor(n int) {
	if len(m.toc.matches) == 0 {
		m.toc.cursor = 0
		return
	}
	m.toc.cursor += n
	if m.toc.cursor < 0 {
		m.toc.cursor = 0
	}
	if m.toc.cursor >= len(m.toc.matches) {
		m.toc.cursor = len(m.toc.matches) - 1
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestTOC_ -v
```

Expected: PASS for all TOC tests so far.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/toc_test.go
git commit -m "Implement tree-mode navigation keys (j/k/PgDn/PgUp/g/G/esc/t)"
```

---

### Task 9: Enter jumps to selected heading with backstack push

**Files:**
- Modify: `view/toc.go`
- Test: `view/toc_test.go`

On `enter`, jump to the selected heading (via `SelectAnchor`), push the prior selection onto the backstack if non-nil (matching `FollowLink`), and dismiss.

- [ ] **Step 1: Write the failing tests**

Append to `view/toc_test.go`:

```go
func TestTOC_EnterJumpsAndDismisses(t *testing.T) {
	m := newTestModelWithTOC(t)
	// Open TOC and move cursor to Section B (index 3 in the fixture).
	pressKey(t, m, "t")
	for m.toc.allEntries[m.toc.matches[m.toc.cursor]].text != "Section B" {
		if m.toc.cursor == len(m.toc.matches)-1 {
			t.Fatal("could not find Section B entry")
		}
		pressKey(t, m, "j")
	}
	pressKey(t, m, "enter")

	assert.False(t, m.TOCActive())
	require.NotNil(t, m.selection)
	// Selection should correspond to the Section B heading.
	// We don't assert the exact node here — just that a selection exists.
}

func TestTOC_EnterBackstackPreservesPriorSelection(t *testing.T) {
	m := newTestModelWithTOC(t)
	// Establish a prior selection by jumping to Section A first.
	m.SelectAnchor("section-a")
	priorSelection := m.selection
	require.NotNil(t, priorSelection)

	pressKey(t, m, "t")
	// Move cursor to Section B.
	for m.toc.allEntries[m.toc.matches[m.toc.cursor]].text != "Section B" {
		if m.toc.cursor == len(m.toc.matches)-1 {
			t.Fatal("could not find Section B entry")
		}
		pressKey(t, m, "j")
	}
	pressKey(t, m, "enter")

	assert.False(t, m.TOCActive())
	require.Len(t, m.backstack, 1)
	assert.Same(t, priorSelection, m.backstack[0])
}

func TestTOC_EnterWithNoPriorSelectionDoesNotPushBackstack(t *testing.T) {
	m := newTestModelWithTOC(t)
	require.Nil(t, m.selection)
	pressKey(t, m, "t")
	pressKey(t, m, "enter")
	assert.False(t, m.TOCActive())
	assert.Empty(t, m.backstack)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestTOC_Enter -v
```

Expected: FAIL — `enter` is not handled; overlay stays open.

- [ ] **Step 3: Implement Enter handling**

Edit the `handleTOCTreeKey` switch in `view/toc.go`, adding a new case before the default-return:

```go
	case "enter":
		m.jumpToSelectedTOCEntry()
		return nil
```

Also add the same case to `handleTOCFilterKey` (still delegating to `handleTOCTreeKey` for now; the explicit `enter` case ensures it's handled once filter mode is added — but to keep tasks independent, just duplicate the case in both switches when you implement filter mode).

Append to `view/toc.go`:

```go
// jumpToSelectedTOCEntry navigates to the heading at the current cursor,
// pushes the prior selection onto the backstack (matching FollowLink), and
// dismisses the overlay. If no entry is selected (empty matches), it only
// dismisses.
func (m *Model) jumpToSelectedTOCEntry() {
	if len(m.toc.matches) == 0 || m.toc.cursor < 0 || m.toc.cursor >= len(m.toc.matches) {
		m.toc = tocState{}
		return
	}
	entry := m.toc.allEntries[m.toc.matches[m.toc.cursor]]
	prev := m.selection
	if m.SelectAnchor(entry.anchor) && prev != nil {
		m.backstack = append(m.backstack, prev)
	}
	m.toc = tocState{}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestTOC_Enter -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/toc_test.go
git commit -m "Jump to selected heading on Enter with FollowLink backstack semantics"
```

---

### Task 10: Dismiss on `SetText`

**Files:**
- Modify: `view/markdown_view.go`
- Test: `view/toc_test.go`

Already covered by `Clear()` (called at the top of `SetText`), but verify with a test.

- [ ] **Step 1: Write the failing test**

Append to `view/toc_test.go`:

```go
func TestTOC_DismissedOnSetText(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	m.SetText("other.md", "# Other\n")
	assert.False(t, m.TOCActive())
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test ./view/ -run TestTOC_DismissedOnSetText -v
```

Expected: PASS (because `Clear()` already resets `toc`). If it fails, the `m.toc = tocState{}` line from Task 2 is missing; add it.

- [ ] **Step 3: Commit**

```bash
git add view/toc_test.go
git commit -m "Verify TOC dismisses when SetText swaps the document"
```

---

### Task 11: Tree-mode body rendering

**Files:**
- Modify: `view/toc.go`
- Test: `view/toc_test.go`

Render the entries with box-drawing tree edges (`├──`, `└──`, `│`), cursor marker, and truncation. This step produces the body string; the next task composes it into a dialog over `View()`.

- [ ] **Step 1: Write the failing tests**

Append to `view/toc_test.go`:

```go
import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)
```

(Merge into the test file's imports.)

```go
func TestRenderTOCBody_TreeMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	body := m.renderTOCBody(40)

	lines := strings.Split(body, "\n")
	stripped := make([]string, len(lines))
	for i, ln := range lines {
		stripped[i] = ansi.Strip(ln)
	}
	require.GreaterOrEqual(t, len(stripped), 4)

	// Fixture order: Top, Section A, Subsection A1, Section B.
	// Tree-edge expectations:
	//   Top           (no prefix, it's the sole level-1 root)
	//   ├── Section A
	//   │   └── Subsection A1
	//   └── Section B
	assert.Contains(t, stripped[0], "Top")
	assert.Contains(t, stripped[1], "├──")
	assert.Contains(t, stripped[1], "Section A")
	assert.Contains(t, stripped[2], "│")
	assert.Contains(t, stripped[2], "└──")
	assert.Contains(t, stripped[2], "Subsection A1")
	assert.Contains(t, stripped[3], "└──")
	assert.Contains(t, stripped[3], "Section B")

	// The line at the cursor position should have the ">" marker.
	cursorLine := stripped[m.toc.cursor]
	assert.Contains(t, cursorLine, ">")
}

func TestRenderTOCBody_TruncatesLongLabels(t *testing.T) {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("long.md", "# "+strings.Repeat("A", 200)+"\n")
	pressKey(t, m, "t")
	body := m.renderTOCBody(20)
	for _, ln := range strings.Split(body, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(ln), 20)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestRenderTOCBody_ -v
```

Expected: compile error (`m.renderTOCBody undefined`).

- [ ] **Step 3: Implement `renderTOCBody`**

Append to `view/toc.go`:

```go
import (
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)
```

(Merge imports.)

```go
// renderTOCBody produces the body string (no border) for the overlay,
// respecting innerWidth. In tree mode it draws box-drawing tree edges;
// in filter mode (not yet implemented here) it falls back to simple
// indentation.
func (m *Model) renderTOCBody(innerWidth int) string {
	if innerWidth < 4 {
		innerWidth = 4
	}
	var b strings.Builder
	accent, muted := m.tocStyles()

	if len(m.toc.matches) == 0 {
		return muted.Render("  No matches")
	}

	// Determine which match indices are the last sibling at each ancestor
	// depth; we recompute it from allEntries (lastChild is already set).
	// For tree mode we render all matches in order.
	for row, midx := range m.toc.matches {
		entry := m.toc.allEntries[midx]
		cursorMark := "  "
		if row == m.toc.cursor {
			cursorMark = "> "
		}

		prefix := m.tocTreePrefix(midx)
		label := entry.text
		// Truncate label to fit innerWidth - len(cursorMark) - ansi.StringWidth(prefix).
		avail := innerWidth - ansi.StringWidth(cursorMark) - ansi.StringWidth(prefix)
		if avail < 4 {
			// Deep nesting fallback: drop the box-drawing prefix.
			prefix = strings.Repeat("  ", entry.level-1)
			avail = innerWidth - ansi.StringWidth(cursorMark) - ansi.StringWidth(prefix)
			if avail < 1 {
				avail = 1
			}
		}
		if ansi.StringWidth(label) > avail {
			label = ansi.Truncate(label, avail, "…")
		}

		line := cursorMark + prefix + label
		// Pad to innerWidth so the selection highlight spans full width.
		w := ansi.StringWidth(line)
		if w < innerWidth {
			line = line + strings.Repeat(" ", innerWidth-w)
		}
		if row == m.toc.cursor {
			line = accent.Render(line)
		}
		if row > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// tocTreePrefix returns the box-drawing prefix for the entry at allEntries[i].
// Uses lastChild flags and ancestor depth to draw │/├/└ connectors.
func (m *Model) tocTreePrefix(i int) string {
	entry := m.toc.allEntries[i]
	if entry.level <= 1 {
		return ""
	}

	// For each ancestor depth (1..level-1), decide whether to draw "│   "
	// (branch continues) or "    " (branch done). Branch continues at
	// depth d when there is a later entry whose level <= d (meaning a
	// sibling of the ancestor at that depth appears after this entry).
	prefix := ""
	for depth := 1; depth < entry.level-1; depth++ {
		if m.tocBranchContinues(i, depth) {
			prefix += "│   "
		} else {
			prefix += "    "
		}
	}

	// Own connector:
	if entry.lastChild {
		prefix += "└── "
	} else {
		prefix += "├── "
	}
	return prefix
}

// tocBranchContinues reports whether the ancestor at depth `depth` has any
// further sibling after the entry at allEntries[i].
func (m *Model) tocBranchContinues(i, depth int) bool {
	for j := i + 1; j < len(m.toc.allEntries); j++ {
		lvl := m.toc.allEntries[j].level
		if lvl <= depth {
			return lvl == depth
		}
	}
	return false
}

// tocStyles returns (accent, muted) lipgloss styles derived from the theme.
func (m *Model) tocStyles() (lipgloss.Style, lipgloss.Style) {
	accent := lipgloss.NewStyle().Reverse(true)
	muted := lipgloss.NewStyle()
	if m.theme != nil {
		if c := m.theme.Get(chroma.Comment).Colour; c.IsSet() {
			muted = muted.Foreground(lipgloss.Color(
				fmt.Sprintf("#%02x%02x%02x", c.Red(), c.Green(), c.Blue())))
		}
	}
	return accent, muted
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestRenderTOCBody_ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/toc_test.go
git commit -m "Render TOC tree body with box-drawing edges and cursor marker"
```

---

### Task 12: Compose TOC overlay into `View()`

**Files:**
- Modify: `view/markdown_view.go`
- Test: `view/toc_test.go`

When `m.toc.active`, call `renderTOCBody`, wrap in `renderDialog`, and layer via `placeOverlay`.

- [ ] **Step 1: Write the failing test**

Append to `view/toc_test.go`:

```go
func TestTOC_ViewIncludesOverlayWhenActive(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.True(t, m.TOCActive())
	out := m.View()
	// The rendered output should contain a rounded border corner.
	assert.Contains(t, ansi.Strip(out), "╭")
	assert.Contains(t, ansi.Strip(out), "╯")
	// And a known heading.
	assert.Contains(t, ansi.Strip(out), "Section A")
}

func TestTOC_ViewOmitsOverlayWhenInactive(t *testing.T) {
	m := newTestModelWithTOC(t)
	out := m.View()
	assert.NotContains(t, ansi.Strip(out), "╭")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestTOC_View -v
```

Expected: `TestTOC_ViewIncludesOverlayWhenActive` FAILs (no overlay rendering in `View()` yet).

- [ ] **Step 3: Compose overlay in `View()`**

Edit `view/markdown_view.go`. In `View()`, right before `return buf.String()` (around line 1320), add:

```go
	base := buf.String()
	if m.toc.active {
		base = m.renderTOCOverlay(base)
	}
	return base
}
```

And replace `return buf.String()` with the new variable return.

Append to `view/toc.go`:

```go
// renderTOCOverlay composes the TOC dialog on top of the base content.
func (m *Model) renderTOCOverlay(base string) string {
	innerWidth := m.tocInnerWidth()
	body := m.renderTOCBody(innerWidth)

	// In filter mode, prepend a filter-input line above the entry list.
	if m.toc.mode == tocModeFilter {
		_, muted := m.tocStyles()
		prompt := muted.Render("  filter: ") + m.toc.query + "_"
		pad := innerWidth - ansi.StringWidth(ansi.Strip(prompt))
		if pad > 0 {
			prompt = prompt + strings.Repeat(" ", pad)
		}
		body = prompt + "\n" + body
	}

	// Clip body to tocInnerHeight lines.
	maxBody := m.tocInnerHeight()
	lines := strings.Split(body, "\n")
	// Apply scroll window (cursor-centric) for tree mode entry list.
	if m.toc.mode == tocModeTree {
		lines = m.applyTOCScroll(lines, maxBody)
	} else {
		// Filter mode: reserve row 0 for the prompt, scroll the rest.
		if len(lines) > 1 {
			prompt := lines[0]
			rest := m.applyTOCScroll(lines[1:], maxBody-1)
			lines = append([]string{prompt}, rest...)
		}
	}
	body = strings.Join(lines, "\n")

	title := "TOC"
	dialog := m.renderDialog(title, body, innerWidth)
	return placeOverlay(m.width, m.height, dialog, base)
}

// tocInnerWidth computes the dialog's inner content width (excluding border
// + padding), clamped to [tocMinWidth-4, viewportW*3/4 - 4].
func (m *Model) tocInnerWidth() int {
	// Start by sizing to the widest rendered row (approximated by widest
	// label + max tree prefix for current depth).
	widest := 20
	for _, e := range m.toc.allEntries {
		w := ansi.StringWidth(e.text) + 4*e.level + 2 // heuristic
		if w > widest {
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

// tocInnerHeight computes the dialog's available body height, clamped to
// viewportH*3/4 - 2 (title + bottom border).
func (m *Model) tocInnerHeight() int {
	maxOuter := m.height * 3 / 4
	if maxOuter < tocMinHeight {
		maxOuter = tocMinHeight
	}
	return maxOuter - 2
}

// applyTOCScroll returns at most maxBody lines from `lines`, shifted so the
// cursor row is visible. Updates m.toc.scroll.
func (m *Model) applyTOCScroll(lines []string, maxBody int) []string {
	if len(lines) <= maxBody || maxBody <= 0 {
		m.toc.scroll = 0
		return lines
	}
	// Clamp scroll so cursor is visible.
	if m.toc.cursor < m.toc.scroll {
		m.toc.scroll = m.toc.cursor
	}
	if m.toc.cursor >= m.toc.scroll+maxBody {
		m.toc.scroll = m.toc.cursor - maxBody + 1
	}
	if m.toc.scroll < 0 {
		m.toc.scroll = 0
	}
	end := m.toc.scroll + maxBody
	if end > len(lines) {
		end = len(lines)
	}
	return lines[m.toc.scroll:end]
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestTOC_View -v
```

Expected: PASS. Also run the full view tests to catch regressions: `go test ./view/ -v`.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/markdown_view.go view/toc_test.go
git commit -m "Compose TOC dialog into Model.View output"
```

---

### Task 13: `subsequenceMatchPositions` helper

**Files:**
- Modify: `view/toc.go`
- Test: `view/toc_test.go`

New helper: case-insensitive subsequence match that returns the visible column positions of the consumed needle runes. Returns `nil` for a non-match, empty slice for an empty needle (matches everything).

- [ ] **Step 1: Write the failing tests**

Append to `view/toc_test.go`:

```go
func TestSubsequenceMatchPositions_Basic(t *testing.T) {
	pos := subsequenceMatchPositions("Introduction", "intro")
	assert.Equal(t, []int{0, 1, 2, 3, 4}, pos)
}

func TestSubsequenceMatchPositions_Gapped(t *testing.T) {
	// "i" matches at 0, "t" at 2, "d" at 6.
	pos := subsequenceMatchPositions("introduction", "itd")
	assert.Equal(t, []int{0, 2, 6}, pos)
}

func TestSubsequenceMatchPositions_CaseInsensitive(t *testing.T) {
	pos := subsequenceMatchPositions("HELLO", "hlo")
	assert.Equal(t, []int{0, 2, 4}, pos)
}

func TestSubsequenceMatchPositions_NoMatch(t *testing.T) {
	pos := subsequenceMatchPositions("abc", "xyz")
	assert.Nil(t, pos)
}

func TestSubsequenceMatchPositions_EmptyNeedle(t *testing.T) {
	pos := subsequenceMatchPositions("abc", "")
	assert.Equal(t, []int{}, pos)
}

func TestSubsequenceMatchPositions_WideChars(t *testing.T) {
	// Each CJK char has visual width 2; positions are visible column starts.
	pos := subsequenceMatchPositions("a日b", "ab")
	// "a" at col 0, "日" at cols 1-2, "b" at col 3.
	assert.Equal(t, []int{0, 3}, pos)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestSubsequenceMatchPositions_ -v
```

Expected: compile error.

- [ ] **Step 3: Implement the helper**

Append to `view/toc.go`:

```go
import (
	"unicode"
)
```

(Merge imports.)

```go
// subsequenceMatchPositions returns the visible column positions of the
// runes of needle consumed from haystack, case-insensitive, greedy
// left-to-right. Returns an empty slice for an empty needle, nil when the
// needle is not a subsequence of haystack.
func subsequenceMatchPositions(haystack, needle string) []int {
	if len(needle) == 0 {
		return []int{}
	}
	needleRunes := []rune(needle)
	ni := 0
	positions := make([]int, 0, len(needleRunes))
	col := 0
	for _, r := range haystack {
		w := ansi.StringWidth(string(r))
		if ni < len(needleRunes) &&
			unicode.ToLower(r) == unicode.ToLower(needleRunes[ni]) {
			positions = append(positions, col)
			ni++
		}
		col += w
	}
	if ni < len(needleRunes) {
		return nil
	}
	return positions
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestSubsequenceMatchPositions_ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/toc_test.go
git commit -m "Add subsequenceMatchPositions (fuzzy match with char positions)"
```

---

### Task 14: Filter mode — enter filter, accept input, rebuild matches

**Files:**
- Modify: `view/toc.go`
- Test: `view/toc_test.go`

`/` in tree mode switches to filter mode; printable chars append to `query`; `backspace` pops; query changes rebuild `matches`.

- [ ] **Step 1: Write the failing tests**

Append to `view/toc_test.go`:

```go
func TestTOC_SlashEntersFilterMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	require.Equal(t, tocModeTree, m.toc.mode)
	pressKey(t, m, "/")
	assert.Equal(t, tocModeFilter, m.toc.mode)
	assert.Equal(t, "", m.toc.query)
	assert.Len(t, m.toc.matches, len(m.toc.allEntries))
}

func TestTOC_FilterMatches(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "b")
	// Subsequence "sb" matches "Section B" but not "Section A" or "Subsection A1".
	require.Len(t, m.toc.matches, 1)
	assert.Equal(t, "Section B", m.toc.allEntries[m.toc.matches[0]].text)
}

func TestTOC_FilterBackspace(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "b")
	pressKey(t, m, "backspace")
	// Only "s" left: matches any entry with 's'.
	assert.Greater(t, len(m.toc.matches), 1)
	assert.Equal(t, "s", m.toc.query)
}

func TestTOC_FilterNoMatches(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "z")
	pressKey(t, m, "z")
	pressKey(t, m, "z")
	assert.Empty(t, m.toc.matches)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestTOC_Filter -v
```

Expected: FAIL — `/` not handled; filter-mode key handling missing.

- [ ] **Step 3: Handle `/` in tree mode and implement filter key handling**

Edit `view/toc.go`. Add to the `handleTOCTreeKey` switch (alongside the other cases):

```go
	case "/":
		m.toc.mode = tocModeFilter
		m.toc.query = ""
		m.rebuildTOCMatches()
		return nil
```

Replace the `handleTOCFilterKey` stub:

```go
// handleTOCFilterKey handles keys in filter mode.
func (m *Model) handleTOCFilterKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		// Return to tree mode and clear filter.
		m.toc.mode = tocModeTree
		m.toc.query = ""
		m.rebuildTOCMatches()
		return nil
	case "ctrl+c":
		m.toc = tocState{}
		return nil
	case "enter":
		m.jumpToSelectedTOCEntry()
		return nil
	case "up", "ctrl+p":
		m.moveTOCCursor(-1)
		return nil
	case "down", "ctrl+n":
		m.moveTOCCursor(1)
		return nil
	case "pgup":
		m.moveTOCCursor(-10)
		return nil
	case "pgdown":
		m.moveTOCCursor(10)
		return nil
	case "backspace":
		if len(m.toc.query) > 0 {
			// Drop last rune.
			r := []rune(m.toc.query)
			m.toc.query = string(r[:len(r)-1])
			m.rebuildTOCMatches()
		}
		return nil
	}
	// Accept printable input.
	if msg.Text != "" {
		m.toc.query += msg.Text
		m.rebuildTOCMatches()
	}
	return nil
}

// rebuildTOCMatches recomputes matches for the current query. When the query
// is empty, all entries match (in tree mode that's just the full list; in
// filter mode, that's still "match everything"). Resets cursor and scroll.
func (m *Model) rebuildTOCMatches() {
	if m.toc.query == "" {
		matches := make([]int, len(m.toc.allEntries))
		for i := range m.toc.allEntries {
			matches[i] = i
			m.toc.allEntries[i].matchCols = nil
		}
		m.toc.matches = matches
		m.toc.cursor = 0
		m.toc.scroll = 0
		return
	}
	matches := m.toc.matches[:0] // reuse capacity
	for i := range m.toc.allEntries {
		pos := subsequenceMatchPositions(m.toc.allEntries[i].text, m.toc.query)
		if pos == nil {
			m.toc.allEntries[i].matchCols = nil
			continue
		}
		m.toc.allEntries[i].matchCols = pos
		matches = append(matches, i)
	}
	m.toc.matches = matches
	m.toc.cursor = 0
	m.toc.scroll = 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestTOC_Filter -v
```

Expected: PASS. Also re-run the tree-mode tests to confirm they still pass: `go test ./view/ -run TestTOC_ -v`.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/toc_test.go
git commit -m "Add filter mode: '/' enters, typing/backspace edit query, esc returns"
```

---

### Task 15: Filter-mode body rendering with match highlights and breadcrumbs

**Files:**
- Modify: `view/toc.go`
- Test: `view/toc_test.go`

In filter mode, `renderTOCBody` shows a flat list of matches, highlights matched characters, and appends a muted breadcrumb.

- [ ] **Step 1: Write the failing tests**

Append to `view/toc_test.go`:

```go
func TestRenderTOCBody_FilterMode(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "b")
	body := m.renderTOCBody(50)
	stripped := ansi.Strip(body)

	// Flat list: only Section B matches.
	require.Equal(t, 1, strings.Count(stripped, "\n")+1,
		"expected single-line filter result, got:\n%s", stripped)
	assert.Contains(t, stripped, "Section B")
	// Breadcrumb appended (matched entry's parent is "Top").
	assert.Contains(t, stripped, "Top")
}

func TestRenderTOCBody_FilterModeNoMatches(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "z")
	pressKey(t, m, "z")
	pressKey(t, m, "z")
	body := m.renderTOCBody(30)
	assert.Contains(t, ansi.Strip(body), "No matches")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./view/ -run TestRenderTOCBody_Filter -v
```

Expected: FAIL — renderTOCBody treats filter mode as tree mode.

- [ ] **Step 3: Implement filter-mode rendering**

Edit `view/toc.go`. At the top of `renderTOCBody`, split by mode:

```go
func (m *Model) renderTOCBody(innerWidth int) string {
	if innerWidth < 4 {
		innerWidth = 4
	}
	if len(m.toc.matches) == 0 {
		_, muted := m.tocStyles()
		return muted.Render("  No matches")
	}
	if m.toc.mode == tocModeFilter {
		return m.renderTOCFilterBody(innerWidth)
	}
	return m.renderTOCTreeBody(innerWidth)
}
```

Rename the existing body logic from `renderTOCBody` to `renderTOCTreeBody` (move everything after the early returns into the new function; keep the same implementation).

Append a new `renderTOCFilterBody`:

```go
// renderTOCFilterBody renders a flat list of filtered matches with matched
// characters highlighted and a muted ancestor breadcrumb suffix.
func (m *Model) renderTOCFilterBody(innerWidth int) string {
	var b strings.Builder
	accent, muted := m.tocStyles()

	for row, midx := range m.toc.matches {
		entry := m.toc.allEntries[midx]
		cursorMark := "  "
		if row == m.toc.cursor {
			cursorMark = "> "
		}

		label := highlightMatch(entry.text, entry.matchCols)
		labelW := ansi.StringWidth(entry.text)

		var crumb string
		if len(entry.ancestors) > 0 {
			crumb = "  " + strings.Join(entry.ancestors, " › ")
		}
		crumbW := ansi.StringWidth(crumb)

		avail := innerWidth - ansi.StringWidth(cursorMark)
		// Prefer showing the label in full; drop the breadcrumb if needed.
		if labelW+crumbW > avail {
			crumb = ""
			crumbW = 0
			if labelW > avail {
				label = ansi.Truncate(entry.text, avail, "…")
				labelW = ansi.StringWidth(label)
			}
		}
		pad := avail - labelW - crumbW
		if pad < 0 {
			pad = 0
		}

		line := cursorMark + label + strings.Repeat(" ", pad) + muted.Render(crumb)
		if row == m.toc.cursor {
			line = accent.Render(line)
		}
		if row > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// highlightMatch returns s with matched-character positions rendered in
// reverse-video SGR. Positions are in visible-column units, matching the
// output of subsequenceMatchPositions.
func highlightMatch(s string, positions []int) string {
	if len(positions) == 0 {
		return s
	}
	set := make(map[int]bool, len(positions))
	for _, p := range positions {
		set[p] = true
	}
	var out strings.Builder
	col := 0
	for _, r := range s {
		w := ansi.StringWidth(string(r))
		if set[col] {
			out.WriteString("\033[7m")
			out.WriteRune(r)
			out.WriteString("\033[27m")
		} else {
			out.WriteRune(r)
		}
		col += w
	}
	return out.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./view/ -run TestRenderTOCBody_Filter -v
```

Expected: PASS. Re-run full view suite: `go test ./view/ -v`.

- [ ] **Step 5: Commit**

```bash
git add view/toc.go view/toc_test.go
git commit -m "Render filter-mode body with match highlights and breadcrumbs"
```

---

### Task 16: Filter-mode esc returns to tree mode; enter in filter jumps

**Files:**
- Test: `view/toc_test.go`

The key handling was implemented in Task 14; this task just asserts the full round-trip.

- [ ] **Step 1: Write the tests**

Append to `view/toc_test.go`:

```go
func TestTOC_FilterEscReturnsToTree(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "esc")
	assert.Equal(t, tocModeTree, m.toc.mode)
	assert.Equal(t, "", m.toc.query)
	assert.Len(t, m.toc.matches, len(m.toc.allEntries))
}

func TestTOC_FilterEnterJumps(t *testing.T) {
	m := newTestModelWithTOC(t)
	pressKey(t, m, "t")
	pressKey(t, m, "/")
	pressKey(t, m, "s")
	pressKey(t, m, "b")
	require.Len(t, m.toc.matches, 1)
	pressKey(t, m, "enter")
	assert.False(t, m.TOCActive())
	require.NotNil(t, m.selection)
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
go test ./view/ -run TestTOC_Filter -v
```

Expected: PASS. (Task 14 already handled these keys.)

- [ ] **Step 3: Commit**

```bash
git add view/toc_test.go
git commit -m "Verify filter-mode esc/enter transitions"
```

---

### Task 17: Golden-style rendering snapshot

**Files:**
- Create: `view/testdata/toc_snapshot.txt`
- Modify: `view/toc_test.go`

Pin the visual layout by comparing ANSI-stripped output against a committed fixture. If the fixture doesn't exist yet, create it from the first test run after manually verifying the output is sensible.

- [ ] **Step 1: Write the test (no fixture yet)**

Append to `view/toc_test.go`:

```go
import (
	"os"
	"path/filepath"
)
```

(Merge imports.)

```go
func TestTOC_RenderSnapshot(t *testing.T) {
	const md = `# Overview

Intro text.

## Getting Started

Text.

### Installation

Text.

### Configuration

Text.

## Usage

Text.

## Reference

Text.
`
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(60),
		WithHeight(18),
	)
	m.SetText("snapshot.md", md)
	pressKey(t, m, "t")

	got := ansi.Strip(m.View())

	fixturePath := filepath.Join("testdata", "toc_snapshot.txt")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
			require.NoError(t, os.MkdirAll("testdata", 0o755))
			require.NoError(t, os.WriteFile(fixturePath, []byte(got), 0o644))
			t.Skip("wrote new snapshot; rerun without UPDATE_SNAPSHOTS")
		}
		t.Fatalf("missing fixture %s; run with UPDATE_SNAPSHOTS=1 to create", fixturePath)
	}

	want, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}
```

- [ ] **Step 2: Generate the snapshot**

```bash
UPDATE_SNAPSHOTS=1 go test ./view/ -run TestTOC_RenderSnapshot -v
```

Expected: the test writes `view/testdata/toc_snapshot.txt` and skips.

- [ ] **Step 3: Inspect the snapshot**

```bash
cat view/testdata/toc_snapshot.txt
```

The output should show a bordered dialog centered in a 60x18 viewport containing headings with tree edges. If it looks wrong, fix the bug (not the snapshot) and regenerate.

- [ ] **Step 4: Run the snapshot test without the env var**

```bash
go test ./view/ -run TestTOC_RenderSnapshot -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add view/toc_test.go view/testdata/toc_snapshot.txt
git commit -m "Pin TOC overlay rendering with golden snapshot test"
```

---

### Task 18: Full-suite regression check

**Files:**
- (no code changes)

Run all tests in the repo, including the ones with build tags, to make sure nothing upstream broke.

- [ ] **Step 1: Run the view test suite**

```bash
go test ./view/ -v
```

Expected: all tests PASS.

- [ ] **Step 2: Run the whole module test suite**

```bash
go test ./...
```

Expected: all tests PASS. (Note: `./docsearch/` tests require `-tags sqlite_fts5`; skip or run separately.)

- [ ] **Step 3: Verify the build**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 4: `go vet`**

```bash
go vet ./...
```

Expected: no warnings.

- [ ] **Step 5: No commit needed (verification step only)**

---

## Self-review

### Spec coverage map

| Spec requirement | Task |
|---|---|
| Default key `t` on view.KeyMap | Task 1 |
| `TOCActive()` public accessor | Task 2 |
| Dismiss on Clear / SetText | Task 2, Task 10 |
| Centered modal rendering (placeOverlay) | Task 3 |
| Bordered dialog with title | Task 4 |
| Flat entry list from indexer | Task 5 |
| No-headings guard + status message | Task 6 |
| Terminal-too-small guard | Task 6 |
| No-index guard | Task 6 |
| Pre-select enclosing heading | Task 7 |
| Tree navigation (j/k/PgUp/PgDn/g/G) | Task 8 |
| Esc / `t` / Ctrl+C dismiss in tree | Task 8 |
| Enter jumps via SelectAnchor | Task 9 |
| FollowLink-equivalent backstack push | Task 9 |
| Box-drawing tree rendering | Task 11 |
| Deep-nesting fallback | Task 11 |
| View() overlay composition | Task 12 |
| Modal sizing (width*3/4 × height*3/4) | Task 12 |
| Cursor-centric scroll within dialog | Task 12 |
| subsequenceMatchPositions | Task 13 |
| `/` enters filter, typing/backspace | Task 14 |
| Filter match rebuild | Task 14 |
| Filter esc → tree, ctrl+c → dismiss | Task 14 |
| Filter-mode body rendering + breadcrumbs | Task 15 |
| Match highlight | Task 15 |
| Filter enter jumps | Task 16 |
| Rendering snapshot | Task 17 |
| Regression check | Task 18 |

### Placeholder scan

All code steps contain complete code. Two inline "fix notes" in Task 3 and Task 14 call out a typo in the illustrative test and implementation code respectively — the corrected form immediately follows. Engineer should apply only the corrected form.

### Type consistency

- `tocEntry.anchor` used by Task 9 (`jumpToSelectedTOCEntry`) — defined in Task 2.
- `tocEntry.matchCols` set by Task 14 (`rebuildTOCMatches`) and read by Task 15 (`renderTOCFilterBody` via `highlightMatch`) — defined in Task 2.
- `tocEntry.lastChild` set by Task 5 (`visitTOCSections`) and read by Task 11 (`tocTreePrefix`) — defined in Task 2.
- `tocState.{active,mode,query,allEntries,matches,cursor,scroll}` — all defined in Task 2, used consistently through Tasks 6–17.
- `subsequenceMatchPositions` signature (`string, string) []int`) consistent between Task 13 (definition) and Task 14 (use).
- `openTOC`, `handleTOCKey`, `handleTOCTreeKey`, `handleTOCFilterKey`, `moveTOCCursor`, `jumpToSelectedTOCEntry`, `rebuildTOCMatches`, `buildTOCEntries`, `findEnclosingTOCEntry`, `renderTOCBody`, `renderTOCTreeBody`, `renderTOCFilterBody`, `renderTOCOverlay`, `tocInnerWidth`, `tocInnerHeight`, `applyTOCScroll`, `tocTreePrefix`, `tocBranchContinues`, `tocStyles`, `highlightMatch`, `placeOverlay`, `renderDialog` — all cross-referenced consistently.
