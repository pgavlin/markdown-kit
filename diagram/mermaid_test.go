package diagram

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMermaidRendererFlowchart(t *testing.T) {
	r := MermaidRenderer()
	output, err := r("mermaid", []byte("graph LR\n    A-->B\n    B-->C\n"))
	require.NoError(t, err)

	// The output should contain box-drawing characters.
	assert.True(t, strings.ContainsRune(output, '┌') || strings.ContainsRune(output, '╭'),
		"output should contain box-drawing corners: %s", output)
	assert.True(t, strings.Contains(output, "A"), "output should contain node A: %s", output)
	assert.True(t, strings.Contains(output, "B"), "output should contain node B: %s", output)
	assert.True(t, strings.Contains(output, "C"), "output should contain node C: %s", output)
}

func TestMermaidRendererUnsupportedLanguage(t *testing.T) {
	r := MermaidRenderer()
	_, err := r("python", []byte("print('hello')"))
	assert.Error(t, err, "should return error for unsupported language")
	assert.Contains(t, err.Error(), "unsupported diagram language")
}
