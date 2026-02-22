# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
go build ./...                      # Build all packages and CLI tools
go test ./...                       # Run all tests
go test ./renderer                  # Test a single package
go test ./renderer -run TestSpec    # Run a specific test
go test ./... -coverprofile=coverage.out  # Tests with coverage (CI command)
```

The module requires Go 1.24+ (per go.mod). CI runs across ubuntu, macOS, and Windows.

## Architecture

markdown-kit is a Go toolkit for rendering Markdown to multiple output formats. It uses a custom fork of goldmark (`github.com/pgavlin/goldmark`) for parsing.

### Rendering pipeline

```
Markdown → goldmark parser → AST → backend renderer → output
```

### Packages

- **`renderer`** — Terminal renderer with ANSI colorization, word wrapping, table rendering (Unicode box-drawing), image encoding (Kitty graphics protocol, ANSI), and document span tracking (`NodeSpan` tree maps AST nodes to byte offsets in output). Uses a style stack for nested formatting and Chroma for syntax highlighting.
- **`odt`** — Converts Markdown to OpenDocument Text (.odt). Generates ODF 1.3 compliant ZIP archives with manifest, mimetype, and content.xml.
- **`view`** — Interactive Bubble Tea model (`MarkdownView`) for displaying and navigating Markdown in a terminal UI. Parses ANSI sequences for styled text, supports heading/URL navigation, code block navigation, and content copying.
- **`indexer`** — Builds a document index (table of contents) from headings with GFM-style anchor generation.
- **`styles`** — Color theme definitions (e.g., `Pulumi` theme) and custom Chroma token types for tables.
- **`internal/kitty`** — Kitty terminal graphics protocol encoding/decoding.

### CLI tools (under `cmd/`)

- **`mdcat`** — Renders Markdown to the terminal with colors and optional image display.
- **`md`** — Interactive terminal-based Markdown reader using Bubble Tea.
- **`md2odt`** — Converts Markdown files to .odt format.

### Testing patterns

- **Golden file testing**: `odt` tests compare output against saved `.odt` files in `internal/testdata/`.
- **Spec-based testing**: `renderer` validates against CommonMark spec.
- **Test fixtures**: Markdown samples live in `internal/testdata/`.
