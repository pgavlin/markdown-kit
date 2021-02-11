# `markdown-kit`: Markdown Utilities for Go

[![PkgGoDev](https://pkg.go.dev/badge/github.com/pgavlin/markdown-kit)](https://pkg.go.dev/github.com/pgavlin/markdown-kit)
[![codecov](https://codecov.io/gh/pgavlin/markdown-kit/branch/master/graph/badge.svg)](https://codecov.io/gh/pgavlin/markdown-kit)
[![Go Report Card](https://goreportcard.com/badge/github.com/pgavlin/markdown-kit)](https://goreportcard.com/report/github.com/pgavlin/markdown-kit)
[![Test](https://github.com/pgavlin/markdown-kit/workflows/Test/badge.svg)](https://github.com/pgavlin/markdown-kit/actions?query=workflow%3ATest)

`markdown-kit` provides a small set of packages for working with Markdown in Go,
as well as a selection of CLI tools built atop these packages.

- Package `renderer` provides a Markdown renderer with support for colorization, word wrapping, and 
  document navigation. The `mdcat` tool uses this package to render colorized Markdown to the 
  terminal.
- Package `tview` provides a [`tview`](https://github.com/rivo/tview) component for displaying and 
  navigating Markdown documents. The `mdreader` tool uses this package to implement a 
  terminal-based reader for Markdown documents.
- Package `odt` provides a converter from Markdown to OpenDocument text. The `md2odt` tool provides 
  a CLI wrapper over this package.

These packages build on top of [a fork](https://github.com/pgavlin/goldmark) of 
[goldmark](https://github.com/yuin/goldmark). Many thanks to [yuin](https://github.com/yuin), 
without whose work none of this would be possible.
