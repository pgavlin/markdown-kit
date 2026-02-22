package styles

import "image/color"

// Trilean is a tri-state boolean: Yes, No, or Pass (inherit).
type Trilean uint8

const (
	Pass Trilean = iota
	Yes
	No
)

// TokenType identifies a syntactic element for styling purposes.
// Values match chroma.TokenType so that a simple cast converts between them.
type TokenType int

// Token type hierarchy uses decimal ranges:
//   - Categories are in ranges of 1000 (e.g. Keyword = 1000, Name = 2000)
//   - Sub-categories are in ranges of 100 (e.g. LiteralString = 3100)
//   - Individual types are offsets within their sub-category/category

// Meta token types.
const (
	Background TokenType = -1
	Error      TokenType = -7
	EOFType    TokenType = 0
)

// Keywords.
const (
	Keyword TokenType = 1000 + iota
	KeywordConstant
	KeywordDeclaration
	KeywordNamespace
	KeywordPseudo
	KeywordReserved
	KeywordType
)

// Names.
const (
	Name TokenType = 2000 + iota
	NameAttribute
	NameBuiltin
	NameBuiltinPseudo
	NameClass
	NameConstant
	NameDecorator
	NameEntity
	NameException
	NameFunction
	NameFunctionMagic
	NameKeyword
	NameLabel
	NameNamespace
	NameOperator
	NameOther
	NamePseudo
	NameProperty
	NameTag
)

// Literals.
const (
	Literal TokenType = 3000 + iota
	LiteralDate
)

// Strings.
const (
	LiteralString TokenType = 3100 + iota
	LiteralStringAffix
	LiteralStringAtom
	LiteralStringBacktick
	LiteralStringBoolean
	LiteralStringChar
	LiteralStringDelimiter
	LiteralStringDoc
	LiteralStringDouble
	LiteralStringEscape
	LiteralStringHeredoc
)

// Numbers.
const (
	LiteralNumber TokenType = 3200 + iota
)

// Operators.
const (
	Operator TokenType = 4000 + iota
)

// Punctuation.
const (
	Punctuation TokenType = 5000 + iota
)

// Comments.
const (
	Comment TokenType = 6000 + iota
)

// Generic tokens.
const (
	Generic TokenType = 7000 + iota
	GenericDeleted
	GenericEmph
	GenericError
	GenericHeading
	GenericInserted
	GenericOutput
	GenericPrompt
	GenericStrong
	GenericSubheading
	GenericTraceback
	GenericUnderline
)

// Text.
const (
	Text TokenType = 8000 + iota
)

func (t TokenType) category() TokenType {
	return t / 1000 * 1000
}

func (t TokenType) subCategory() TokenType {
	return t / 100 * 100
}

// StyleEntry defines the visual style for a single token type.
type StyleEntry struct {
	Colour     color.Color
	Background color.Color
	Bold       Trilean
	Italic     Trilean
	Underline  Trilean
}

// IsZero returns true if all fields are at their zero/nil/Pass values.
func (e StyleEntry) IsZero() bool {
	return e.Colour == nil && e.Background == nil && e.Bold == Pass && e.Italic == Pass && e.Underline == Pass
}

// inherit fills in unset fields from other.
func (e StyleEntry) inherit(other StyleEntry) StyleEntry {
	if e.Colour == nil {
		e.Colour = other.Colour
	}
	if e.Background == nil {
		e.Background = other.Background
	}
	if e.Bold == Pass {
		e.Bold = other.Bold
	}
	if e.Italic == Pass {
		e.Italic = other.Italic
	}
	if e.Underline == Pass {
		e.Underline = other.Underline
	}
	return e
}

// Theme maps token types to style entries.
type Theme struct {
	entries map[TokenType]StyleEntry
}

// NewTheme creates a new Theme from a map of token types to style entries.
func NewTheme(entries map[TokenType]StyleEntry) *Theme {
	return &Theme{entries: entries}
}

// Get returns the style entry for the given token type, inheriting from
// parent token types (sub-category, category, Text, Background) as needed.
// This matches chroma's Style.Get behavior.
func (t *Theme) Get(token TokenType) StyleEntry {
	entry := t.entries[token]
	// Inherit from most-specific to least-specific ancestor.
	if sub := token.subCategory(); sub != token {
		entry = entry.inherit(t.entries[sub])
	}
	if cat := token.category(); cat != token && cat != token.subCategory() {
		entry = entry.inherit(t.entries[cat])
	}
	entry = entry.inherit(t.entries[Text])
	entry = entry.inherit(t.entries[Background])
	return entry
}
