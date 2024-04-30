// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"errors"
	"fmt"
	"reflect"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"go.uber.org/fx"
)

var (
	compOutType = reflect.TypeOf((*compdef.Out)(nil)).Elem()
	fxInType    = reflect.TypeOf(fx.In{})
	fxOutType   = reflect.TypeOf(fx.Out{})
)

// ProvideComponentConstructor takes as input a Component constructor function
// that uses plain (non-fx aware) structs as its argument and return value, and
// returns an fx.Provide'd Option that will properly include that Component
// into the fx constructor graph.
//
// For example, given:
//
//	type Provides struct {
//	    My MyComponent
//	}
//	type Requires struct {
//	    Dep MyDependency
//	}
//	func NewComponent(reqs Requires) Provides { ... }
//
// then:
//
//	ProvideComponentConstructor(NewComponent)
//
// will create these anonymous types:
//
//	type FxAwareProvides struct {
//	    fx.Out
//	    My MyComponent
//	}
//	type FxAwareRequires struct {
//	    fx.In
//	    Dep MyDependency
//	}
//
// and then Provide those types into fx's dependency graph
func ProvideComponentConstructor(compCtorFunc interface{}) fx.Option {
	// type-check the input argument to the constructor
	ctorFuncType := reflect.TypeOf(compCtorFunc)
	if ctorFuncType.Kind() != reflect.Func || ctorFuncType.NumIn() > 1 || ctorFuncType.NumOut() == 0 || ctorFuncType.NumOut() > 2 {
		return fx.Error(errors.New("argument must be a function with 0 or 1 arguments, and 1 or 2 return values"))
	}
	if ctorFuncType.NumIn() > 0 && ctorFuncType.In(0).Kind() != reflect.Struct {
		return fx.Error(errors.New(`constructor must either take 0 arguments, or 1 "requires" struct`))
	}
	hasZeroArg := ctorFuncType.NumIn() == 0

	inFxType, outFxType, hasErrRet, err := constructFxInAndOut(ctorFuncType)
	if err != nil {
		return fx.Error(err)
	}

	// build reflect.Type of the constructor function that will be provided to `fx.Provide`
	funcFxType := reflect.FuncOf([]reflect.Type{inFxType}, []reflect.Type{outFxType}, false)
	if hasErrRet {
		funcFxType = reflect.FuncOf([]reflect.Type{inFxType}, []reflect.Type{outFxType, errorInterface}, false)
	}

	// wrapper that receives fx-aware requirements, converts them into regular requirements, and calls the
	// constructor function value that will inform fx what the Components are
	fxAwareProviderFunc := reflect.MakeFunc(funcFxType, func(args []reflect.Value) []reflect.Value {
		// invoke the regular constructor with the correct arguments
		var ctorArgs []reflect.Value
		if !hasZeroArg {
			ctorArgs = makeConstructorArgs(args[0])
		}
		plainOuts := reflect.ValueOf(compCtorFunc).Call(ctorArgs)
		// create return value, an fx-ware provides struct and an optional error
		res := []reflect.Value{makeFxAwareProvides(plainOuts[0], outFxType)}
		if hasErrRet {
			res = append(res, plainOuts[1])
		}
		return res
	})

	return fx.Provide(fxAwareProviderFunc.Interface())
}

// get the element at the index if the index is within the limit
func getWithinLimit[T any](index int, get func(int) T, limit func() int) T {
	if index < limit() {
		return get(index)
	}
	var zero T
	return zero
}

// create a struct that represents the (possibly nil) input type
func asStruct(typ reflect.Type) (reflect.Type, error) {
	if typ == nil {
		return reflect.StructOf([]reflect.StructField{}), nil
	}
	if typ.Kind() == reflect.Interface {
		return reflect.StructOf([]reflect.StructField{{Name: typ.Name(), Type: typ}}), nil
	}
	if typ.Kind() == reflect.Struct {
		return typ, nil
	}
	return nil, fmt.Errorf("unexpected argument: %T, must be struct or interface", typ)
}

// create a struct field for embedding the type as an anonymous field
func toEmbedField(typ reflect.Type) reflect.StructField {
	return reflect.StructField{Type: typ, Name: typ.Name(), Anonymous: true}
}

// return true if the type is an error, or false if it is nil, return an error otherwise
func ensureErrorOrNil(typ reflect.Type) (bool, error) {
	if typ == nil {
		return false, nil
	}
	if typ == reflect.TypeOf((*error)(nil)).Elem() {
		return true, nil
	}
	return false, fmt.Errorf("second return value must be error, got %v", typ)
}

// return true if the struct type has an embed field of the given type
func hasEmbedField(typ, embed reflect.Type) bool {
	if typ.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type == embed {
			return true
		}
	}
	return false
}

// construct fx-aware types for the input and output of the given constructor function
func constructFxInAndOut(ctorFuncType reflect.Type) (reflect.Type, reflect.Type, bool, error) {
	ctorInType, err1 := asStruct(getWithinLimit(0, ctorFuncType.In, ctorFuncType.NumIn))
	ctorOutType, err2 := asStruct(ctorFuncType.Out(0))
	hasErrRet, err3 := ensureErrorOrNil(getWithinLimit(1, ctorFuncType.Out, ctorFuncType.NumOut))
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, nil, false, err
	}

	// create types that have fx-aware embed-fields
	// these are used to construct a function that can build the fx graph
	inFxType, err := constructFxInType(ctorInType)
	if err != nil {
		return nil, nil, hasErrRet, err
	}
	outFxType, err := constructFxOutType(ctorOutType)
	return inFxType, outFxType, hasErrRet, err
}

func constructFxInType(plainType reflect.Type) (reflect.Type, error) {
	return constructFxAwareStruct(plainType, false)
}

func constructFxOutType(plainType reflect.Type) (reflect.Type, error) {
	return constructFxAwareStruct(plainType, true)
}

// construct a new fx-aware struct type that matches the plainType, but has fx.In / fx.Out embedded
func constructFxAwareStruct(plainType reflect.Type, isOut bool) (reflect.Type, error) {
	var oldEmbed, newEmbed reflect.Type
	if isOut {
		oldEmbed = compOutType
		newEmbed = fxOutType
	} else {
		newEmbed = fxInType
	}
	if plainType == nil {
		return reflect.StructOf([]reflect.StructField{toEmbedField(newEmbed)}), nil
	}
	if plainType.Kind() == reflect.Interface {
		field := reflect.StructField{Name: plainType.Name(), Type: plainType}
		return reflect.StructOf([]reflect.StructField{toEmbedField(newEmbed), field}), nil
	}
	if plainType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("bad type: %T", plainType)
	}
	return replaceStructEmbeds(plainType, oldEmbed, newEmbed, true), nil
}

// replaceStructEmbeds copies a struct type to a newly created struct type, removing
// the oldEmbed fields and prepending the newEmbed field, if given. This is done
// recursively for fields that themselves contain an embedding type
func replaceStructEmbeds(typ, oldEmbed, newEmbed reflect.Type, assumeEmbed bool) reflect.Type {
	hasEmbed := assumeEmbed || hasEmbedField(typ, oldEmbed)
	if !hasEmbed {
		return typ
	}

	newFields := make([]reflect.StructField, 0, typ.NumField())
	for n := 0; n < typ.NumField(); n++ {
		field := typ.Field(n)
		if field.Type == oldEmbed {
			continue
		}
		if field.Type.Kind() == reflect.Struct && oldEmbed != nil && newEmbed != nil && hasEmbed {
			field = reflect.StructField{Name: field.Name, Type: replaceStructEmbeds(field.Type, oldEmbed, newEmbed, false)}
		}
		newFields = append(newFields, reflect.StructField{Name: field.Name, Type: field.Type})
	}

	if hasEmbed && newEmbed != nil {
		newFields = append([]reflect.StructField{toEmbedField(newEmbed)}, newFields...)
	}
	return reflect.StructOf(newFields)
}

// create arguments that are ready to be passed to the plain constructor by
// removing fx specific fields from the fx-aware requires struct
func makeConstructorArgs(fxAwareReqs reflect.Value) []reflect.Value {
	if fxAwareReqs.Kind() != reflect.Struct {
		panic("pre-condition failure: must be called with Struct")
	}
	plainType := replaceStructEmbeds(fxAwareReqs.Type(), fxInType, nil, false)
	return []reflect.Value{coerceStructTo(fxAwareReqs, plainType, fxOutType, nil)}
}

// change the return value from the plain constructor into an fx-aware provides struct
func makeFxAwareProvides(plainSource reflect.Value, outFxType reflect.Type) reflect.Value {
	if plainSource.Kind() == reflect.Interface {
		// convert an interface into a struct that only contains it
		fxAwareResult := reflect.New(outFxType).Elem()
		fxAwareResult.Field(1).Set(plainSource)
		return fxAwareResult
	}
	return coerceStructTo(plainSource, outFxType, compOutType, fxOutType)
}

// create a struct of the outType and copy fields-by-name from the input to it, replacing embeds recursively
func coerceStructTo(input reflect.Value, outType reflect.Type, oldEmbed, newEmbed reflect.Type) reflect.Value {
	result := reflect.New(outType).Elem()
	for i := 0; i < result.NumField(); i++ {
		target := result.Type().Field(i)
		if target.Type == newEmbed {
			continue
		}
		if v := input.FieldByName(target.Name); v.IsValid() {
			if hasEmbedField(v.Type(), oldEmbed) {
				v = coerceStructTo(v, replaceStructEmbeds(v.Type(), oldEmbed, newEmbed, true), oldEmbed, newEmbed)
			}
			result.FieldByName(target.Name).Set(v)
		}
	}
	return result
}

var _ compdef.Lifecycle = (*fxLifecycleAdapter)(nil)

type fxLifecycleAdapter struct {
	lc fx.Lifecycle
}

func newFxLifecycleAdapter(lc fx.Lifecycle) compdef.Lifecycle {
	return &fxLifecycleAdapter{lc: lc}
}

func (a *fxLifecycleAdapter) Append(h compdef.Hook) {
	a.lc.Append(fx.Hook{
		OnStart: h.OnStart,
		OnStop:  h.OnStop,
	})
}
