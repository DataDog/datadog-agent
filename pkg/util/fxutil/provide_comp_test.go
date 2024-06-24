// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestValidArgumentAndReturnValue(t *testing.T) {
	// constructor of 0 arguments and 1 interface return value is okay
	opt := ProvideComponentConstructor(func() FirstComp { return &firstImpl{} })
	assertNoCtorError(t, opt)

	// constructor of 0 arguments and 1 composite struct return value is okay
	opt = ProvideComponentConstructor(func() provides1 { return provides1{} })
	assertNoCtorError(t, opt)

	// constructor of 1 composite argument and 1 composite struct return value is okay
	opt = ProvideComponentConstructor(func(reqs requires1) provides1 { return provides1{} })
	assertNoCtorError(t, opt)

	// constructor of 1 composite argument and 2 return values (2nd is error) is okay
	opt = ProvideComponentConstructor(func(reqs requires1) (provides1, error) { return provides1{}, nil })
	assertNoCtorError(t, opt)
}

func TestInvalidArgumentOrReturnValue(t *testing.T) {
	errOpt := ProvideComponentConstructor(1)
	assertIsSingleError(t, errOpt, "argument must be a function with 0 or 1 arguments, and 1 or 2 return values")

	errOpt = ProvideComponentConstructor(func() {})
	assertIsSingleError(t, errOpt, "argument must be a function with 0 or 1 arguments, and 1 or 2 return values")

	errOpt = ProvideComponentConstructor(func(comp FirstComp) SecondComp { return &secondImpl{} })
	assertIsSingleError(t, errOpt, `constructor must either take 0 arguments, or 1 "requires" struct`)

	errOpt = ProvideComponentConstructor(func() (FirstComp, SecondComp) { return &firstImpl{}, &secondImpl{} })
	assertIsSingleError(t, errOpt, "second return value must be error, got fxutil.SecondComp")

	errOpt = ProvideComponentConstructor(func(reqs requires1, reqs2 requires2) FirstComp { return &firstImpl{} })
	assertIsSingleError(t, errOpt, "argument must be a function with 0 or 1 arguments, and 1 or 2 return values")
}

func TestGetConstructorTypes(t *testing.T) {
	// constructor returns 1 component interface
	ctorTypes, err := getConstructorTypes(reflect.TypeOf(func() FirstComp { return &firstImpl{} }))
	require.NoError(t, err)

	expect := `struct {}`
	require.Equal(t, expect, ctorTypes.inPlain.String())

	expect = `struct { In dig.In }`
	require.Equal(t, expect, ctorTypes.inFx.String())

	expect = `struct { Out dig.Out; FirstComp fxutil.FirstComp }`
	require.Equal(t, expect, ctorTypes.outFx.String())

	// constructor needs a `requires` struct and returns 1 component interface
	ctorTypes, err = getConstructorTypes(reflect.TypeOf(func(reqs FirstComp) SecondComp { return &secondImpl{} }))
	require.NoError(t, err)

	expect = `struct { FirstComp fxutil.FirstComp }`
	require.Equal(t, expect, ctorTypes.inPlain.String())

	expect = `struct { In dig.In; FirstComp fxutil.FirstComp }`
	require.Equal(t, expect, ctorTypes.inFx.String())

	expect = `struct { Out dig.Out; SecondComp fxutil.SecondComp }`
	require.Equal(t, expect, ctorTypes.outFx.String())

	// constructor returns a struct that has 3 total components
	ctorTypes, err = getConstructorTypes(reflect.TypeOf(func() provides3 { return provides3{} }))
	require.NoError(t, err)

	expect = `struct {}`
	require.Equal(t, expect, ctorTypes.inPlain.String())

	expect = `struct { In dig.In }`
	require.Equal(t, expect, ctorTypes.inFx.String())

	expect = `struct { Out dig.Out; A fxutil.Apple; B fxutil.Banana; C struct { Out dig.Out; E fxutil.Egg } }`
	require.Equal(t, expect, ctorTypes.outFx.String())

	// constructor needs a `requiresLc` struct and returns 1 component interface
	ctorTypes, err = getConstructorTypes(reflect.TypeOf(func(reqs requiresLc) SecondComp { return &secondImpl{} }))
	require.NoError(t, err)

	expect = `fxutil.requiresLc`
	require.Equal(t, expect, ctorTypes.inPlain.String())

	expect = `struct { In dig.In; Lc compdef.Lifecycle }`
	require.Equal(t, expect, ctorTypes.inFx.String())

	expect = `struct { Out dig.Out; SecondComp fxutil.SecondComp }`
	require.Equal(t, expect, ctorTypes.outFx.String())
}

func TestConstructCompdefIn(t *testing.T) {
	// the required type `requires3` contains an embedded compdef.In, which doesn't have any
	// effect and works just as well as if it weren't there
	ctorTypes, err := getConstructorTypes(reflect.TypeOf(func(reqs requires3) provides1 {
		return provides1{
			First: &firstImpl{},
		}
	}))
	require.NoError(t, err)

	expect := `struct { In dig.In; Second fxutil.SecondComp }`
	require.Equal(t, expect, ctorTypes.inFx.String())

	expect = `struct { Out dig.Out; First fxutil.FirstComp }`
	require.Equal(t, expect, ctorTypes.outFx.String())
}

func TestConstructCompdefOut(t *testing.T) {
	// the provided type `provides5` contains an embedded compdef.Out, which is optional at
	// the top-level
	ctorTypes, err := getConstructorTypes(reflect.TypeOf(func() provides5 {
		return provides5{
			First: &firstImpl{},
		}
	}))
	require.NoError(t, err)

	expect := `struct {}`
	require.Equal(t, expect, ctorTypes.inPlain.String())

	expect = `struct { In dig.In }`
	require.Equal(t, expect, ctorTypes.inFx.String())

	expect = `struct { Out dig.Out; First fxutil.FirstComp }`
	require.Equal(t, expect, ctorTypes.outFx.String())
}

func TestConstructorErrors(t *testing.T) {
	testCases := []struct {
		name   string
		ctor   reflect.Type
		errMsg string
	}{
		{
			// it is an error to have provides5 (with compdef.Out) as an input parameter
			name: "input has embed Out",
			ctor: reflect.TypeOf(func(p provides5) FirstComp {
				return &firstImpl{}
			}),
			errMsg: "invalid embedded field: compdef.Out",
		},
		{
			// it is an error to have requires1 (with compdef.In) as a return value
			name: "output has embed In",
			ctor: reflect.TypeOf(func(reqs requires1) requires3 {
				return requires3{Second: &secondImpl{}}
			}),
			errMsg: "invalid embedded field: compdef.In",
		},
		{
			// it is an error to have requiresLc (with compdef.Lifecycle) as a return value
			name: "output has Lifecycle",
			ctor: reflect.TypeOf(func(reqs requires1) requiresLc {
				return requiresLc{}
			}),
			errMsg: "invalid embedded field: compdef.Lifecycle",
		},
		{
			name: "output is fx-aware",
			ctor: reflect.TypeOf(func(reqs requires1) fxAwareProvides {
				return fxAwareProvides{B: &bananaImpl{}}
			}),
		},
		{
			name: "input is fx-aware",
			ctor: reflect.TypeOf(func(reqs fxAwareReqs) provides1 {
				return provides1{First: &firstImpl{}}
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := getConstructorTypes(tc.ctor)
			if tc.errMsg == "" {
				require.Error(t, err)
			} else {
				require.EqualError(t, err, tc.errMsg)
			}
		})
	}
}

func TestMakeConstructorArgs(t *testing.T) {
	inPlainReqs := plainReqs{
		In: compdef.In{},
		A:  Apple{},
	}
	fxReqs := fxAwareReqs{
		In: fx.In{},
		A:  Apple{},
	}
	expectFields := []string{"In", "A"}
	require.Equal(t, expectFields, getFieldNames(fxReqs))

	// make a struct that doesn't have the fx.In field
	plainReqs := makeConstructorArgs(reflect.ValueOf(fxReqs), reflect.TypeOf(inPlainReqs))[0].Interface()
	expectFields = []string{"In", "A"}
	require.Equal(t, expectFields, getFieldNames(plainReqs))
}

func TestMakeFxAwareProvides(t *testing.T) {
	provides := provides1{
		First: &firstImpl{},
	}
	expectFields := []string{"First"}
	require.Equal(t, expectFields, getFieldNames(provides))

	// make a struct that adds the fx.Out field
	fxAwareProv := makeFxAwareProvides(reflect.ValueOf(provides), reflect.TypeOf(fxProvides1{})).Interface()
	expectFields = []string{"Out", "First"}
	require.Equal(t, expectFields, getFieldNames(fxAwareProv))
}

func TestMakeFxAwareProvidesForCompoundResult(t *testing.T) {
	provides := provides3{
		A: Apple{
			D: Donut{
				X: 5,
			},
		},
		B: &bananaImpl{
			Color: "yellow",
		},
		C: CherryProvider{
			E: Egg{
				Y: 6,
			},
		},
	}
	fxOutType, err := constructFxOutType(reflect.TypeOf(provides))
	require.NoError(t, err)
	expectFields := []string{"Out", "A", "B", "C"}
	require.Equal(t, expectFields, getFieldNames(fxOutType))

	fxAwareProv := makeFxAwareProvides(reflect.ValueOf(provides), fxOutType).Interface()
	data, err := json.Marshal(fxAwareProv)
	require.NoError(t, err)
	expectData := `{"A":{"D":{"X":5}},"B":{"Color":"yellow"},"C":{"E":{"Y":6}}}`
	require.Equal(t, expectData, string(data))
}

func TestConstructFxAwareStruct(t *testing.T) {
	typ, err := constructFxAwareStruct(reflect.TypeOf(requires1{}), false)
	require.NoError(t, err)
	expectFields := []string{"In", "First"}
	require.Equal(t, expectFields, getFieldNames(typ))

	typ, err = constructFxAwareStruct(reflect.TypeOf(provides1{}), true)
	require.NoError(t, err)
	expectFields = []string{"Out", "First"}
	require.Equal(t, expectFields, getFieldNames(typ))
}

func TestConstructFxAwareStructCompoundResult(t *testing.T) {
	typ, err := constructFxAwareStruct(reflect.TypeOf(provides3{}), true)
	require.NoError(t, err)
	expectFields := []string{"Out", "A", "B", "C"}
	require.Equal(t, expectFields, getFieldNames(typ))
}

func TestConstructFxAwareStructWithScalarField(t *testing.T) {
	typ, err := constructFxAwareStruct(reflect.TypeOf(provides4{}), true)
	require.NoError(t, err)
	expectFields := []string{"Out", "C", "F"}
	require.Equal(t, expectFields, getFieldNames(typ))
}

func TestConstructFxAwareStructWithLifecycle(t *testing.T) {
	typ, err := constructFxAwareStruct(reflect.TypeOf(requiresLc{}), false)
	require.NoError(t, err)
	expectFields := []string{"In", "Lc"}
	require.Equal(t, expectFields, getFieldNames(typ))
	// ensure the fx-aware struct uses fx types
	require.Equal(t, typ.Field(0).Type, reflect.TypeOf(fx.In{}))
	require.Equal(t, typ.Field(1).Type, reflect.TypeOf((*compdef.Lifecycle)(nil)).Elem())
}

// test that fx App is able to use constructor with no reqs
func TestFxNoRequirements(t *testing.T) {
	// plain component constructor, no fx
	NewAgentComponent := func() FirstComp {
		return &firstImpl{}
	}
	// define an entry point that uses the component
	start := func(my FirstComp) {
		require.Equal(t, "1st", my.String())
	}
	// ProvideComponentConstructor adds fx to plain constructor
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	Test[FirstComp](t, fx.Invoke(start), module)
}

// test that fx App is able to use constructor with one dependency
func TestFxOneDependency(t *testing.T) {
	// plain component constructor, no fx
	NewAgentComponent := func(reqs requires1) SecondComp {
		return &secondImpl{First: reqs.First}
	}
	// define an entry point that uses the component
	start := func(second SecondComp) {
		require.Equal(t, "2nd", second.Second())
	}
	// ProvideComponentConstructor adds fx to plain constructor
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	Test[SecondComp](t, fx.Invoke(start), module, fx.Provide(func() FirstComp { return &firstImpl{} }))
}

// test that fx App is able to use `requires` and `provides`
func TestFxReqsAndProvides(t *testing.T) {
	// plain component constructor, no fx
	NewAgentComponent := func(reqs requires1) provides2 {
		return provides2{
			Second: &secondImpl{First: reqs.First},
		}
	}
	// define an entry point that uses the component
	start := func(second SecondComp) {
		require.Equal(t, "2nd", second.Second())
	}
	// ProvideComponentConstructor adds fx to plain constructor
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	Test[SecondComp](t, fx.Invoke(start), module, fx.Provide(func() FirstComp { return &firstImpl{} }))
}

// test that fx App can use embedded structs to make compound types
func TestFxProvideEmbed(t *testing.T) {
	NewAgentComponent := func() provides4 {
		return provides4{
			C: CherryProvider{
				E: Egg{
					Y: 4,
				},
			},
			F: FruitProvider{
				Z: 5,
			},
		}
	}
	// both Egg and int are available because their containing struct uses compdef.Out
	start := func(one Egg, two int) {
		require.Equal(t, 4, one.Y)
		require.Equal(t, 5, two)
	}
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	Test[Egg](t, fx.Invoke(start), module)
}

func TestFxCanUseTwice(t *testing.T) {
	// plain component constructor, no fx
	NewAgentComponent := func() FirstComp {
		return &firstImpl{}
	}
	NewAnotherComponent := func(reqs requires1) SecondComp {
		return &secondImpl{First: reqs.First}
	}
	// define an entry point that uses the component
	start := func(second SecondComp) {
		require.Equal(t, "2nd", second.Second())
	}
	// ProvideComponentConstructor can be used twice
	module := Component(ProvideComponentConstructor(NewAgentComponent), ProvideComponentConstructor(NewAnotherComponent))
	Test[SecondComp](t, fx.Invoke(start), module)
}

func TestFxCompdefIn(t *testing.T) {
	// plain component constructor, uses compdef.In embed field
	NewAgentComponent := func(reqs requires3) Banana {
		return &bananaImpl{}
	}
	// define an entry point that uses the component
	start := func(b Banana) {
		require.Equal(t, "*fxutil.bananaImpl", fmt.Sprintf("%T", b))
	}
	// ProvideComponentConstructor adds fx to plain constructor
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	// Test[SecondComp](t, fx.Invoke(start), module, fx.Provide(func() FirstComp { return &firstImpl{} }))
	Test[Banana](t, fx.Invoke(start), module, fx.Provide(func() SecondComp { return &secondImpl{} }))
}

// type that fx App can use Lifecycle hooks
func TestFxLifecycle(t *testing.T) {
	counter := 0
	NewAgentComponent := func(reqs requiresLc) providesService {
		reqs.Lc.Append(compdef.Hook{
			OnStart: func(context.Context) error {
				counter++
				return nil
			},
			OnStop: func(context.Context) error { return nil },
		})
		return providesService{First: &firstImpl{}}
	}
	start := func(one FirstComp) {
		counter += 2
	}
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	Test[FirstComp](t, fx.Invoke(start), module)
	// ensures that both the OnStart hook and entry point are called
	require.Equal(t, 3, counter)
}

func TestFxReturnNoError(t *testing.T) {
	// plain component constructor, no fx
	NewAgentComponent := func(reqs requires1) (provides2, error) {
		return provides2{
			Second: &secondImpl{First: reqs.First},
		}, nil
	}
	// define an entry point that uses the component
	start := func(second SecondComp) {
		require.Equal(t, "2nd", second.Second())
	}
	// ProvideComponentConstructor adds fx to plain constructor
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	Test[SecondComp](t, fx.Invoke(start), module, fx.Provide(func() FirstComp { return &firstImpl{} }))
}

func TestFxReturnAnError(t *testing.T) {
	// plain component constructor, no fx
	NewAgentComponent := func(reqs requires1) (provides2, error) {
		return provides2{
			Second: &secondImpl{First: reqs.First},
		}, fmt.Errorf("fail construction")
	}
	// define an entry point that uses the component
	start := func(second SecondComp) {
		require.Fail(t, "should not reach this point because constructor failed")
	}
	// ProvideComponentConstructor adds fx to plain constructor
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	app := fx.New(fx.Invoke(start), module, fx.Provide(func() FirstComp { return &firstImpl{} }))
	require.Error(t, app.Err())
}

func assertNoCtorError(t *testing.T, arg fx.Option) {
	t.Helper()
	app := fx.New(arg)
	err := app.Err()
	if err != nil {
		t.Fatalf("expected no error, instead got %s", err)
	}
}

func assertIsSingleError(t *testing.T, arg fx.Option, errMsg string) {
	t.Helper()
	app := fx.New(arg)
	err := app.Err()
	if err == nil {
		t.Fatalf("expected an error, instead got %v of type %T", arg, arg)
	} else if err.Error() != errMsg {
		t.Fatalf("errror mismatch, expected %v, got %v", errMsg, err.Error())
	}
}

func getFieldNames(it interface{}) []string {
	var typ reflect.Type
	if t, ok := it.(reflect.Type); ok {
		typ = t
	} else {
		typ = reflect.TypeOf(it)
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	fields := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, typ.Field(i).Name)
	}
	return fields
}

// sample test data follows

// FirstComp is an interface for a Component, it doesn't do anything

type FirstComp interface {
	String() string
}

type firstImpl struct{}

func (m *firstImpl) String() string {
	return "1st"
}

// SecondComp is an interface for a Component, it depends on FirstComp for construction

type SecondComp interface {
	Second() string
}

type secondImpl struct {
	First FirstComp
}

func (m *secondImpl) Second() string {
	return "2nd"
}

// provides1 provides 1 component using a composite struct

type provides1 struct {
	First FirstComp
}

type fxProvides1 struct {
	fx.Out
	First FirstComp
}

// provides2 provides a different component

type provides2 struct {
	Second SecondComp
}

// provides3 provides a composite struct that embeds another

type provides3 struct {
	A Apple
	B Banana
	C CherryProvider
}

type Apple struct {
	D Donut
}

type Banana interface{}

type bananaImpl struct {
	Color string
}

type CherryProvider struct {
	compdef.Out
	E Egg
}

type Donut struct {
	X int
}

type Egg struct {
	Y int
}

// provides4 contains two different embedding structs

type provides4 struct {
	C CherryProvider
	F FruitProvider
}

type FruitProvider struct {
	compdef.Out
	Z int
}

// provides5 is just like provides1 but also embeds compdef.Out (no difference in functionality)

type provides5 struct {
	compdef.Out
	First FirstComp
}

// requires1 requires 1 component using a composite struct

type requires1 struct {
	First FirstComp
}

// requires2 requires a different component

type requires2 struct {
	Second SecondComp
}

// requires3 embeds a compdef.In (optional, for convenience)

type requires3 struct {
	compdef.In
	Second SecondComp
}

// requiresLc uses Lifecycles

type requiresLc struct {
	Lc compdef.Lifecycle
}

type providesService struct {
	First FirstComp
}

// fxAwareReqs is an fx-aware requires struct

type fxAwareReqs struct {
	fx.In
	A Apple
}

type plainReqs struct {
	compdef.In
	A Apple
}

// fxAwareProvides is an fx-aware provides struct

type fxAwareProvides struct {
	fx.Out
	B Banana
}
