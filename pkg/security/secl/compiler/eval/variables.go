// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
)

var (
	variableRegex         = regexp.MustCompile(`\${[^}]*}`)
	errAppendNotSupported = errors.New("append is not supported")
)

// VariableValue describes a SECL variable value
type VariableValue interface {
	GetEvaluator() interface{}
}

// MutableVariable is the interface implemented by modifiable variables
type MutableVariable interface {
	Set(ctx *Context, value interface{}) error
	Append(ctx *Context, value interface{}) error
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

// Append a value to the variable
func (v *Variable) Append(ctx *Context, value interface{}) error {
	return errAppendNotSupported
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
		ValueType: VariableValueType,
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

// Set the array values
func (s *StringArrayVariable) Set(ctx *Context, value interface{}) error {
	if s, ok := value.(string); ok {
		value = []string{s}
	}
	return s.Variable.Set(ctx, value)
}

// Append a value to the array
func (s *StringArrayVariable) Append(ctx *Context, value interface{}) error {
	return s.Set(ctx, append(s.strFnc(ctx), value.([]string)...))
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

// Set the array values
func (s *IntArrayVariable) Set(ctx *Context, value interface{}) error {
	if i, ok := value.(int); ok {
		value = []int{i}
	}
	return s.Variable.Set(ctx, value)
}

// Append a value to the array
func (s *IntArrayVariable) Append(ctx *Context, value interface{}) error {
	return s.Set(ctx, append(s.intFnc(ctx), value.([]int)...))
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

// Append a value to the integer
func (m *MutableIntVariable) Append(ctx *Context, value interface{}) error {
	switch value := value.(type) {
	case int:
		m.Value += value
	default:
		return errAppendNotSupported
	}
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

// Append a value to the boolean
func (m *MutableBoolVariable) Append(ctx *Context, value interface{}) error {
	return errAppendNotSupported
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
		ValueType: VariableValueType,
		EvalFnc: func(ctx *Context) string {
			return m.Value
		},
	}
}

// Append a value to the string
func (m *MutableStringVariable) Append(ctx *Context, value interface{}) error {
	switch value := value.(type) {
	case string:
		m.Value += value
	default:
		return errAppendNotSupported
	}
	return nil
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
	StringValues
}

// Set the variable with the specified value
func (m *MutableStringArrayVariable) Set(ctx *Context, values interface{}) error {
	if s, ok := values.(string); ok {
		values = []string{s}
	}

	m.StringValues = StringValues{}
	for _, v := range values.([]string) {
		m.AppendScalarValue(v)
	}
	return nil
}

// Append a value to the array
func (m *MutableStringArrayVariable) Append(ctx *Context, value interface{}) error {
	switch value := value.(type) {
	case string:
		m.AppendScalarValue(value)
	case []string:
		for _, v := range value {
			m.AppendScalarValue(v)
		}
	default:
		return errAppendNotSupported
	}
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *MutableStringArrayVariable) GetEvaluator() interface{} {
	return &StringArrayEvaluator{
		EvalFnc: func(ctx *Context) []string {
			return m.GetScalarValues()
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
	if i, ok := values.(int); ok {
		values = []int{i}
	}
	m.Values = values.([]int)
	return nil
}

// Append a value to the array
func (m *MutableIntArrayVariable) Append(ctx *Context, value interface{}) error {
	switch value := value.(type) {
	case int:
		m.Values = append(m.Values, value)
	case []int:
		m.Values = append(m.Values, value...)
	default:
		return errAppendNotSupported
	}
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

// ScopedVariable is the interface to be implemented by scoped variable in order to be released
type ScopedVariable interface {
	SetReleaseCallback(callback func())
}

// Scoper maps a variable to the entity its scoped to
type Scoper func(ctx *Context) ScopedVariable

// GlobalVariables holds a set of global variables
type GlobalVariables struct{}

// GetVariable returns new variable of the type of the specified value
func (v *GlobalVariables) GetVariable(name string, value interface{}) (VariableValue, error) {
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

// Variables holds a set of variables
type Variables struct {
	vars map[string]interface{}
}

// GetBool returns the boolean value of the specified variable
func (v *Variables) GetBool(name string) bool {
	if _, found := v.vars[name]; !found {
		return false
	}
	return v.vars[name].(bool)
}

// GetInt returns the integer value of the specified variable
func (v *Variables) GetInt(name string) int {
	if _, found := v.vars[name]; !found {
		return 0
	}
	return v.vars[name].(int)
}

// GetString returns the string value of the specified variable
func (v *Variables) GetString(name string) string {
	if _, found := v.vars[name]; !found {
		return ""
	}
	return v.vars[name].(string)
}

// GetStringArray returns the string array value of the specified variable
func (v *Variables) GetStringArray(name string) []string {
	if _, found := v.vars[name]; !found {
		return nil
	}
	return v.vars[name].([]string)
}

// GetIntArray returns the integer array value of the specified variable
func (v *Variables) GetIntArray(name string) []int {
	if _, found := v.vars[name]; !found {
		return nil
	}
	return v.vars[name].([]int)
}

// Set the value of the specified variable
func (v *Variables) Set(name string, value interface{}) bool {
	existed := false
	if v.vars == nil {
		v.vars = make(map[string]interface{})
	} else {
		_, existed = v.vars[name]
	}

	v.vars[name] = value
	return !existed
}

// ScopedVariables holds a set of scoped variables
type ScopedVariables struct {
	scoper Scoper
	vars   map[ScopedVariable]*Variables
}

// Len returns the length of the variable map
func (v *ScopedVariables) Len() int {
	return len(v.vars)
}

// GetVariable returns new variable of the type of the specified value
func (v *ScopedVariables) GetVariable(name string, value interface{}) (VariableValue, error) {
	getVariables := func(ctx *Context) *Variables {
		v := v.vars[v.scoper(ctx)]
		return v
	}

	setVariable := func(ctx *Context, value interface{}) error {
		key := v.scoper(ctx)
		if key == nil {
			return fmt.Errorf("failed to scope variable '%s'", name)
		}
		vars := v.vars[key]
		if vars == nil {
			key.SetReleaseCallback(func() {
				v.ReleaseVariable(key)
			})
			vars = &Variables{}
			v.vars[key] = vars
		}
		vars.Set(name, value)
		return nil
	}

	switch value.(type) {
	case int:
		return NewIntVariable(func(ctx *Context) int {
			if vars := getVariables(ctx); vars != nil {
				return vars.GetInt(name)
			}
			return 0
		}, setVariable), nil
	case bool:
		return NewBoolVariable(func(ctx *Context) bool {
			if vars := getVariables(ctx); vars != nil {
				return vars.GetBool(name)
			}
			return false
		}, setVariable), nil
	case string:
		return NewStringVariable(func(ctx *Context) string {
			if vars := getVariables(ctx); vars != nil {
				return vars.GetString(name)
			}
			return ""
		}, setVariable), nil
	case []string:
		return NewStringArrayVariable(func(ctx *Context) []string {
			if vars := getVariables(ctx); vars != nil {
				return vars.GetStringArray(name)
			}
			return nil
		}, setVariable), nil
	case []int:
		return NewIntArrayVariable(func(ctx *Context) []int {
			if vars := getVariables(ctx); vars != nil {
				return vars.GetIntArray(name)
			}
			return nil

		}, setVariable), nil
	default:
		return nil, fmt.Errorf("unsupported variable type %s for '%s'", reflect.TypeOf(value), name)
	}
}

// ReleaseVariable releases a scoped variable
func (v *ScopedVariables) ReleaseVariable(key ScopedVariable) {
	delete(v.vars, key)
}

// NewScopedVariables returns a new set of scope variables
func NewScopedVariables(scoper Scoper) *ScopedVariables {
	return &ScopedVariables{
		scoper: scoper,
		vars:   make(map[ScopedVariable]*Variables),
	}
}
