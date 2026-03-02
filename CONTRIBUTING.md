# Contributing to markdown-kit

Thanks for your interest in contributing to markdown-kit! This document
explains how to get started.

## Getting started

### Prerequisites

- **Go 1.24+** (see `go.mod` for the exact version)
- **gofumpt** for code formatting (`go install mvdan.cc/gofumpt@latest`)
- Git

### Setup

```bash
git clone https://github.com/pgavlin/markdown-kit.git
cd markdown-kit
go build ./...
go test ./...
```

## Making changes

1. Fork the repository and create a branch from `main`.
2. Make your changes. Add or update tests as appropriate.
3. Run the checks described below before opening a pull request.

### Code style

- Format code with **gofumpt** (enforced in CI):
  ```bash
  gofumpt -w .
  ```
- Run `go vet`:
  ```bash
  go vet ./...
  ```

### Running tests

```bash
go test ./...                             # all tests
go test ./renderer                        # single package
go test ./renderer -run TestSpec          # single test
go test ./... -coverprofile=coverage.out  # with coverage
```

CI runs the full test suite on Ubuntu, macOS, and Windows.

## Project layout

```
renderer/       Terminal renderer (ANSI, tables, images)
view/           Interactive Bubble Tea model
odt/            Markdown to OpenDocument Text
indexer/        Table of contents / heading index
styles/         Color themes and Chroma token types
internal/kitty/ Kitty graphics protocol
cmd/mdcat/      CLI: render markdown to terminal
cmd/md/         CLI: interactive terminal reader
cmd/md2odt/     CLI: markdown to ODT conversion
```

The rendering pipeline is: Markdown source -> goldmark parser -> AST ->
backend renderer -> output. See `CLAUDE.md` for more architectural detail.

### Testing patterns

- **Spec-based**: `renderer` validates against the CommonMark spec.
- **Golden files**: `odt` compares output against saved `.odt` files in
  `internal/testdata/`.
- **Fixtures**: Markdown samples live in `internal/testdata/`.

## Reporting bugs

Open an issue at
<https://github.com/pgavlin/markdown-kit/issues>. Include:

- What you did (steps to reproduce).
- What you expected to happen.
- What actually happened (error messages, screenshots, etc.).
- Your OS, Go version, and terminal emulator.

## Suggesting features

Open an issue describing the use case and proposed behavior. Discussion
before implementation helps avoid duplicate work and ensures the feature
fits the project's direction.

## Pull requests

- Keep PRs focused on a single change.
- Reference any related issue in the PR description.
- Make sure `go build ./...`, `go vet ./...`, and `go test ./...` pass
  locally before pushing.
- CI must be green before a PR can be merged.

## License

By contributing you agree that your contributions will be licensed under
the [MIT License](LICENSE).
