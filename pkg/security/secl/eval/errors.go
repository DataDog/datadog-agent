package eval

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/pkg/errors"
)

var RuleWithoutEventErr = errors.New("rule without event")

type NoApprover struct {
	Fields []string
}

func (e NoApprover) Error() string {
	return fmt.Sprintf("no approver for fields `%s`", strings.Join(e.Fields, ", "))
}

type ValueTypeUnknown struct {
	Field string
}

func (e *ValueTypeUnknown) Error() string {
	return fmt.Sprintf("value type unknown for `%s`", e.Field)
}

type FieldTypeUnknown struct {
	Field string
}

func (e *FieldTypeUnknown) Error() string {
	return fmt.Sprintf("field type unknown for `%s`", e.Field)
}

type DuplicateRuleID struct {
	ID string
}

func (e DuplicateRuleID) Error() string {
	return fmt.Sprintf("duplicate rule ID `%s`", e.ID)
}

type NoEventTypeBucket struct {
	EventType string
}

func (e NoEventTypeBucket) Error() string {
	return fmt.Sprintf("no bucket for event type `%s`", e.EventType)
}

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
