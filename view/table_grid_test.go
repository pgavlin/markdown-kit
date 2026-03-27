package view

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/extension"
	goldmark_parser "github.com/pgavlin/goldmark/parser"
	goldmark_renderer "github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update golden files")

// renderTableDoc renders a markdown document containing tables using the given
// renderer options. It returns the raw ANSI output.
func renderTableDoc(t *testing.T, source []byte, opts ...renderer.RendererOption) string {
	t.Helper()

	parser := goldmark.DefaultParser()
	parser.AddOptions(goldmark_parser.WithParagraphTransformers(
		util.Prioritized(extension.NewTableParagraphTransformer(), 200),
	))
	document := parser.Parse(text.NewReader(source))

	var buf bytes.Buffer
	r := renderer.New(opts...)
	gmr := goldmark_renderer.NewRenderer(goldmark_renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	err := gmr.Render(&buf, source, document)
	require.NoError(t, err)

	return buf.String()
}

// tableGoldenTest defines a single golden-file test case.
type tableGoldenTest struct {
	name      string // test name (used in golden filename)
	inputFile string // markdown input file under testdata/
	width     int    // word-wrap width (0 = no wrap)
}

var tableGoldenTests = []tableGoldenTest{
	{name: "simple-80", inputFile: "table-simple.md", width: 80},
	{name: "simple-40", inputFile: "table-simple.md", width: 40},
	{name: "wide-120", inputFile: "table-wide.md", width: 120},
	{name: "wide-80", inputFile: "table-wide.md", width: 80},
	{name: "wide-60", inputFile: "table-wide.md", width: 60},
	{name: "links-80", inputFile: "table-links.md", width: 80},
	{name: "aligned-80", inputFile: "table-aligned.md", width: 80},
	{name: "mixed-80", inputFile: "table-mixed-content.md", width: 80},
	{name: "mixed-60", inputFile: "table-mixed-content.md", width: 60},
}

// TestTableGolden_Builtin tests the built-in table renderer against golden files.
func TestTableGolden_Builtin(t *testing.T) {
	for _, tc := range tableGoldenTests {
		t.Run(tc.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(testdataPath, tc.inputFile))
			require.NoError(t, err)

			opts := []renderer.RendererOption{
				renderer.WithWordWrap(tc.width),
				renderer.WithSoftBreak(tc.width > 0),
			}
			output := renderTableDoc(t, source, opts...)
			stripped := ansi.Strip(output)

			goldenFile := filepath.Join(testdataPath, fmt.Sprintf("table-%s.builtin.txt", tc.name))

			if *update {
				err := os.WriteFile(goldenFile, []byte(stripped), 0644)
				require.NoError(t, err)
				return
			}

			expected, err := os.ReadFile(goldenFile)
			require.NoError(t, err, "golden file %s not found; run with -update to create", goldenFile)
			assert.Equal(t, string(expected), stripped)
		})
	}
}

// TestTableGolden_TeaGrid tests the tea-grid interactive table renderer against golden files.
func TestTableGolden_TeaGrid(t *testing.T) {
	for _, tc := range tableGoldenTests {
		t.Run(tc.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(testdataPath, tc.inputFile))
			require.NoError(t, err)

			tr := NewTeaGridTableRenderer(styles.Pulumi)
			opts := []renderer.RendererOption{
				renderer.WithTheme(styles.Pulumi),
				renderer.WithWordWrap(tc.width),
				renderer.WithSoftBreak(tc.width > 0),
				renderer.WithTableRenderer(tr),
			}
			output := renderTableDoc(t, source, opts...)
			stripped := ansi.Strip(output)

			goldenFile := filepath.Join(testdataPath, fmt.Sprintf("table-%s.teagrid.txt", tc.name))

			if *update {
				err := os.WriteFile(goldenFile, []byte(stripped), 0644)
				require.NoError(t, err)
				return
			}

			expected, err := os.ReadFile(goldenFile)
			require.NoError(t, err, "golden file %s not found; run with -update to create", goldenFile)
			assert.Equal(t, string(expected), stripped)
		})
	}
}

// TestTableGolden_VisualComparison compares the stripped (plain text) output of
// the built-in and tea-grid renderers side by side and reports differences. This
// test always passes — it logs the differences for investigation.
func TestTableGolden_VisualComparison(t *testing.T) {
	for _, tc := range tableGoldenTests {
		t.Run(tc.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(testdataPath, tc.inputFile))
			require.NoError(t, err)

			// Built-in renderer (no theme, plain text).
			builtinOpts := []renderer.RendererOption{
				renderer.WithWordWrap(tc.width),
				renderer.WithSoftBreak(tc.width > 0),
			}
			builtinOutput := ansi.Strip(renderTableDoc(t, source, builtinOpts...))

			// Tea-grid renderer.
			tr := NewTeaGridTableRenderer(styles.Pulumi)
			teagridOpts := []renderer.RendererOption{
				renderer.WithTheme(styles.Pulumi),
				renderer.WithWordWrap(tc.width),
				renderer.WithSoftBreak(tc.width > 0),
				renderer.WithTableRenderer(tr),
			}
			teagridOutput := ansi.Strip(renderTableDoc(t, source, teagridOpts...))

			if builtinOutput == teagridOutput {
				t.Log("outputs are identical")
				return
			}

			builtinLines := strings.Split(builtinOutput, "\n")
			teagridLines := strings.Split(teagridOutput, "\n")

			// Report structural differences.
			t.Logf("built-in: %d lines, tea-grid: %d lines", len(builtinLines), len(teagridLines))

			// Check line width violations.
			if tc.width > 0 {
				for i, line := range builtinLines {
					w := ansi.StringWidth(line)
					if w > tc.width {
						t.Errorf("built-in line %d exceeds width %d: got %d: %q", i, tc.width, w, line)
					}
				}
				for i, line := range teagridLines {
					w := ansi.StringWidth(line)
					if w > tc.width {
						t.Errorf("tea-grid line %d exceeds width %d: got %d: %q", i, tc.width, w, line)
					}
				}
			}

			// Check border consistency for built-in: content rows should start/end with │.
			checkBorderConsistency(t, "built-in", builtinLines)
		})
	}
}

// checkBorderConsistency verifies that non-border table lines start and end with │.
func checkBorderConsistency(t *testing.T, label string, lines []string) {
	t.Helper()
	inTable := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inTable {
				inTable = false
			}
			continue
		}
		// Detect table start.
		if strings.HasPrefix(trimmed, "╭") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		// Border rows.
		if strings.HasPrefix(trimmed, "├") || strings.HasPrefix(trimmed, "╰") {
			if strings.HasPrefix(trimmed, "╰") {
				inTable = false
			}
			continue
		}
		// Content rows should start and end with │.
		if !strings.HasPrefix(trimmed, "│") {
			t.Errorf("%s line %d: content row should start with │: %q", label, i, trimmed)
		}
		if !strings.HasSuffix(trimmed, "│") {
			t.Errorf("%s line %d: content row should end with │: %q", label, i, trimmed)
		}
	}
}

// TestTeaGridTable_WidthRespected verifies the tea-grid renderer does not
// produce lines exceeding the specified wrap width.
func TestTeaGridTable_WidthRespected(t *testing.T) {
	widths := []int{40, 60, 80, 120}
	inputs := []string{
		"| A | B |\n| - | - |\n| 1 | 2 |\n",
		"| Name | Description |\n| ---- | ----------- |\n| foo | A short description |\n| bar | A much longer description that should need wrapping at narrow widths |\n",
	}

	for _, width := range widths {
		for i, input := range inputs {
			t.Run(fmt.Sprintf("input%d-width%d", i, width), func(t *testing.T) {
				tr := NewTeaGridTableRenderer(styles.Pulumi)
				opts := []renderer.RendererOption{
					renderer.WithTheme(styles.Pulumi),
					renderer.WithWordWrap(width),
					renderer.WithSoftBreak(true),
					renderer.WithTableRenderer(tr),
				}
				output := renderTableDoc(t, []byte(input), opts...)
				stripped := ansi.Strip(output)

				for j, line := range strings.Split(strings.TrimRight(stripped, "\n"), "\n") {
					w := ansi.StringWidth(line)
					if w > width {
						t.Errorf("line %d exceeds width %d: got %d: %q", j, width, w, line)
					}
				}
			})
		}
	}
}

// TestTeaGridTable_ContentPreserved verifies that all cell text from the source
// appears somewhere in the tea-grid output.
func TestTeaGridTable_ContentPreserved(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple",
			input:    "| Name | Age |\n| ---- | --- |\n| Alice | 30 |\n| Bob | 25 |\n",
			expected: []string{"Name", "Age", "Alice", "30", "Bob", "25"},
		},
		{
			name:     "with-code",
			input:    "| Func | Returns |\n| ---- | ------- |\n| `len()` | int |\n| `cap()` | int |\n",
			expected: []string{"Func", "Returns", "len()", "int", "cap()"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTeaGridTableRenderer(styles.Pulumi)
			opts := []renderer.RendererOption{
				renderer.WithTheme(styles.Pulumi),
				renderer.WithWordWrap(80),
				renderer.WithSoftBreak(true),
				renderer.WithTableRenderer(tr),
			}
			output := renderTableDoc(t, []byte(tc.input), opts...)
			stripped := ansi.Strip(output)

			for _, text := range tc.expected {
				assert.Contains(t, stripped, text, "output should contain %q", text)
			}
		})
	}
}

// TestTeaGridTable_BorderStructure documents the visual differences in border
// rendering between the tea-grid and built-in renderers.
//
// Known differences:
//   - Tea-grid has no outer border (no ╭╮╰╯ corners, no left/right frame)
//   - Tea-grid uses a full-width ─── divider under the header instead of ├┼┤
//   - Tea-grid inserts blank lines between data rows
//   - Tea-grid distributes columns to equal flex widths rather than fitting content
//   - Tea-grid truncates long cell content with … instead of wrapping into multi-line rows
func TestTeaGridTable_BorderStructure(t *testing.T) {
	input := "| A | B |\n| - | - |\n| 1 | 2 |\n"

	tr := NewTeaGridTableRenderer(styles.Pulumi)
	opts := []renderer.RendererOption{
		renderer.WithTheme(styles.Pulumi),
		renderer.WithWordWrap(80),
		renderer.WithTableRenderer(tr),
	}
	output := renderTableDoc(t, []byte(input), opts...)
	stripped := ansi.Strip(output)

	// Tea-grid uses │ as column separator.
	assert.True(t, strings.ContainsRune(stripped, '│'), "should contain column separator │")
	// Tea-grid uses ─ for the header divider.
	assert.True(t, strings.ContainsRune(stripped, '─'), "should contain horizontal divider ─")

	// Tea-grid does NOT produce outer border characters that the built-in
	// renderer uses. This is a visual difference.
	assert.False(t, strings.ContainsRune(stripped, '╭'), "tea-grid should not contain ╭ (no outer border)")
	assert.False(t, strings.ContainsRune(stripped, '╮'), "tea-grid should not contain ╮ (no outer border)")
	assert.False(t, strings.ContainsRune(stripped, '╰'), "tea-grid should not contain ╰ (no outer border)")
	assert.False(t, strings.ContainsRune(stripped, '╯'), "tea-grid should not contain ╯ (no outer border)")

	// Verify content is still present.
	assert.Contains(t, stripped, "A")
	assert.Contains(t, stripped, "B")
	assert.Contains(t, stripped, "1")
	assert.Contains(t, stripped, "2")
}

// TestTeaGridTable_BlankRowSeparators documents that the tea-grid renderer
// inserts blank lines between each data row, unlike the built-in renderer.
func TestTeaGridTable_BlankRowSeparators(t *testing.T) {
	input := "| Name | Age |\n| ---- | --- |\n| Alice | 30 |\n| Bob | 25 |\n| Carol | 35 |\n"

	tr := NewTeaGridTableRenderer(styles.Pulumi)
	opts := []renderer.RendererOption{
		renderer.WithTheme(styles.Pulumi),
		renderer.WithWordWrap(80),
		renderer.WithTableRenderer(tr),
	}
	teagridOutput := ansi.Strip(renderTableDoc(t, []byte(input), opts...))

	builtinOutput := ansi.Strip(renderTableDoc(t, []byte(input),
		renderer.WithWordWrap(80),
	))

	teagridLines := strings.Split(strings.TrimRight(teagridOutput, "\n"), "\n")
	builtinLines := strings.Split(strings.TrimRight(builtinOutput, "\n"), "\n")

	// Tea-grid produces more lines due to blank separators.
	assert.Greater(t, len(teagridLines), len(builtinLines),
		"tea-grid should produce more lines than built-in due to blank row separators")

	// Count blank lines in tea-grid output.
	blankCount := 0
	for _, line := range teagridLines {
		if strings.TrimSpace(line) == "" {
			blankCount++
		}
	}
	assert.Greater(t, blankCount, 0, "tea-grid should have blank lines between rows")
}

// TestTeaGridTable_EqualColumnWidths documents that the tea-grid renderer
// distributes column widths equally (flex: 1 for all), while the built-in
// renderer sizes columns to fit their content.
func TestTeaGridTable_EqualColumnWidths(t *testing.T) {
	input := "| ID | A very long description column |\n| -- | ----- |\n| 1 | Short |\n"

	tr := NewTeaGridTableRenderer(styles.Pulumi)
	opts := []renderer.RendererOption{
		renderer.WithTheme(styles.Pulumi),
		renderer.WithWordWrap(80),
		renderer.WithTableRenderer(tr),
	}
	teagridOutput := ansi.Strip(renderTableDoc(t, []byte(input), opts...))

	builtinOutput := ansi.Strip(renderTableDoc(t, []byte(input),
		renderer.WithWordWrap(80),
	))

	// In the built-in renderer, "ID" column is narrow.
	builtinLines := strings.Split(strings.TrimRight(builtinOutput, "\n"), "\n")
	// Find a data row in built-in output.
	for _, line := range builtinLines {
		if strings.Contains(line, "1") && strings.Contains(line, "Short") {
			parts := strings.SplitN(line, "│", 3)
			if len(parts) >= 3 {
				idCol := parts[1]
				descCol := parts[2]
				// Built-in should have a narrow ID column.
				assert.Less(t, len(idCol), len(descCol),
					"built-in renderer: ID column should be narrower than description column")
			}
			break
		}
	}

	// In the tea-grid renderer, columns get equal flex width.
	teagridLines := strings.Split(strings.TrimRight(teagridOutput, "\n"), "\n")
	for _, line := range teagridLines {
		if strings.Contains(line, "1") && strings.Contains(line, "Short") {
			parts := strings.SplitN(line, "│", 3)
			if len(parts) >= 3 {
				idCol := parts[0]     // No leading │ in tea-grid
				descCol := parts[1]
				// Tea-grid gives columns equal width (both ~half of total).
				idWidth := ansi.StringWidth(idCol)
				descWidth := ansi.StringWidth(descCol)
				// They should be roughly equal (within a few chars).
				diff := idWidth - descWidth
				if diff < 0 {
					diff = -diff
				}
				assert.LessOrEqual(t, diff, 2,
					"tea-grid renderer: columns should have roughly equal width (got ID=%d, desc=%d)", idWidth, descWidth)
			}
			break
		}
	}
}

// TestTeaGridTable_ContentTruncation documents that the tea-grid renderer
// truncates long cell content with … instead of wrapping it into multi-line
// rows like the built-in renderer does.
func TestTeaGridTable_ContentTruncation(t *testing.T) {
	input := "| Package | Description |\n| ------- | ----------- |\n| `renderer` | Terminal renderer with ANSI colorization, word wrapping, table rendering using Unicode box-drawing characters, and document span tracking |\n"

	tr := NewTeaGridTableRenderer(styles.Pulumi)
	opts := []renderer.RendererOption{
		renderer.WithTheme(styles.Pulumi),
		renderer.WithWordWrap(60),
		renderer.WithSoftBreak(true),
		renderer.WithTableRenderer(tr),
	}
	teagridOutput := ansi.Strip(renderTableDoc(t, []byte(input), opts...))

	builtinOutput := ansi.Strip(renderTableDoc(t, []byte(input),
		renderer.WithWordWrap(60),
		renderer.WithSoftBreak(true),
	))

	// Tea-grid truncates with … instead of wrapping.
	assert.Contains(t, teagridOutput, "…", "tea-grid should truncate long content with …")

	// Built-in wraps content into multiple lines.
	builtinLines := strings.Split(strings.TrimRight(builtinOutput, "\n"), "\n")
	teagridLines := strings.Split(strings.TrimRight(teagridOutput, "\n"), "\n")

	// Count data lines in each (lines with │ content, excluding borders).
	builtinDataLines := countDataLines(builtinLines)
	teagridDataLines := countDataLines(teagridLines)

	// Built-in should have more data lines due to wrapping.
	assert.Greater(t, builtinDataLines, teagridDataLines,
		"built-in should have more data lines (wrapping) vs tea-grid (truncation)")
}

// countDataLines counts lines containing │ but not border characters.
func countDataLines(lines []string) int {
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "╭") || strings.HasPrefix(trimmed, "├") || strings.HasPrefix(trimmed, "╰") {
			continue
		}
		if strings.ContainsRune(trimmed, '│') {
			count++
		}
	}
	return count
}
