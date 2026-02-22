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
