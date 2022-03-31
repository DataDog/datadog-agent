// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// Evaluator is the interface of an evaluator
type Evaluator interface {
	Eval(ctx *Context) interface{}
	IsDeterministicFor(field Field) bool
	GetField() string
	IsScalar() bool
}

// BoolEvaluator returns a bool as result of the evaluation
type BoolEvaluator struct {
	EvalFnc     BoolEvalFnc
	Field       Field
	Value       bool
	Weight      int
	OpOverrides *OpOverrides

	// used during compilation of partial
	isDeterministic bool
}

// Eval returns the result of the evaluation
func (b *BoolEvaluator) Eval(ctx *Context) interface{} {
	return b.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (b *BoolEvaluator) IsDeterministicFor(field Field) bool {
	return b.isDeterministic || (b.Field != "" && b.Field == field)
}

// GetField returns field name used by this evaluator
func (b *BoolEvaluator) GetField() string {
	return b.Field
}

// IsScalar returns whether the evaluator is a scalar
func (b *BoolEvaluator) IsScalar() bool {
	return b.EvalFnc == nil
}

// IntEvaluator returns an int as result of the evaluation
type IntEvaluator struct {
	EvalFnc     func(ctx *Context) int
	Field       Field
	Value       int
	Weight      int
	OpOverrides *OpOverrides

	// used during compilation of partial
	isDeterministic bool
	isDuration      bool
}

// Eval returns the result of the evaluation
func (i *IntEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (i *IntEvaluator) IsDeterministicFor(field Field) bool {
	return i.isDeterministic || (i.Field != "" && i.Field == field)
}

// GetField returns field name used by this evaluator
func (i *IntEvaluator) GetField() string {
	return i.Field
}

// IsScalar returns whether the evaluator is a scalar
func (i *IntEvaluator) IsScalar() bool {
	return i.EvalFnc == nil
}

// StringEvaluator returns a string as result of the evaluation
type StringEvaluator struct {
	EvalFnc     func(ctx *Context) string
	Field       Field
	Value       string
	Weight      int
	OpOverrides *OpOverrides
	ValueType   FieldValueType

	// used during compilation of partial
	isDeterministic bool

	stringMatcher StringMatcher
}

// Eval returns the result of the evaluation
func (s *StringEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (s *StringEvaluator) IsDeterministicFor(field Field) bool {
	return s.isDeterministic || (s.Field != "" && s.Field == field)
}

// GetField returns field name used by this evaluator
func (s *StringEvaluator) GetField() string {
	return s.Field
}

// IsScalar returns whether the evaluator is a scalar
func (s *StringEvaluator) IsScalar() bool {
	return s.EvalFnc == nil
}

// GetValue returns the evaluator value
func (s *StringEvaluator) GetValue(ctx *Context) string {
	if s.EvalFnc == nil {
		return s.Value
	}
	return s.EvalFnc(ctx)
}

// Compile compile internal object
func (s *StringEvaluator) Compile() error {
	switch s.ValueType {
	case PatternValueType, RegexpValueType:
		matcher, err := NewStringMatcher(s.ValueType, s.Value)
		if err != nil {
			return err
		}
		s.stringMatcher = matcher
	default:
		return fmt.Errorf("invalid pattern or regexp '%s'", s.Value)
	}
	return nil
}

// StringArrayEvaluator returns an array of strings
type StringArrayEvaluator struct {
	EvalFnc     func(ctx *Context) []string
	Values      []string
	Field       Field
	Weight      int
	OpOverrides *OpOverrides

	// used during compilation of partial
	isDeterministic bool
}

// Eval returns the result of the evaluation
func (s *StringArrayEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (s *StringArrayEvaluator) IsDeterministicFor(field Field) bool {
	return s.isDeterministic || (s.Field != "" && s.Field == field)
}

// GetField returns field name used by this evaluator
func (s *StringArrayEvaluator) GetField() string {
	return s.Field
}

// IsScalar returns whether the evaluator is a scalar
func (s *StringArrayEvaluator) IsScalar() bool {
	return s.EvalFnc == nil
}

// AppendValue append the given value
func (s *StringArrayEvaluator) AppendValue(value string) {
	s.Values = append(s.Values, value)
}

// StringValuesEvaluator returns an array of strings
type StringValuesEvaluator struct {
	EvalFnc func(ctx *Context) *StringValues
	Values  StringValues
	Weight  int

	// used during compilation of partial
	isDeterministic bool
}

// Eval returns the result of the evaluation
func (s *StringValuesEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (s *StringValuesEvaluator) IsDeterministicFor(field Field) bool {
	return s.isDeterministic
}

// GetField returns field name used by this evaluator
func (s *StringValuesEvaluator) GetField() string {
	return ""
}

// IsScalar returns whether the evaluator is a scalar
func (s *StringValuesEvaluator) IsScalar() bool {
	return s.EvalFnc == nil
}

// AppendFieldValues append field values
func (s *StringValuesEvaluator) AppendFieldValues(values ...FieldValue) error {
	for _, value := range values {
		if err := s.Values.AppendFieldValue(value); err != nil {
			return err
		}
	}

	return nil
}

// SetFieldValues apply field values
func (s *StringValuesEvaluator) SetFieldValues(values ...FieldValue) error {
	return s.Values.SetFieldValues(values...)
}

// AppendMembers add members to the evaluator
func (s *StringValuesEvaluator) AppendMembers(members ...ast.StringMember) error {
	var values []FieldValue
	var value FieldValue

	for _, member := range members {
		if member.Pattern != nil {
			value = FieldValue{
				Value: *member.Pattern,
				Type:  PatternValueType,
			}
		} else if member.Regexp != nil {
			value = FieldValue{
				Value: *member.Regexp,
				Type:  RegexpValueType,
			}
		} else {
			value = FieldValue{
				Value: *member.String,
				Type:  ScalarValueType,
			}
		}
		values = append(values, value)
	}

	return s.AppendFieldValues(values...)
}

// IntArrayEvaluator returns an array of int
type IntArrayEvaluator struct {
	EvalFnc     func(ctx *Context) []int
	Field       Field
	Values      []int
	Weight      int
	OpOverrides *OpOverrides

	// used during compilation of partial
	isDeterministic bool
}

// Eval returns the result of the evaluation
func (i *IntArrayEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (i *IntArrayEvaluator) IsDeterministicFor(field Field) bool {
	return i.isDeterministic || (i.Field != "" && i.Field == field)
}

// GetField returns field name used by this evaluator
func (i *IntArrayEvaluator) GetField() string {
	return i.Field
}

// IsScalar returns whether the evaluator is a scalar
func (i *IntArrayEvaluator) IsScalar() bool {
	return i.EvalFnc == nil
}

// AppendValues to the array evaluator
func (i *IntArrayEvaluator) AppendValues(values ...int) {
	i.Values = append(i.Values, values...)
}

// BoolArrayEvaluator returns an array of bool
type BoolArrayEvaluator struct {
	EvalFnc     func(ctx *Context) []bool
	Field       Field
	Values      []bool
	Weight      int
	OpOverrides *OpOverrides

	// used during compilation of partial
	isDeterministic bool
}

// Eval returns the result of the evaluation
func (b *BoolArrayEvaluator) Eval(ctx *Context) interface{} {
	return b.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (b *BoolArrayEvaluator) IsDeterministicFor(field Field) bool {
	return b.isDeterministic || (b.Field != "" && b.Field == field)
}

// GetField returns field name used by this evaluator
func (b *BoolArrayEvaluator) GetField() string {
	return b.Field
}

// IsScalar returns whether the evaluator is a scalar
func (b *BoolArrayEvaluator) IsScalar() bool {
	return b.EvalFnc == nil
}
