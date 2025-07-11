// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"sync"
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
	GetVariableOpts() VariableOpts
	SetVariableOpts(opts VariableOpts)
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
	GetVariableOpts() VariableOpts
	SetVariableOpts(opts VariableOpts)
}

// settableVariable describes a SECL variable
type settableVariable struct {
	setFnc func(ctx *Context, value interface{}) error
	opts   VariableOpts
}

type expirableVariable interface {
	CleanupExpired()
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

// GetVariableOpts returns the variable options
func (v *settableVariable) GetVariableOpts() VariableOpts {
	return v.opts
}

// SetVariableOpts sets the variable VariableOpts
func (v *settableVariable) SetVariableOpts(opts VariableOpts) {
	v.opts = opts
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
			opts: VariableOpts{
				Private: false,
			},
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
			opts: VariableOpts{
				Private: false,
			},
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
			opts: VariableOpts{
				Private: false,
			},
		},
	}
}

// ScopedIPVariable describes a scoped IP variable
type ScopedIPVariable struct {
	settableVariable
	ipFnc func(ctx *Context) (net.IPNet, bool)
}

// GetEvaluator returns the variable SECL evaluator
func (i *ScopedIPVariable) GetEvaluator() interface{} {
	return &CIDREvaluator{
		EvalFnc: func(ctx *Context) net.IPNet {
			i, _ := i.ipFnc(ctx)
			return i
		},
	}
}

// GetValue returns the variable value
func (i *ScopedIPVariable) GetValue(ctx *Context) (interface{}, bool) {
	return i.ipFnc(ctx)
}

// NewScopedIPVariable returns a new scoped IP variable
func NewScopedIPVariable(ipFnc func(ctx *Context) (net.IPNet, bool), setFnc func(ctx *Context, value interface{}) error) *ScopedIPVariable {
	return &ScopedIPVariable{
		ipFnc: ipFnc,
		settableVariable: settableVariable{
			setFnc: setFnc,
			opts: VariableOpts{
				Private: false,
			},
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
			opts: VariableOpts{
				Private: false,
			},
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
			opts: VariableOpts{
				Private: false,
			},
		},
	}
}

// ScopedIPArrayVariable describes a scoped IP array variable
type ScopedIPArrayVariable struct {
	settableVariable
	ipFnc func(ctx *Context) ([]net.IPNet, bool)
}

// GetEvaluator returns the variable SECL evaluator
func (i *ScopedIPArrayVariable) GetEvaluator() interface{} {
	return &CIDRArrayEvaluator{
		EvalFnc: func(ctx *Context) []net.IPNet {
			i, _ := i.ipFnc(ctx)
			return i
		},
	}
}

// GetValue returns the variable value
func (i *ScopedIPArrayVariable) GetValue(ctx *Context) (interface{}, bool) {
	return i.ipFnc(ctx)
}

// Set the array values
func (i *ScopedIPArrayVariable) Set(ctx *Context, value interface{}) error {
	if ip, ok := value.(net.IPNet); ok {
		value = []net.IPNet{ip}
	}
	return i.settableVariable.Set(ctx, value)
}

// Append a value to the array
func (i *ScopedIPArrayVariable) Append(ctx *Context, value interface{}) error {
	if val, ok := value.(net.IPNet); ok {
		value = []net.IPNet{val}
	}
	values, _ := i.ipFnc(ctx)
	return i.Set(ctx, append(values, value.([]net.IPNet)...))
}

// NewScopedIPArrayVariable returns a new IP array variable
func NewScopedIPArrayVariable(ipFnc func(ctx *Context) ([]net.IPNet, bool), setFnc func(ctx *Context, value interface{}) error) *ScopedIPArrayVariable {
	return &ScopedIPArrayVariable{
		ipFnc: ipFnc,
		settableVariable: settableVariable{
			setFnc: setFnc,
			opts: VariableOpts{
				Private: false,
			},
		},
	}
}

type variableWithTTL struct {
	ttl       time.Duration
	expiresAt time.Time
}

func (v *variableWithTTL) isExpired() bool {
	return v.ttl > 0 && v.expiresAt.Before(time.Now())
}

func (v *variableWithTTL) touchTTL() {
	if v.ttl > 0 {
		v.expiresAt = time.Now().Add(v.ttl)
	}
}

// IntVariable describes a global integer variable
type IntVariable struct {
	isSet bool
	Value int
	variableWithTTL
	opts VariableOpts
}

// GetValue returns the variable value
func (m *IntVariable) GetValue() (interface{}, bool) {
	if m.isExpired() {
		var defaultValue int
		return defaultValue, false
	}
	return m.Value, m.isSet
}

// Set the variable with the specified value
func (m *IntVariable) Set(_ *Context, value interface{}) error {
	m.Value = value.(int)
	m.isSet = true
	m.touchTTL()
	return nil
}

// Append a value to the integer
func (m *IntVariable) Append(_ *Context, value interface{}) error {
	switch value := value.(type) {
	case int:
		m.Value += value
		m.isSet = true
		m.touchTTL()
	default:
		return errAppendNotSupported
	}
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *IntVariable) GetEvaluator() interface{} {
	return &IntEvaluator{
		EvalFnc: func(*Context) int {
			value, _ := m.GetValue()
			return value.(int)
		},
	}
}

// GetVariableOpts returns the variable VariableOpts
func (m *IntVariable) GetVariableOpts() VariableOpts {
	return m.opts
}

// SetVariableOpts sets the variable VariableOpts
func (m *IntVariable) SetVariableOpts(opts VariableOpts) {
	m.opts = opts
}

// BoolVariable describes a mutable boolean variable
type BoolVariable struct {
	isSet bool
	Value bool
	variableWithTTL
	opts VariableOpts
}

// GetEvaluator returns the variable SECL evaluator
func (m *BoolVariable) GetEvaluator() interface{} {
	return &BoolEvaluator{
		EvalFnc: func(*Context) bool {
			value, _ := m.GetValue()
			return value.(bool)
		},
	}
}

// GetVariableOpts returns the variable VariableOpts
func (m *BoolVariable) GetVariableOpts() VariableOpts {
	return m.opts
}

// SetVariableOpts sets the variable VariableOpts
func (m *BoolVariable) SetVariableOpts(opts VariableOpts) {
	m.opts = opts
}

// NewIntVariable returns a new mutable integer variable
func NewIntVariable(value int, opts VariableOpts) *IntVariable {
	return &IntVariable{
		Value: value,
		variableWithTTL: variableWithTTL{
			ttl: opts.TTL,
		},
		opts: opts,
	}
}

// GetValue returns the variable value
func (m *BoolVariable) GetValue() (interface{}, bool) {
	if m.isExpired() {
		var defaultValue bool
		return defaultValue, false
	}
	return m.Value, m.isSet
}

// Set the variable with the specified value
func (m *BoolVariable) Set(_ *Context, value interface{}) error {
	m.Value = value.(bool)
	m.isSet = true
	m.touchTTL()
	return nil
}

// Append a value to the boolean
func (m *BoolVariable) Append(_ *Context, _ interface{}) error {
	return errAppendNotSupported
}

// NewBoolVariable returns a new mutable boolean variable
func NewBoolVariable(value bool, opts VariableOpts) *BoolVariable {
	return &BoolVariable{
		Value: value,
		variableWithTTL: variableWithTTL{
			ttl: opts.TTL,
		},
		opts: opts,
	}
}

// StringVariable describes a mutable string variable
type StringVariable struct {
	Value string
	isSet bool
	variableWithTTL
	opts VariableOpts
}

// GetEvaluator returns the variable SECL evaluator
func (m *StringVariable) GetEvaluator() interface{} {
	return &StringEvaluator{
		ValueType: VariableValueType,
		EvalFnc: func(_ *Context) string {
			value, _ := m.GetValue()
			return value.(string)
		},
	}
}

// GetVariableOpts returns the variable VariableOpts
func (m *StringVariable) GetVariableOpts() VariableOpts {
	return m.opts
}

// SetVariableOpts sets the variable VariableOpts
func (m *StringVariable) SetVariableOpts(opts VariableOpts) {
	m.opts = opts
}

// GetValue returns the variable value
func (m *StringVariable) GetValue() (interface{}, bool) {
	if m.isExpired() {
		var defaultValue string
		return defaultValue, false
	}
	return m.Value, m.isSet
}

// Append a value to the string
func (m *StringVariable) Append(_ *Context, value interface{}) error {
	switch value := value.(type) {
	case string:
		m.Value += value
		m.isSet = true
		m.touchTTL()
	default:
		return errAppendNotSupported
	}
	return nil
}

// Set the variable with the specified value
func (m *StringVariable) Set(_ *Context, value interface{}) error {
	m.Value = value.(string)
	m.isSet = true
	m.touchTTL()
	return nil
}

// NewStringVariable returns a new mutable string variable
func NewStringVariable(value string, opts VariableOpts) *StringVariable {
	return &StringVariable{
		Value: value,
		variableWithTTL: variableWithTTL{
			ttl: opts.TTL,
		},
		opts: opts,
	}
}

// IPVariable describes a global IP variable
type IPVariable struct {
	Value net.IPNet
	isSet bool
	variableWithTTL
	opts VariableOpts
}

// GetValue returns the variable value
func (m *IPVariable) GetValue() (interface{}, bool) {
	if m.isExpired() {
		var defaultValue net.IPNet
		return defaultValue, false
	}
	return m.Value, m.isSet
}

// Set the variable with the specified value
func (m *IPVariable) Set(_ *Context, value interface{}) error {
	m.Value = value.(net.IPNet)
	m.isSet = true
	m.touchTTL()
	return nil
}

// Append a value to the IP
func (m *IPVariable) Append(_ *Context, _ interface{}) error {
	return errAppendNotSupported
}

// GetEvaluator returns the variable SECL evaluator
func (m *IPVariable) GetEvaluator() interface{} {
	return &CIDREvaluator{
		EvalFnc: func(*Context) net.IPNet {
			value, _ := m.GetValue()
			return value.(net.IPNet)
		},
	}
}

// GetVariableOpts returns the variable VariableOpts
func (m *IPVariable) GetVariableOpts() VariableOpts {
	return m.opts
}

// SetVariableOpts sets the variable VariableOpts
func (m *IPVariable) SetVariableOpts(opts VariableOpts) {
	m.opts = opts
}

// NewIPVariable returns a new mutable IP variable
func NewIPVariable(value net.IPNet, opts VariableOpts) *IPVariable {
	return &IPVariable{
		Value: value,
		variableWithTTL: variableWithTTL{
			ttl: opts.TTL,
		},
		opts: opts,
	}
}

// StringArrayVariable describes a mutable string array variable
type StringArrayVariable struct {
	isSet bool
	LRU   *ttlcache.Cache[string, bool]
	opts  VariableOpts
}

// GetValue returns the variable value
func (m *StringArrayVariable) GetValue() (interface{}, bool) {
	keys := m.LRU.Keys()
	return keys, len(keys) != 0 && m.isSet
}

func (m *StringArrayVariable) set(_ *Context, values interface{}) error {
	if s, ok := values.(string); ok {
		values = []string{s}
	}
	for _, v := range values.([]string) {
		m.LRU.Set(v, true, ttlcache.DefaultTTL)
	}
	return nil
}

// Set the variable with the specified value
func (m *StringArrayVariable) Set(ctx *Context, values interface{}) error {
	if err := m.set(ctx, values); err != nil {
		return err
	}
	m.isSet = true
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
	m.isSet = true
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

// GetVariableOpts returns the variable VariableOpts
func (m *StringArrayVariable) GetVariableOpts() VariableOpts {
	return m.opts
}

// SetVariableOpts sets the variable VariableOpts
func (m *StringArrayVariable) SetVariableOpts(opts VariableOpts) {
	m.opts = opts
}

// CleanupExpired cleans up expired values from the variable
// note that this method in only used to free up memory
// as expired entries are already not returned by the LRU.Keys() method
func (m *StringArrayVariable) CleanupExpired() {
	m.LRU.DeleteExpired()
}

// NewStringArrayVariable returns a new mutable string array variable
func NewStringArrayVariable(value []string, opts VariableOpts) *StringArrayVariable {
	if opts.Size == 0 {
		opts.Size = defaultMaxVariables
	}

	lru := ttlcache.New(ttlcache.WithCapacity[string, bool](uint64(opts.Size)), ttlcache.WithTTL[string, bool](opts.TTL))

	v := &StringArrayVariable{
		LRU:  lru,
		opts: opts,
	}
	_ = v.set(nil, value)
	return v
}

// IntArrayVariable describes a mutable integer array variable
type IntArrayVariable struct {
	isSet bool
	LRU   *ttlcache.Cache[int, bool]
	opts  VariableOpts
}

// GetValue returns the variable value
func (m *IntArrayVariable) GetValue() (interface{}, bool) {
	keys := m.LRU.Keys()
	return keys, len(keys) != 0 && m.isSet
}

func (m *IntArrayVariable) set(_ *Context, values interface{}) error {
	if i, ok := values.(int); ok {
		values = []int{i}
	}

	for _, v := range values.([]int) {
		m.LRU.Set(v, true, ttlcache.DefaultTTL)
	}

	return nil
}

// Set the variable with the specified value
func (m *IntArrayVariable) Set(ctx *Context, values interface{}) error {
	if err := m.set(ctx, values); err != nil {
		return err
	}
	m.isSet = true
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
	m.isSet = true
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

// GetVariableOpts returns the variable VariableOpts
func (m *IntArrayVariable) GetVariableOpts() VariableOpts {
	return m.opts
}

// SetVariableOpts sets the variable VariableOpts
func (m *IntArrayVariable) SetVariableOpts(opts VariableOpts) {
	m.opts = opts
}

// CleanupExpired cleans up expired values from the variable
// note that this method in only used to free up memory
// as expired entries are already not returned by the LRU.Keys() method
func (m *IntArrayVariable) CleanupExpired() {
	m.LRU.DeleteExpired()
}

// NewIntArrayVariable returns a new mutable integer array variable
func NewIntArrayVariable(value []int, opts VariableOpts) *IntArrayVariable {
	if opts.Size == 0 {
		opts.Size = defaultMaxVariables
	}

	lru := ttlcache.New(ttlcache.WithCapacity[int, bool](uint64(opts.Size)), ttlcache.WithTTL[int, bool](opts.TTL))

	v := &IntArrayVariable{
		LRU:  lru,
		opts: opts,
	}
	_ = v.set(nil, value)
	return v
}

// IPArrayVariable describes a global IP array variable
type IPArrayVariable struct {
	LRU   *ttlcache.Cache[string, bool]
	isSet bool
	opts  VariableOpts
}

// GetValue returns the variable value
func (m *IPArrayVariable) GetValue() (interface{}, bool) {
	keys := []net.IPNet{}
	for _, v := range m.LRU.Keys() {
		_, ipNet, err := net.ParseCIDR(v)
		if err == nil {
			keys = append(keys, *ipNet)
		}
	}
	return keys, len(keys) != 0 && m.isSet
}

func (m *IPArrayVariable) set(_ *Context, values interface{}) error {
	if ip, ok := values.(net.IPNet); ok {
		values = []net.IPNet{ip}
	}

	for _, v := range values.([]net.IPNet) {
		m.LRU.Set(v.String(), true, ttlcache.DefaultTTL)
	}

	return nil
}

// Set the variable with the specified value
func (m *IPArrayVariable) Set(ctx *Context, values interface{}) error {
	if err := m.set(ctx, values); err != nil {
		return err
	}
	m.isSet = true
	return nil
}

// Append a value to the array
func (m *IPArrayVariable) Append(_ *Context, value interface{}) error {
	switch value := value.(type) {
	case net.IPNet:
		m.LRU.Set(value.String(), true, ttlcache.DefaultTTL)
	case []net.IPNet:
		for _, v := range value {
			m.LRU.Set(v.String(), true, ttlcache.DefaultTTL)
		}
	default:
		return errAppendNotSupported
	}

	m.isSet = true
	return nil
}

// GetEvaluator returns the variable SECL evaluator
func (m *IPArrayVariable) GetEvaluator() interface{} {
	return &CIDRArrayEvaluator{
		EvalFnc: func(*Context) []net.IPNet {
			keys := []net.IPNet{}
			for _, v := range m.LRU.Keys() {
				_, ipNet, err := net.ParseCIDR(v)
				if err == nil {
					keys = append(keys, *ipNet)
				}
			}
			return keys
		},
	}
}

// GetVariableOpts returns the variable VariableOpts
func (m *IPArrayVariable) GetVariableOpts() VariableOpts {
	return m.opts
}

// SetVariableOpts sets the variable VariableOpts
func (m *IPArrayVariable) SetVariableOpts(opts VariableOpts) {
	m.opts = opts
}

// CleanupExpired cleans up expired values from the variable
// note that this method in only used to free up memory
// as expired entries are already not returned by the LRU.Keys() method
func (m *IPArrayVariable) CleanupExpired() {
	m.LRU.DeleteExpired()
}

// NewIPArrayVariable returns a new mutable IP array variable
func NewIPArrayVariable(value []net.IPNet, opts VariableOpts) *IPArrayVariable {
	if opts.Size == 0 {
		opts.Size = defaultMaxVariables
	}

	lru := ttlcache.New(ttlcache.WithCapacity[string, bool](uint64(opts.Size)), ttlcache.WithTTL[string, bool](opts.TTL))

	v := &IPArrayVariable{
		LRU:  lru,
		opts: opts,
	}
	_ = v.set(nil, value)
	return v
}

// VariableScope is the interface to be implemented by scoped variable in order to be released
type VariableScope interface {
	AppendReleaseCallback(callback func())
	Hash() string
	ParentScope() (VariableScope, bool)
}

// Scoper maps a variable to the entity its scoped to
type Scoper func(ctx *Context) VariableScope

// Variables holds a set of variables
type Variables struct {
	expirablesLock sync.RWMutex
	expirables     []expirableVariable
}

// VariableOpts holds the options of a variable set
type VariableOpts struct {
	Size      int
	TTL       time.Duration
	Private   bool // When a variable is marked as private, it will not be included in the serialized event
	Inherited bool
}

// NewVariables returns a new set of global variables
func NewVariables() *Variables {
	return &Variables{}
}

func newSECLVariable(value interface{}, opts VariableOpts) (MutableSECLVariable, error) {
	switch value := value.(type) {
	case bool:
		return NewBoolVariable(value, opts), nil
	case int:
		return NewIntVariable(value, opts), nil
	case string:
		return NewStringVariable(value, opts), nil
	case net.IPNet:
		return NewIPVariable(value, opts), nil
	case []string:
		return NewStringArrayVariable(value, opts), nil
	case []int:
		return NewIntArrayVariable(value, opts), nil
	case []net.IPNet:
		return NewIPArrayVariable(value, opts), nil
	default:
		return nil, fmt.Errorf("unsupported value type: %v", reflect.TypeOf(value))
	}
}

// NewSECLVariable returns new variable of the type of the specified value
func (v *Variables) NewSECLVariable(_ string, value interface{}, opts VariableOpts) (SECLVariable, error) {
	seclVariable, err := newSECLVariable(value, opts)
	if err != nil {
		return nil, err
	}

	if expirable, ok := seclVariable.(expirableVariable); ok && opts.TTL > 0 {
		v.expirablesLock.Lock()
		v.expirables = append(v.expirables, expirable)
		v.expirablesLock.Unlock()
	}

	return seclVariable.(SECLVariable), nil
}

// CleanupExpiredVariables cleans up expired variables
func (v *Variables) CleanupExpiredVariables() {
	v.expirablesLock.RLock()
	defer v.expirablesLock.RUnlock()
	for _, expirable := range v.expirables {
		expirable.CleanupExpired()
	}
}

// MutableSECLVariable describes the interface implemented by mutable SECL variable
type MutableSECLVariable interface {
	Variable
	MutableVariable
	SECLVariable
}

// ScopedVariables holds a set of scoped variables
type ScopedVariables struct {
	scoperName     string
	scoper         Scoper
	vars           map[string]map[string]MutableSECLVariable
	expirablesLock sync.RWMutex
	expirables     map[string][]expirableVariable
}

// Len returns the length of the variable map
func (v *ScopedVariables) Len() int {
	return len(v.vars)
}

// NewSECLVariable returns new variable of the type of the specified value
func (v *ScopedVariables) NewSECLVariable(name string, value any, opts VariableOpts) (SECLVariable, error) {
	getVariable := func(ctx *Context) MutableSECLVariable {
		scope := v.scoper(ctx)
		if scope == nil {
			return nil
		}
		key := scope.Hash()
		vars := v.vars[key]
		if (vars == nil || vars[name] == nil) && opts.Inherited {
			var ok bool
			scope, ok = scope.ParentScope()
			for vars == nil && ok {
				key := scope.Hash()
				vars = v.vars[key]
				scope, ok = scope.ParentScope()
			}
		}
		return vars[name]
	}

	setVariable := func(ctx *Context, value any) error {
		scope := v.scoper(ctx)
		if scope == nil {
			return fmt.Errorf("`%s` scoper failed to scope variable '%s'", v.scoperName, name)
		}

		key := scope.Hash()
		vars := v.vars[key]
		if vars == nil {
			scope.AppendReleaseCallback(func() {
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
			if expirable, ok := seclVariable.(expirableVariable); ok && opts.TTL > 0 {
				v.expirablesLock.Lock()
				v.expirables[key] = append(v.expirables[key], expirable)
				v.expirablesLock.Unlock()
			}
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
	case net.IPNet:
		return NewScopedIPVariable(func(ctx *Context) (net.IPNet, bool) {
			if v := getVariable(ctx); v != nil {
				value, set := v.GetValue()
				return value.(net.IPNet), set
			}
			return net.IPNet{}, false
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
	case []net.IPNet:
		return NewScopedIPArrayVariable(func(ctx *Context) ([]net.IPNet, bool) {
			if v := getVariable(ctx); v != nil {
				value, set := v.GetValue()
				return value.([]net.IPNet), set
			}
			return nil, false
		}, setVariable), nil
	default:
		return nil, fmt.Errorf("unsupported variable type %s for '%s'", reflect.TypeOf(value), name)
	}
}

// CleanupExpiredVariables cleans up expired variables
func (v *ScopedVariables) CleanupExpiredVariables() {
	v.expirablesLock.RLock()
	defer v.expirablesLock.RUnlock()
	for _, expirables := range v.expirables {
		for _, expirable := range expirables {
			expirable.CleanupExpired()
		}
	}
}

// ReleaseVariable releases a scoped variable
func (v *ScopedVariables) ReleaseVariable(key string) {
	delete(v.vars, key)
	v.expirablesLock.Lock()
	delete(v.expirables, key)
	v.expirablesLock.Unlock()
}

// NewScopedVariables returns a new set of scope variables
func NewScopedVariables(scoperName string, scoper Scoper) *ScopedVariables {
	return &ScopedVariables{
		scoperName: scoperName,
		scoper:     scoper,
		vars:       make(map[string]map[string]MutableSECLVariable),
		expirables: make(map[string][]expirableVariable),
	}
}
