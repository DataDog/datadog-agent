package secl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

// SprintExprAt returns a string sed to highlight the precise location of an error
func SprintExprAt(expr string, pos lexer.Position) string {
	column := pos.Column
	if column > 0 {
		column--
	}

	str := fmt.Sprintf("%s\n", expr)
	str += strings.Repeat(" ", column)
	str += "^"
	return str
}
