package odt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testdataPath = filepath.Join("..", "internal", "testdata")

func TestWordWrap(t *testing.T) {
	input, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	expected, err := os.ReadFile(filepath.Join(testdataPath, "getting-started.odt"))
	require.NoError(t, err)

	var buf bytes.Buffer
	err = FromMarkdown(&buf, input)
	require.NoError(t, err)

	assert.Equal(t, expected, buf.Bytes())
}

// TestFromMarkdown_TaskList exercises the public FromMarkdown entry point
// with task list syntax. It confirms (a) the task-checkbox parser is wired,
// (b) the list is emitted with the bullet-suppressing TaskList style, and
// (c) the checkbox glyphs land in content.xml.
func TestFromMarkdown_TaskList(t *testing.T) {
	odt := renderODT(t, "- [ ] write tests\n- [x] ship feature\n")
	content := string(extractODFPart(t, odt, "content.xml"))

	assert.Contains(t, content, `<text:list text:style-name="TaskList"`,
		"task list should use the TaskList style instead of UnorderedList")
	assert.NotContains(t, content,
		`<text:list text:style-name="UnorderedList" text:continue-numbering="false">`+"\n"+`			<text:list-item>`+"\n"+`			<text:p text:style-name="Paragraph">☐`,
		"UnorderedList style should not be used for a pure task list")
	assert.Contains(t, content, "☐ write tests")
	assert.Contains(t, content, "✓ ship feature")
}
