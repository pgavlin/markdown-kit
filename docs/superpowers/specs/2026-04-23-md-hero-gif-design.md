# `cmd/md` Hero GIF — Design

**Date:** 2026-04-23
**Goal:** Add a hero demo GIF for `cmd/md` to the top-level `README.md`, produced via a committed, reproducible VHS tape.

## Motivation

The README currently describes `md` in prose but gives no visual sense of
the reader. A short hero GIF lets visitors see the interactive experience —
tabs, navigation, TOC, search, code-block copy, source toggle — within the
first screen of the README.

The tape must be **reproducible**: future contributors need to be able to
re-render the GIF after UI changes without hand-editing a screen recording.

## Artifacts

All new files live in a new `demo/` directory at the repo root:

| Path | Purpose |
|------|---------|
| `demo/demo.tape` | VHS script that drives `md` and writes `md.gif` |
| `demo/hello.md` | Primary demo document |
| `demo/code.md` | Secondary demo document (opened in a new tab) |
| `demo/md.gif` | Rendered output; committed |
| `README.md` | Updated to display `demo/md.gif` near the top |

`demo/` is chosen over `assets/` because the tape and inputs belong
together — the GIF is output, not a source asset.

## Demo documents

The two Markdown documents are hand-sized so item navigation (`]` / `[`)
lands on predictable elements, keeping the tape robust against small
content changes. They also give the renderer enough material to show off
(heading hierarchy, inline code, fenced code block, lists, links).

`hello.md` contains, in this order:

1. A top-level heading
2. A short intro paragraph with inline `code`
3. A subheading, a short paragraph, an internal anchor link
4. A subheading, a fenced code block (the target for the copy demo)
5. A subheading, a paragraph, a relative link to `./code.md`

`code.md` is smaller — a single heading plus a paragraph and a small code
block — just enough to look distinct in the tab bar.

## Tape settings

- **Dimensions:** 1200×720 — fits a GitHub README without horizontal clip
- **Font size:** 14
- **Theme:** Dracula (built into VHS)
- **Padding:** 20
- **Typing speed:** ~50ms
- **Shell environment:** `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME`
  are set to `/tmp/md-demo/...` so the recording does not depend on or
  pollute the host's `md` config, search index, or cache

## Storyboard

Target length ~50s. Keys match `md`'s defaults.

1. **Open** — `md hello.md`
2. **Basic nav** — a few `j` scrolls; `}` twice to show next-heading; `{` once for prev-heading
3. **TOC** — `t` to open the TOC overlay, `↓` a few times, `Enter` to jump
4. **Item nav** — `]` to cycle through navigable items (link / code / heading)
5. **Search** — `/code`, `Enter`, `n` to advance, `Esc` to clear
6. **Copy code block** — continue `]` until a code block is selected, then `y`
7. **View source** — `Ctrl+U` to switch to source, `Ctrl+U` to return
8. **Multi-doc**
   - `Ctrl+T` to open the file picker in a new tab
   - Type `code`, `Enter` — `code.md` opens as a new tab
   - `Shift+Tab` to cycle back to `hello.md`
   - `]`/`[` to the link to `code.md`, then `T` (open link in new tab)
9. **Quit** — `q`

## What's out of scope

- Document search picker (`S`) and semantic search
- History picker (`H`)
- Theme showcase (the tape uses whatever `md`'s default resolves to in the
  scratch config)
- Gist export (`Ctrl+G`) — relies on `gh`

These are intentionally omitted to keep the gif short and the tape
reproducible without external auth or network.

## Risks and mitigations

- **Item-nav order:** `hello.md` must be structured so `]` lands on the
  expected elements in order. Validated by rendering and watching frame
  output; `hello.md` adjusted if counts drift.
- **System clipboard inside VHS's embedded terminal:** OSC 52 may not
  propagate to the host clipboard. That's fine — the visual story is the
  highlighted selection and the "Copied" toast (if present), not actual
  clipboard state. No assertion is needed.
- **Output size:** target < 5 MB. If the first render is larger, lower
  framerate or dimensions before tuning content.
- **Bubble Tea sizing:** first render may expose mismatches between the
  tape's declared `Set Width/Height` and `md`'s rendering column. Adjust
  tape dimensions rather than `md`'s internal defaults.

## Validation

1. `vhs demo/demo.tape` renders without error
2. `demo/md.gif` exists, is under ~5 MB, and visually covers all beats
   listed in the storyboard
3. The rendered GIF loads in GitHub's Markdown preview of the README
