package eval

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

type InvalidPattern struct {
	Pattern string
}

func (e InvalidPattern) Error() string {
	return fmt.Sprintf("invalid pattern `%s`", e.Pattern)
}

type AstToEvalError struct {
	Pos  lexer.Position
	Text string
}

func (r *AstToEvalError) Error() string {
	return fmt.Sprintf("%s: %s", r.Text, r.Pos)
}

func NewError(pos lexer.Position, text string) *AstToEvalError {
	return &AstToEvalError{Pos: pos, Text: text}
}

func NewTypeError(pos lexer.Position, kind reflect.Kind) *AstToEvalError {
	return NewError(pos, fmt.Sprintf("%s expected", kind))
}

func NewOpUnknownError(pos lexer.Position, op string) *AstToEvalError {
	return NewError(pos, fmt.Sprintf("operator `%s` unknown", op))
}

func NewOpError(pos lexer.Position, op string, err error) *AstToEvalError {
	return NewError(pos, fmt.Sprintf("operator `%s` error: %s", op, err))
}

type RuleParseError struct {
	pos  lexer.Position
	expr string
}

func (e *RuleParseError) Error() string {
	column := e.pos.Column
	if column > 0 {
		column--
	}

	str := fmt.Sprintf("%s\n", e.expr)
	str += strings.Repeat(" ", column)
	str += "^"
	return str
}
