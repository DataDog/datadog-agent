// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"fmt"
	"iter"
	"maps"
	"net"
	"regexp"
	"sync"
	"time"

	ttlcache "github.com/jellydator/ttlcache/v3"
)

var variableRegex = regexp.MustCompile(`\${[^}]*}`)

const defaultMaxVariables = 100

// StaticVariableType lists the types that a static variable can take
type StaticVariableType interface {
	int | string | bool
}

type staticVariable[T StaticVariableType] struct {
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
func NewStaticVariable[T StaticVariableType](getValue func(ctx *Context) T) StaticVariable {
	return &staticVariable[T]{
		getValueCb: getValue,
	}
}

// VariableType lists the types that a SECL variable can take
type VariableType interface {
	string | int | bool | net.IPNet |
		[]string | []int | []net.IPNet
}

// Definition represents the definition of a SECL variable
type Definition interface {
	AddNewInstance(ctx *Context) (Instance, bool, error)
	GetInstance(ctx *Context) (Instance, error)
	GetInstancesCount() int
	GetDefaultValue() any
	IsPrivate() bool
	GetName(withScopePrefix bool) string
	CleanupExpiredVariables()
	getInstances() map[string]Instance
	GetScoper() *VariableScoper
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
	instances     map[string]Instance
}

// NewVariableDefinition returns a new definition of a SECL variable
func NewVariableDefinition[T VariableType](name string, scoper *VariableScoper, defaultValue T, opts VariableOpts) (Definition, error) {
	return &definition[T]{
		name:         name,
		defaultValue: defaultValue,
		valueType:    getValueType[T](defaultValue),
		scoper:       scoper,
		opts:         &opts,
		instances:    make(map[string]Instance),
	}, nil
}

func (def *definition[T]) getInstances() map[string]Instance {
	def.instancesLock.RLock()
	defer def.instancesLock.RUnlock()
	return maps.Clone(def.instances) // return a shallow clone of the map to avoid concurrency issues
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

// GetInstance returns a variable instance that corresponds to the given Context or nil if no instance was found
func (def *definition[T]) GetInstance(ctx *Context) (Instance, error) {
	var instance Instance

	scope, err := def.scoper.GetScope(ctx)
	if err != nil {
		return nil, &ErrScopeFailure{VarName: def.name, ScoperType: def.scoper.scoperType, ScoperErr: err}
	}

	def.instancesLock.RLock()

	key, ok := scope.Key()
	if ok {
		instance = def.instances[key]
	}

	if (!ok || instance == nil) && def.opts.Inherited {
		scope, ok = scope.ParentScope()
		for ok && instance == nil {
			key, ok = scope.Key()
			if ok {
				instance = def.instances[key]
			}
			scope, ok = scope.ParentScope()
		}
	}

	def.instancesLock.RUnlock()

	if instance != nil && instance.IsExpired() {
		def.instancesLock.Lock()
		instance.free()
		def.instancesLock.Unlock()
		instance = nil
	}

	// instance can be nil here if no instance exists
	return instance, nil
}

func (def *definition[T]) newInstance(freeCb func()) Instance {
	switch def := any(def).(type) {
	case *definition[string]:
		return newSingleValueVariableInstance[string](def.defaultValue, def.opts.TTL, freeCb)
	case *definition[int]:
		return newSingleValueVariableInstance[int](def.defaultValue, def.opts.TTL, freeCb)
	case *definition[bool]:
		return newSingleValueVariableInstance[bool](def.defaultValue, def.opts.TTL, freeCb)
	case *definition[net.IPNet]:
		return newSingleValueVariableInstance[net.IPNet](def.defaultValue, def.opts.TTL, freeCb)
	case *definition[[]string]:
		return newArrayValueVariable[string](def.opts.TTL, def.opts.Size, freeCb)
	case *definition[[]int]:
		return newArrayValueVariable[int](def.opts.TTL, def.opts.Size, freeCb)
	case *definition[[]net.IPNet]:
		return newIPArrayVariable(def.opts.TTL, def.opts.Size, freeCb)
	default:
		panic("unexpected type")
	}
}

// AddNewInstance instanciates and adds a new variable instance for the given Context
func (def *definition[T]) AddNewInstance(ctx *Context) (Instance, bool, error) {
	scope, err := def.scoper.GetScope(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get scope `%s` of variable `%s`: %w", def.scoper.Type(), def.name, err)
	}

	var newInstance Instance
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

	return newInstance, ok, nil
}

// GetDefaultValue returns the default value of the definition
func (def *definition[T]) GetDefaultValue() any {
	return def.defaultValue
}

// IsPrivate returns whether the variable is definied as private
func (def *definition[T]) IsPrivate() bool {
	return def.opts.Private
}

func (def *definition[T]) GetScoper() *VariableScoper {
	return def.scoper
}

// GetName returns the name if the variable
func (def *definition[T]) GetName(withScopePrefix bool) string {
	if !withScopePrefix {
		return def.name
	}
	if def.scoper.Type() == GlobalScoperType {
		return def.name
	}
	return def.scoper.Type().VariablePrefix() + "." + def.name
}

// Instance represents an instance of a variable, indenpdently of its type
type Instance interface {
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

type singleValueVariableType interface {
	string | int | bool | net.IPNet
}

type instance[T singleValueVariableType] struct {
	instanceCommon

	value          T
	ttl            time.Duration
	expirationDate time.Time
}

func newSingleValueVariableInstance[T singleValueVariableType](defaultValue T, ttl time.Duration, freeCb func()) *instance[T] {
	newInstance := &instance[T]{
		value: defaultValue,
		ttl:   ttl,
	}

	newInstance.touchTTL()

	newInstance.freeCb = freeCb
	return newInstance
}

func (i *instance[T]) touchTTL() {
	if i.ttl > 0 {
		i.expirationDate = time.Now().Add(i.ttl)
	}
}

// Set sets the value of the variable instance
func (i *instance[T]) Set(value any) error {
	if v, ok := value.(T); ok {
		i.value = v
		i.touchTTL()
		return nil
	}
	var expected T
	return &ErrUnexpectedValueType{Expected: expected, Got: value}
}

// Append appends the given value to the variable instance
func (i *instance[T]) Append(_ any) error {
	return ErrOperatorNotSupported
}

// GetValue returns the value of the variable instance
func (i *instance[T]) GetValue() any {
	if i.IsExpired() {
		var value T
		return value
	}
	return i.value
}

// IsExpired returns whether the variable instance is expired
func (i *instance[T]) IsExpired() bool {
	return i.ttl > 0 && time.Now().After(i.expirationDate)
}

type ipArrayVariable struct {
	instanceCommon

	lru *ttlcache.Cache[string, bool]
}

func newIPArrayVariable(ttl time.Duration, size int, freeCb func()) *ipArrayVariable {
	if size <= 0 {
		size = defaultMaxVariables
	}

	newInstance := &ipArrayVariable{
		lru: ttlcache.New(ttlcache.WithCapacity[string, bool](uint64(size)), ttlcache.WithTTL[string, bool](ttl)),
	}

	newInstance.freeCb = freeCb

	return newInstance
}

// Set sets the value of the variable instance
func (iav *ipArrayVariable) Set(value any) error {
	return iav.Append(value)
}

// Append appends the given value to the variable instance
func (iav *ipArrayVariable) Append(value any) error {
	switch value := value.(type) {
	case net.IPNet:
		iav.lru.Set(value.String(), true, ttlcache.DefaultTTL)
	case []net.IPNet:
		for _, v := range value {
			iav.lru.Set(v.String(), true, ttlcache.DefaultTTL)
		}
	default:
		var expected []net.IPNet
		return &ErrUnexpectedValueType{Expected: expected, Got: value}
	}
	return nil
}

// GetValue returns the value of the variable instance
func (iav *ipArrayVariable) GetValue() any {
	keys := iav.lru.Keys()
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
func (iav *ipArrayVariable) IsExpired() bool {
	iav.lru.DeleteExpired()
	return iav.lru.Len() == 0
}

type arrayValueVariableType interface {
	string | int
}

type arrayValueVariable[T arrayValueVariableType] struct {
	instanceCommon

	lru *ttlcache.Cache[T, bool]
}

func newArrayValueVariable[T arrayValueVariableType](ttl time.Duration, size int, freeCb func()) *arrayValueVariable[T] {
	if size <= 0 {
		size = defaultMaxVariables
	}

	newInstance := &arrayValueVariable[T]{
		lru: ttlcache.New(ttlcache.WithCapacity[T, bool](uint64(size)), ttlcache.WithTTL[T, bool](ttl)),
	}

	newInstance.freeCb = freeCb

	return newInstance
}

// Set sets the value of the variable instance
func (avv *arrayValueVariable[T]) Set(value any) error {
	return avv.Append(value)
}

// Append appends the given value to the variable instance
func (avv *arrayValueVariable[T]) Append(value any) error {
	switch value := value.(type) {
	case T:
		avv.lru.Set(value, true, ttlcache.DefaultTTL)
	case []T:
		for _, v := range value {
			avv.lru.Set(v, true, ttlcache.DefaultTTL)
		}
	default:
		var expected []T
		return &ErrUnexpectedValueType{Expected: expected, Got: value}
	}
	return nil
}

// GetValue returns the value of the variable instance
func (avv *arrayValueVariable[T]) GetValue() any {
	return avv.lru.Keys()
}

// IsExpired returns whether the variable instance is expired
func (avv *arrayValueVariable[T]) IsExpired() bool {
	avv.lru.DeleteExpired()
	return avv.lru.Len() == 0
}

// Store represents a collection of variables
type Store struct {
	staticVars  map[VariableName]StaticVariable
	definitions map[VariableName]Definition
}

// NewStore returns a new Store
func NewStore() *Store {
	return &Store{
		staticVars:  make(map[VariableName]StaticVariable),
		definitions: make(map[VariableName]Definition),
	}
}

// AddStaticVariable adds a static variable to the store
func (s *Store) AddStaticVariable(varName VariableName, variable StaticVariable) {
	s.staticVars[varName] = variable
}

// GetDefinition returns the definition of a variable based on the given name
func (s *Store) GetDefinition(varName VariableName) (Definition, bool) {
	def, exists := s.definitions[varName]
	return def, exists
}

// AddDefinition adds a variable definition to the store
func (s *Store) AddDefinition(varName VariableName, definition Definition) {
	s.definitions[varName] = definition
}

// GetEvaluator returns an evaluator based on the given variable name
func (s *Store) GetEvaluator(varName VariableName) (any, error) {
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
				instance, _ := def.GetInstance(ctx) // TODO(yoanngh): how to deal with scope errors here?
				if instance == nil {                // the variable has no instance, thus use the default value
					return def.defaultValue
				}
				return instance.GetValue().(string)
			},
		}, nil
	case *definition[int]:
		return &IntEvaluator{
			EvalFnc: func(ctx *Context) int {
				instance, _ := def.GetInstance(ctx) // TODO(yoanngh): how to deal with scope errors here?
				if instance == nil {                // the variable has no instance, thus use the default value
					return def.defaultValue
				}
				return instance.GetValue().(int)
			},
		}, nil
	case *definition[bool]:
		return &BoolEvaluator{
			EvalFnc: func(ctx *Context) bool {
				instance, _ := def.GetInstance(ctx) // TODO(yoanngh): how to deal with scope errors here?
				if instance == nil {                // the variable has no instance, thus use the default value
					return def.defaultValue
				}
				return instance.GetValue().(bool)
			},
		}, nil
	case *definition[net.IPNet]:
		return &CIDREvaluator{
			EvalFnc: func(ctx *Context) net.IPNet {
				instance, _ := def.GetInstance(ctx) // TODO(yoanngh): how to deal with scope errors here?
				if instance == nil {                // the variable has no instance, thus use the default value
					return def.defaultValue
				}
				return instance.GetValue().(net.IPNet)
			},
		}, nil
	case *definition[[]string]:
		return &StringArrayEvaluator{
			EvalFnc: func(ctx *Context) []string {
				instance, _ := def.GetInstance(ctx) // TODO(yoanngh): how to deal with scope errors here?
				if instance == nil {                // the variable has no instance, thus use the default value
					return def.defaultValue
				}
				return instance.GetValue().([]string)
			},
		}, nil
	case *definition[[]int]:
		return &IntArrayEvaluator{
			EvalFnc: func(ctx *Context) []int {
				instance, _ := def.GetInstance(ctx) // TODO(yoanngh): how to deal with scope errors here?
				if instance == nil {                // the variable has no instance, thus use the default value
					return def.defaultValue
				}
				return instance.GetValue().([]int)
			},
		}, nil
	case *definition[[]net.IPNet]:
		return &CIDRArrayEvaluator{
			EvalFnc: func(ctx *Context) []net.IPNet {
				instance, _ := def.GetInstance(ctx) // TODO(yoanngh): how to deal with scope errors here?
				if instance == nil {                // the variable has no instance, thus use the default value
					return def.defaultValue
				}
				return instance.GetValue().([]net.IPNet)
			},
		}, nil
	}

	return nil, fmt.Errorf("variable `%s` has unexpected type definition: %T", varName, def.GetDefaultValue())
}

// CleanupExpiredVariables deletes expired variable instances from the store
func (s *Store) CleanupExpiredVariables() {
	for _, definition := range s.definitions {
		definition.CleanupExpiredVariables()
	}
}

// GetOpts are the options to retrieve variable definitions
type GetOpts struct {
	ScoperType InternalScoperType
}

// GetDefinitions returns an iterator over variable definitions that match the given options
func (s *Store) GetDefinitions(opts *GetOpts) iter.Seq[Definition] {
	return func(yield func(Definition) bool) {
		for _, definition := range s.definitions {
			if opts.ScoperType != UndefinedScoperType && definition.GetScoper().Type() != opts.ScoperType {
				continue
			}
			if !yield(definition) {
				return
			}
		}
	}
}

// IterateVariables calls the given callback function on all variables
func (s *Store) IterateVariables(cb func(definition Definition, instances map[string]Instance)) {
	for _, definition := range s.definitions {
		cb(definition, definition.getInstances())
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
