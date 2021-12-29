// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"reflect"
	"regexp"

	"github.com/pkg/errors"
)

var (
	variableRegex = regexp.MustCompile(`\${[^}]*}`)
)

// VariableValue describes a SECL variable value
type VariableValue interface {
	GetEvaluator() interface{}
}

// MutableVariable is the interface implemented by modifiable variables
type MutableVariable interface {
	Set(ctx *Context, value interface{}) error
}

// Variable describes a SECL variable
type Variable struct {
	setFnc func(ctx *Context, value interface{}) error
}

// Set the variable with the specified value
func (v *Variable) Set(ctx *Context, value interface{}) error {
	if v.setFnc == nil {
		return errors.New("variable is not mutable")
	}

	return v.setFnc(ctx, value)
}

// IntVariable describes an integer variable
type IntVariable struct {
	Variable
	intFnc func(ctx *Context) int
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
	return &IntVariable{
		Variable: Variable{
			setFnc: setFnc,
		},
		intFnc: intFnc,
	}
}

// StringVariable describes a string variable
type StringVariable struct {
	Variable
	strFnc func(ctx *Context) string
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
	return &StringVariable{
		strFnc: strFnc,
		Variable: Variable{
			setFnc: setFnc,
		},
	}
}

// BoolVariable describes a boolean variable
type BoolVariable struct {
	Variable
	boolFnc func(ctx *Context) bool
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
	return &BoolVariable{
		boolFnc: boolFnc,
		Variable: Variable{
			setFnc: setFnc,
		},
	}
}

// StringArrayVariable describes a string array variable
type StringArrayVariable struct {
	Variable
	strFnc func(ctx *Context) []string
}

// GetEvaluator returns the variable SECL evaluator
func (s *StringArrayVariable) GetEvaluator() interface{} {
	return &StringArrayEvaluator{
		EvalFnc: s.strFnc,
	}
}

// NewStringArrayVariable returns a new string array variable
func NewStringArrayVariable(strFnc func(ctx *Context) []string, setFnc func(ctx *Context, value interface{}) error) *StringArrayVariable {
	return &StringArrayVariable{
		strFnc: strFnc,
		Variable: Variable{
			setFnc: setFnc,
		},
	}
}

// IntArrayVariable describes an integer array variable
type IntArrayVariable struct {
	Variable
	intFnc func(ctx *Context) []int
}

// GetEvaluator returns the variable SECL evaluator
func (s *IntArrayVariable) GetEvaluator() interface{} {
	return &IntArrayEvaluator{
		EvalFnc: s.intFnc,
	}
}

// NewIntArrayVariable returns a new integer array variable
func NewIntArrayVariable(intFnc func(ctx *Context) []int, setFnc func(ctx *Context, value interface{}) error) *IntArrayVariable {
	return &IntArrayVariable{
		intFnc: intFnc,
		Variable: Variable{
			setFnc: setFnc,
		},
	}
}

// MutableIntVariable describes a mutable integer variable
type MutableIntVariable struct {
	Value int
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

// GetEvaluator returns the variable SECL evaluator
func (m *MutableBoolVariable) GetEvaluator() interface{} {
	return &BoolEvaluator{
		EvalFnc: func(ctx *Context) bool {
			return m.Value
		},
	}
}

// Set the variable with the specified value
func (m *MutableBoolVariable) Set(ctx *Context, value interface{}) error {
	m.Value = value.(bool)
	return nil
}

// NewMutableBoolVariable returns a new mutable boolean variable
func NewMutableBoolVariable() *MutableBoolVariable {
	return &MutableBoolVariable{}
}

// MutableStringVariable describes a mutable string variable
type MutableStringVariable struct {
	Value string
}

// GetEvaluator returns the variable SECL evaluator
func (m *MutableStringVariable) GetEvaluator() interface{} {
	return &StringEvaluator{
		EvalFnc: func(ctx *Context) string {
			return m.Value
		},
	}
}

// Set the variable with the specified value
func (m *MutableStringVariable) Set(ctx *Context, value interface{}) error {
	m.Value = value.(string)
	return nil
}

// NewMutableStringVariable returns a new mutable string variable
func NewMutableStringVariable() *MutableStringVariable {
	return &MutableStringVariable{}
}

// MutableStringArrayVariable describes a mutable string array variable
type MutableStringArrayVariable struct {
	Values []string
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
