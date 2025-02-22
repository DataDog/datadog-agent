// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

const defaultMaxVariables = 100

var (
	variableRegex         = regexp.MustCompile(`\${[^}]*}`)
	errAppendNotSupported = errors.New("append is not supported")
)

// SECLVariable describes a SECL variable value
type SECLVariable interface {
	GetEvaluator() interface{}
}

// Variable is the interface implemented by variables
type Variable interface {
	GetValue() (interface{}, bool)
}

// ScopedVariable is the interface implemented by scoped variables
type ScopedVariable interface {
	GetValue(ctx *Context) (interface{}, bool)
	IsMutable() bool
}

// MutableVariable is the interface by variables whose value can be changed
type MutableVariable interface {
	Set(ctx *Context, value interface{}) error
	Append(ctx *Context, value interface{}) error
}

// settableVariable describes a SECL variable
type settableVariable struct {
	setFnc func(ctx *Context, value interface{}) error
}

// Set the variable with the specified value
func (v *settableVariable) Set(ctx *Context, value interface{}) error {
	if v.setFnc == nil {
		return errors.New("variable is not mutable")
	}

	return v.setFnc(ctx, value)
}

// Append a value to the variable
func (v *settableVariable) Append(_ *Context, _ interface{}) error {
	return errAppendNotSupported
}

// IsMutable returns whether the variable is settable
func (v *settableVariable) IsMutable() bool {
	return v.setFnc != nil
}

// ScopedIntVariable describes a scoped integer variable
type ScopedIntVariable struct {
	settableVariable
	intFnc func(ctx *Context) (int, bool)
}

// GetEvaluator returns the variable SECL evaluator
func (i *ScopedIntVariable) GetEvaluator() interface{} {
	return &IntEvaluator{
		EvalFnc: func(ctx *Context) int {
			i, _ := i.intFnc(ctx)
			return i
		},
	}
}

// GetValue returns the variable value
func (i *ScopedIntVariable) GetValue(ctx *Context) (interface{}, bool) {
	return i.intFnc(ctx)
}

// NewScopedIntVariable returns a new integer variable
func NewScopedIntVariable(intFnc func(ctx *Context) (int, bool), setFnc func(ctx *Context, value interface{}) error) *ScopedIntVariable {
	return &ScopedIntVariable{
		settableVariable: settableVariable{
			setFnc: setFnc,
		},
		intFnc: intFnc,
	}
}

// ScopedStringVariable describes a string variable
type ScopedStringVariable struct {
	settableVariable
	strFnc func(ctx *Context) (string, bool)
}

// GetEvaluator returns the variable SECL evaluator
func (s *ScopedStringVariable) GetEvaluator() interface{} {
	return &StringEvaluator{
		ValueType: VariableValueType,
		EvalFnc: func(ctx *Context) string {
			v, _ := s.strFnc(ctx)
			return v
		},
	}
}

// GetValue returns the variable value
func (s *ScopedStringVariable) GetValue(ctx *Context) (interface{}, bool) {
	return s.strFnc(ctx)
}

// NewScopedStringVariable returns a new scoped string variable
func NewScopedStringVariable(strFnc func(ctx *Context) (string, bool), setFnc func(ctx *Context, value interface{}) error) *ScopedStringVariable {
	return &ScopedStringVariable{
		strFnc: strFnc,
		settableVariable: settableVariable{
			setFnc: setFnc,
		},
	}
}

// ScopedBoolVariable describes a boolean variable
type ScopedBoolVariable struct {
	settableVariable
	boolFnc func(ctx *Context) (bool, bool)
}

// GetEvaluator returns the variable SECL evaluator
func (b *ScopedBoolVariable) GetEvaluator() interface{} {
	return &BoolEvaluator{
		EvalFnc: func(ctx *Context) bool {
			v, _ := b.boolFnc(ctx)
			return v
		},
	}
}

// GetValue returns the variable value
func (b *ScopedBoolVariable) GetValue(ctx *Context) (interface{}, bool) {
	return b.boolFnc(ctx)
}

// NewScopedBoolVariable returns a new boolean variable
func NewScopedBoolVariable(boolFnc func(ctx *Context) (bool, bool), setFnc func(ctx *Context, value interface{}) error) *ScopedBoolVariable {
	return &ScopedBoolVariable{
		boolFnc: boolFnc,
		settableVariable: settableVariable{
			setFnc: setFnc,
		},
	}
}

// ScopedStringArrayVariable describes a scoped string array variable
type ScopedStringArrayVariable struct {
	settableVariable
	strFnc func(ctx *Context) ([]string, bool)
}

// GetEvaluator returns the variable SECL evaluator
func (s *ScopedStringArrayVariable) GetEvaluator() interface{} {
	return &StringArrayEvaluator{
		EvalFnc: func(ctx *Context) []string {
			v, _ := s.strFnc(ctx)
			return v
		},
	}
}

// GetValue returns the variable value
func (s *ScopedStringArrayVariable) GetValue(ctx *Context) (interface{}, bool) {
	return s.strFnc(ctx)
}

// Set the array values
func (s *ScopedStringArrayVariable) Set(ctx *Context, value interface{}) error {
	if s, ok := value.(string); ok {
		value = []string{s}
	}
	return s.settableVariable.Set(ctx, value)
}

// Append a value to the array
func (s *ScopedStringArrayVariable) Append(ctx *Context, value interface{}) error {
	if val, ok := value.(string); ok {
		value = []string{val}
	}
	values, _ := s.strFnc(ctx)
	return s.Set(ctx, append(values, value.([]string)...))
}

// NewScopedStringArrayVariable returns a new scoped string array variable
func NewScopedStringArrayVariable(strFnc func(ctx *Context) ([]string, bool), setFnc func(ctx *Context, value interface{}) error) *ScopedStringArrayVariable {
	return &ScopedStringArrayVariable{
		strFnc: strFnc,
		settableVariable: settableVariable{
			setFnc: setFnc,
		},
	}
}

// ScopedIntArrayVariable describes a scoped integer array variable
type ScopedIntArrayVariable struct {
	settableVariable
	intFnc func(ctx *Context) ([]int, bool)
}

// GetEvaluator returns the variable SECL evaluator
func (v *ScopedIntArrayVariable) GetEvaluator() interface{} {
	return &IntArrayEvaluator{
		EvalFnc: func(ctx *Context) []int {
			s, _ := v.intFnc(ctx)
			return s
		},
	}
}

// GetValue returns the variable value
func (v *ScopedIntArrayVariable) GetValue(ctx *Context) (interface{}, bool) {
	return v.intFnc(ctx)
}

// Set the array values
func (v *ScopedIntArrayVariable) Set(ctx *Context, value interface{}) error {
	if i, ok := value.(int); ok {
		value = []int{i}
	}
	return v.settableVariable.Set(ctx, value)
}

// Append a value to the array
func (v *ScopedIntArrayVariable) Append(ctx *Context, value interface{}) error {
	if val, ok := value.(int); ok {
		value = []int{val}
	}
	values, _ := v.intFnc(ctx)
	return v.Set(ctx, append(values, value.([]int)...))
}

// NewScopedIntArrayVariable returns a new integer array variable
func NewScopedIntArrayVariable(intFnc func(ctx *Context) ([]int, bool), setFnc func(ctx *Context, value interface{}) error) *ScopedIntArrayVariable {
	return &ScopedIntArrayVariable{
		intFnc: intFnc,
		settableVariable: settableVariable{
			setFnc: setFnc,
		},
	}
}

// IntVariable describes a global integer variable
type IntVariable struct {
	set   bool
	Value int
}

// GetValue returns the variable value
func (m *IntVariable) GetValue() (interface{}, bool) {
	return m.Value, m.set
}

// Set the variable with the specified value
func (m *IntVariable) Set(_ *Context, value interface{}) error {
	m.set = true
	m.Value = value.(int)
	return nil
}

// Append a value to the integer
func (m *IntVariable) Append(_ *Context, value interface{}) error {
	switch value := value.(type) {
	case int:
		m.set = true
		m.Value += value
	default:
		return errAppendNotSupported
	}
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *IntVariable) GetEvaluator() interface{} {
	return &IntEvaluator{
		EvalFnc: func(*Context) int {
			return m.Value
		},
	}
}

// BoolVariable describes a mutable boolean variable
type BoolVariable struct {
	set   bool
	Value bool
}

// GetEvaluator returns the variable SECL evaluator
func (m *BoolVariable) GetEvaluator() interface{} {
	return &BoolEvaluator{
		EvalFnc: func(*Context) bool {
			return m.Value
		},
	}
}

// NewIntVariable returns a new mutable integer variable
func NewIntVariable() *IntVariable {
	return &IntVariable{}
}

// GetValue returns the variable value
func (m *BoolVariable) GetValue() (interface{}, bool) {
	return m.Value, m.set
}

// Set the variable with the specified value
func (m *BoolVariable) Set(_ *Context, value interface{}) error {
	m.Value = value.(bool)
	m.set = true
	return nil
}

// Append a value to the boolean
func (m *BoolVariable) Append(_ *Context, _ interface{}) error {
	return errAppendNotSupported
}

// NewBoolVariable returns a new mutable boolean variable
func NewBoolVariable() *BoolVariable {
	return &BoolVariable{}
}

// StringVariable describes a mutable string variable
type StringVariable struct {
	Value string
	set   bool
}

// GetEvaluator returns the variable SECL evaluator
func (m *StringVariable) GetEvaluator() interface{} {
	return &StringEvaluator{
		ValueType: VariableValueType,
		EvalFnc: func(_ *Context) string {
			return m.Value
		},
	}
}

// GetValue returns the variable value
func (m *StringVariable) GetValue() (interface{}, bool) {
	return m.Value, m.set
}

// Append a value to the string
func (m *StringVariable) Append(_ *Context, value interface{}) error {
	switch value := value.(type) {
	case string:
		m.Value += value
		m.set = true
	default:
		return errAppendNotSupported
	}
	return nil
}

// Set the variable with the specified value
func (m *StringVariable) Set(_ *Context, value interface{}) error {
	m.Value = value.(string)
	m.set = true
	return nil
}

// NewStringVariable returns a new mutable string variable
func NewStringVariable() *StringVariable {
	return &StringVariable{}
}

// StringArrayVariable describes a mutable string array variable
type StringArrayVariable struct {
	LRU *ttlcache.Cache[string, bool]
}

// GetValue returns the variable value
func (m *StringArrayVariable) GetValue() (interface{}, bool) {
	keys := m.LRU.Keys()
	return keys, len(keys) != 0
}

// Set the variable with the specified value
func (m *StringArrayVariable) Set(_ *Context, values interface{}) error {
	if s, ok := values.(string); ok {
		values = []string{s}
	}

	for _, v := range values.([]string) {
		m.LRU.Set(v, true, ttlcache.DefaultTTL)
	}

	return nil
}

// Append a value to the array
func (m *StringArrayVariable) Append(_ *Context, value interface{}) error {
	switch value := value.(type) {
	case string:
		m.LRU.Set(value, true, ttlcache.DefaultTTL)
	case []string:
		for _, v := range value {
			m.LRU.Set(v, true, ttlcache.DefaultTTL)
		}
	default:
		return errAppendNotSupported
	}
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *StringArrayVariable) GetEvaluator() interface{} {
	return &StringArrayEvaluator{
		EvalFnc: func(*Context) []string {
			return m.LRU.Keys()
		},
	}
}

// NewStringArrayVariable returns a new mutable string array variable
func NewStringArrayVariable(size int, ttl time.Duration) *StringArrayVariable {
	if size == 0 {
		size = defaultMaxVariables
	}

	lru := ttlcache.New(ttlcache.WithCapacity[string, bool](uint64(size)), ttlcache.WithTTL[string, bool](ttl))
	go lru.Start()

	return &StringArrayVariable{
		LRU: lru,
	}
}

// IntArrayVariable describes a mutable integer array variable
type IntArrayVariable struct {
	LRU *ttlcache.Cache[int, bool]
}

// GetValue returns the variable value
func (m *IntArrayVariable) GetValue() (interface{}, bool) {
	keys := m.LRU.Keys()
	return keys, len(keys) != 0
}

// Set the variable with the specified value
func (m *IntArrayVariable) Set(_ *Context, values interface{}) error {
	if i, ok := values.(int); ok {
		values = []int{i}
	}

	for _, v := range values.([]int) {
		m.LRU.Set(v, true, ttlcache.DefaultTTL)
	}

	return nil
}

// Append a value to the array
func (m *IntArrayVariable) Append(_ *Context, value interface{}) error {
	switch value := value.(type) {
	case int:
		m.LRU.Set(value, true, ttlcache.DefaultTTL)
	case []int:
		for _, v := range value {
			m.LRU.Set(v, true, ttlcache.DefaultTTL)
		}
	default:
		return errAppendNotSupported
	}
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *IntArrayVariable) GetEvaluator() interface{} {
	return &IntArrayEvaluator{
		EvalFnc: func(*Context) []int {
			return m.LRU.Keys()
		},
	}
}

// NewIntArrayVariable returns a new mutable integer array variable
func NewIntArrayVariable(size int, ttl time.Duration) *IntArrayVariable {
	if size == 0 {
		size = defaultMaxVariables
	}

	lru := ttlcache.New(ttlcache.WithCapacity[int, bool](uint64(size)), ttlcache.WithTTL[int, bool](ttl))
	go lru.Start()

	return &IntArrayVariable{
		LRU: lru,
	}
}

// VariableScope is the interface to be implemented by scoped variable in order to be released
type VariableScope interface {
	AppendReleaseCallback(callback func())
}

// Scoper maps a variable to the entity its scoped to
type Scoper func(ctx *Context) VariableScope

// Variables holds a set of variables
type Variables struct{}

// VariableOpts holds the options of a variable set
type VariableOpts struct {
	Size int
	TTL  time.Duration
}

// NewVariables returns a new set of global variables
func NewVariables() *Variables {
	return &Variables{}
}

func newSECLVariable(value interface{}, opts VariableOpts) (MutableSECLVariable, error) {
	switch value := value.(type) {
	case bool:
		return NewBoolVariable(), nil
	case int:
		return NewIntVariable(), nil
	case string:
		return NewStringVariable(), nil
	case []string:
		return NewStringArrayVariable(opts.Size, opts.TTL), nil
	case []int:
		return NewIntArrayVariable(opts.Size, opts.TTL), nil
	default:
		return nil, fmt.Errorf("unsupported value type: %s", reflect.TypeOf(value))
	}
}

// NewSECLVariable returns new variable of the type of the specified value
func (v *Variables) NewSECLVariable(_ string, value interface{}, opts VariableOpts) (SECLVariable, error) {
	seclVariable, err := newSECLVariable(value, opts)
	if err != nil {
		return nil, err
	}
	return seclVariable.(SECLVariable), nil
}

// MutableSECLVariable describes the interface implemented by mutable SECL variable
type MutableSECLVariable interface {
	Variable
	MutableVariable
}

// ScopedVariables holds a set of scoped variables
type ScopedVariables struct {
	scoper Scoper
	vars   map[VariableScope]map[string]MutableSECLVariable
}

// Len returns the length of the variable map
func (v *ScopedVariables) Len() int {
	return len(v.vars)
}

// NewSECLVariable returns new variable of the type of the specified value
func (v *ScopedVariables) NewSECLVariable(name string, value interface{}, opts VariableOpts) (SECLVariable, error) {
	getVariable := func(ctx *Context) MutableSECLVariable {
		v := v.vars[v.scoper(ctx)]
		return v[name]
	}

	setVariable := func(ctx *Context, value interface{}) error {
		key := v.scoper(ctx)
		if key == nil {
			return fmt.Errorf("failed to scope variable '%s'", name)
		}

		vars := v.vars[key]
		if vars == nil {
			key.AppendReleaseCallback(func() {
				v.ReleaseVariable(key)
			})

			v.vars[key] = make(map[string]MutableSECLVariable)
		}

		if _, found := v.vars[key][name]; !found {
			seclVariable, err := newSECLVariable(value, opts)
			if err != nil {
				return err
			}
			v.vars[key][name] = seclVariable
		}

		return v.vars[key][name].Set(ctx, value)
	}

	switch value.(type) {
	case int:
		return NewScopedIntVariable(func(ctx *Context) (int, bool) {
			if v := getVariable(ctx); v != nil {
				value, set := v.GetValue()
				return value.(int), set
			}
			return 0, false
		}, setVariable), nil
	case bool:
		return NewScopedBoolVariable(func(ctx *Context) (bool, bool) {
			if v := getVariable(ctx); v != nil {
				value, set := v.GetValue()
				return value.(bool), set
			}
			return false, false
		}, setVariable), nil
	case string:
		return NewScopedStringVariable(func(ctx *Context) (string, bool) {
			if v := getVariable(ctx); v != nil {
				value, set := v.GetValue()
				return value.(string), set
			}
			return "", false
		}, setVariable), nil
	case []string:
		return NewScopedStringArrayVariable(func(ctx *Context) ([]string, bool) {
			if v := getVariable(ctx); v != nil {
				value, set := v.GetValue()
				return value.([]string), set
			}
			return nil, false
		}, setVariable), nil
	case []int:
		return NewScopedIntArrayVariable(func(ctx *Context) ([]int, bool) {
			if v := getVariable(ctx); v != nil {
				value, set := v.GetValue()
				return value.([]int), set
			}
			return nil, false

		}, setVariable), nil
	default:
		return nil, fmt.Errorf("unsupported variable type %s for '%s'", reflect.TypeOf(value), name)
	}
}

// ReleaseVariable releases a scoped variable
func (v *ScopedVariables) ReleaseVariable(key VariableScope) {
	delete(v.vars, key)
}

// NewScopedVariables returns a new set of scope variables
func NewScopedVariables(scoper Scoper) *ScopedVariables {
	return &ScopedVariables{
		scoper: scoper,
		vars:   make(map[VariableScope]map[string]MutableSECLVariable),
	}
}
