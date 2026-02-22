package styles

import (
	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/styles"
)

var GlamourDark = styles.Register(chroma.MustNewStyle("glamour-dark", chroma.StyleEntries{
	// Markdown structural tokens
	chroma.Generic:              "#d0d0d0",           // Document text (glamour color 252)
	chroma.GenericHeading:       "#00afff bold",      // H1-H2 (glamour heading: color 39, bold)
	chroma.GenericSubheading:    "#00afff",           // H3+ (heading color without bold)
	chroma.GenericEmph:          "italic",            // Blockquotes
	chroma.GenericUnderline:     "#008787 underline", // Links (glamour link: color 30, underline)
	chroma.LiteralStringHeredoc: "#808080",           // Code block fences (glamour code_block: color 244)

	// Chroma syntax highlighting (from glamour dark.json chroma section)
	chroma.Text:                "#C4C4C4",
	chroma.Error:               "#F1F1F1 bg:#F05B5B",
	chroma.Comment:             "#676767",
	chroma.CommentPreproc:      "#FF875F",
	chroma.Keyword:             "#00AAFF",
	chroma.KeywordReserved:     "#FF5FD2",
	chroma.KeywordNamespace:    "#FF5F87",
	chroma.KeywordType:         "#6E6ED8",
	chroma.Operator:            "#EF8080",
	chroma.Punctuation:         "#E8E8A8",
	chroma.Name:                "#C4C4C4",
	chroma.NameBuiltin:         "#FF8EC7",
	chroma.NameTag:             "#B083EA",
	chroma.NameAttribute:       "#7A7AE6",
	chroma.NameClass:           "#F1F1F1 underline bold",
	chroma.NameDecorator:       "#FFFF87",
	chroma.NameFunction:        "#00D787",
	chroma.LiteralNumber:       "#6EEFC0",
	chroma.LiteralString:       "#C69669",
	chroma.LiteralStringEscape: "#AFFFD7",
	chroma.GenericDeleted:      "#FD5B5B",
	chroma.GenericInserted:     "#00D787",
	chroma.GenericStrong:       "bold",
	StrongEmph:                 "bold italic",

	// Inline code
	CodeSpan: "#C4C4C4 bg:#303030",

	// Table tokens
	Table:       "bg:#373737",
	TableHeader: "#00afff",
	TableRowAlt: "bg:#404040",
}))
