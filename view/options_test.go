package view

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
)

func TestWithTheme(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi))
	assert.Equal(t, styles.Pulumi, m.theme)
}

func TestWithTheme_Nil(t *testing.T) {
	m := NewModel(WithTheme(nil))
	assert.Nil(t, m.theme)
}

func TestWithKeyMap(t *testing.T) {
	km := DefaultKeyMap()
	km.Down = key.NewBinding(key.WithKeys("n"))
	m := NewModel(WithKeyMap(km))
	assert.Equal(t, km, m.KeyMap)
}

func TestWithWrap(t *testing.T) {
	// Default is true; setting false should take effect.
	m := NewModel(WithWrap(false))
	assert.False(t, m.wrap)
}

func TestWithWrap_True(t *testing.T) {
	m := NewModel(WithWrap(true))
	assert.True(t, m.wrap)
}

func TestWithGutter(t *testing.T) {
	m := NewModel(WithGutter(true))
	assert.True(t, m.showGutter)
}

func TestWithGutter_False(t *testing.T) {
	m := NewModel(WithGutter(false))
	assert.False(t, m.showGutter)
}

func TestWithContentWidth(t *testing.T) {
	m := NewModel(WithContentWidth(120))
	assert.Equal(t, 120, m.contentWidth)
}

func TestWithContentWidth_Zero(t *testing.T) {
	m := NewModel(WithContentWidth(0))
	assert.Equal(t, 0, m.contentWidth)
}

func TestWithWidth(t *testing.T) {
	m := NewModel(WithWidth(100))
	assert.Equal(t, 100, m.width)
}

func TestWithHeight(t *testing.T) {
	m := NewModel(WithHeight(50))
	assert.Equal(t, 50, m.height)
}

func TestMultipleOptions(t *testing.T) {
	m := NewModel(
		WithTheme(styles.Pulumi),
		WithWrap(false),
		WithGutter(true),
		WithContentWidth(160),
		WithWidth(200),
		WithHeight(40),
	)
	assert.Equal(t, styles.Pulumi, m.theme)
	assert.False(t, m.wrap)
	assert.True(t, m.showGutter)
	assert.Equal(t, 160, m.contentWidth)
	assert.Equal(t, 200, m.width)
	assert.Equal(t, 40, m.height)
}

func TestNoOptions_Defaults(t *testing.T) {
	m := NewModel()
	assert.Nil(t, m.theme)
	assert.True(t, m.wrap)
	assert.False(t, m.showGutter)
	assert.Equal(t, 0, m.contentWidth)
	assert.Equal(t, 0, m.width)
	assert.Equal(t, 0, m.height)
	assert.Equal(t, DefaultKeyMap(), m.KeyMap)
}

func TestOptionOrder_LastWins(t *testing.T) {
	m := NewModel(WithContentWidth(80), WithContentWidth(120))
	assert.Equal(t, 120, m.contentWidth)
}
