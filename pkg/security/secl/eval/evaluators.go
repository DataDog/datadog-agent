// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/pkg/errors"
)

// Evaluator is the interface of an evaluator
type Evaluator interface {
	Eval(ctx *Context) interface{}
	IsPartial() bool
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

	isPartial bool
}

// Eval returns the result of the evaluation
func (b *BoolEvaluator) Eval(ctx *Context) interface{} {
	return b.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (b *BoolEvaluator) IsPartial() bool {
	return b.isPartial
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

	isPartial  bool
	isDuration bool
}

// Eval returns the result of the evaluation
func (i *IntEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (i *IntEvaluator) IsPartial() bool {
	return i.isPartial
}

// GetField returns field name used by this evaluator
func (i *IntEvaluator) GetField() string {
	return i.Field
}

// IsScalar returns whether the evaluator is a scalar
func (i *IntEvaluator) IsScalar() bool {
	return i.EvalFnc == nil
}

// AppendMembers to the array evaluator
func (i *IntArrayEvaluator) AppendMembers(members ...int) {
	i.Values = append(i.Values, members...)
}

// StringEvaluator returns a string as result of the evaluation
type StringEvaluator struct {
	EvalFnc     func(ctx *Context) string
	Field       Field
	Value       string
	Weight      int
	OpOverrides *OpOverrides
	ValueType   FieldValueType

	isPartial bool

	// cache
	regexp *regexp.Regexp
}

// Eval returns the result of the evaluation
func (s *StringEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (s *StringEvaluator) IsPartial() bool {
	return s.isPartial
}

// GetField returns field name used by this evaluator
func (s *StringEvaluator) GetField() string {
	return s.Field
}

// IsScalar returns whether the evaluator is a scalar
func (s *StringEvaluator) IsScalar() bool {
	return s.EvalFnc == nil
}

// Compile compile internal object
func (s *StringEvaluator) Compile() error {
	switch s.ValueType {
	case PatternValueType:
		reg, err := patternToRegexp(s.Value)
		if err != nil {
			return fmt.Errorf("invalid pattern '%s': %s", s.Value, err)
		}
		s.regexp = reg
	case RegexpValueType:
		reg, err := regexp.Compile(s.Value)
		if err != nil {
			return fmt.Errorf("invalid regexp '%s': %s", s.Value, err)
		}
		s.regexp = reg
	}
	return nil
}

// StringArrayEvaluator returns an array of strings
type StringArrayEvaluator struct {
	EvalFnc     func(ctx *Context) []string
	Field       Field
	Values      []string
	Weight      int
	OpOverrides *OpOverrides

	isPartial bool

	fieldValues []FieldValue

	// cache
	scalars map[string]bool
	regexps []*regexp.Regexp
}

// Eval returns the result of the evaluation
func (s *StringArrayEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (s *StringArrayEvaluator) IsPartial() bool {
	return s.isPartial
}

// GetField returns field name used by this evaluator
func (s *StringArrayEvaluator) GetField() string {
	return s.Field
}

// IsScalar returns whether the evaluator is a scalar
func (s *StringArrayEvaluator) IsScalar() bool {
	return s.EvalFnc == nil
}

// GetFieldValues return current field values
func (s *StringArrayEvaluator) GetFieldValues() []FieldValue {
	return s.fieldValues
}

// AppendFieldValues append field values
func (s *StringArrayEvaluator) AppendFieldValues(values ...FieldValue) error {
	if s.scalars == nil {
		s.scalars = make(map[string]bool)
	}

	for _, value := range values {
		switch value.Type {
		case PatternValueType:
			if err := value.Compile(); err != nil {
				return err
			}

			s.Values = append(s.Values, value.Value.(string))
			s.regexps = append(s.regexps, value.Regexp)
			s.fieldValues = append(s.fieldValues, value)
		case RegexpValueType:
			if err := value.Compile(); err != nil {
				return err
			}

			s.Values = append(s.Values, value.Value.(string))
			s.regexps = append(s.regexps, value.Regexp)
			s.fieldValues = append(s.fieldValues, value)
		default:
			str := value.Value.(string)
			s.Values = append(s.Values, str)
			s.scalars[str] = true
			s.fieldValues = append(s.fieldValues, value)
		}
	}

	return nil
}

// SetFieldValues apply field values
func (s *StringArrayEvaluator) SetFieldValues(values ...FieldValue) error {
	s.Values = []string{}

	// reset internal caches
	s.fieldValues = []FieldValue{}
	s.regexps = []*regexp.Regexp{}
	s.scalars = nil

	return s.AppendFieldValues(values...)
}

// AppendMembers add members to the evaluator
func (s *StringArrayEvaluator) AppendMembers(members ...ast.StringMember) error {
	if s.scalars == nil {
		s.scalars = make(map[string]bool)
	}

	var values []FieldValue
	for _, member := range members {
		if member.Pattern != nil {
			value := FieldValue{
				Value: *member.Pattern,
				Type:  PatternValueType,
			}
			values = append(values, value)
		} else if member.Regexp != nil {
			value := FieldValue{
				Value: *member.Regexp,
				Type:  RegexpValueType,
			}
			values = append(values, value)
		} else {
			value := FieldValue{
				Value: *member.String,
				Type:  ScalarValueType,
			}
			values = append(values, value)
		}
	}

	return s.AppendFieldValues(values...)
}

// AppendStringEvaluator add string evaluator to the evaluator
func (s *StringArrayEvaluator) AppendStringEvaluator(evaluators ...*StringEvaluator) error {
	for _, evaluator := range evaluators {
		if evaluator.ValueType == PatternValueType {
			if err := s.AppendMembers(ast.StringMember{Pattern: &evaluator.Value}); err != nil {
				return err
			}
		} else if evaluator.ValueType == RegexpValueType {
			if err := s.AppendMembers(ast.StringMember{Regexp: &evaluator.Value}); err != nil {
				return err
			}
		} else if evaluator.EvalFnc == nil {
			if err := s.AppendMembers(ast.StringMember{String: &evaluator.Value}); err != nil {
				return err
			}
		} else {
			return errors.New("only scalar evaluator are supported")
		}
	}

	return nil
}

// IntArrayEvaluator returns an array of int
type IntArrayEvaluator struct {
	EvalFnc     func(ctx *Context) []int
	Field       Field
	Values      []int
	Weight      int
	OpOverrides *OpOverrides

	isPartial bool
}

// Eval returns the result of the evaluation
func (i *IntArrayEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (i *IntArrayEvaluator) IsPartial() bool {
	return i.isPartial
}

// GetField returns field name used by this evaluator
func (i *IntArrayEvaluator) GetField() string {
	return i.Field
}

// IsScalar returns whether the evaluator is a scalar
func (i *IntArrayEvaluator) IsScalar() bool {
	return i.EvalFnc == nil
}

// BoolArrayEvaluator returns an array of bool
type BoolArrayEvaluator struct {
	EvalFnc     func(ctx *Context) []bool
	Field       Field
	Values      []bool
	Weight      int
	OpOverrides *OpOverrides

	isPartial bool
}

// Eval returns the result of the evaluation
func (b *BoolArrayEvaluator) Eval(ctx *Context) interface{} {
	return b.EvalFnc(ctx)
}

// IsPartial returns whether the evaluator is partial
func (b *BoolArrayEvaluator) IsPartial() bool {
	return b.isPartial
}

// GetField returns field name used by this evaluator
func (b *BoolArrayEvaluator) GetField() string {
	return b.Field
}

// IsScalar returns whether the evaluator is a scalar
func (b *BoolArrayEvaluator) IsScalar() bool {
	return b.EvalFnc == nil
}
