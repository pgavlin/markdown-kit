package styles

import (
	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/styles"
)

var Pulumi = styles.Register(chroma.MustNewStyle("pulumi", chroma.StyleEntries{
	chroma.Text:                "#d7d7d7",
	chroma.Error:               "#d75f5f",
	chroma.Comment:             "#afafaf",
	chroma.Keyword:             "#af87af",
	chroma.Operator:            "#5fafd7",
	chroma.Punctuation:         "#d7afff",
	chroma.Name:                "#d7d7d7",
	chroma.NameAttribute:       "#d7d7d7",
	chroma.NameClass:           "#d7d7d7",
	chroma.NameConstant:        "#d7d7d7",
	chroma.NameDecorator:       "#d7d7d7",
	chroma.NameException:       "#d7d7d7",
	chroma.NameFunction:        "#d7d7d7",
	chroma.NameOther:           "#d7d7d7",
	chroma.NameTag:             "#d7d7d7",
	chroma.LiteralNumber:       "#87ffaf",
	chroma.Literal:             "#00d7af",
	chroma.LiteralDate:         "#00d7af",
	chroma.LiteralString:       "#ffaf5f",
	chroma.LiteralStringEscape: "#5f5f87",
	chroma.GenericDeleted:      "#d75f5f",
	chroma.GenericEmph:         "italic",
	chroma.GenericHeading:      "#d787af bold",
	chroma.GenericInserted:     "#5f875f",
	chroma.GenericStrong:       "bold",
	chroma.GenericSubheading:   "#d787af",
	chroma.GenericUnderline:    "underline",
	chroma.Background:          "bg:#121212",
}))
