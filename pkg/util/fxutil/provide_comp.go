// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	compDef "github.com/DataDog/datadog-agent/comp/def"
	"go.uber.org/fx"
)

var (
	compOutType = reflect.TypeOf((*compDef.Out)(nil)).Elem()
	compLcType  = reflect.TypeOf((*compDef.Lifecycle)(nil)).Elem()
	fxInType    = reflect.TypeOf((*fx.In)(nil)).Elem()
	fxOutType   = reflect.TypeOf((*fx.Out)(nil)).Elem()
	fxLcType    = reflect.TypeOf((*fx.Lifecycle)(nil)).Elem()
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
// and essentially make this happen:
//
//	fx.Provide(func(fxr FxAwareRequires) fxp FxAwareProvides {
//	    r := makePlainReqs(fxr)
//	    p := NewComponent(r)
//	    return makeFxAwareProvides(p)
//	})
func ProvideComponentConstructor(compCtorFunc interface{}) fx.Option {
	inFxType, outFxType, outEmbeds, err := constructFxInAndOut(compCtorFunc)
	if err != nil {
		return fx.Error(err)
	}
	// build reflect.Type of the constructor function that will be provided to `fx.Provide`
	funcFxType := reflect.FuncOf([]reflect.Type{inFxType}, []reflect.Type{outFxType}, false)

	hs := &compDef.HookStorage{}

	// wrapper that receives fx-aware requirements, converts them into regular requirements, and calls the constructor
	wrapperForConstructor := func(fxAwareReqs reflect.Value) reflect.Value {
		constructorFunc := reflect.ValueOf(compCtorFunc)
		// invoke the regular constructor with the correct arguments
		ctorArgs := makeConstructorArgs(constructorFunc, fxAwareReqs, hs)
		plainOuts := constructorFunc.Call(ctorArgs)
		maybeAppendFxLifecycle(ctorArgs, fxAwareReqs)
		// calling `constructFxInAndOut` earlier ensures that outs has exactly 1 element
		return makeFxAwareProvides(plainOuts[0], outFxType, outEmbeds)
	}

	// constructor function value that will inform fx what the Components are
	fxAwareProviderFunc := reflect.MakeFunc(funcFxType, func(args []reflect.Value) []reflect.Value {
		reqs := args[0]
		res := wrapperForConstructor(reqs)
		return []reflect.Value{res}
	})

	fn := fxAwareProviderFunc.Interface()
	return fx.Provide(fn)
}

func makeConstructorArgs(constructorFunc, fxAwareReqs reflect.Value, hs *compDef.HookStorage) []reflect.Value {
	// plain constructor either takes 0 or 1 arguments
	if constructorFunc.Type().NumIn() == 0 {
		return nil
	}
	// make requirements without fx.In
	plainReqs := makePlainReqs(fxAwareReqs, hs)
	// if constructor takes a single interface, extract it from reqs struct
	if constructorFunc.Type().In(0).Kind() == reflect.Interface {
		plainReqs = plainReqs.Field(0).Elem()
	}
	return []reflect.Value{plainReqs}
}

func constructFxInAndOut(compCtorFunc interface{}) (reflect.Type, reflect.Type, structEmbeds, error) {
	// validate the constructor is a function with 0 or 1 argument(s) and 1 return value
	ctorFuncVal := reflect.TypeOf(compCtorFunc)
	if ctorFuncVal.Kind() != reflect.Func || ctorFuncVal.NumIn() > 1 || ctorFuncVal.NumOut() != 1 {
		return nil, nil, nil, errors.New("argument to ProvideComponentConstructor must be a function with at most 1 argument and exactly 1 return value")
	}

	var ctorInType reflect.Type
	if ctorFuncVal.NumIn() == 1 {
		ctorInType = ctorFuncVal.In(0)
	}
	ctorOutType := ctorFuncVal.Out(0)

	// create types that have fx-aware meta-fields
	// these are used to construct a function that can build the fx graph
	inFxType, _, err := constructFxInType(ctorInType)
	if err != nil {
		return nil, nil, nil, err
	}
	outFxType, outEmbeds, err := constructFxOutType(ctorOutType)
	return inFxType, outFxType, outEmbeds, err
}

type structEmbeds []reflect.Type

func constructFxInType(plainType reflect.Type) (reflect.Type, structEmbeds, error) {
	return constructFxAwareStruct(plainType, false)
}

func constructFxOutType(plainType reflect.Type) (reflect.Type, structEmbeds, error) {
	return constructFxAwareStruct(plainType, true)
}

func constructFxAwareStruct(plainType reflect.Type, isOut bool) (reflect.Type, structEmbeds, error) {
	componentList, outEmbeds, err := collectComponentList(plainType, "", true)
	if err != nil {
		return nil, nil, err
	}

	// if plain type had a compDef.Lifecycle, switch to fx.Lifecycle
	for i := 0; i < len(componentList); i++ {
		if componentList[i].Type == compLcType {
			componentList[i] = reflect.StructField{Type: fxLcType, Name: "Lc"}
		}
	}

	// create an anonymous struct that matches the plainType,
	// except it also has "fx.In" / "fx.Out" embedded
	var metaField reflect.StructField
	if isOut {
		metaField = reflect.StructField{Name: "Out", Type: fxOutType, Anonymous: true}
	} else {
		metaField = reflect.StructField{Name: "In", Type: fxInType, Anonymous: true}
		outEmbeds = nil
	}
	// prepend the metafield so it shows up first
	// this is slightly less efficient, but is more similar to conventional fx, making it easier to use and debug
	allFields := append([]reflect.StructField{metaField}, componentList...)
	return reflect.StructOf(allFields), outEmbeds, nil
}

func collectComponentList(arg reflect.Type, argName string, isTopLevel bool) ([]reflect.StructField, structEmbeds, error) {
	var res []reflect.StructField
	var outEmbeds structEmbeds
	if arg == nil {
		return res, nil, nil
	}
	if argName == "" {
		argName = arg.Name()
	}
	if arg.Kind() == reflect.Interface {
		res = append(res, reflect.StructField{Name: argName, Type: arg})
	} else if arg.Kind() == reflect.Struct {
		elems, embeds, err := maybeRecursivelyExtractFields(arg, isTopLevel)
		if err != nil {
			return nil, nil, err
		}
		res = append(res, elems...)
		outEmbeds = append(outEmbeds, embeds...)
	} else if !isTopLevel {
		res = append(res, reflect.StructField{Name: argName, Type: arg})
	} else {
		return nil, nil, fmt.Errorf("invalid field %s of type %T", arg, arg)
	}

	return res, outEmbeds, nil
}

// recursively extract fields from the struct, if the struct contains compDef.Out or alwaysExtract
func maybeRecursivelyExtractFields(typ reflect.Type, alwaysExtract bool) ([]reflect.StructField, structEmbeds, error) {
	if typ.Kind() != reflect.Struct {
		panic("pre-condition failure: must be called with Struct")
	}
	hasOutMeta := false
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type == compOutType {
			hasOutMeta = true
			break
		}
	}

	if !alwaysExtract && !hasOutMeta {
		// scalar field, assume it is a special value supplied for fx purposes
		simpleName := typ.String()
		parts := strings.Split(simpleName, ".")
		if len(parts) > 1 {
			simpleName = parts[len(parts)-1]
		}
		return []reflect.StructField{{Name: simpleName, Type: typ}}, nil, nil
	}

	var outEmbeds structEmbeds
	if hasOutMeta {
		outEmbeds = append(outEmbeds, typ)
	}

	// NOTE: slice might still need to be reallocated if this function recurses
	res := make([]reflect.StructField, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type == compOutType {
			continue
		}
		elems, embeds, err := collectComponentList(field.Type, field.Name, false)
		if err != nil {
			return nil, nil, err
		}
		res = append(res, elems...)
		outEmbeds = append(outEmbeds, embeds...)
	}
	return res, outEmbeds, nil
}

// convert dependencies from fx-aware deps into a plain struct that
// can be used to call the real constructor for the Component
func makePlainReqs(fxAwareReqs reflect.Value, hs *compDef.HookStorage) reflect.Value {
	if fxAwareReqs.Kind() != reflect.Struct {
		panic("pre-condition failure: must be called with Struct")
	}
	fxAwareType := fxAwareReqs.Type()

	// create an anonymous struct that matches our input,
	// except it removes the embedded `fx.In`, and switches fx.Lifecycle to compDef.Lifecycle
	newFields := make([]reflect.StructField, 0, fxAwareReqs.NumField()-1)
	for i := 0; i < fxAwareType.NumField(); i++ {
		if fxAwareType.Field(i).Type == fxInType {
			continue
		}
		newF := reflect.StructField{
			Type: fxAwareType.Field(i).Type,
			Name: fxAwareType.Field(i).Name,
		}
		if fxAwareType.Field(i).Type == fxLcType {
			newF = reflect.StructField{
				Type: compLcType,
				Name: fxAwareType.Field(i).Name,
			}
		}
		newFields = append(newFields, newF)
	}

	// copy the field values
	makeResult := reflect.New(reflect.StructOf(newFields)).Elem()
	for i, j := 0, 0; i < fxAwareType.NumField(); i++ {
		if fxAwareType.Field(i).Type == fxInType {
			continue
		}
		if fxAwareType.Field(i).Type == fxLcType {
			lc := compDef.Lifecycle{}
			lc.SetStorage(hs)
			makeResult.Field(j).Set(reflect.ValueOf(lc))
			j++
			continue
		}
		makeResult.Field(j).Set(fxAwareReqs.Field(i))
		j++
	}
	return makeResult
}

type state struct {
	val reflect.Value
	idx int
}

func makeFxAwareProvides(plainProvides reflect.Value, outFxType reflect.Type, outEmbeds structEmbeds) reflect.Value {
	fxAwareResult := reflect.New(outFxType).Elem()

	if plainProvides.Kind() == reflect.Interface {
		fxAwareResult.Field(1).Set(plainProvides)
		return fxAwareResult
	}

	// provides can contain embedded structs due to compDef.Out, need to use a stack to recursively
	// handle this tree of structs. The `k` is used to index into our list of which struct types contain
	// compDef.Out, as their order should match those found in the tree of provide structs
	var stack []state
	j, k := 0, 0

	for i := 0; i < fxAwareResult.NumField(); i++ {
		if fxAwareResult.Field(i).Type() == fxOutType {
			continue
		}

		if outEmbeds != nil && plainProvides.Field(j).Type() == outEmbeds[k] {
			k++
			stack = append(stack, state{plainProvides, j})
			plainProvides = plainProvides.Field(j)
			j = 0
		}

		if plainProvides.Field(j).Type() == compOutType {
			j++
		}

		fxAwareResult.Field(i).Set(plainProvides.Field(j))
		j++

		if j >= plainProvides.NumField() && len(stack) > 0 {
			last := stack[len(stack)-1]
			plainProvides = last.val
			j = last.idx + 1
			stack = stack[:len(stack)-1]
		}
	}
	return fxAwareResult
}

// if hooks were appended to for the plain constructor, copy them to the fx.Lifecycle
func maybeAppendFxLifecycle(plainArgs []reflect.Value, fxReqs reflect.Value) {
	hooks := retrieveLcHooks(plainArgs)
	if len(hooks) == 0 {
		return
	}
	if fxReqs.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < fxReqs.NumField(); i++ {
		if fxReqs.Field(i).Type() != fxLcType {
			continue
		}
		f := fxReqs.Field(i).Elem().Interface()
		if lc, ok := f.(fx.Lifecycle); ok {
			for j := 0; j < len(hooks); j++ {
				lc.Append(fx.Hook{
					OnStart: hooks[j].OnStart,
					OnStop:  hooks[j].OnStop,
				})
			}
		}
	}
}

// retrieve hooks appended to the `requires` given to the plain constructor
func retrieveLcHooks(args []reflect.Value) []compDef.Hook {
	if len(args) == 0 {
		return nil
	}
	v := args[0]
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Type() != compLcType {
			continue
		}
		if lc, ok := v.Field(i).Interface().(compDef.Lifecycle); ok {
			return lc.Hooks()
		}
	}
	return nil
}
