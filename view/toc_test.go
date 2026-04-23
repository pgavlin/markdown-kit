package view

import (
	"testing"

	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
)

// newTestModelWithTOC builds a Model with a non-empty TOC-shaped document.
func newTestModelWithTOC(t *testing.T) *Model {
	t.Helper()
	md := `# Top

Intro paragraph.

## Section A

Text.

### Subsection A1

Text.

## Section B

Text.
`
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithGutter(true),
		WithWidth(80),
		WithHeight(25),
	)
	m.SetText("test.md", md)
	return &m
}

func TestTOC_InitiallyInactive(t *testing.T) {
	m := newTestModelWithTOC(t)
	assert.False(t, m.TOCActive())
}

func TestTOC_ClearResetsState(t *testing.T) {
	m := newTestModelWithTOC(t)
	m.toc.active = true
	m.Clear()
	assert.False(t, m.TOCActive())
	assert.Nil(t, m.toc.allEntries)
}
