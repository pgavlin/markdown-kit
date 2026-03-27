package view

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/pgavlin/tea-grid/data"
	"github.com/pgavlin/tea-grid/grid"
)

// tableRow holds the pre-rendered cell data for a single row in a tea-grid table.
type tableRow struct {
	cells []tableCell
}

// tableCell holds a single cell's rendered content and associated link nodes.
type tableCell struct {
	rendered string     // ANSI-styled rendered content
	plain    string     // plain text for sorting/filtering
	links    []ast.Node // link nodes for span registration
}

// NewTeaGridTableRenderer returns a renderer.TableRenderer that renders tables
// using the tea-grid component. The theme is used to derive grid styles.
func NewTeaGridTableRenderer(theme *chroma.Style) renderer.TableRenderer {
	return func(ctx *renderer.TableRenderContext) error {
		table := ctx.Table

		// Extract rows and cells from the AST.
		var rows []tableRow
		var numCols int
		for rowIdx, row := 0, table.FirstChild(); row != nil; rowIdx, row = rowIdx+1, row.NextSibling() {
			var tr tableRow
			for col, cell := 0, row.FirstChild(); cell != nil; col, cell = col+1, cell.NextSibling() {
				rendered, links, err := ctx.RenderCell(cell, 0, rowIdx)
				if err != nil {
					return err
				}
				plain := stripANSI(rendered)
				tr.cells = append(tr.cells, tableCell{
					rendered: rendered,
					plain:    plain,
					links:    links,
				})
				if col+1 > numCols {
					numCols = col + 1
				}
			}
			rows = append(rows, tr)
		}

		if len(rows) == 0 || numCols == 0 {
			return fmt.Errorf("empty table")
		}

		// First row is the header — use it for column definitions.
		headerRow := rows[0]
		dataRows := rows[1:]

		// Build column definitions.
		cols := make([]data.Column[tableRow], numCols)
		for i := 0; i < numCols; i++ {
			colIdx := i
			headerName := ""
			if colIdx < len(headerRow.cells) {
				headerName = headerRow.cells[colIdx].plain
			}
			cols[i] = data.Column[tableRow]{
				ColumnID:   fmt.Sprintf("col%d", i),
				HeaderName: headerName,
				ValueGetter: func(row tableRow) any {
					if colIdx < len(row.cells) {
						return row.cells[colIdx].plain
					}
					return ""
				},
				ValueFormatter: func(value any, row tableRow) string {
					if colIdx < len(row.cells) {
						return row.cells[colIdx].rendered
					}
					return ""
				},
				Sortable:   true,
				Filterable: true,
				Flex:       1,
				MinWidth:   4,
			}
		}

		// Build grid styles from the chroma theme.
		gridStyles := buildGridStyles(theme)

		// Determine the grid width.
		width := ctx.Width
		if width <= 0 {
			// Compute from content.
			width = 80
		}

		// Compute height: header (1) + header border (1) + data rows.
		height := 2 + len(dataRows)

		g := grid.New(
			grid.WithColumns(cols),
			grid.WithRows(dataRows),
			grid.WithWidth[tableRow](width),
			grid.WithHeight[tableRow](height),
			grid.WithStyles[tableRow](gridStyles),
		)

		// Record byte offset before writing.
		startOffset := ctx.ByteOffset()

		// Write the grid output.
		output := g.View()
		if output == "" {
			return fmt.Errorf("empty grid output")
		}

		if _, err := ctx.WriteString(output); err != nil {
			return err
		}
		if !strings.HasSuffix(output, "\n") {
			if _, err := ctx.WriteString("\n"); err != nil {
				return err
			}
		}

		// Register link spans for all links in data rows.
		endOffset := ctx.ByteOffset()
		for _, row := range dataRows {
			for _, cell := range row.cells {
				for _, link := range cell.links {
					ctx.InsertSpan(link, startOffset, endOffset)
				}
			}
		}

		return nil
	}
}

// buildGridStyles creates grid.Styles derived from a chroma theme.
func buildGridStyles(theme *chroma.Style) grid.Styles {
	s := grid.DefaultStyles()

	// Use rounded border to match the renderer's ╭┬╮├┼┤╰┴╯│─ style.
	s.Border = lipgloss.RoundedBorder()
	s.BorderHeader = true
	s.BorderRow = false
	s.BorderColumn = true

	if theme == nil {
		return s
	}

	// Map chroma style entries to lipgloss styles.
	if entry := theme.Get(chroma.TokenType(styles.Table)); entry.Background != 0 {
		bg := chromaToLipglossColor(entry.Background)
		s.Table = s.Table.Background(bg)
		s.Cell = s.Cell.Background(bg)
		s.CellEvenRow = s.CellEvenRow.Background(bg)
	}

	if entry := theme.Get(chroma.TokenType(styles.TableHeader)); entry.Colour != 0 {
		fg := chromaToLipglossColor(entry.Colour)
		s.HeaderCell = s.HeaderCell.Foreground(fg).Bold(true)
	}

	if entry := theme.Get(chroma.TokenType(styles.TableRowAlt)); entry.Background != 0 {
		bg := chromaToLipglossColor(entry.Background)
		s.CellOddRow = s.CellOddRow.Background(bg)
	}

	return s
}

// chromaToLipglossColor converts a chroma.Colour to a color.Color.
func chromaToLipglossColor(c chroma.Colour) color.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.Red(), c.Green(), c.Blue()))
}

// stripANSI removes ANSI escape sequences from a string, returning plain text.
func stripANSI(s string) string {
	return ansi.Strip(s)
}
