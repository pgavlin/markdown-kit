package view

import "github.com/alecthomas/chroma"

// Option configures a [Model] during construction.
type Option func(*Model)

// WithTheme sets the color theme for rendering.
func WithTheme(theme *chroma.Style) Option {
	return func(m *Model) {
		m.theme = theme
	}
}

// WithKeyMap sets the key bindings for the model.
func WithKeyMap(keyMap KeyMap) Option {
	return func(m *Model) {
		m.KeyMap = keyMap
	}
}

// WithWrap sets whether long lines should be wrapped.
func WithWrap(wrap bool) Option {
	return func(m *Model) {
		m.wrap = wrap
	}
}

// WithGutter sets whether to show the gutter with document name and position.
func WithGutter(showGutter bool) Option {
	return func(m *Model) {
		m.showGutter = showGutter
	}
}

// WithContentWidth sets the desired content width. 0 means use full viewport width.
func WithContentWidth(width int) Option {
	return func(m *Model) {
		m.contentWidth = width
	}
}

// WithWidth sets the viewport width.
func WithWidth(width int) Option {
	return func(m *Model) {
		m.width = width
	}
}

// WithHeight sets the viewport height.
func WithHeight(height int) Option {
	return func(m *Model) {
		m.height = height
	}
}
