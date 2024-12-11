// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// Evaluator is the interface of an evaluator
type Evaluator interface {
	Eval(ctx *Context) interface{}
	IsDeterministicFor(field Field) bool
	GetField() string
	IsStatic() bool
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

// IsStatic returns whether the evaluator is a scalar
func (b *BoolEvaluator) IsStatic() bool {
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
	isDeterministic           bool
	isDuration                bool
	isFromArithmeticOperation bool
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

// IsStatic returns whether the evaluator is a scalar
func (i *IntEvaluator) IsStatic() bool {
	return i.EvalFnc == nil
}

// StringEvaluator returns a string as result of the evaluation
type StringEvaluator struct {
	EvalFnc       func(ctx *Context) string
	Field         Field
	Value         string
	Weight        int
	OpOverrides   *OpOverrides
	ValueType     FieldValueType
	StringCmpOpts StringCmpOpts // only Field evaluator can set this value

	// used during compilation of partial
	isDeterministic bool
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

// IsStatic returns whether the evaluator is a scalar
func (s *StringEvaluator) IsStatic() bool {
	return s.EvalFnc == nil
}

// GetValue returns the evaluator value
func (s *StringEvaluator) GetValue(ctx *Context) string {
	if s.EvalFnc == nil {
		return s.Value
	}
	return s.EvalFnc(ctx)
}

// ToStringMatcher returns a StringMatcher of the evaluator
func (s *StringEvaluator) ToStringMatcher(opts StringCmpOpts) (StringMatcher, error) {
	if s.IsStatic() {
		matcher, err := NewStringMatcher(s.ValueType, s.Value, opts)
		if err != nil {
			return nil, err
		}
		return matcher, nil
	}

	return nil, nil
}

// StringArrayEvaluator returns an array of strings
type StringArrayEvaluator struct {
	EvalFnc       func(ctx *Context) []string
	Values        []string
	Field         Field
	Weight        int
	OpOverrides   *OpOverrides
	StringCmpOpts StringCmpOpts // only Field evaluator can set this value

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

// IsStatic returns whether the evaluator is a scalar
func (s *StringArrayEvaluator) IsStatic() bool {
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
func (s *StringValuesEvaluator) IsDeterministicFor(_ Field) bool {
	return s.isDeterministic
}

// GetField returns field name used by this evaluator
func (s *StringValuesEvaluator) GetField() string {
	return ""
}

// IsStatic returns whether the evaluator is a scalar
func (s *StringValuesEvaluator) IsStatic() bool {
	return s.EvalFnc == nil
}

// Compile the underlying StringValues
func (s *StringValuesEvaluator) Compile(opts StringCmpOpts) error {
	return s.Values.Compile(opts)
}

// SetFieldValues apply field values
func (s *StringValuesEvaluator) SetFieldValues(values ...FieldValue) error {
	return s.Values.SetFieldValues(values...)
}

// AppendMembers add members to the evaluator
func (s *StringValuesEvaluator) AppendMembers(members ...ast.StringMember) {
	for _, member := range members {
		var value FieldValue
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
		s.Values.AppendFieldValue(value)
	}
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

// IsStatic returns whether the evaluator is a scalar
func (i *IntArrayEvaluator) IsStatic() bool {
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

// IsStatic returns whether the evaluator is a scalar
func (b *BoolArrayEvaluator) IsStatic() bool {
	return b.EvalFnc == nil
}

// AppendValues to the array evaluator
func (b *BoolArrayEvaluator) AppendValues(values ...bool) {
	b.Values = append(b.Values, values...)
}

// CIDREvaluator returns a net.IP
type CIDREvaluator struct {
	EvalFnc     func(ctx *Context) net.IPNet
	Field       Field
	Value       net.IPNet
	Weight      int
	OpOverrides *OpOverrides
	ValueType   FieldValueType

	// used during compilation of partial
	isDeterministic bool
}

// Eval returns the result of the evaluation
func (s *CIDREvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (s *CIDREvaluator) IsDeterministicFor(field Field) bool {
	return s.isDeterministic || (s.Field != "" && s.Field == field)
}

// GetField returns field name used by this evaluator
func (s *CIDREvaluator) GetField() string {
	return s.Field
}

// IsStatic returns whether the evaluator is a scalar
func (s *CIDREvaluator) IsStatic() bool {
	return s.EvalFnc == nil
}

// CIDRValuesEvaluator returns a net.IP
type CIDRValuesEvaluator struct {
	EvalFnc   func(ctx *Context) *CIDRValues
	Value     CIDRValues
	Weight    int
	ValueType FieldValueType

	// used during compilation of partial
	isDeterministic bool
}

// Eval returns the result of the evaluation
func (s *CIDRValuesEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (s *CIDRValuesEvaluator) IsDeterministicFor(_ Field) bool {
	return s.isDeterministic
}

// GetField returns field name used by this evaluator
func (s *CIDRValuesEvaluator) GetField() string {
	return ""
}

// IsStatic returns whether the evaluator is a scalar
func (s *CIDRValuesEvaluator) IsStatic() bool {
	return s.EvalFnc == nil
}

// CIDRArrayEvaluator returns an array of net.IPNet
type CIDRArrayEvaluator struct {
	EvalFnc     func(ctx *Context) []net.IPNet
	Field       Field
	Value       []net.IPNet
	Weight      int
	OpOverrides *OpOverrides
	ValueType   FieldValueType

	// used during compilation of partial
	isDeterministic bool
}

// Eval returns the result of the evaluation
func (s *CIDRArrayEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
}

// IsDeterministicFor returns whether the evaluator is partial
func (s *CIDRArrayEvaluator) IsDeterministicFor(field Field) bool {
	return s.isDeterministic || (s.Field != "" && s.Field == field)
}

// GetField returns field name used by this evaluator
func (s *CIDRArrayEvaluator) GetField() string {
	return s.Field
}

// IsStatic returns whether the evaluator is a scalar
func (s *CIDRArrayEvaluator) IsStatic() bool {
	return s.EvalFnc == nil
}
