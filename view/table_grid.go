package view

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
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

// gridState holds the state of a single tea-grid table, including its model
// and the line range it occupies in the rendered output.
type gridState struct {
	model     grid.Model[tableRow]
	startLine int // first line index in m.lines
	endLine   int // one past last line index in m.lines
}

// GridTableRenderer manages tea-grid table models for interactive focus.
// It persists grid state across renders so that grids can be focused and
// interacted with without recreating them.
type GridTableRenderer struct {
	theme *chroma.Style
	grids []*gridState
}

// NewGridTableRenderer creates a new GridTableRenderer with the given theme.
func NewGridTableRenderer(theme *chroma.Style) *GridTableRenderer {
	return &GridTableRenderer{theme: theme}
}

// Reset clears all stored grid states. Call before a full re-render.
func (gtr *GridTableRenderer) Reset() {
	gtr.grids = nil
}

// Grids returns the slice of grid states for focus lookup.
func (gtr *GridTableRenderer) Grids() []*gridState {
	return gtr.grids
}

// Renderer returns a renderer.TableRenderer closure that creates grid models
// and stores them in the GridTableRenderer for later interactive focus.
func (gtr *GridTableRenderer) Renderer() renderer.TableRenderer {
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

		// Build grid styles from the chroma theme.
		gridStyles := buildGridStyles(gtr.theme)

		// Determine the grid width.
		width := ctx.Width
		if width <= 0 {
			width = 80
		}

		// Compute content width per column for wrapping.
		// All columns use Flex:1, so widths are equal.
		// Subtract (numCols-1) for column border characters.
		borderCols := numCols - 1
		if borderCols < 0 {
			borderCols = 0
		}
		colWidth := (width - borderCols) / numCols
		// Cell style has horizontal frame (padding). Default cell padding is 1 on each side.
		cellHorizontalFrame := gridStyles.Cell.GetHorizontalFrameSize()
		contentWidth := colWidth - cellHorizontalFrame
		if contentWidth < 1 {
			contentWidth = 1
		}

		// Re-render cells with wrapping width.
		for rowIdx, row := 0, table.FirstChild(); row != nil; rowIdx, row = rowIdx+1, row.NextSibling() {
			if rowIdx >= len(rows) {
				break
			}
			for col, cell := 0, row.FirstChild(); cell != nil; col, cell = col+1, cell.NextSibling() {
				if col >= len(rows[rowIdx].cells) {
					break
				}
				rendered, links, err := ctx.RenderCell(cell, contentWidth, rowIdx)
				if err != nil {
					return err
				}
				rows[rowIdx].cells[col].rendered = rendered
				rows[rowIdx].cells[col].plain = stripANSI(rendered)
				rows[rowIdx].cells[col].links = links
			}
		}

		// Compute per-row heights based on wrapped content.
		rowHeights := make([]int, len(dataRows))
		for i, dr := range dataRows {
			maxH := 1
			for _, cell := range dr.cells {
				h := strings.Count(cell.rendered, "\n") + 1
				if h > maxH {
					maxH = h
				}
			}
			rowHeights[i] = maxH
		}

		// Build column definitions with custom CellRenderer to bypass truncation.
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
				CellRenderer: wrappedCellRenderer[tableRow](colIdx),
				Sortable:     true,
				Filterable:   true,
				Flex:         1,
				MinWidth:     4,
			}
		}

		// Compute total height: header (1) + header border (1) + sum of row heights.
		totalDataHeight := 0
		for _, h := range rowHeights {
			totalDataHeight += h
		}
		height := 2 + totalDataHeight

		g := grid.New(
			grid.WithColumns(cols),
			grid.WithRows(dataRows),
			grid.WithWidth[tableRow](width),
			grid.WithHeight[tableRow](height),
			grid.WithStyles[tableRow](gridStyles),
			grid.WithDynamicRowHeight(func(row tableRow) int {
				maxH := 1
				for _, cell := range row.cells {
					h := strings.Count(cell.rendered, "\n") + 1
					if h > maxH {
						maxH = h
					}
				}
				return maxH
			}),
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

		// Store grid state for interactive focus.
		gtr.grids = append(gtr.grids, &gridState{
			model: g,
		})

		return nil
	}
}

// wrappedCellRenderer returns a CellRenderer that outputs the pre-rendered
// wrapped content directly, bypassing the default truncation logic.
func wrappedCellRenderer[T tableRow](colIdx int) data.CellRendererFunc[T] {
	return func(ctx data.CellContext[T]) string {
		row := any(ctx.Data).(tableRow)
		if colIdx < len(row.cells) {
			return row.cells[colIdx].rendered
		}
		return ""
	}
}

// NewTeaGridTableRenderer returns a renderer.TableRenderer that renders tables
// using the tea-grid component. The theme is used to derive grid styles.
// This is a convenience wrapper that creates a GridTableRenderer internally.
func NewTeaGridTableRenderer(theme *chroma.Style) renderer.TableRenderer {
	gtr := NewGridTableRenderer(theme)
	return gtr.Renderer()
}

// UpdateGrid forwards a tea.Msg to the focused grid and returns the updated
// grid output lines.
func (gs *gridState) UpdateGrid(msg tea.Msg) []string {
	gs.model, _ = gs.model.Update(msg)
	output := gs.model.View()
	return strings.Split(strings.TrimRight(output, "\n"), "\n")
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
