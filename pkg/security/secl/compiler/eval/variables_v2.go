// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"fmt"
	"iter"
	"net"
	"regexp"
	"sync"
	"time"

	ttlcache "github.com/jellydator/ttlcache/v3"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

var variableRegex = regexp.MustCompile(`\${[^}]*}`)

const defaultMaxVariables = 100

// StaticVariables

type StaticVariableType interface {
	int | string | bool
}

type staticVariable[T StaticVariableType] struct {
	getValueCb func(ctx *Context) T
}

func (s *staticVariable[T]) GetValue(ctx *Context) any {
	return s.getValueCb(ctx)
}

type StaticVariable interface {
	GetValue(ctx *Context) any
}

func NewStaticVariable[T StaticVariableType](getValue func(ctx *Context) T) StaticVariable {
	return &staticVariable[T]{
		getValueCb: getValue,
	}
}

// VariableDefinition

type VariableType interface {
	string | int | bool | net.IPNet |
		[]string | []int | []net.IPNet
}

type Definition interface {
	GetInstances() map[string]Instance
	AddNewInstance(ctx *Context) (Instance, bool, error)
	GetInstance(ctx *Context) (Instance, error)
	// NewInstance() Instance
	// AddInstance(*Context, Instance) (bool, error)
	GetDefaultValue() any
	IsPrivate() bool
	GetScoper() *VariableScoper
	GetName(withScopePrefix bool) string
}

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

	instances map[string]Instance
}

func NewVariableDefinition[T VariableType](name string, scoper *VariableScoper, defaultValue T, opts VariableOpts) (Definition, error) {
	// scoper, ok := s.scopers[scope]
	// if !ok {
	// 	return nil, &ErrUnsupportedScope{VarName: name, Scope: scope}
	// }

	return &definition[T]{
		name:         name,
		defaultValue: defaultValue,
		valueType:    getValueType[T](defaultValue),
		scoper:       scoper,
		opts:         &opts,
		instances:    make(map[string]Instance),
	}, nil
}

func (def *definition[T]) GetInstances() map[string]Instance {
	return def.instances
}

func (def *definition[T]) GetInstance(ctx *Context) (Instance, error) {
	var instance Instance

	scope, err := def.scoper.GetScope(ctx)
	if err != nil {
		return nil, &ErrScopeFailure{VarName: def.name, ScoperType: def.scoper.scoperType, ScoperErr: err}
	}

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

	if instance != nil && instance.IsExpired() {
		instance.free()
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
		return newArrayValueVariable[string](def.defaultValue, def.opts.TTL, def.opts.Size, freeCb)
	case *definition[[]int]:
		return newArrayValueVariable[int](def.defaultValue, def.opts.TTL, def.opts.Size, freeCb)
	case *definition[[]net.IPNet]:
		return newIpArrayVariable(def.defaultValue, def.opts.TTL, def.opts.Size, freeCb)
	default:
		panic("unexpected type")
	}
}

// func (def *definition[T]) newInstance() Instance {
// 	// TODO(yoanngh): switch case on def.(type) (or check def.opts.Append?): create instance with type specific for array values
// 	instance := &instance[T]{
// 		value: def.defaultValue,
// 	}

// 	if def.opts.TTL > 0 {
// 		instance.ttl = def.opts.TTL
// 		instance.expirationDate = time.Now().Add(def.opts.TTL)
// 	}

// 	return instance
// }

// func (def *definition[T]) AddInstance(ctx *Context, instance Instance) (bool, error) {
// 	var ok bool
// 	key := InstanceKey{Name: def.name}
// 	scope, err := def.scoper.GetScope(ctx)
// 	if err != nil {
// 		return false, fmt.Errorf("failed to get scope `%s` of variable `%s`: %w", def.scoper.Name(), def.name, err)
// 	}

// 	key.ScopeKey, ok = scope.Key()
// 	if ok {
// 		varType, err := GetValueType(def.defaultValue)
// 		if err != nil {
// 			return false, err
// 		}
// 		def.instances[key] = instance
// 		if def.opts.Telemetry != nil {
// 			def.opts.Telemetry.TotalVariables.Inc(varType, def.scoper.Name())
// 		}
// 		if releaseable, ok := scope.(ReleasableVariableScope); ok {
// 			releaseable.AppendReleaseCallback(func() {
// 				if def.opts.Telemetry != nil {
// 					def.opts.Telemetry.TotalVariables.Dec(varType, def.scoper.Name())
// 				}
// 				delete(def.instances, key)
// 			})
// 		}
// 	}

// 	// TODO(yoanngh): check if we need/want to attach the variable to a parent scope here

// 	return ok, nil
// }

// func (def *definition[T]) NewInstance() Instance {
// 	return def.newInstance()
// }

func (def *definition[T]) AddNewInstance(ctx *Context) (Instance, bool, error) {
	scope, err := def.scoper.GetScope(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get scope `%s` of variable `%s`: %w", def.scoper.Type(), def.name, err)
	}

	// var newInstance *instance[T]
	var newInstance Instance

	key, ok := scope.Key()
	if ok {
		newInstance = def.newInstance(func() {
			if def.opts.Telemetry != nil {
				def.opts.Telemetry.TotalVariables.Dec(def.valueType, def.scoper.Type().String())
			}
			delete(def.instances, key)
		})

		if def.opts.Telemetry != nil {
			def.opts.Telemetry.TotalVariables.Inc(def.valueType, def.scoper.Type().String())
		}

		// newInstance = &instance[T]{
		// 	value: def.defaultValue,
		// }

		// if def.opts.TTL > 0 {
		// 	newInstance.ttl = def.opts.TTL
		// 	newInstance.expirationDate = time.Now().Add(def.opts.TTL)
		// }

		// if def.opts.Telemetry != nil {
		// 	def.opts.Telemetry.TotalVariables.Inc(def.valueType, def.scoper.Type().String())
		// }

		// newInstance.freeCb = func() {
		// 	if def.opts.Telemetry != nil {
		// 		def.opts.Telemetry.TotalVariables.Dec(def.valueType, def.scoper.Type().String())
		// 	}
		// 	delete(def.instances, key)
		// }

		if releaseable, ok := scope.(ReleasableVariableScope); ok {
			releaseable.AppendReleaseCallback(newInstance.free)
		}

		def.instances[key] = newInstance
	}

	// TODO(yoanngh): check if we need/want to attach the variable to a parent scope here

	return newInstance, ok, nil
}

func (def *definition[T]) GetDefaultValue() any {
	return def.defaultValue
}

func (def *definition[T]) IsPrivate() bool {
	return def.opts.Private
}

func (def *definition[T]) GetScoper() *VariableScoper {
	return def.scoper
}

func (def *definition[T]) GetName(withScopePrefix bool) string {
	if !withScopePrefix {
		return def.name
	}
	if def.scoper.Type() == GlobalScoperType {
		return def.name
	}
	return def.scoper.Type().VariablePrefix() + "." + def.name
}

// VariableInstance

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

func (ic *instanceCommon) free() {
	if ic.freeCb != nil {
		ic.freeOnce.Do(ic.freeCb)
	}
}

// singleValueVariableType

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

func (i *instance[T]) Set(value any) error {
	if v, ok := value.(T); ok {
		i.value = v
		i.touchTTL()
		return nil
	}
	var expected T
	return &ErrUnexpectedValueType{Expected: expected, Got: value}
}

func (i *instance[T]) Append(value any) error {
	return ErrOperatorNotSupported
}

func (i *instance[T]) GetValue() any {
	if i.IsExpired() {
		var value T
		return value
	}
	return i.value
}

func (i *instance[T]) IsExpired() bool {
	return i.ttl > 0 && time.Now().After(i.expirationDate)
}

// arrayValueVariableType

// type LRU[T arrayValueVariableType, K comparable, V any] interface {
// 	Set(key T, value V, ttl time.Duration) *ttlcache.Item[K, V]
// 	Keys() []T
// 	DeleteExpired()
// 	Len() int
// }

// // TODO(yoanngh): use this struct to handle net.IPNet array-backed variable instances
// type LRUWithCast[T arrayValueVariableType, K comparable, V any] struct {
// 	TtoK func(T) K
// 	KtoT func(K) T

// 	lru *ttlcache.Cache[K, V]
// }

// func (lwc *LRUWithCast[T, K, V]) Set(key T, value V, ttl time.Duration) *ttlcache.Item[K, V] {
// 	return lwc.lru.Set(lwc.TtoK(key), value, ttl)
// }

// func (lwc *LRUWithCast[T, K, V]) Keys() []T {
// 	keys := lwc.lru.Keys()
// 	keysT := make([]T, 0, len(keys))
// 	for _, key := range keys {
// 		keysT = append(keysT, lwc.KtoT(key))
// 	}
// 	return keysT
// }

type ipArrayVariable struct {
	instanceCommon

	lru *ttlcache.Cache[string, bool]
}

func newIpArrayVariable(defaultValue []net.IPNet, ttl time.Duration, size int, freeCb func()) *ipArrayVariable {
	if size <= 0 {
		size = defaultMaxVariables
	}

	newInstance := &ipArrayVariable{
		lru: ttlcache.New(ttlcache.WithCapacity[string, bool](uint64(size)), ttlcache.WithTTL[string, bool](ttl)),
	}

	// newInstance.Set(defaultValue)

	newInstance.freeCb = freeCb

	return newInstance
}

func (iav *ipArrayVariable) Set(value any) error {
	return iav.Append(value)
}

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

func newArrayValueVariable[T arrayValueVariableType](defaultValue []T, ttl time.Duration, size int, freeCb func()) *arrayValueVariable[T] {
	if size <= 0 {
		size = defaultMaxVariables
	}

	newInstance := &arrayValueVariable[T]{
		lru: ttlcache.New(ttlcache.WithCapacity[T, bool](uint64(size)), ttlcache.WithTTL[T, bool](ttl)),
	}

	// newInstance.Set(defaultValue)

	newInstance.freeCb = freeCb

	return newInstance
}

func (avv *arrayValueVariable[T]) Set(value any) error {
	return avv.Append(value)
}

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

func (avv *arrayValueVariable[T]) GetValue() any {
	return avv.lru.Keys()
}

func (avv *arrayValueVariable[T]) IsExpired() bool {
	avv.lru.DeleteExpired()
	return avv.lru.Len() == 0
}

// VariableStore

type Store struct {
	staticVars  map[VariableName]StaticVariable
	definitions map[VariableName]Definition
	// scopers     map[string]VariableScoper
}

func NewStore() *Store {
	return &Store{
		staticVars:  make(map[VariableName]StaticVariable),
		definitions: make(map[VariableName]Definition),
		// scopers:     make(map[string]VariableScoper),
	}
}

// func (s *Store) SetScopers(scopers map[string]VariableScoper) {
// 	s.scopers = scopers
// }

func (s *Store) AddStaticVariable(varName VariableName, variable StaticVariable) {
	s.staticVars[varName] = variable
}

func (s *Store) GetDefinition(varName VariableName) (Definition, bool) {
	def, exists := s.definitions[varName]
	return def, exists
}

func (s *Store) AddDefinition(varName VariableName, definition Definition) {
	s.definitions[varName] = definition
}

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

func (s *Store) CleanupExpiredVariables() {
	for _, definition := range s.definitions {
		for _, instance := range definition.GetInstances() {
			if instance.IsExpired() {
				instance.free()
			}
		}
	}
}

type GetOpts struct {
	ScoperType InternalScoperType
}

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

func (s *Store) IterateVariables(cb func(definition Definition, instances map[string]Instance)) {
	for _, definition := range s.definitions {
		cb(definition, definition.GetInstances())
	}
}

func (s *Store) GetSECLVariables(globalScopeKey string) map[string]*api.SECLVariableState {
	seclVariableStates := make(map[string]*api.SECLVariableState)

	for _, definition := range s.definitions {
		name := definition.GetName(true)
		switch definition.GetScoper().Type() {
		case GlobalScoperType:
			instances := definition.GetInstances()
			globalInstance, ok := instances[globalScopeKey]
			if ok && !globalInstance.IsExpired() { // skip variables that expired but are yet to be cleaned up
				seclVariableStates[name] = &api.SECLVariableState{
					Name:  name,
					Value: fmt.Sprintf("%+v", globalInstance.GetValue()),
				}
			}
		case ProcessScoperType, CGroupScoperType, ContainerScoperType:
			for scopeKey, instance := range definition.GetInstances() {
				if instance.IsExpired() { // skip variables that expired but are yet to be cleaned up
					continue
				}
				scopedName := fmt.Sprintf("%s.%s", name, scopeKey)
				seclVariableStates[scopedName] = &api.SECLVariableState{
					Name:  scopedName,
					Value: fmt.Sprintf("%+v", instance.GetValue()),
				}
			}
		}
	}

	return seclVariableStates
}

// func (s *Store) GetInstances(opts *GetOpts) iter.Seq2[Definition, Instance] {
// 	return func(yield func(Definition, Instance) bool) {
// 		for _, definition := range s.definitions {
// 			if opts.Scope != "" && definition.GetScope() != opts.Scope {
// 				continue
// 			}
// 			for _, instance := range definition.GetInstances() {
// 				if !yield(definition, instance) {
// 					return
// 				}
// 			}
// 		}
// 	}
// }

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

type VariableName string

func GetVariableName(scope, name string) VariableName {
	if scope == "" {
		return VariableName(name)
	}
	return VariableName(scope + "." + name)
}
