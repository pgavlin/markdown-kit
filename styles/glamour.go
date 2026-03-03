package styles

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma"
	chromaStyles "github.com/alecthomas/chroma/styles"
	"github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
)

// parseColor converts a glamour color string (hex or ANSI-256) to a color.Color.
// Returns nil if the pointer is nil or the string is empty.
func parseColor(s *string) color.Color {
	if s == nil || *s == "" {
		return nil
	}
	return lipgloss.Color(*s)
}

// boolToTrilean converts a *bool to a Trilean.
func boolToTrilean(b *bool) Trilean {
	if b == nil {
		return Pass
	}
	if *b {
		return Yes
	}
	return No
}

// primitiveToEntry converts a glamour StylePrimitive to a StyleEntry.
func primitiveToEntry(p ansi.StylePrimitive) StyleEntry {
	return StyleEntry{
		Colour:     parseColor(p.Color),
		Background: parseColor(p.BackgroundColor),
		Bold:       boolToTrilean(p.Bold),
		Italic:     boolToTrilean(p.Italic),
		Underline:  boolToTrilean(p.Underline),
	}
}

// overlay merges b onto a: non-zero fields in b override those in a.
func overlay(a, b StyleEntry) StyleEntry {
	if b.Colour != nil {
		a.Colour = b.Colour
	}
	if b.Background != nil {
		a.Background = b.Background
	}
	if b.Bold != Pass {
		a.Bold = b.Bold
	}
	if b.Italic != Pass {
		a.Italic = b.Italic
	}
	if b.Underline != Pass {
		a.Underline = b.Underline
	}
	return a
}

// FromStyleConfig converts a glamour ansi.StyleConfig into a *Theme.
//
// Chroma syntax-highlighting entries are applied first (lower priority),
// then structural markdown fields are overlaid on top.
func FromStyleConfig(cfg ansi.StyleConfig) *Theme {
	entries := map[TokenType]StyleEntry{}

	set := func(tok TokenType, entry StyleEntry) {
		if !entry.IsZero() {
			entries[tok] = entry
		}
	}

	// Chroma entries (lower priority).
	if c := cfg.CodeBlock.Chroma; c != nil {
		set(Text, primitiveToEntry(c.Text))
		set(Error, primitiveToEntry(c.Error))
		set(Comment, primitiveToEntry(c.Comment))
		set(Keyword, primitiveToEntry(c.Keyword))
		set(KeywordReserved, primitiveToEntry(c.KeywordReserved))
		set(KeywordNamespace, primitiveToEntry(c.KeywordNamespace))
		set(KeywordType, primitiveToEntry(c.KeywordType))
		set(Operator, primitiveToEntry(c.Operator))
		set(Punctuation, primitiveToEntry(c.Punctuation))
		set(Name, primitiveToEntry(c.Name))
		set(NameBuiltin, primitiveToEntry(c.NameBuiltin))
		set(NameTag, primitiveToEntry(c.NameTag))
		set(NameAttribute, primitiveToEntry(c.NameAttribute))
		set(NameClass, primitiveToEntry(c.NameClass))
		set(NameConstant, primitiveToEntry(c.NameConstant))
		set(NameDecorator, primitiveToEntry(c.NameDecorator))
		set(NameException, primitiveToEntry(c.NameException))
		set(NameFunction, primitiveToEntry(c.NameFunction))
		set(NameOther, primitiveToEntry(c.NameOther))
		set(Literal, primitiveToEntry(c.Literal))
		set(LiteralNumber, primitiveToEntry(c.LiteralNumber))
		set(LiteralDate, primitiveToEntry(c.LiteralDate))
		set(LiteralString, primitiveToEntry(c.LiteralString))
		set(LiteralStringEscape, primitiveToEntry(c.LiteralStringEscape))
		set(GenericDeleted, primitiveToEntry(c.GenericDeleted))
		set(GenericEmph, primitiveToEntry(c.GenericEmph))
		set(GenericInserted, primitiveToEntry(c.GenericInserted))
		set(GenericStrong, primitiveToEntry(c.GenericStrong))
		set(GenericSubheading, primitiveToEntry(c.GenericSubheading))
		// Note: c.Background is intentionally NOT mapped to the Background
		// token here. In glamour configs, Chroma.Background is the code
		// block syntax-highlighting background. The Background token is
		// inherited by ALL tokens via Get(), so placing the code block bg
		// there would leak it into the entire document. Instead, we merge
		// it into LiteralStringHeredoc (the code block style) below.
	}

	// Structural markdown fields (higher priority — overlay on chroma).

	// Document colors → Background token (inherited by all tokens).
	if docColor := parseColor(cfg.Document.Color); docColor != nil {
		bg := entries[Background]
		bg.Colour = docColor
		entries[Background] = bg
	}
	if docBgColor := parseColor(cfg.Document.BackgroundColor); docBgColor != nil {
		bg := entries[Background]
		bg.Background = docBgColor
		entries[Background] = bg
	}

	// Text.
	if e := primitiveToEntry(cfg.Text); !e.IsZero() {
		entries[Text] = overlay(entries[Text], e)
	}

	// Headings: H1/H2 → GenericHeading, H3+ → GenericSubheading.
	headingBase := primitiveToEntry(cfg.Heading.StylePrimitive)
	h1 := overlay(headingBase, primitiveToEntry(cfg.H1.StylePrimitive))
	if !h1.IsZero() {
		entries[GenericHeading] = overlay(entries[GenericHeading], h1)
	}

	h3 := overlay(headingBase, primitiveToEntry(cfg.H3.StylePrimitive))
	if !h3.IsZero() {
		entries[GenericSubheading] = overlay(entries[GenericSubheading], h3)
	}

	// Emph → GenericEmph.
	emphEntry := primitiveToEntry(cfg.Emph)
	if !emphEntry.IsZero() {
		entries[GenericEmph] = overlay(entries[GenericEmph], emphEntry)
	}

	// Strong → GenericStrong.
	strongEntry := primitiveToEntry(cfg.Strong)
	if !strongEntry.IsZero() {
		entries[GenericStrong] = overlay(entries[GenericStrong], strongEntry)
	}

	// StrongEmph = merge of Emph + Strong.
	strongEmph := overlay(emphEntry, strongEntry)
	set(TokenType(StrongEmph), strongEmph)

	// Strikethrough → GenericDeleted.
	if e := primitiveToEntry(cfg.Strikethrough); !e.IsZero() {
		entries[GenericDeleted] = overlay(entries[GenericDeleted], e)
	}

	// Link → GenericUnderline.
	set(GenericUnderline, primitiveToEntry(cfg.Link))

	// Code (inline) → CodeSpan.
	set(TokenType(CodeSpan), primitiveToEntry(cfg.Code.StylePrimitive))

	// CodeBlock → LiteralStringHeredoc. Merge the Chroma.Background (code
	// block syntax-highlighting background) into the code block style so it
	// applies only inside code blocks, not the entire document.
	codeBlockEntry := primitiveToEntry(cfg.CodeBlock.StylePrimitive)
	if c := cfg.CodeBlock.Chroma; c != nil {
		codeBlockEntry = overlay(codeBlockEntry, primitiveToEntry(c.Background))
	}
	set(LiteralStringHeredoc, codeBlockEntry)

	// Table.
	set(TokenType(Table), primitiveToEntry(cfg.Table.StylePrimitive))

	return NewTheme(entries)
}

// colorToHex converts a glamour color string to a chroma-compatible hex string.
// Hex strings pass through; ANSI-256 numbers are resolved to hex via lipgloss.
func colorToHex(s *string) string {
	if s == nil || *s == "" {
		return ""
	}
	v := *s
	if strings.HasPrefix(v, "#") {
		return v
	}
	// ANSI-256 number — resolve to hex.
	c := lipgloss.Color(v)
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// primitiveToChromaString converts a glamour StylePrimitive to a chroma style string
// (e.g. "#C4C4C4 bg:#303030 bold italic").
func primitiveToChromaString(p ansi.StylePrimitive) string {
	var parts []string
	if c := colorToHex(p.Color); c != "" {
		parts = append(parts, c)
	}
	if bg := colorToHex(p.BackgroundColor); bg != "" {
		parts = append(parts, "bg:"+bg)
	}
	if p.Bold != nil && *p.Bold {
		parts = append(parts, "bold")
	}
	if p.Italic != nil && *p.Italic {
		parts = append(parts, "italic")
	}
	if p.Underline != nil && *p.Underline {
		parts = append(parts, "underline")
	}
	return strings.Join(parts, " ")
}

// mergePrimitive returns a copy of base with non-nil fields from specific overlaid.
func mergePrimitive(base, specific ansi.StylePrimitive) ansi.StylePrimitive {
	result := base
	if specific.Color != nil {
		result.Color = specific.Color
	}
	if specific.BackgroundColor != nil {
		result.BackgroundColor = specific.BackgroundColor
	}
	if specific.Bold != nil {
		result.Bold = specific.Bold
	}
	if specific.Italic != nil {
		result.Italic = specific.Italic
	}
	if specific.Underline != nil {
		result.Underline = specific.Underline
	}
	return result
}

// ChromaStyleFromConfig converts a glamour ansi.StyleConfig into a *chroma.Style
// registered under the given name. This produces the same format used by the
// renderer's WithTheme option.
func ChromaStyleFromConfig(name string, cfg ansi.StyleConfig) *chroma.Style {
	entries := chroma.StyleEntries{}

	set := func(tok chroma.TokenType, style string) {
		if style != "" {
			entries[tok] = style
		}
	}

	// Chroma syntax-highlighting entries.
	if c := cfg.CodeBlock.Chroma; c != nil {
		set(chroma.Text, primitiveToChromaString(c.Text))
		set(chroma.Error, primitiveToChromaString(c.Error))
		set(chroma.Comment, primitiveToChromaString(c.Comment))
		set(chroma.CommentPreproc, primitiveToChromaString(c.CommentPreproc))
		set(chroma.Keyword, primitiveToChromaString(c.Keyword))
		set(chroma.KeywordReserved, primitiveToChromaString(c.KeywordReserved))
		set(chroma.KeywordNamespace, primitiveToChromaString(c.KeywordNamespace))
		set(chroma.KeywordType, primitiveToChromaString(c.KeywordType))
		set(chroma.Operator, primitiveToChromaString(c.Operator))
		set(chroma.Punctuation, primitiveToChromaString(c.Punctuation))
		set(chroma.Name, primitiveToChromaString(c.Name))
		set(chroma.NameBuiltin, primitiveToChromaString(c.NameBuiltin))
		set(chroma.NameTag, primitiveToChromaString(c.NameTag))
		set(chroma.NameAttribute, primitiveToChromaString(c.NameAttribute))
		set(chroma.NameClass, primitiveToChromaString(c.NameClass))
		set(chroma.NameConstant, primitiveToChromaString(c.NameConstant))
		set(chroma.NameDecorator, primitiveToChromaString(c.NameDecorator))
		set(chroma.NameException, primitiveToChromaString(c.NameException))
		set(chroma.NameFunction, primitiveToChromaString(c.NameFunction))
		set(chroma.NameOther, primitiveToChromaString(c.NameOther))
		set(chroma.Literal, primitiveToChromaString(c.Literal))
		set(chroma.LiteralNumber, primitiveToChromaString(c.LiteralNumber))
		set(chroma.LiteralDate, primitiveToChromaString(c.LiteralDate))
		set(chroma.LiteralString, primitiveToChromaString(c.LiteralString))
		set(chroma.LiteralStringEscape, primitiveToChromaString(c.LiteralStringEscape))
		set(chroma.GenericDeleted, primitiveToChromaString(c.GenericDeleted))
		set(chroma.GenericEmph, primitiveToChromaString(c.GenericEmph))
		set(chroma.GenericInserted, primitiveToChromaString(c.GenericInserted))
		set(chroma.GenericStrong, primitiveToChromaString(c.GenericStrong))
		set(chroma.GenericSubheading, primitiveToChromaString(c.GenericSubheading))
		// Note: c.Background is intentionally NOT mapped to chroma.Background.
		// See comment in FromStyleConfig for rationale.
	}

	// Structural markdown fields — these overlay on top of chroma entries.

	// Document text color → chroma.Generic (base text color for the document).
	if c := colorToHex(cfg.Document.Color); c != "" {
		entries[chroma.Generic] = c
	}

	// Document background → chroma.Background (inherited by all tokens).
	if bg := colorToHex(cfg.Document.BackgroundColor); bg != "" {
		entries[chroma.Background] = "bg:" + bg
	}

	// Headings.
	headingBase := cfg.Heading.StylePrimitive
	h1 := mergePrimitive(headingBase, cfg.H1.StylePrimitive)
	set(chroma.GenericHeading, primitiveToChromaString(h1))

	h3 := mergePrimitive(headingBase, cfg.H3.StylePrimitive)
	set(chroma.GenericSubheading, primitiveToChromaString(h3))

	// Emphasis and strong.
	set(chroma.GenericEmph, primitiveToChromaString(cfg.Emph))
	set(chroma.GenericStrong, primitiveToChromaString(cfg.Strong))

	// StrongEmph = merge of Emph + Strong.
	strongEmph := mergePrimitive(cfg.Emph, cfg.Strong)
	set(StrongEmph, primitiveToChromaString(strongEmph))

	// Link → GenericUnderline.
	set(chroma.GenericUnderline, primitiveToChromaString(cfg.Link))

	// Code (inline) → CodeSpan.
	set(CodeSpan, primitiveToChromaString(cfg.Code.StylePrimitive))

	// Code block fence → LiteralStringHeredoc. Merge the Chroma.Background
	// (code block syntax-highlighting background) so it applies only inside
	// code blocks, not the entire document.
	codeBlockPrim := cfg.CodeBlock.StylePrimitive
	if cfg.CodeBlock.Chroma != nil {
		codeBlockPrim = mergePrimitive(codeBlockPrim, cfg.CodeBlock.Chroma.Background)
	}
	set(chroma.LiteralStringHeredoc, primitiveToChromaString(codeBlockPrim))

	// Table.
	set(Table, primitiveToChromaString(cfg.Table.StylePrimitive))

	return chromaStyles.Register(chroma.MustNewStyle(name, entries))
}

// Glamour themes converted from github.com/charmbracelet/glamour/styles.
var (
	GlamourDark       = ChromaStyleFromConfig("glamour-dark", glamourstyles.DarkStyleConfig)
	GlamourLight      = ChromaStyleFromConfig("glamour-light", glamourstyles.LightStyleConfig)
	GlamourDracula    = ChromaStyleFromConfig("glamour-dracula", glamourstyles.DraculaStyleConfig)
	GlamourTokyoNight = ChromaStyleFromConfig("glamour-tokyo-night", glamourstyles.TokyoNightStyleConfig)
	GlamourPink       = ChromaStyleFromConfig("glamour-pink", glamourstyles.PinkStyleConfig)
)

// AutoTheme returns GlamourDark or GlamourLight depending on the terminal's
// background color. It queries stdin/stdout to detect whether the terminal
// has a dark background, matching the behavior of glamour's "auto" style.
func AutoTheme() *chroma.Style {
	if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		return GlamourDark
	}
	return GlamourLight
}
