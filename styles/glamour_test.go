package styles

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	"github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }

func TestParseColor(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.Nil(t, parseColor(nil))
	})
	t.Run("empty", func(t *testing.T) {
		s := ""
		assert.Nil(t, parseColor(&s))
	})
	t.Run("hex", func(t *testing.T) {
		s := "#C4C4C4"
		c := parseColor(&s)
		require.NotNil(t, c)
		assert.Equal(t, lipgloss.Color("#C4C4C4"), c)
	})
	t.Run("ansi256", func(t *testing.T) {
		s := "212"
		c := parseColor(&s)
		require.NotNil(t, c)
		assert.Equal(t, lipgloss.Color("212"), c)
	})
}

func TestColorToHex(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.Equal(t, "", colorToHex(nil))
	})
	t.Run("empty", func(t *testing.T) {
		assert.Equal(t, "", colorToHex(stringPtr("")))
	})
	t.Run("hex passthrough", func(t *testing.T) {
		assert.Equal(t, "#C4C4C4", colorToHex(stringPtr("#C4C4C4")))
	})
	t.Run("ansi256 to hex", func(t *testing.T) {
		// ANSI-256 color 252 is in the grayscale ramp: RGB(208,208,208) = #d0d0d0
		hex := colorToHex(stringPtr("252"))
		assert.Equal(t, "#d0d0d0", hex)
	})
	t.Run("ansi256 color 39", func(t *testing.T) {
		// Color 39 = 6x6x6 cube: r=0, g=3, b=5 → #00afff
		hex := colorToHex(stringPtr("39"))
		assert.Equal(t, "#00afff", hex)
	})
}

func TestBoolToTrilean(t *testing.T) {
	assert.Equal(t, Pass, boolToTrilean(nil))
	assert.Equal(t, Yes, boolToTrilean(boolPtr(true)))
	assert.Equal(t, No, boolToTrilean(boolPtr(false)))
}

func TestFromStyleConfigEmpty(t *testing.T) {
	theme := FromStyleConfig(ansi.StyleConfig{})
	entry := theme.Get(Text)
	assert.Nil(t, entry.Colour)
	assert.Equal(t, Pass, entry.Bold)
}

func TestFromStyleConfigNilChroma(t *testing.T) {
	cfg := ansi.StyleConfig{
		Emph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
	}
	theme := FromStyleConfig(cfg)
	entry := theme.Get(GenericEmph)
	assert.Equal(t, Yes, entry.Italic)
}

func TestFromStyleConfigDark(t *testing.T) {
	theme := FromStyleConfig(glamourstyles.DarkStyleConfig)

	t.Run("text from chroma", func(t *testing.T) {
		entry := theme.entries[Text]
		require.NotNil(t, entry.Colour)
		assert.Equal(t, lipgloss.Color("#C4C4C4"), entry.Colour)
	})

	t.Run("heading bold", func(t *testing.T) {
		entry := theme.entries[GenericHeading]
		assert.Equal(t, Yes, entry.Bold)
		require.NotNil(t, entry.Colour, "heading should have a color")
	})

	t.Run("emph italic", func(t *testing.T) {
		entry := theme.entries[GenericEmph]
		assert.Equal(t, Yes, entry.Italic)
	})

	t.Run("strong bold", func(t *testing.T) {
		entry := theme.entries[GenericStrong]
		assert.Equal(t, Yes, entry.Bold)
	})

	t.Run("link underline", func(t *testing.T) {
		entry := theme.entries[GenericUnderline]
		assert.Equal(t, Yes, entry.Underline)
		require.NotNil(t, entry.Colour)
	})

	t.Run("code span", func(t *testing.T) {
		entry := theme.entries[TokenType(CodeSpan)]
		require.NotNil(t, entry.Colour)
		require.NotNil(t, entry.Background)
	})

	t.Run("code block fence", func(t *testing.T) {
		entry := theme.entries[LiteralStringHeredoc]
		require.NotNil(t, entry.Colour)
		// Code block should have the Chroma.Background merged in.
		require.NotNil(t, entry.Background, "code block should have background from Chroma.Background")
	})

	t.Run("chroma keyword", func(t *testing.T) {
		entry := theme.entries[Keyword]
		require.NotNil(t, entry.Colour)
		assert.Equal(t, lipgloss.Color("#00AAFF"), entry.Colour)
	})

	t.Run("background token", func(t *testing.T) {
		// glamour-dark has no Document.BackgroundColor, so the Background
		// token should not have a background color (code block bg should
		// NOT leak here).
		entry := theme.entries[Background]
		assert.Nil(t, entry.Background, "Background token should not inherit code block bg")
	})
}

func TestFromStyleConfigStrongEmph(t *testing.T) {
	cfg := ansi.StyleConfig{
		Emph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
	}
	theme := FromStyleConfig(cfg)
	entry := theme.entries[TokenType(StrongEmph)]
	assert.Equal(t, Yes, entry.Bold, "StrongEmph should inherit bold from Strong")
	assert.Equal(t, Yes, entry.Italic, "StrongEmph should inherit italic from Emph")
}

func TestFromStyleConfigStructuralOverridesChroma(t *testing.T) {
	cfg := ansi.StyleConfig{
		Emph: ansi.StylePrimitive{
			Italic: boolPtr(true),
			Color:  stringPtr("#FF0000"),
		},
		CodeBlock: ansi.StyleCodeBlock{
			Chroma: &ansi.Chroma{
				GenericEmph: ansi.StylePrimitive{
					Italic: boolPtr(true),
					Color:  stringPtr("#00FF00"),
				},
			},
		},
	}
	theme := FromStyleConfig(cfg)
	entry := theme.entries[GenericEmph]
	assert.Equal(t, lipgloss.Color("#FF0000"), entry.Colour)
	assert.Equal(t, Yes, entry.Italic)
}

// ChromaStyleFromConfig tests

func TestChromaStyleFromConfigEmpty(t *testing.T) {
	style := ChromaStyleFromConfig("test-empty", ansi.StyleConfig{})
	require.NotNil(t, style)
	assert.Equal(t, "test-empty", style.Name)
}

func TestChromaStyleFromConfigDark(t *testing.T) {
	style := GlamourDark
	require.NotNil(t, style)
	assert.Equal(t, "glamour-dark", style.Name)

	t.Run("chroma keyword color", func(t *testing.T) {
		entry := style.Get(chroma.Keyword)
		assert.True(t, entry.Colour.IsSet(), "keyword should have a color")
	})

	t.Run("heading bold", func(t *testing.T) {
		entry := style.Get(chroma.GenericHeading)
		assert.True(t, entry.Bold == chroma.Yes)
	})

	t.Run("emph italic", func(t *testing.T) {
		entry := style.Get(chroma.GenericEmph)
		assert.True(t, entry.Italic == chroma.Yes)
	})

	t.Run("strong bold", func(t *testing.T) {
		entry := style.Get(chroma.GenericStrong)
		assert.True(t, entry.Bold == chroma.Yes)
	})

	t.Run("strong emph", func(t *testing.T) {
		entry := style.Get(StrongEmph)
		assert.True(t, entry.Bold == chroma.Yes)
		assert.True(t, entry.Italic == chroma.Yes)
	})

	t.Run("link underline", func(t *testing.T) {
		entry := style.Get(chroma.GenericUnderline)
		assert.True(t, entry.Underline == chroma.Yes)
	})

	t.Run("code span has background", func(t *testing.T) {
		entry := style.Get(CodeSpan)
		assert.True(t, entry.Background.IsSet())
	})

	t.Run("code block fence color", func(t *testing.T) {
		entry := style.Get(chroma.LiteralStringHeredoc)
		assert.True(t, entry.Colour.IsSet())
	})

	t.Run("document text color via Generic", func(t *testing.T) {
		entry := style.Get(chroma.Generic)
		assert.True(t, entry.Colour.IsSet())
	})
}

func TestAllGlamourThemesRegistered(t *testing.T) {
	themes := []struct {
		name  string
		style *chroma.Style
	}{
		{"glamour-dark", GlamourDark},
		{"glamour-light", GlamourLight},
		{"glamour-dracula", GlamourDracula},
		{"glamour-tokyo-night", GlamourTokyoNight},
		{"glamour-pink", GlamourPink},
	}
	for _, tt := range themes {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.style, "theme var should not be nil")
			assert.Equal(t, tt.name, tt.style.Name)
		})
	}
}

func TestGlamourThemesHaveHeadingStyle(t *testing.T) {
	// All styled glamour themes (except ASCII) define a heading style.
	themes := []struct {
		name  string
		style *chroma.Style
	}{
		{"glamour-dark", GlamourDark},
		{"glamour-light", GlamourLight},
		{"glamour-dracula", GlamourDracula},
		{"glamour-tokyo-night", GlamourTokyoNight},
		{"glamour-pink", GlamourPink},
	}
	for _, tt := range themes {
		t.Run(tt.name, func(t *testing.T) {
			entry := tt.style.Get(chroma.GenericHeading)
			assert.True(t, entry.Bold == chroma.Yes || entry.Colour.IsSet(),
				"heading should have bold or color set")
		})
	}
}

func TestGlamourDraculaHasChromaColors(t *testing.T) {
	style := GlamourDracula
	// Dracula theme has a full chroma section.
	entry := style.Get(chroma.Keyword)
	assert.True(t, entry.Colour.IsSet(), "Dracula should have keyword color")
}

func TestGlamourTokyoNightHasChromaColors(t *testing.T) {
	style := GlamourTokyoNight
	entry := style.Get(chroma.Keyword)
	assert.True(t, entry.Colour.IsSet(), "Tokyo Night should have keyword color")
}
