// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

// ErrNonStaticPattern when pattern operator is used on a non static value
type ErrNonStaticPattern struct {
	Field Field
}

func (e ErrNonStaticPattern) Error() string {
	return fmt.Sprintf("unable to apply pattern on non static value `%s`", e.Field)
}

// ErrInvalidPattern is returned for an invalid regular expression
type ErrInvalidPattern struct {
	Pattern string
}

func (e ErrInvalidPattern) Error() string {
	return fmt.Sprintf("invalid pattern `%s`", e.Pattern)
}

// ErrInvalidRegexp is returned for an invalid regular expression
type ErrInvalidRegexp struct {
	Regexp string
}

func (e ErrInvalidRegexp) Error() string {
	return fmt.Sprintf("invalid regexp `%s`", e.Regexp)
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

// NewArrayTypeError returns a new ErrAstToEval error when an invalid type was used
func NewArrayTypeError(pos lexer.Position, arrayKind reflect.Kind, kind reflect.Kind) *ErrAstToEval {
	return NewError(pos, fmt.Sprintf("%s of %s expected", arrayKind, kind))
}

// NewOpUnknownError returns a new ErrAstToEval error when an unknown operator was used
func NewOpUnknownError(pos lexer.Position, op string) *ErrAstToEval {
	return NewError(pos, fmt.Sprintf("operator `%s` unknown", op))
}

// NewOpError returns a new ErrAstToEval error when an operator was used in an invalid manner
func NewOpError(pos lexer.Position, op string, err error) *ErrAstToEval {
	return NewError(pos, fmt.Sprintf("operator `%s` error: %s", op, err))
}

// NewRegisterMultipleFields returns a new ErrAstToEval error when a register is used across multiple fields
func NewRegisterMultipleFields(pos lexer.Position, regID RegisterID, err error) *ErrAstToEval {
	return NewError(pos, fmt.Sprintf("register `%s` error: %s", regID, err))
}

// NewRegisterNameNotAllowed returns a new ErrAstToEval error when a register name is not allowed
func NewRegisterNameNotAllowed(pos lexer.Position, regID RegisterID, err error) *ErrAstToEval {
	return NewError(pos, fmt.Sprintf("register name `%s` error: %s", regID, err))
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

// ErrFieldNotFound error when a field is not present in the model
type ErrFieldNotFound struct {
	Field string
}

func (e ErrFieldNotFound) Error() string {
	return fmt.Sprintf("field `%s` not found", e.Field)
}

// ErrIteratorNotSupported error when a field doesn't support iteration
type ErrIteratorNotSupported struct {
	Field string
}

func (e ErrIteratorNotSupported) Error() string {
	return fmt.Sprintf("field `%s` doesn't support iteration", e.Field)
}

// ErrNotSupported returned when something is not supported on a field
type ErrNotSupported struct {
	Field string
}

func (e ErrNotSupported) Error() string {
	return fmt.Sprintf("not supported by field `%s`", e.Field)
}

// ErrValueTypeMismatch error when the given value is not having the correct type
type ErrValueTypeMismatch struct {
	Field string
}

func (e ErrValueTypeMismatch) Error() string {
	return fmt.Sprintf("incorrect value type for `%s`", e.Field)
}

// ErrRuleNotCompiled error returned by functions that require to have the rule compiled
type ErrRuleNotCompiled struct {
	RuleID string
}

func (e ErrRuleNotCompiled) Error() string {
	return fmt.Sprintf("rule not compiled `%s`", e.RuleID)
}
