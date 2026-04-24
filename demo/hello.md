# Welcome to md

md is a **fast, interactive Markdown reader** for your terminal. It speaks
CommonMark and GitHub-Flavored Markdown, with styling, syntax highlighting,
and a Bubble Tea-driven UI that feels right at home next to `vim`, `less`,
and `fzf`.

> A Markdown-formatted document should be publishable as-is, as plain text,
> without looking like it's been marked up with tags.
>
> — *John Gruber*

## Navigation at a glance

Use vim-style keys anywhere in a document:

- `j` / `k` — scroll one line at a time
- `{` / `}` — jump between headings
- `[` / `]` — cycle through *navigable items* (links, code blocks, tables,
  anchors)
- `/` — search; `n` and `N` move through matches
- `t` — toggle the table-of-contents overlay
- `?` — show the full help overlay

Every binding is one-tap; two-letter combos are avoided to keep flow tight.

## Reading multiple files

Open documents in tabs, follow links across files, and keep a full back
history per tab. Try opening [code.md](./code.md) in a new tab with `T`.

## Try the code

Select the code block with `]` and press `y` to copy its contents to the
system clipboard. `Ctrl+U` toggles a raw-source view for the same block.

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello from md!")
}
```

## Supported constructs

md renders the full breadth of CommonMark and GFM, with themed styling:

| Construct    | Example                        |
| ------------ | ------------------------------ |
| Emphasis     | *italic*, **bold**, ***both*** |
| Inline code  | `md --version`                 |
| Lists        | ordered, unordered, nested     |
| Block quotes | like the Gruber quote above    |
| Tables       | GFM alignment and headers      |
| Fenced code  | highlighted via Chroma         |
| Links        | local files, URLs, anchors     |

Images render inline in terminals that support the Kitty graphics protocol.

## That's it

Press `q` to quit, or `?` to open the help overlay at any time.
