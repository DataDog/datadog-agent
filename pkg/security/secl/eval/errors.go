package eval

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

// ErrInvalidPattern is returned for an invalid regular expression
type ErrInvalidPattern struct {
	Pattern string
}

func (e ErrInvalidPattern) Error() string {
	return fmt.Sprintf("invalid pattern `%s`", e.Pattern)
}

// ErrAstToEval describes an error that occurred during the conversion from the AST to an evaluator
type ErrAstToEval struct {
	Pos  lexer.Position
	Text string
}

func (r *ErrAstToEval) Error() string {
	return fmt.Sprintf("%s: %s", r.Text, r.Pos)
}

// NewError returns a new ErrAstToEval error
func NewError(pos lexer.Position, text string) *ErrAstToEval {
	return &ErrAstToEval{Pos: pos, Text: text}
}

// NewTypeError returns a new ErrAstToEval error when an invalid type was used
func NewTypeError(pos lexer.Position, kind reflect.Kind) *ErrAstToEval {
	return NewError(pos, fmt.Sprintf("%s expected", kind))
}

// NewOpUnknownError returns a new ErrAstToEval error when an unknown operator was used
func NewOpUnknownError(pos lexer.Position, op string) *ErrAstToEval {
	return NewError(pos, fmt.Sprintf("operator `%s` unknown", op))
}

// NewOpError returns a new ErrAstToEval error when an operator was used in an invalid manner
func NewOpError(pos lexer.Position, op string, err error) *ErrAstToEval {
	return NewError(pos, fmt.Sprintf("operator `%s` error: %s", op, err))
}

// ErrRuleParse describes a parsing error and its position in the expression
type ErrRuleParse struct {
	pos  lexer.Position
	expr string
}

func (e *ErrRuleParse) Error() string {
	column := e.pos.Column
	if column > 0 {
		column--
	}

	str := fmt.Sprintf("%s\n", e.expr)
	str += strings.Repeat(" ", column)
	str += "^"
	return str
}
