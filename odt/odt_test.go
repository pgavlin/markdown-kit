package odt

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testdataPath = filepath.Join("..", "internal", "testdata")

func TestWordWrap(t *testing.T) {
	input, err := ioutil.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	expected, err := ioutil.ReadFile(filepath.Join(testdataPath, "getting-started.odt"))
	require.NoError(t, err)

	var buf bytes.Buffer
	err = FromMarkdown(&buf, input)
	require.NoError(t, err)

	assert.Equal(t, expected, buf.Bytes())
}
