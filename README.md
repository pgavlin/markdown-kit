# markdown-kit

[![PkgGoDev](https://pkg.go.dev/badge/github.com/pgavlin/markdown-kit)](https://pkg.go.dev/github.com/pgavlin/markdown-kit)
[![codecov](https://codecov.io/gh/pgavlin/markdown-kit/branch/main/graph/badge.svg)](https://codecov.io/gh/pgavlin/markdown-kit)
[![Go Report Card](https://goreportcard.com/badge/github.com/pgavlin/markdown-kit)](https://goreportcard.com/report/github.com/pgavlin/markdown-kit)
[![Test](https://github.com/pgavlin/markdown-kit/workflows/Test/badge.svg)](https://github.com/pgavlin/markdown-kit/actions?query=workflow%3ATest)

A Go toolkit for rendering Markdown to multiple output formats, plus a set
of CLI tools built on top.

## Packages

| Package | Description |
|---------|-------------|
| [`renderer`](https://pkg.go.dev/github.com/pgavlin/markdown-kit/renderer) | Terminal renderer with ANSI colorization, word wrapping, table rendering (Unicode box-drawing), image encoding (Kitty graphics protocol, ANSI), and document span tracking. |
| [`view`](https://pkg.go.dev/github.com/pgavlin/markdown-kit/view) | Interactive [Bubble Tea](https://github.com/charmbracelet/bubbletea) model for displaying and navigating Markdown in a terminal. Supports heading/URL/code-block navigation, search, and content copying. |
| [`odt`](https://pkg.go.dev/github.com/pgavlin/markdown-kit/odt) | Converts Markdown to OpenDocument Text (.odt). Generates ODF 1.3 compliant ZIP archives. |
| [`indexer`](https://pkg.go.dev/github.com/pgavlin/markdown-kit/indexer) | Builds a document index (table of contents) from headings with GFM-style anchor generation. |
| [`styles`](https://pkg.go.dev/github.com/pgavlin/markdown-kit/styles) | Color theme definitions and custom Chroma token types. |

All packages build on [a fork](https://github.com/pgavlin/goldmark) of
[goldmark](https://github.com/yuin/goldmark). Many thanks to
[yuin](https://github.com/yuin), without whose work none of this would be
possible.

## CLI tools

### `md` — Interactive terminal reader

A tabbed, interactive Markdown reader with:

- Tabbed browsing with back/forward history
- Link following for local files and HTTP URLs
- Fuzzy file picker and URL input
- Full-text search within documents
- Configurable key bindings and color themes
- External format converters for non-Markdown files (e.g. reStructuredText, AsciiDoc)

```console
go install github.com/pgavlin/markdown-kit/cmd/md@latest
md README.md
md https://example.com/doc.md
```

### `mdcat` — Render Markdown to the terminal

Renders colorized Markdown to stdout with optional image display (Kitty
graphics protocol).

```console
go install github.com/pgavlin/markdown-kit/cmd/mdcat@latest
mdcat README.md
```

### `md2odt` — Convert Markdown to OpenDocument Text

```console
go install github.com/pgavlin/markdown-kit/cmd/md2odt@latest
md2odt input.md -o output.odt
```

## Building from source

Requires **Go 1.24+**.

```console
git clone https://github.com/pgavlin/markdown-kit.git
cd markdown-kit
go build ./...
go test ./...
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[MIT](LICENSE)
