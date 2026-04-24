# `cmd/md` Hero GIF Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a reproducible hero GIF for `cmd/md` via a committed VHS tape, and display it near the top of `README.md`.

**Architecture:** A new top-level `demo/` directory holds a VHS tape (`demo.tape`), two small Markdown fixtures (`hello.md`, `code.md`), and the rendered `md.gif`. The tape isolates `md`'s state into `/tmp/md-demo` via `XDG_*` env vars so the render is host-independent. Readme is updated once the gif looks right.

**Tech Stack:** VHS (v0.11.0, already installed at `/Users/pgavlin/go/bin/vhs`), `cmd/md` (built from the current working tree), bash.

Spec: `docs/superpowers/specs/2026-04-23-md-hero-gif-design.md`.

---

## Notes on iteration

Demo gifs don't map cleanly onto TDD — there is no failing test to write, and the "success" signal is visual. The plan structures work as: build the inputs, render, inspect, adjust. Each task has an explicit inspection step.

The tape depends on a working `md` binary. Tasks use `go run ./cmd/md` rather than a `go install`ed binary so the demo always uses the current working tree.

---

### Task 1: Create demo directory and Markdown fixtures

**Files:**
- Create: `demo/hello.md`
- Create: `demo/code.md`

The content is hand-sized for predictable item navigation (`]`). `hello.md`
has items in this order: heading, internal anchor link, heading, link to
`code.md`, code block, heading.

- [ ] **Step 1: Create `demo/hello.md`**

```markdown
# Welcome to md

A fast terminal Markdown reader. Scroll with `j`/`k`, jump headings with
`{`/`}`, and cycle navigable items (links, code blocks, anchors) with
`[`/`]`. Press `?` to see every binding.

## Why md

`md` renders Markdown with full styling, syntax highlighting, and image
support right in your terminal. Jump to [the example](#example) below to
see a code block you can copy with `y`.

## Reading multiple files

Open documents in tabs, follow links across files, and keep a full back
history per tab. Try opening [code.md](./code.md) in a new tab with `T`.

## Example

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello from md!")
}
```

## That's it

Press `q` to quit, `?` to toggle the help overlay at any time.
```

- [ ] **Step 2: Create `demo/code.md`**

```markdown
# More code

A smaller document, handy for showing tabbed browsing in `md`. Switch back
with `shift+tab`.

```python
def greet(name: str) -> str:
    return f"Hello, {name}!"


if __name__ == "__main__":
    print(greet("world"))
```
```

- [ ] **Step 3: Verify both files exist and render sanely**

Run: `go run ./cmd/mdcat demo/hello.md | head -20`
Expected: colorized output of the first few elements of `hello.md`, no errors.

Run: `go run ./cmd/mdcat demo/code.md | head -20`
Expected: colorized output, no errors.

- [ ] **Step 4: Commit**

```bash
git add demo/hello.md demo/code.md
git commit -m "Add demo fixtures for cmd/md hero gif"
```

---

### Task 2: Write the VHS tape

**Files:**
- Create: `demo/demo.tape`

- [ ] **Step 1: Write the tape**

```tape
# VHS tape for the cmd/md hero GIF.
# Re-render with:  vhs demo/demo.tape
# Uses the in-tree md binary via `go run ./cmd/md` so the gif always
# reflects the current working tree.

Output demo/md.gif

Set Shell bash
Set FontSize 14
Set Width 1200
Set Height 720
Set Padding 20
Set TypingSpeed 50ms
Set PlaybackSpeed 1.0
Set Theme "Dracula"

# Isolate md's config/cache/index from the host so the gif is reproducible.
Env XDG_CONFIG_HOME /tmp/md-demo/config
Env XDG_CACHE_HOME  /tmp/md-demo/cache
Env XDG_DATA_HOME   /tmp/md-demo/data

Hide
Type "rm -rf /tmp/md-demo && mkdir -p /tmp/md-demo/config /tmp/md-demo/cache /tmp/md-demo/data"
Enter
Type "cd demo"
Enter
Type "clear"
Enter
Show

# --- Open the primary document ---
Type "go run ../cmd/md hello.md"
Enter
Sleep 4s

# --- Act 1: basic navigation ---
Type@120ms "jjjj"
Sleep 800ms
Type "}"
Sleep 600ms
Type "}"
Sleep 600ms
Type "{"
Sleep 800ms

# --- Act 2: table of contents ---
Type "t"
Sleep 1s
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Enter
Sleep 1500ms

# --- Act 3: item nav + in-doc search ---
Type "]"
Sleep 400ms
Type "]"
Sleep 600ms
Type "/"
Sleep 400ms
Type "code"
Enter
Sleep 1s
Type "n"
Sleep 600ms
Escape
Sleep 500ms

# --- Act 4: copy a code block ---
# Land item selection on the Go code block, then copy with `y`.
Type "]"
Sleep 400ms
Type "]"
Sleep 400ms
Type "]"
Sleep 600ms
Type "y"
Sleep 1500ms

# --- Act 5: view source toggle ---
Ctrl+U
Sleep 2s
Ctrl+U
Sleep 1s

# --- Act 6: multi-doc workflow ---
# Open code.md in a new tab via the file picker.
Ctrl+T
Sleep 1s
Type "code"
Sleep 800ms
Enter
Sleep 2s

# Flip back to hello.md.
Shift+Tab
Sleep 1200ms

# Navigate to the relative link to code.md, then open it in a new tab.
Type "["
Sleep 300ms
Type "["
Sleep 400ms
Type "T"
Sleep 2s

# --- Quit ---
Type "q"
Sleep 1s
```

- [ ] **Step 2: Sanity-check the tape parses**

Run: `vhs validate demo/demo.tape`
Expected: no output (success), exit 0. If `vhs validate` is not a
subcommand in this VHS version, run `vhs demo/demo.tape --help` or skip to
the next task's render step and rely on VHS's parse errors there.

- [ ] **Step 3: Commit**

```bash
git add demo/demo.tape
git commit -m "Add VHS tape for cmd/md hero gif"
```

---

### Task 3: First render and inspect

**Files:**
- Create (build artifact): `demo/md.gif`

- [ ] **Step 1: Render the tape**

Run: `vhs demo/demo.tape`
Expected: completes without error; `demo/md.gif` exists.

```bash
vhs demo/demo.tape
ls -la demo/md.gif
```

- [ ] **Step 2: Inspect gif size**

Run: `ls -la demo/md.gif | awk '{print $5}'`
Expected: a file size under ~5_000_000 bytes (5 MB). If larger, note the
size; tuning happens in Task 4.

- [ ] **Step 3: Open the gif and watch it end-to-end**

Run: `open demo/md.gif`
What to check, beat by beat:

1. The doc opens — title visible, no stack trace
2. Scroll and heading nav move the cursor/viewport
3. `t` opens the TOC overlay, `Enter` jumps to a heading
4. Search prompt opens on `/`, query runs, matches highlight
5. A code block gets the "selected" highlight before `y` is pressed
6. `Ctrl+U` swaps into source view (rendered Markdown → raw inside a code block) and back
7. A second tab appears after `Ctrl+T`, a third after `T`; tab bar visible
8. `q` cleanly exits

Note any failing beat — those drive Task 4. If every beat works and the gif
is under 5 MB, Task 4 may be skipped.

---

### Task 4: Tune tape (iterate until clean)

**Files:**
- Modify: `demo/demo.tape` (and `demo/hello.md` only if content-ordering needed for item nav)

This task is conditional: only run if Task 3's inspection found issues.
Typical fixes and the tape knobs that address them:

| Symptom | Fix |
|---------|-----|
| Item nav lands on the wrong element | Adjust the count of `Type "]"` in the relevant act, or reorder elements in `hello.md` |
| Search finds zero matches | Change the query in Act 3 to a word that appears |
| `md` binary takes longer than 4s to start | Increase the post-launch `Sleep` in Act 1 |
| Frames look rushed | Increase `Sleep` between beats |
| Gif too large (>5 MB) | Lower `Set Width`/`Set Height` in 100px steps, or shorten sleeps |
| Text clipped on the right | Decrease `Set FontSize` to 13, or increase `Set Width` |
| TOC overlay does not appear | Ensure `hello.md` has ≥ 2 headings (it has 4) and that the terminal is ≥ 40×6 (current settings are far larger) |

- [ ] **Step 1: Apply fixes**

Edit `demo/demo.tape` (and, if needed, `demo/hello.md`) to address items
from Task 3 step 3.

- [ ] **Step 2: Re-render**

Run: `vhs demo/demo.tape`

- [ ] **Step 3: Re-inspect**

Run: `open demo/md.gif`
Repeat Task 3 step 3's checklist. Loop Task 4 steps 1-3 until every beat
works and the gif is ≤ 5 MB.

- [ ] **Step 4: Commit tape (and fixtures) updates**

```bash
git add demo/demo.tape demo/hello.md demo/code.md
git commit -m "Tune hero gif tape"
```

(Skip files that weren't modified.)

---

### Task 5: Commit the rendered gif

**Files:**
- Create: `demo/md.gif` (committed binary)

- [ ] **Step 1: Stage and commit the gif**

```bash
git add demo/md.gif
git commit -m "Add rendered cmd/md hero gif"
```

- [ ] **Step 2: Confirm the gif is tracked**

Run: `git ls-files demo/`
Expected output includes `demo/md.gif`.

---

### Task 6: Reference the gif in `README.md`

**Files:**
- Modify: `README.md` (insert an image reference below the tagline and above the "## Packages" heading).

- [ ] **Step 1: Edit `README.md`**

Find, in `README.md`:

```markdown
A Go toolkit for rendering Markdown to multiple output formats, plus a set
of CLI tools built on top.

## Packages
```

Replace with:

```markdown
A Go toolkit for rendering Markdown to multiple output formats, plus a set
of CLI tools built on top.

![md — an interactive terminal Markdown reader](demo/md.gif)

## Packages
```

- [ ] **Step 2: Verify the README renders with the gif**

Run: `grep -n 'demo/md.gif' README.md`
Expected: one match, on a line between the tagline and the `## Packages`
heading.

Optional visual check:

Run: `go run ./cmd/mdcat README.md | head -40`
Expected: the image reference is present in the rendered output (as an
image line or placeholder depending on terminal support).

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Show cmd/md hero gif in README"
```

---

### Task 7: Verification

- [ ] **Step 1: Clean re-render from scratch**

Run:

```bash
rm -f demo/md.gif
vhs demo/demo.tape
git diff --stat demo/md.gif
```

Expected: `demo/md.gif` re-appears; `git diff --stat` may show byte-level
differences (gif encoding is not fully deterministic) but file should
exist and be roughly the same size.

If the newly-rendered gif differs meaningfully, revert it to the committed
version with `git checkout demo/md.gif` — we don't need to commit
re-renders unless the tape changed.

- [ ] **Step 2: Confirm all four feature groups are visible**

From the spec:

1. Basic nav (scroll, heading nav, item nav, search)
2. Copy code block (rendered view, not source)
3. View source toggle
4. Multi-doc (file picker new tab + link in new tab)

Spot-check the gif covers all four. If not, return to Task 4.

- [ ] **Step 3: Final commit if any follow-up fixes were needed**

If Task 7 surfaced issues, fix in place and commit with a descriptive
message. Otherwise, the implementation is complete.
