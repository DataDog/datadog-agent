// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"regexp"
	"sync"
	"time"

	ttlcache "github.com/jellydator/ttlcache/v3"
)

var variableRegex = regexp.MustCompile(`\${[^}]*}`)

const defaultMaxVariables = 100

// VariableScope represents the scope of a variable
type VariableScope interface {
	Key() (string, bool)
	ParentScope() (VariableScope, bool)
}

// ReleasableVariableScope represents a scope that can be released
type ReleasableVariableScope interface {
	AppendReleaseCallback(callback func())
}

// VariableScoper represents a variable scoper
type VariableScoper struct {
	scoperType InternalScoperType
	getScopeCb func(ctx *Context) (VariableScope, error)
}

// NewVariableScoper returns a new variable scoper
func NewVariableScoper(scoperType InternalScoperType, cb func(ctx *Context) (VariableScope, error)) *VariableScoper {
	return &VariableScoper{
		scoperType: scoperType,
		getScopeCb: cb,
	}
}

// Type returns the type of the variable scoper
func (vs *VariableScoper) Type() InternalScoperType {
	return vs.scoperType
}

// GetScope returns a variable scope based on the given Context
func (vs *VariableScoper) GetScope(ctx *Context) (VariableScope, error) {
	return vs.getScopeCb(ctx)
}

// InternalScoperType represents the type of a scoper
type InternalScoperType int

const (
	// UndefinedScoperType is the undefinied scoper
	UndefinedScoperType InternalScoperType = iota
	// GlobalScoperType handles the global scope
	GlobalScoperType
	// ProcessScoperType handles process scopes
	ProcessScoperType
	// ContainerScoperType handles container scopes
	ContainerScoperType
	// CGroupScoperType handles cgroup scopes
	CGroupScoperType
)

// String returns the name of the scoper
func (isn InternalScoperType) String() string {
	switch isn {
	case GlobalScoperType:
		return "global"
	case ProcessScoperType:
		return "process"
	case ContainerScoperType:
		return "container"
	case CGroupScoperType:
		return "cgroup"
	default:
		return ""
	}
}

// VariablePrefix returns the variable prefix that corresponds to this scoper type
func (isn InternalScoperType) VariablePrefix() string {
	switch isn {
	case ProcessScoperType:
		return "process"
	case ContainerScoperType:
		return "container"
	case CGroupScoperType:
		return "cgroup"
	default:
		return ""
	}
}

// VariableType lists the types that a SECL variable can take
type VariableType interface {
	string | int | bool | net.IPNet |
		[]string | []int | []net.IPNet
}

type staticVariable[T VariableType] struct {
	getValueCb func(ctx *Context) T
}

// GetValue returns the value of the variable
func (s *staticVariable[T]) GetValue(ctx *Context) any {
	return s.getValueCb(ctx)
}

// StaticVariable represents a hard-coded variable
type StaticVariable interface {
	GetValue(ctx *Context) any
}

// NewStaticVariable returns a new static variable
func NewStaticVariable[T VariableType](getValue func(ctx *Context) T) StaticVariable {
	return &staticVariable[T]{
		getValueCb: getValue,
	}
}

// VariableDefinition represents the definition of a SECL variable
type VariableDefinition interface {
	VariableName(withScopePrefix bool) string
	DefaultValue() any
	IsPrivate() bool
	Scoper() *VariableScoper
	AddNewInstance(ctx *Context) (VariableInstance, bool, error)
	GetInstance(ctx *Context) (VariableInstance, bool, error)
	GetInstancesCount() int
	CleanupExpiredVariables()
	getInstances() map[string]VariableInstance
}

// VariableOpts represents the options of a SECL variable
type VariableOpts struct {
	TTL       time.Duration
	Size      int
	Private   bool
	Inherited bool
	Append    bool
	Telemetry *Telemetry
}

type definition[T VariableType] struct {
	name         string
	valueType    string
	defaultValue T
	scoper       *VariableScoper
	opts         *VariableOpts

	instancesLock sync.RWMutex
	instances     map[string]VariableInstance
}

// NewVariableDefinition returns a new definition of a SECL variable
func NewVariableDefinition[T VariableType](name string, scoper *VariableScoper, defaultValue T, opts VariableOpts) VariableDefinition {
	return &definition[T]{
		name:         name,
		defaultValue: defaultValue,
		valueType:    getValueType[T](defaultValue),
		scoper:       scoper,
		opts:         &opts,
		instances:    make(map[string]VariableInstance),
	}
}

// Name returns the name of the variable
func (def *definition[T]) VariableName(withScopePrefix bool) string {
	if !withScopePrefix || def.scoper.Type() == GlobalScoperType {
		return def.name
	}
	return def.scoper.Type().VariablePrefix() + "." + def.name
}

// DefaultValue returns the default value of the definition
func (def *definition[T]) DefaultValue() any {
	return def.defaultValue
}

// IsPrivate returns whether the variable is definied as private
func (def *definition[T]) IsPrivate() bool {
	return def.opts.Private
}

// Scoper returns the scoper of the variable definition
func (def *definition[T]) Scoper() *VariableScoper {
	return def.scoper
}

// AddNewInstance instanciates and adds a new variable instance for the given Context
func (def *definition[T]) AddNewInstance(ctx *Context) (VariableInstance, bool, error) {
	scope, err := def.scoper.GetScope(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get scope `%s` of variable `%s`: %w", def.scoper.Type(), def.name, err)
	}

	var newInstance VariableInstance
	key, ok := scope.Key()
	if ok {
		newInstance = def.newInstance(func() {
			if def.opts.Telemetry != nil {
				def.opts.Telemetry.TotalVariables.Dec(def.valueType, def.scoper.Type().String())
			}
			delete(def.instances, key) // instancesLock must be held when this closure is called
		})

		if def.opts.Telemetry != nil {
			def.opts.Telemetry.TotalVariables.Inc(def.valueType, def.scoper.Type().String())
		}

		if releaseable, ok := scope.(ReleasableVariableScope); ok {
			releaseable.AppendReleaseCallback(func() {
				def.instancesLock.Lock()
				defer def.instancesLock.Unlock()
				newInstance.free()
			})
		}

		def.instancesLock.Lock()
		defer def.instancesLock.Unlock()
		def.instances[key] = newInstance
	}

	return newInstance, ok, nil // newInstance can be nil here
}

// GetInstance returns a variable instance that corresponds to the given Context or nil if no instance was found
func (def *definition[T]) GetInstance(ctx *Context) (VariableInstance, bool, error) {
	var instance VariableInstance
	var instanceOk bool

	scope, err := def.scoper.GetScope(ctx)
	if err != nil {
		return nil, false, &ErrScopeFailure{VarName: def.name, ScoperType: def.scoper.scoperType, ScoperErr: err}
	}

	def.instancesLock.RLock()
	defer def.instancesLock.RUnlock()

	key, scopeOk := scope.Key()
	if scopeOk {
		instance, instanceOk = def.instances[key]
	}

	if !instanceOk && def.opts.Inherited {
		var parentScopeOk bool
		scope, parentScopeOk = scope.ParentScope()
		for parentScopeOk && !instanceOk {
			key, scopeOk = scope.Key()
			if scopeOk {
				instance, instanceOk = def.instances[key]
			}
			scope, parentScopeOk = scope.ParentScope()
		}
	}

	// instance can be nil here if no instance exists
	return instance, instanceOk, nil
}

// GetInstancesCount returns the number of instances this variable current has
func (def *definition[T]) GetInstancesCount() int {
	def.instancesLock.RLock()
	defer def.instancesLock.RUnlock()
	return len(def.instances)
}

// CleanupExpiredVariables deletes expired variable instances
func (def *definition[T]) CleanupExpiredVariables() {
	def.instancesLock.Lock()
	defer def.instancesLock.Unlock()
	for _, instance := range def.instances {
		if instance.IsExpired() {
			instance.free()
		}
	}
}

func (def *definition[T]) getInstances() map[string]VariableInstance {
	def.instancesLock.RLock()
	defer def.instancesLock.RUnlock()
	return maps.Clone(def.instances) // return a shallow clone of the map to avoid concurrency issues
}

func (def *definition[T]) newInstance(freeCb func()) VariableInstance {
	switch def := any(def).(type) {
	case *definition[string]:
		return newScalarInstance[string](def.defaultValue, def.opts.TTL, freeCb)
	case *definition[int]:
		return newScalarInstance[int](def.defaultValue, def.opts.TTL, freeCb)
	case *definition[bool]:
		return newScalarInstance[bool](def.defaultValue, def.opts.TTL, freeCb)
	case *definition[net.IPNet]:
		return newScalarInstance[net.IPNet](def.defaultValue, def.opts.TTL, freeCb)
	case *definition[[]string]:
		return newArrayInstance[string](def.opts.TTL, def.opts.Size, freeCb)
	case *definition[[]int]:
		return newArrayInstance[int](def.opts.TTL, def.opts.Size, freeCb)
	case *definition[[]net.IPNet]:
		return newIPArrayInstance(def.opts.TTL, def.opts.Size, freeCb)
	default:
		panic("unexpected type")
	}
}

// VariableInstance represents an instance of a variable, indenpdently of its type
type VariableInstance interface {
	Set(any) error
	Append(any) error
	GetValue() any
	IsExpired() bool
	free()
}

type instanceCommon struct {
	freeOnce sync.Once
	freeCb   func()
}

// instancesLock must be held when free() is called
func (ic *instanceCommon) free() {
	if ic.freeCb != nil {
		ic.freeOnce.Do(ic.freeCb)
	}
}

type scalarInstanceType interface {
	string | int | bool | net.IPNet
}

type scalarInstance[T scalarInstanceType] struct {
	instanceCommon

	value          T
	ttl            time.Duration
	expirationDate time.Time
}

func newScalarInstance[T scalarInstanceType](defaultValue T, ttl time.Duration, freeCb func()) *scalarInstance[T] {
	newInstance := &scalarInstance[T]{
		value: defaultValue,
		ttl:   ttl,
		instanceCommon: instanceCommon{
			freeCb: freeCb,
		},
	}

	newInstance.touchTTL()

	return newInstance
}

func (i *scalarInstance[T]) touchTTL() {
	if i.ttl > 0 {
		i.expirationDate = time.Now().Add(i.ttl)
	}
}

// Set sets the value of the variable instance
func (i *scalarInstance[T]) Set(value any) error {
	if v, ok := value.(T); ok {
		i.value = v
		i.touchTTL()
		return nil
	}
	var expected T
	return &ErrUnexpectedValueType{Expected: expected, Got: value}
}

// Append appends the given value to the variable instance
func (i *scalarInstance[T]) Append(_ any) error {
	return ErrOperatorNotSupported
}

// GetValue returns the value of the variable instance
func (i *scalarInstance[T]) GetValue() any {
	if i.IsExpired() {
		var value T
		return value
	}
	return i.value
}

// IsExpired returns whether the variable instance is expired
func (i *scalarInstance[T]) IsExpired() bool {
	return i.ttl > 0 && time.Now().After(i.expirationDate)
}

type arrayInstanceType interface {
	string | int
}

type arrayInstance[T arrayInstanceType] struct {
	instanceCommon

	lru *ttlcache.Cache[T, bool]
}

func newArrayInstance[T arrayInstanceType](ttl time.Duration, size int, freeCb func()) *arrayInstance[T] {
	if size <= 0 {
		size = defaultMaxVariables
	}

	newInstance := &arrayInstance[T]{
		lru: ttlcache.New(ttlcache.WithCapacity[T, bool](uint64(size)), ttlcache.WithTTL[T, bool](ttl)),
		instanceCommon: instanceCommon{
			freeCb: freeCb,
		},
	}

	return newInstance
}

// Set sets the value of the variable instance
func (ai *arrayInstance[T]) Set(value any) error {
	return ai.Append(value)
}

// Append appends the given value to the variable instance
func (ai *arrayInstance[T]) Append(value any) error {
	switch value := value.(type) {
	case T:
		ai.lru.Set(value, true, ttlcache.DefaultTTL)
	case []T:
		for _, v := range value {
			ai.lru.Set(v, true, ttlcache.DefaultTTL)
		}
	default:
		var expected []T
		return &ErrUnexpectedValueType{Expected: expected, Got: value}
	}
	return nil
}

// GetValue returns the value of the variable instance
func (ai *arrayInstance[T]) GetValue() any {
	return ai.lru.Keys()
}

// IsExpired returns whether the variable instance is expired
func (ai *arrayInstance[T]) IsExpired() bool {
	ai.lru.DeleteExpired()
	return ai.lru.Len() == 0
}

type ipArrayInstance struct {
	instanceCommon

	lru *ttlcache.Cache[string, bool]
}

func newIPArrayInstance(ttl time.Duration, size int, freeCb func()) *ipArrayInstance {
	if size <= 0 {
		size = defaultMaxVariables
	}

	newInstance := &ipArrayInstance{
		lru: ttlcache.New(ttlcache.WithCapacity[string, bool](uint64(size)), ttlcache.WithTTL[string, bool](ttl)),
		instanceCommon: instanceCommon{
			freeCb: freeCb,
		},
	}

	return newInstance
}

// Set sets the value of the variable instance
func (iai *ipArrayInstance) Set(value any) error {
	return iai.Append(value)
}

// Append appends the given value to the variable instance
func (iai *ipArrayInstance) Append(value any) error {
	switch value := value.(type) {
	case net.IPNet:
		iai.lru.Set(value.String(), true, ttlcache.DefaultTTL)
	case []net.IPNet:
		for _, v := range value {
			iai.lru.Set(v.String(), true, ttlcache.DefaultTTL)
		}
	default:
		var expected []net.IPNet
		return &ErrUnexpectedValueType{Expected: expected, Got: value}
	}
	return nil
}

// GetValue returns the value of the variable instance
func (iai *ipArrayInstance) GetValue() any {
	keys := iai.lru.Keys()
	ips := make([]net.IPNet, 0, len(keys))
	for _, key := range keys {
		_, ipNet, err := net.ParseCIDR(key)
		if err == nil {
			ips = append(ips, *ipNet)
		}
	}
	return ips
}

// IsExpired returns whether the variable instance is expired
func (iai *ipArrayInstance) IsExpired() bool {
	iai.lru.DeleteExpired()
	return iai.lru.Len() == 0
}

// VariableStore represents a collection of variables
type VariableStore struct {
	staticVars  map[VariableName]StaticVariable
	definitions map[VariableName]VariableDefinition
}

// NewVariableStore returns a new VariableStore
func NewVariableStore() *VariableStore {
	return &VariableStore{
		staticVars:  make(map[VariableName]StaticVariable),
		definitions: make(map[VariableName]VariableDefinition),
	}
}

// AddStaticVariable adds a static variable to the store
func (s *VariableStore) AddStaticVariable(varName VariableName, variable StaticVariable) {
	s.staticVars[varName] = variable
}

// AddDefinition adds a variable definition to the store
func (s *VariableStore) AddDefinition(varName VariableName, definition VariableDefinition) {
	s.definitions[varName] = definition
}

// GetEvaluator returns an evaluator based on the given variable name
func (s *VariableStore) GetEvaluator(varName VariableName) (any, error) {
	static, ok := s.staticVars[varName]
	if ok {
		switch static := static.(type) {
		case *staticVariable[string]:
			return &StringEvaluator{
				EvalFnc: func(ctx *Context) string {
					return static.getValueCb(ctx)
				},
			}, nil
		case *staticVariable[int]:
			return &IntEvaluator{
				EvalFnc: func(ctx *Context) int {
					return static.getValueCb(ctx)
				},
			}, nil
		case *staticVariable[bool]:
			return &BoolEvaluator{
				EvalFnc: func(ctx *Context) bool {
					return static.getValueCb(ctx)
				},
			}, nil
		case *staticVariable[net.IPNet]:
			return &CIDREvaluator{
				EvalFnc: func(ctx *Context) net.IPNet {
					return static.getValueCb(ctx)
				},
			}, nil
		case *staticVariable[[]string]:
			return &StringArrayEvaluator{
				EvalFnc: func(ctx *Context) []string {
					return static.getValueCb(ctx)
				},
			}, nil
		case *staticVariable[[]int]:
			return &IntArrayEvaluator{
				EvalFnc: func(ctx *Context) []int {
					return static.getValueCb(ctx)
				},
			}, nil
		case *staticVariable[[]net.IPNet]:
			return &CIDRArrayEvaluator{
				EvalFnc: func(ctx *Context) []net.IPNet {
					return static.getValueCb(ctx)
				},
			}, nil
		default:
			return nil, fmt.Errorf("variable `%s` has unsupported type", varName)
		}
	}

	def, ok := s.definitions[varName]
	if !ok {
		return nil, fmt.Errorf("variable `%s` is not defined", varName)
	}

	switch def := def.(type) {
	case *definition[string]:
		return &StringEvaluator{
			EvalFnc: func(ctx *Context) string {
				instance, exists, _ := def.GetInstance(ctx)
				if !exists || instance.IsExpired() {
					return def.defaultValue
				}
				return instance.GetValue().(string)
			},
		}, nil
	case *definition[int]:
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				instance, exists, _ := def.GetInstance(ctx)
				if !exists || instance.IsExpired() {
					return def.defaultValue
				}
				return instance.GetValue().(int)
			},
		}, nil
	case *definition[bool]:
		return &BoolEvaluator{
			EvalFnc: func(ctx *Context) bool {
				instance, exists, _ := def.GetInstance(ctx)
				if !exists || instance.IsExpired() {
					return def.defaultValue
				}
				return instance.GetValue().(bool)
			},
		}, nil
	case *definition[net.IPNet]:
		return &CIDREvaluator{
			EvalFnc: func(ctx *Context) net.IPNet {
				instance, exists, _ := def.GetInstance(ctx)
				if !exists || instance.IsExpired() {
					return def.defaultValue
				}
				return instance.GetValue().(net.IPNet)
			},
		}, nil
	case *definition[[]string]:
		return &StringArrayEvaluator{
			EvalFnc: func(ctx *Context) []string {
				instance, exists, _ := def.GetInstance(ctx)
				if !exists || instance.IsExpired() {
					return def.defaultValue
				}
				return instance.GetValue().([]string)
			},
		}, nil
	case *definition[[]int]:
		return &IntArrayEvaluator{
			EvalFnc: func(ctx *Context) []int {
				instance, exists, _ := def.GetInstance(ctx)
				if !exists || instance.IsExpired() {
					return def.defaultValue
				}
				return instance.GetValue().([]int)
			},
		}, nil
	case *definition[[]net.IPNet]:
		return &CIDRArrayEvaluator{
			EvalFnc: func(ctx *Context) []net.IPNet {
				instance, exists, _ := def.GetInstance(ctx)
				if !exists || instance.IsExpired() {
					return def.defaultValue
				}
				return instance.GetValue().([]net.IPNet)
			},
		}, nil
	}

	return nil, fmt.Errorf("variable `%s` has unexpected type definition: %T", varName, def.DefaultValue())
}

// GetDefinition returns the definition of a variable based on the given name
func (s *VariableStore) GetDefinition(varName VariableName) (VariableDefinition, bool) {
	def, exists := s.definitions[varName]
	return def, exists
}

// CleanupExpiredVariables deletes expired variable instances from the store
func (s *VariableStore) CleanupExpiredVariables() {
	for _, definition := range s.definitions {
		definition.CleanupExpiredVariables()
	}
}

// IterVariables calls the given callback function on all variables
func (s *VariableStore) IterVariables(cb func(definition VariableDefinition, instances map[string]VariableInstance)) {
	for _, definition := range s.definitions {
		cb(definition, definition.getInstances())
	}
}

// IterVariableDefinitions calls the given callback function on all variable defintions
func (s *VariableStore) IterVariableDefinitions(cb func(definition VariableDefinition)) {
	for _, definition := range s.definitions {
		cb(definition)
	}
}

// Gauge tracks the amount of a metric
type Gauge interface {
	// Inc increments the Gauge value.
	Inc(tagsValue ...string)
	// Dec decrements the Gauge value.
	Dec(tagsValue ...string)
}

// Telemetry tracks the values of evaluation metrics
type Telemetry struct {
	TotalVariables Gauge
}

func getValueType[T VariableType](value T) string {
	switch any(value).(type) {
	case string:
		return "string"
	case int:
		return "int"
	case bool:
		return "bool"
	case net.IPNet:
		return "ip"
	case []string:
		return "strings"
	case []int:
		return "integers"
	case []net.IPNet:
		return "ips"
	default:
		panic("unsupported type")
	}
}

// VariableName represents the name of a variable with its scope prefix included
type VariableName string

// GetVariableName returns a VariableName based on the given scope and variable names
func GetVariableName(scope, name string) VariableName {
	if scope == "" {
		return VariableName(name)
	}
	return VariableName(scope + "." + name)
}

// ErrUnexpectedValueType represents an invalid variable type assignment
type ErrUnexpectedValueType struct {
	Expected any
	Got      any
}

// Error returns the error message of the error
func (e *ErrUnexpectedValueType) Error() string {
	return fmt.Sprintf("unexpected value type: expected %T, got %T", e.Expected, e.Got)
}

// ErrUnsupportedScope represents an unsupported scope error
type ErrUnsupportedScope struct {
	VarName string
	Scope   string
}

// Error returns the error message of the error
func (e *ErrUnsupportedScope) Error() string {
	return fmt.Sprintf("variable `%s` has unsupported scope: `%s`", e.VarName, e.Scope)
}

// ErrOperatorNotSupported represents an invalid variable assignment
var ErrOperatorNotSupported = errors.New("operation not supported")

// ErrScopeFailure wraps an error coming from a variable scoper
type ErrScopeFailure struct {
	VarName    string
	ScoperType InternalScoperType
	ScoperErr  error
}

// Error returns the error message of the error
func (e *ErrScopeFailure) Error() string {
	return fmt.Sprintf("failed to get scope `%s` of variable `%s`: %s", e.ScoperType.String(), e.VarName, e.ScoperErr)
}

// Unwrap unwraps the error
func (e *ErrScopeFailure) Unwrap() error {
	return e.ScoperErr
}
