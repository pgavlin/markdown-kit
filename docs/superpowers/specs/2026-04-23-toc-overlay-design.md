# Table-of-contents overlay for `view.Model`

**Date:** 2026-04-23
**Status:** Design — ready for implementation plan

## Goal

Add a table-of-contents overlay to `view.Model` so users can jump around
large Markdown documents without scrolling or sequential `{`/`}` heading
navigation. The overlay is centered-modal, keyboard driven, and combines a
tree view with a subsequence-filter picker.

## Framing

The project already has a clean split:

- **Document navigation** lives in `view/` (scroll, heading jumps, anchor
  follow, in-document search, span selection).
- **Environment navigation** lives in `cmd/md/` (tab management, open
  file/URL, history, cross-document search, help).

A TOC is document navigation: it points at sections inside the current
document. It therefore belongs in `view/`, as state on `view.Model` and a
binding on `view.KeyMap`, so every embedder (not just the `md` CLI) gets
it.

## User-facing behavior

### Trigger

- Default key: `t` (new `KeyMap.ToggleTOC`, bindable by embedders).
- `t` is reserved in the `view` keymap and unclaimed in `cmd/md`'s
  `readerKeyMap`. The uppercase `T` namespace in `cmd/md` is used for
  environment nav (`T` = open link in new tab, `H` = history, `S` = search
  documents, etc.); the lowercase `t` namespace fits document nav.

### Modes

The overlay has two modes, with an explicit state machine:

```
closed ──(t)──▶ tree ──(/)──▶ filter
  ▲              │               │
  │              │ (esc)         │ (esc)
  └──────────────┴───────────────┘
                    (any: enter → jump & close)
```

- **Tree mode** (default on open): shows every heading in the document,
  indented by level with box-drawing tree edges. `j`/`↓` and `k`/`↑` move
  the cursor; `PgDn`/`PgUp` page; `g`/`G` jump to first/last entry;
  `enter` jumps to the selected heading and closes the overlay; `esc` or
  `t` closes the overlay; `/` switches to filter mode.
- **Filter mode**: shows a flat ranked list of headings whose text matches
  the query as a case-insensitive subsequence. Typing appends to the
  query; `backspace` deletes; arrows, `ctrl+n`/`ctrl+p`, `PgDn`/`PgUp`
  move the cursor within the filtered list; `enter` jumps; `esc` returns
  to tree mode (clearing the query); `ctrl+c` closes the overlay. Each
  line shows the heading text (with matched characters highlighted) plus
  a muted breadcrumb (`Parent › Grandparent`) to disambiguate duplicates
  and anchor the match in the outline.

### Initial cursor position

On open, the cursor selects the entry that corresponds to the heading
enclosing the current scroll position — the same heading `renderGutter`
already shows as the innermost breadcrumb. This makes the overlay feel
like a spatial map of the document. If no enclosing heading exists (the
view is scrolled above the first heading), the cursor starts at entry 0.

### Jumping semantics

Selecting a heading (`enter` in either mode) is equivalent to following
an internal anchor link. Concretely: push the current selection (if any)
onto the navigation backstack, then call `SelectAnchor(entry.Anchor)`.
One `Backspace` afterwards returns to the pre-jump selection. This
matches `FollowLink` (`view/markdown_view.go:1796`) exactly: if there
was no selection when the TOC opened, nothing is pushed, and
`Backspace` behaves as it would before the jump — consistent with
how existing internal-link navigation behaves.

### Empty / error states

- **No headings in document**: `t` does not open the overlay; instead a
  transient `"No headings"` status message is shown via
  `SetStatusMessage`.
- **Document not yet set (`m.index == nil`)**: `t` is a no-op.
- **Terminal smaller than 40 cols × 6 rows**: `t` does not open; status
  message `"Terminal too small for TOC"`. Above that threshold, sizing
  clamps handle small windows gracefully.
- **Filter with no matches**: overlay stays open; body renders
  `"No matches"` in muted style; `enter` is a no-op.

### Interaction with other modes

- `Searching()` (search input active) takes precedence — search swallows
  all printable keys, so `t` cannot open the TOC while search input is
  up. This is the correct precedence.
- Cursor mode and visual mode also take precedence over a new `t`
  binding in the same way they swallow movement keys today.
- `SetText` while the overlay is open dismisses it: the section tree
  belongs to the old document.
- Resize while open is preserved: sizing is re-clamped on the next
  `View()`, and the scroll window is adjusted to keep the cursor
  visible.

## Architecture

### File layout

Two new files in `view/`, plus small edits to existing files:

- `view/toc.go` — state type, key handling, entry building, filter,
  rendering of the TOC body.
- `view/toc_test.go` — unit tests for TOC behavior.
- `view/overlay.go` — `placeOverlay` and `dialog` helpers (ANSI-aware
  centered bordered box).
- `view/overlay_test.go` — unit tests for overlay layout.

Edits:

- `view/markdown_view.go` — add `toc tocState` field on `Model`; route
  keys to `handleTOCKey` when `m.toc.active`; compose overlay on top of
  base output in `View()`; clear `toc` in `Clear()` and the `SetText`
  path; add public `TOCActive() bool`.
- `view/keymap.go` — add `ToggleTOC` binding, default `t`; include in
  `FullHelp`; include in `SetEnabled` bulk toggle.

Rationale for factoring: `markdown_view.go` is already 2700 lines;
putting TOC logic in its own file keeps the diff small and reviewable.
`placeOverlay`/`dialog` are split from TOC code because they're the
first general-purpose overlay primitives in the `view` package and are
likely to be reused (and eventually shared with `cmd/md`'s near-
identical helpers — tracked as a follow-up, out of scope for this
change).

### Data model

```go
// view/toc.go

type tocEntry struct {
    section    *indexer.Section // source of truth from DocumentIndex
    level      int              // 1..6
    text       string           // heading plain text
    // ancestors at each level, used to render tree prefix + filter breadcrumb
    ancestors  []string
    // filter-mode: column positions of matched characters (visible cols,
    // not byte offsets), used to highlight the match
    matchCols  []int
    // tree-mode: true if this is the last sibling at its level, used
    // to pick └── vs ├── for the leading connector
    lastChild  bool
}

type tocMode int

const (
    tocModeTree tocMode = iota
    tocModeFilter
)

type tocState struct {
    active     bool
    mode       tocMode
    query      string
    allEntries []tocEntry // flat, document order; computed on open
    matches    []int      // indices into allEntries (tree: 0..n; filter: subsequence matches)
    cursor     int        // index within matches (0..len(matches)-1)
    scroll     int        // first visible index within matches
}
```

### Control flow

1. **Open** — `handleKey` sees `m.KeyMap.ToggleTOC` matches and
   `!m.toc.active`. If `m.index == nil` or no headings, set status
   message and return. Otherwise:
   - Walk `m.index.TableOfContents()` depth-first. For each section
     (skipping the synthetic root), emit a `tocEntry` with `level`,
     `text = section.Start.(*ast.Heading).Text(m.markdown)`,
     `ancestors` built from the recursion stack, and `lastChild` set
     based on the parent's children ordering.
   - Identify the enclosing heading at `m.lineOffset` by reusing the
     same walk that populates `headingBreadcrumbs`. Set `cursor` to the
     matching entry index, or 0 if none.
   - Set `mode = tocModeTree`, `matches = [0..n)`, `scroll = 0` (adjusted
     to contain `cursor` during render).
   - Flip `active = true`.
2. **Tree mode key dispatch** — `handleTOCKey` handles `j/k/↓/↑`,
   `PgDn/PgUp`, `g/G`, `enter`, `/`, `esc`, `t`, `ctrl+c`. Unhandled
   keys are dropped.
3. **Filter mode key dispatch** — `handleTOCKey` sees `mode ==
   tocModeFilter`: navigation keys move the cursor; printable chars
   append to `query`; `backspace` pops a rune; `enter` jumps; `esc`
   clears the query and returns to `tocModeTree` (the previously
   selected entry is *not* preserved across modes — cursor resets to 0,
   matching fzf-style picker behavior).
4. **Filter execution** — whenever `query` changes:
   - Rebuild `matches` by iterating `allEntries` in order. For each
     entry, run a new helper
     `subsequenceMatchPositions(haystack, needle string) []int` that
     returns the visible column positions of the consumed needle runes
     (left-to-right greedy, case-insensitive), or `nil` if the needle
     is not a subsequence. This subsumes `subsequenceMatch`; we store
     the positions on `tocEntry.matchCols` for highlight rendering and
     use `len(positions) > 0 || len(needle) == 0` to decide inclusion.
   - Reset `cursor = 0`, `scroll = 0`.
5. **Jump** — on `enter`:
   - If `matches` is empty or `cursor` out of range, no-op.
   - Resolve `entry := m.toc.allEntries[m.toc.matches[m.toc.cursor]]`.
   - Replicate `FollowLink`'s push: capture `prevSelection := m.selection`,
     call `m.SelectAnchor(entry.section.Anchor)`, and if it returns
     `true` *and* `prevSelection != nil`, append `prevSelection` to
     `m.backstack`.
   - Reset `m.toc = tocState{}` (dismiss overlay).
6. **Dismiss** — on `esc` in tree mode or `ctrl+c` or `t`: `m.toc = tocState{}`.

### Rendering

`Model.View()` builds the base content + gutter string as today. After
that, if `m.toc.active`:

- Build the TOC body string (see below).
- Compute dialog width: `widestRenderedEntry + 4` (2 cols border + 2
  cols padding), clamped to `[40, viewportW * 3/4]`.
- Compute dialog height: `len(matches) + 4` (2 cols chrome + mode bar +
  filter input line in filter mode), clamped to
  `[6, viewportH * 3/4]`. Content beyond the visible height is scrolled
  via `m.toc.scroll`.
- Render the dialog: title bar showing `TOC` (tree mode) or
  `TOC / filter: <query>` (filter mode) in the bordered box style used
  by `cmd/md/reader.go`'s `renderOverlay` — rounded border, muted
  foreground, padding `(0, 1)`.
- Compose via `placeOverlay(viewportW, viewportH, dialog, base)`.

**Tree-mode body**: for each entry `allEntries[matches[i]]` in the
visible scroll window, emit `<prefix><text>`:

- `<prefix>` is assembled from the `ancestors` chain plus the entry's
  own position:
  - For each ancestor level whose branch still has remaining siblings
    after the current entry, use `│   ` (bar + 3 spaces).
  - For each ancestor level whose branch is exhausted, use `    ` (4
    spaces).
  - For the entry's own level, use `├── ` if not last-child, `└── ` if
    last-child.
  - The root-level entry (level 1) uses no connector, just the text.
- `<text>` is `entry.text`, truncated with `…` if prefix + text exceeds
  the inner width.
- Selected row (cursor position): prepend `> ` and render text + prefix
  in an accent color; otherwise prepend `  ` and render in the
  default/muted scheme.

**Filter-mode body**: flat list. For each visible match, emit
`<text-with-highlights>  <breadcrumb>`:

- `<text-with-highlights>`: iterate runes; at columns in `matchCols`,
  wrap with the same highlight SGR used by in-doc search match
  highlighting (reverse video for the current row, subdued highlight
  otherwise).
- `<breadcrumb>`: `strings.Join(entry.ancestors, " › ")` in muted color.
  Dropped if `text + breadcrumb` would exceed available width (prefer
  preserving the heading text).
- Above the entry list, render the current query: `  filter: <query>_`
  in the dialog's title bar area.

**Deep nesting fallback**: if the computed tree prefix alone is wider
than `innerWidth - 10`, fall back to plain indentation `"  " * level`.
Prevents unreadable dialogs on pathologically nested docs. Not expected
to fire in practice.

### Overlay helpers (`view/overlay.go`)

```go
// dialog wraps content in a rounded-border box with an optional title.
// Border color uses the theme's chroma.Comment foreground.
func (m *Model) dialog(title, body string, innerWidth int) string

// placeOverlay pastes dialog centered over base, ANSI-aware.
// Lines of base outside the dialog rectangle are preserved verbatim;
// lines that intersect the dialog are composed via ansiCut +
// dialog-line + ansiCut of the trailing portion.
func placeOverlay(width, height int, dialog, base string) string
```

Both helpers are private (lowercase). `cmd/md` keeps its copies for
now; a future refactor can promote them to a shared package.

### Public API

```go
// view/markdown_view.go

// TOCActive reports whether the table-of-contents overlay is open.
// Embedders should gate their own key handling the same way they gate
// on Searching(): defer input to the view while the overlay is up.
func (m *Model) TOCActive() bool

// view/keymap.go

type KeyMap struct {
    // ...existing fields...
    ToggleTOC key.Binding
}

// default: key "t", help "toggle table of contents"
// Included in SetEnabled bulk toggle.
// Included in FullHelp next to Search-related bindings.
```

In-overlay keys (`j/k/↓/↑`, `PgDn/PgUp`, `g/G`, `/`, `esc`, `enter`,
printable chars) are *not* part of `KeyMap`. They are fixed inside
`handleTOCKey`, matching the existing pattern where search input keys
are not user-configurable. Easy to promote later.

## Testing

### Unit (`view/toc_test.go`)

All tests build a `Model` with a fixture Markdown that has a known
heading structure (at least 2 levels, ~10 headings, some with
duplicate text to exercise multiple matches):

1. `TOCOpens` — `t` on a doc with headings: `TOCActive()` becomes `true`.
2. `TOCNoHeadings` — `t` on a heading-free doc: `TOCActive()` stays
   `false`; status message set to `"No headings"`.
3. `TOCNoIndex` — `t` before `SetText`: no-op, no panic.
4. `TOCTerminalTooSmall` — `t` on a 30-col viewport: overlay does not
   open; status message set.
5. `TOCPreselectsCurrentHeading` — after scrolling to within section
   `X`, opening TOC: cursor is on the entry for `X`.
6. `TOCTreeNavigation` — exercise `j/k/PgDn/PgUp/g/G`; verify cursor
   stays within `[0, n)` and scroll window keeps cursor visible.
7. `TOCEnterJumpsAndDismisses` — press `enter` on entry 3: view scrolls
   to that heading's position; `TOCActive()` is `false` after;
   backstack now contains the pre-jump location.
8. `TOCBackspaceReturnsAfterJump` — after the above, `Backspace` scrolls
   back to the pre-jump position (within the existing backstack
   semantics).
9. `TOCSlashEntersFilter` — `/` from tree mode: `mode == tocModeFilter`,
   `query == ""`, all entries match.
10. `TOCFilterMatches` — type `"intro"`, verify only entries matching
    the subsequence remain, in document order; verify `matchCols`
    contains the expected column positions.
11. `TOCFilterNoMatches` — type a query that matches nothing: `matches`
    empty; `enter` is a no-op; body renders `"No matches"`.
12. `TOCFilterEscReturnsToTree` — `esc` in filter mode: `mode ==
    tocModeTree`, `query == ""`, `matches` back to full set, cursor
    reset to 0.
13. `TOCEscClosesTree` — `esc` in tree mode: `TOCActive() == false`.
14. `TOCDismissOnSetText` — open TOC, call `SetText(...)`: `TOCActive()
    == false`.
15. `TOCResizePreservesState` — open TOC, call `SetSize(...)`:
    `TOCActive()` stays `true`, cursor preserved, sizing re-clamps.

### Unit (`view/overlay_test.go`)

1. `PlaceOverlayCenters` — even and odd viewport dimensions; verify
   dialog is pixel-centered.
2. `PlaceOverlayOversizedClips` — dialog larger than viewport: output is
   viewport-sized; dialog is anchored to top-left rather than
   overflowing.
3. `PlaceOverlayPreservesBaseANSI` — base contains SGR sequences
   outside the dialog rectangle: those sequences survive composition.
4. `PlaceOverlayEmptyInputs` — empty base and/or empty dialog: no panic.
5. `DialogBorderAndTitle` — `dialog("TOC", body, 30)` renders a rounded
   border with the title in the top-left slot of the border line.

### Golden snapshot (`view/toc_test.go::TOCRenderSnapshot`)

One fixture document with a known heading structure, TOC opened with
cursor on a specific entry. Strip ANSI from the rendered `View()` and
compare against a checked-in expected string. Keeps the visual layout
honest across refactors.

## Non-goals

- **Fuzzy ranking with scores** (start-of-word bonus, contiguous-run
  bonus). Subsequence match is good enough for heading names; can be
  replaced later without changing the public surface.
- **Displaying HTML anchor entries** (`<a id="x">` nodes). TOC is
  headings-only for now. Easy to extend since `DocumentIndex` already
  tracks them.
- **Persistent TOC sidebar** (non-modal). That's a bigger change to the
  render pipeline; not justified by current usage.
- **Configurable in-overlay keys**. Fixed for now, matching search.
- **Sharing `placeOverlay` with `cmd/md`**. Follow-up refactor; keep
  this change contained.

## Migration / backwards-compatibility

- New public API (`TOCActive`, `KeyMap.ToggleTOC`). Additive; no
  breakage.
- New default keybinding `t`. `t` was previously unbound in both
  `view.KeyMap` and `cmd/md`'s `readerKeyMap`; no existing users rely
  on it being free.
- Embedders who had bound `t` externally will now have their binding
  shadowed by the view's handler when the view has focus. Document in
  the release notes; mitigation is to disable
  `KeyMap.ToggleTOC` via `SetEnabled` or rebind.
