// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

var (
	variableRegex = regexp.MustCompile(`\${[^}]*}`)
)

// VariableValue describes secl variable
type VariableValue interface {
	GetEvaluator() interface{}
	IntFnc(ctx *Context) int
	StringFnc(ctx *Context) string
}

// MutableVariable is the interface implemented by modifiable variables
type MutableVariable interface {
	Set(ctx *Context, value interface{}) error
}

// IntVariable describes an integer variable
type IntVariable struct {
	intFnc func(ctx *Context) int
	setFnc func(ctx *Context, value interface{}) error
}

// IntFnc returns the variable value as an integer
func (i *IntVariable) IntFnc(ctx *Context) int {
	return i.intFnc(ctx)
}

// StringFnc returns the variable value as a string
func (i *IntVariable) StringFnc(ctx *Context) string {
	return strconv.FormatInt(int64(i.intFnc(ctx)), 10)
}

// Set the variable with the specified value
func (i *IntVariable) Set(ctx *Context, value interface{}) error {
	if i.setFnc == nil {
		return errors.New("variable is not mutable")
	}

	return i.setFnc(ctx, value)
}

// GetEvaluator returns the variable SECL evaluator
func (i *IntVariable) GetEvaluator() interface{} {
	return &IntEvaluator{
		EvalFnc: func(ctx *Context) int {
			return i.intFnc(ctx)
		},
	}
}

// NewIntVariable returns a new integer variable
func NewIntVariable(intFnc func(ctx *Context) int, setFnc func(ctx *Context, value interface{}) error) *IntVariable {
	return &IntVariable{intFnc: intFnc, setFnc: setFnc}
}

// StringVariable describes a string variable
type StringVariable struct {
	strFnc func(ctx *Context) string
	setFnc func(ctx *Context, value interface{}) error
}

// IntFnc returns the variable value as an integer
func (s *StringVariable) IntFnc(ctx *Context) int {
	i, _ := strconv.Atoi(s.strFnc(ctx))
	return i
}

// StringFnc returns the variable value as a string
func (s *StringVariable) StringFnc(ctx *Context) string {
	return s.strFnc(ctx)
}

// Set the variable with the specified value
func (s *StringVariable) Set(ctx *Context, value interface{}) error {
	if s.setFnc == nil {
		return errors.New("variable is not mutable")
	}

	return s.setFnc(ctx, value)
}

// GetEvaluator returns the variable SECL evaluator
func (s *StringVariable) GetEvaluator() interface{} {
	return &StringEvaluator{
		EvalFnc: func(ctx *Context) string {
			return s.strFnc(ctx)
		},
	}
}

// NewStringVariable returns a new string variable
func NewStringVariable(strFnc func(ctx *Context) string, setFnc func(ctx *Context, value interface{}) error) *StringVariable {
	return &StringVariable{strFnc: strFnc, setFnc: setFnc}
}

// BoolVariable describes a boolean variable
type BoolVariable struct {
	boolFnc func(ctx *Context) bool
	setFnc  func(ctx *Context, value interface{}) error
}

// IntFnc returns the variable value as an integer
func (b *BoolVariable) IntFnc(ctx *Context) int {
	if b.boolFnc(ctx) {
		return 1
	}
	return 0
}

// StringFnc returns the variable value as a string
func (b *BoolVariable) StringFnc(ctx *Context) string {
	return strconv.FormatBool(b.boolFnc(ctx))
}

// Set the variable with the specified value
func (b *BoolVariable) Set(ctx *Context, value interface{}) error {
	if b.setFnc == nil {
		return errors.New("variable is not mutable")
	}

	return b.setFnc(ctx, value)
}

// GetEvaluator returns the variable SECL evaluator
func (b *BoolVariable) GetEvaluator() interface{} {
	return &BoolEvaluator{
		EvalFnc: func(ctx *Context) bool {
			return b.boolFnc(ctx)
		},
	}
}

// NewBoolVariable returns a new boolean variable
func NewBoolVariable(boolFnc func(ctx *Context) bool, setFnc func(ctx *Context, value interface{}) error) *BoolVariable {
	return &BoolVariable{boolFnc: boolFnc, setFnc: setFnc}
}

// MutableIntVariable describes a mutable integer variable
type MutableIntVariable struct {
	Value int
}

// IntFnc returns the variable value as an integer
func (m *MutableIntVariable) IntFnc(ctx *Context) int {
	return m.Value
}

// StringFnc returns the variable value as a string
func (m *MutableIntVariable) StringFnc(ctx *Context) string {
	return strconv.FormatInt(int64(m.Value), 10)
}

// Set the variable with the specified value
func (m *MutableIntVariable) Set(ctx *Context, value interface{}) error {
	m.Value = value.(int)
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *MutableIntVariable) GetEvaluator() interface{} {
	return &IntEvaluator{
		EvalFnc: func(ctx *Context) int {
			return m.Value
		},
	}
}

// NewMutableIntVariable returns a new mutable integer variable
func NewMutableIntVariable() *MutableIntVariable {
	return &MutableIntVariable{}
}

// MutableBoolVariable describes a mutable boolean variable
type MutableBoolVariable struct {
	Value bool
}

// IntFnc returns the variable value as an integer
func (m *MutableBoolVariable) IntFnc(ctx *Context) int {
	if m.Value {
		return 1
	}
	return 0
}

// StringFnc returns the variable value as a string
func (m *MutableBoolVariable) StringFnc(ctx *Context) string {
	return strconv.FormatBool(m.Value)
}

// Set the variable with the specified value
func (m *MutableBoolVariable) Set(ctx *Context, value interface{}) error {
	m.Value = value.(bool)
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *MutableBoolVariable) GetEvaluator() interface{} {
	return &BoolEvaluator{
		EvalFnc: func(ctx *Context) bool {
			return m.Value
		},
	}
}

// NewMutableBoolVariable returns a new mutable boolean variable
func NewMutableBoolVariable() *MutableBoolVariable {
	return &MutableBoolVariable{}
}

// MutableStringVariable describes a mutable string variable
type MutableStringVariable struct {
	Value string
}

// IntFnc returns the variable value as an integer
func (m *MutableStringVariable) IntFnc(ctx *Context) int {
	i, _ := strconv.Atoi(m.Value)
	return i
}

// StringFnc returns the variable value as a string
func (m *MutableStringVariable) StringFnc(ctx *Context) string {
	return m.Value
}

// Set the variable with the specified value
func (m *MutableStringVariable) Set(ctx *Context, value interface{}) error {
	m.Value = value.(string)
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *MutableStringVariable) GetEvaluator() interface{} {
	return &StringEvaluator{
		EvalFnc: func(ctx *Context) string {
			return m.Value
		},
	}
}

// NewMutableStringVariable returns a new mutable string variable
func NewMutableStringVariable() *MutableStringVariable {
	return &MutableStringVariable{}
}

// MutableStringArrayVariable describes a mutable string array variable
type MutableStringArrayVariable struct {
	Values []string
}

// IntFnc returns the variable value as an integer
func (m *MutableStringArrayVariable) IntFnc(ctx *Context) int {
	return 0
}

// StringFnc returns the variable value as a string
func (m *MutableStringArrayVariable) StringFnc(ctx *Context) string {
	return strings.Join(m.Values, " ")
}

// Set the variable with the specified value
func (m *MutableStringArrayVariable) Set(ctx *Context, values interface{}) error {
	m.Values = values.([]string)
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *MutableStringArrayVariable) GetEvaluator() interface{} {
	return &StringArrayEvaluator{
		EvalFnc: func(ctx *Context) []string {
			return m.Values
		},
	}
}

// NewMutableStringArrayVariable returns a new mutable string array variable
func NewMutableStringArrayVariable() *MutableStringArrayVariable {
	return &MutableStringArrayVariable{}
}

// MutableIntArrayVariable describes a mutable integer array variable
type MutableIntArrayVariable struct {
	Values []int
}

// IntFnc returns the variable value as an integer
func (m *MutableIntArrayVariable) IntFnc(ctx *Context) int {
	return 0
}

// StringFnc returns the variable value as a string
func (m *MutableIntArrayVariable) StringFnc(ctx *Context) string {
	return ""
}

// Set the variable with the specified value
func (m *MutableIntArrayVariable) Set(ctx *Context, values interface{}) error {
	m.Values = values.([]int)
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *MutableIntArrayVariable) GetEvaluator() interface{} {
	return &IntArrayEvaluator{
		EvalFnc: func(ctx *Context) []int {
			return m.Values
		},
	}
}

// NewMutableIntArrayVariable returns a new mutable integer array variable
func NewMutableIntArrayVariable() *MutableIntArrayVariable {
	return &MutableIntArrayVariable{}
}

// GetVariable returns new variable of the type of the specified value
func GetVariable(name string, value interface{}) (VariableValue, error) {
	switch value := value.(type) {
	case bool:
		return NewMutableBoolVariable(), nil
	case int:
		return NewMutableIntVariable(), nil
	case string:
		return NewMutableStringVariable(), nil
	case []string:
		return NewMutableStringArrayVariable(), nil
	case []int:
		return NewMutableIntArrayVariable(), nil
	default:
		return nil, fmt.Errorf("unsupported value type: %s", reflect.TypeOf(value))
	}
}
