package secl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

func SprintExprAt(expr string, pos lexer.Position) string {
	str := fmt.Sprintf("%s\n", expr)
	str += strings.Repeat(" ", pos.Column-1)
	str += "^"
	return str
}
