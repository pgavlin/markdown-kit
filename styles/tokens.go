package styles

import (
	"github.com/alecthomas/chroma"
)

const (
	Table chroma.TokenType = 9000 + iota
	TableHeader
	TableRow
	TableRowAlt
)
