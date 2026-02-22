package styles

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// TokenType.category()
// ---------------------------------------------------------------------------

func TestTokenType_category(t *testing.T) {
	tests := []struct {
		name     string
		token    TokenType
		expected TokenType
	}{
		{"Keyword is its own category", Keyword, Keyword},
		{"KeywordConstant (1001) → Keyword (1000)", KeywordConstant, Keyword},
		{"KeywordType (1006) → Keyword (1000)", KeywordType, Keyword},
		{"Name is its own category", Name, Name},
		{"NameTag (2018) → Name (2000)", NameTag, Name},
		{"Literal is its own category", Literal, Literal},
		{"LiteralString (3100) → Literal (3000)", LiteralString, Literal},
		{"LiteralStringEscape (3109) → Literal (3000)", LiteralStringEscape, Literal},
		{"LiteralNumber (3200) → Literal (3000)", LiteralNumber, Literal},
		{"Comment is its own category", Comment, Comment},
		{"Generic is its own category", Generic, Generic},
		{"GenericHeading (7004) → Generic (7000)", GenericHeading, Generic},
		{"Text is its own category", Text, Text},
		{"Background (-1) category is 0", Background, TokenType(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.token.category())
		})
	}
}

// ---------------------------------------------------------------------------
// TokenType.subCategory()
// ---------------------------------------------------------------------------

func TestTokenType_subCategory(t *testing.T) {
	tests := []struct {
		name     string
		token    TokenType
		expected TokenType
	}{
		{"Keyword (1000) sub-category is itself", Keyword, Keyword},
		{"KeywordConstant (1001) → Keyword (1000)", KeywordConstant, Keyword},
		{"LiteralString (3100) is its own sub-category", LiteralString, LiteralString},
		{"LiteralStringEscape (3109) → LiteralString (3100)", LiteralStringEscape, LiteralString},
		{"LiteralNumber (3200) is its own sub-category", LiteralNumber, LiteralNumber},
		{"GenericHeading (7004) → Generic (7000)", GenericHeading, Generic},
		{"Text (8000) sub-category is itself", Text, Text},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.token.subCategory())
		})
	}
}

// ---------------------------------------------------------------------------
// StyleEntry.IsZero()
// ---------------------------------------------------------------------------

func TestStyleEntry_IsZero(t *testing.T) {
	t.Run("empty entry is zero", func(t *testing.T) {
		assert.True(t, StyleEntry{}.IsZero())
	})

	t.Run("all Pass trileans is zero", func(t *testing.T) {
		e := StyleEntry{Bold: Pass, Italic: Pass, Underline: Pass}
		assert.True(t, e.IsZero())
	})

	t.Run("non-nil Colour makes it non-zero", func(t *testing.T) {
		e := StyleEntry{Colour: color.White}
		assert.False(t, e.IsZero())
	})

	t.Run("non-nil Background makes it non-zero", func(t *testing.T) {
		e := StyleEntry{Background: color.Black}
		assert.False(t, e.IsZero())
	})

	t.Run("Bold=Yes makes it non-zero", func(t *testing.T) {
		e := StyleEntry{Bold: Yes}
		assert.False(t, e.IsZero())
	})

	t.Run("Bold=No makes it non-zero", func(t *testing.T) {
		e := StyleEntry{Bold: No}
		assert.False(t, e.IsZero())
	})

	t.Run("Italic=Yes makes it non-zero", func(t *testing.T) {
		e := StyleEntry{Italic: Yes}
		assert.False(t, e.IsZero())
	})

	t.Run("Underline=Yes makes it non-zero", func(t *testing.T) {
		e := StyleEntry{Underline: Yes}
		assert.False(t, e.IsZero())
	})

	t.Run("fully populated entry is non-zero", func(t *testing.T) {
		e := StyleEntry{
			Colour:     color.RGBA{R: 255, A: 255},
			Background: color.RGBA{B: 128, A: 255},
			Bold:       Yes,
			Italic:     No,
			Underline:  Yes,
		}
		assert.False(t, e.IsZero())
	})
}

// ---------------------------------------------------------------------------
// StyleEntry.inherit()
// ---------------------------------------------------------------------------

func TestStyleEntry_inherit(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	green := color.RGBA{G: 255, A: 255}
	black := color.RGBA{A: 255}

	t.Run("empty entry inherits all fields from parent", func(t *testing.T) {
		parent := StyleEntry{
			Colour:     red,
			Background: black,
			Bold:       Yes,
			Italic:     No,
			Underline:  Yes,
		}
		child := StyleEntry{}
		result := child.inherit(parent)

		assert.Equal(t, red, result.Colour)
		assert.Equal(t, black, result.Background)
		assert.Equal(t, Yes, result.Bold)
		assert.Equal(t, No, result.Italic)
		assert.Equal(t, Yes, result.Underline)
	})

	t.Run("set fields are preserved over parent", func(t *testing.T) {
		parent := StyleEntry{
			Colour:     red,
			Background: black,
			Bold:       Yes,
			Italic:     Yes,
			Underline:  Yes,
		}
		child := StyleEntry{
			Colour:    blue,
			Bold:      No,
			Underline: No,
		}
		result := child.inherit(parent)

		assert.Equal(t, blue, result.Colour, "child Colour preserved")
		assert.Equal(t, black, result.Background, "Background inherited from parent")
		assert.Equal(t, No, result.Bold, "child Bold preserved")
		assert.Equal(t, Yes, result.Italic, "Italic inherited from parent")
		assert.Equal(t, No, result.Underline, "child Underline preserved")
	})

	t.Run("inheriting from empty parent is identity", func(t *testing.T) {
		child := StyleEntry{
			Colour:     green,
			Background: blue,
			Bold:       Yes,
			Italic:     No,
			Underline:  Yes,
		}
		result := child.inherit(StyleEntry{})

		assert.Equal(t, child, result)
	})

	t.Run("both empty produces empty", func(t *testing.T) {
		result := StyleEntry{}.inherit(StyleEntry{})
		assert.True(t, result.IsZero())
	})

	t.Run("inherit does not mutate receiver", func(t *testing.T) {
		child := StyleEntry{Bold: Pass}
		parent := StyleEntry{Bold: Yes}
		_ = child.inherit(parent)

		// The original child should be unchanged (inherit is on a value receiver).
		assert.Equal(t, Pass, child.Bold)
	})
}

// ---------------------------------------------------------------------------
// NewTheme() and Theme.Get() basics
// ---------------------------------------------------------------------------

func TestNewTheme(t *testing.T) {
	entries := map[TokenType]StyleEntry{
		Text: {Colour: color.White},
	}
	theme := NewTheme(entries)
	assert.NotNil(t, theme)
}

func TestTheme_Get_directEntry(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	entries := map[TokenType]StyleEntry{
		Keyword: {Colour: red, Bold: Yes},
	}
	theme := NewTheme(entries)

	result := theme.Get(Keyword)
	assert.Equal(t, red, result.Colour)
	assert.Equal(t, Yes, result.Bold)
}

// ---------------------------------------------------------------------------
// Theme.Get() inheritance chain
// ---------------------------------------------------------------------------

func TestTheme_Get_inheritanceChain(t *testing.T) {
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	darkBg := color.RGBA{R: 18, G: 18, B: 18, A: 255}
	orange := color.RGBA{R: 255, G: 175, B: 95, A: 255}
	teal := color.RGBA{R: 0, G: 215, B: 175, A: 255}
	gray := color.RGBA{R: 95, G: 95, B: 135, A: 255}

	entries := map[TokenType]StyleEntry{
		Background:          {Background: darkBg},
		Text:                {Colour: white},
		Literal:             {Colour: teal},
		LiteralString:       {Colour: orange, Italic: Yes},
		LiteralStringEscape: {Colour: gray},
	}
	theme := NewTheme(entries)

	t.Run("token inherits from sub-category, category, Text, Background", func(t *testing.T) {
		result := theme.Get(LiteralStringEscape)

		// Colour from LiteralStringEscape itself.
		assert.Equal(t, gray, result.Colour)
		// Italic from LiteralString (sub-category).
		assert.Equal(t, Yes, result.Italic)
		// Bold is Pass everywhere, remains Pass.
		assert.Equal(t, Pass, result.Bold)
		// Background from Background entry.
		assert.Equal(t, darkBg, result.Background)
	})

	t.Run("sub-category inherits from category, Text, Background", func(t *testing.T) {
		result := theme.Get(LiteralString)

		assert.Equal(t, orange, result.Colour)
		assert.Equal(t, Yes, result.Italic)
		assert.Equal(t, darkBg, result.Background)
	})

	t.Run("category inherits from Text and Background", func(t *testing.T) {
		result := theme.Get(Literal)

		assert.Equal(t, teal, result.Colour)
		assert.Equal(t, darkBg, result.Background)
	})

	t.Run("Text inherits Background", func(t *testing.T) {
		result := theme.Get(Text)

		assert.Equal(t, white, result.Colour)
		assert.Equal(t, darkBg, result.Background)
	})
}

func TestTheme_Get_fallbackToParents(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	bgColor := color.RGBA{R: 30, G: 30, B: 30, A: 255}

	entries := map[TokenType]StyleEntry{
		Background: {Background: bgColor},
		Text:       {Colour: red, Bold: No},
		Keyword:    {Italic: Yes},
	}
	theme := NewTheme(entries)

	t.Run("undefined token with defined category", func(t *testing.T) {
		// KeywordConstant has no direct entry, but its category Keyword does.
		result := theme.Get(KeywordConstant)

		// Italic from Keyword (category).
		assert.Equal(t, Yes, result.Italic)
		// Colour from Text (no category/sub-category defines it).
		assert.Equal(t, red, result.Colour)
		// Bold from Text.
		assert.Equal(t, No, result.Bold)
		// Background from Background.
		assert.Equal(t, bgColor, result.Background)
	})

	t.Run("completely undefined token falls back to Text and Background", func(t *testing.T) {
		// Comment has no direct entry, no sub-category, no category entry.
		result := theme.Get(Comment)

		assert.Equal(t, red, result.Colour)
		assert.Equal(t, No, result.Bold)
		assert.Equal(t, bgColor, result.Background)
	})

	t.Run("empty theme returns zero entry", func(t *testing.T) {
		emptyTheme := NewTheme(map[TokenType]StyleEntry{})
		result := emptyTheme.Get(Keyword)
		assert.True(t, result.IsZero())
	})

	t.Run("only Background entry set", func(t *testing.T) {
		theme := NewTheme(map[TokenType]StyleEntry{
			Background: {Background: blue},
		})
		result := theme.Get(Name)
		assert.Nil(t, result.Colour)
		assert.Equal(t, blue, result.Background)
	})
}

// ---------------------------------------------------------------------------
// Theme.Get() verifies sub-category vs category distinction
// ---------------------------------------------------------------------------

func TestTheme_Get_subCategoryDistinctFromCategory(t *testing.T) {
	// For LiteralStringEscape: sub-category is LiteralString (3100),
	// category is Literal (3000). Both should be consulted.
	catColor := color.RGBA{R: 100, A: 255}
	subCatColor := color.RGBA{G: 100, A: 255}

	entries := map[TokenType]StyleEntry{
		Literal:       {Bold: Yes, Colour: catColor},
		LiteralString: {Italic: Yes, Colour: subCatColor},
	}
	theme := NewTheme(entries)

	result := theme.Get(LiteralStringEscape)

	// Colour from LiteralString (sub-category, more specific).
	assert.Equal(t, subCatColor, result.Colour)
	// Italic from LiteralString.
	assert.Equal(t, Yes, result.Italic)
	// Bold from Literal (category).
	assert.Equal(t, Yes, result.Bold)
}

// For tokens where subCategory == category (e.g. KeywordConstant: sub=1000, cat=1000),
// the category inherit step is skipped to avoid double-inheriting.
func TestTheme_Get_subCategoryEqualsCategory(t *testing.T) {
	kwColor := color.RGBA{R: 175, G: 135, B: 175, A: 255}

	entries := map[TokenType]StyleEntry{
		Keyword: {Colour: kwColor, Bold: Yes},
	}
	theme := NewTheme(entries)

	result := theme.Get(KeywordConstant)

	// KeywordConstant's subCategory is Keyword (1000), same as category (1000).
	// Should still inherit from Keyword.
	assert.Equal(t, kwColor, result.Colour)
	assert.Equal(t, Yes, result.Bold)
}

// ---------------------------------------------------------------------------
// Trilean type values
// ---------------------------------------------------------------------------

func TestTrilean_values(t *testing.T) {
	assert.Equal(t, Trilean(0), Pass)
	assert.Equal(t, Trilean(1), Yes)
	assert.Equal(t, Trilean(2), No)
}

// ---------------------------------------------------------------------------
// Pulumi theme smoke test
// ---------------------------------------------------------------------------

func TestPulumi_isNotNil(t *testing.T) {
	assert.NotNil(t, Pulumi, "Pulumi theme should be registered and non-nil")
}

func TestGlamourDark_isNotNil(t *testing.T) {
	assert.NotNil(t, GlamourDark, "GlamourDark theme should be registered and non-nil")
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestTheme_Get_backgroundToken(t *testing.T) {
	bg := color.RGBA{R: 18, G: 18, B: 18, A: 255}
	entries := map[TokenType]StyleEntry{
		Background: {Background: bg, Bold: No},
	}
	theme := NewTheme(entries)

	// Background token category: -1 / 1000 * 1000 = 0
	// Background token subCategory: -1 / 100 * 100 = 0
	// So sub != token, cat != token, cat == sub → category step skipped.
	// Then inherits from Text (empty) and Background (itself).
	result := theme.Get(Background)
	assert.Equal(t, bg, result.Background)
	assert.Equal(t, No, result.Bold)
}

func TestTheme_Get_textToken(t *testing.T) {
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	bg := color.RGBA{A: 255}

	entries := map[TokenType]StyleEntry{
		Background: {Background: bg},
		Text:       {Colour: white, Bold: Yes},
	}
	theme := NewTheme(entries)

	// Text (8000): subCategory = 8000, category = 8000 → both equal token.
	// Inherits from Text (itself) and Background.
	result := theme.Get(Text)
	assert.Equal(t, white, result.Colour)
	assert.Equal(t, Yes, result.Bold)
	assert.Equal(t, bg, result.Background)
}
