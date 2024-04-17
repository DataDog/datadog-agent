package fxutil

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	compDef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestValidArgumentAndReturnValue(t *testing.T) {
	// constructor of 0 arguments and 1 interface return value is okay
	opt := ProvideComponentConstructor(func() FirstComp { return &firstImpl{} })
	assertNoCtorError(t, opt)

	// constructor of 1 interface argument and 1 interface return value is okay
	opt = ProvideComponentConstructor(func(reqs FirstComp) SecondComp { return &secondImpl{} })
	assertNoCtorError(t, opt)

	// constructor of 0 arguments and 1 composite struct return value is okay
	opt = ProvideComponentConstructor(func() provides1 { return provides1{} })
	assertNoCtorError(t, opt)

	// constructor of 1 composite argument and 1 composite struct return value is okay
	opt = ProvideComponentConstructor(func(reqs requires1) provides1 { return provides1{} })
	assertNoCtorError(t, opt)
}

func TestInvalidArgumentOrReturnValue(t *testing.T) {
	errOpt := ProvideComponentConstructor(1)
	assertIsSingleError(t, errOpt, "argument to ProvideComponentConstructor must be a function with at most 1 argument and exactly 1 return value")

	errOpt = ProvideComponentConstructor(func() {})
	assertIsSingleError(t, errOpt, "argument to ProvideComponentConstructor must be a function with at most 1 argument and exactly 1 return value")

	errOpt = ProvideComponentConstructor(func(reqs FirstComp) {})
	assertIsSingleError(t, errOpt, "argument to ProvideComponentConstructor must be a function with at most 1 argument and exactly 1 return value")

	errOpt = ProvideComponentConstructor(func() (FirstComp, SecondComp) { return &firstImpl{}, &secondImpl{} })
	assertIsSingleError(t, errOpt, "argument to ProvideComponentConstructor must be a function with at most 1 argument and exactly 1 return value")

	errOpt = ProvideComponentConstructor(func(reqs requires1, reqs2 requires2) FirstComp { return &firstImpl{} })
	assertIsSingleError(t, errOpt, "argument to ProvideComponentConstructor must be a function with at most 1 argument and exactly 1 return value")
}

func TestConstructFxInAndOut(t *testing.T) {
	// constructor returns 1 component interface
	inType, outType, _, err := constructFxInAndOut(func() FirstComp { return &firstImpl{} })
	require.NoError(t, err)

	expect := `struct { In dig.In }`
	require.Equal(t, expect, inType.String())

	expect = `struct { Out dig.Out; FirstComp fxutil.FirstComp }`
	require.Equal(t, expect, outType.String())

	// constructor needs a `requires`` struct and returns 1 component interface
	inType, outType, _, err = constructFxInAndOut(func(reqs FirstComp) SecondComp { return &secondImpl{} })
	require.NoError(t, err)

	expect = `struct { In dig.In; FirstComp fxutil.FirstComp }`
	require.Equal(t, expect, inType.String())

	expect = `struct { Out dig.Out; SecondComp fxutil.SecondComp }`
	require.Equal(t, expect, outType.String())

	// constructor returns a struct that has 3 total components
	inType, outType, _, err = constructFxInAndOut(func() provides3 { return provides3{} })
	require.NoError(t, err)

	expect = `struct { In dig.In }`
	require.Equal(t, expect, inType.String())

	expect = `struct { Out dig.Out; Apple fxutil.Apple; B fxutil.Banana; Egg fxutil.Egg }`
	require.Equal(t, expect, outType.String())
}

func TestMakePlainReqs(t *testing.T) {
	fxReqs := fxAwareReqs{
		In: fx.In{},
		A:  Apple{},
	}
	expectFields := []string{"In", "A"}
	require.Equal(t, expectFields, getFieldNames(fxReqs))

	// make a struct that doesn't have the fx.In field
	plainReqs := makePlainReqs(reflect.ValueOf(fxReqs), nil).Interface()
	expectFields = []string{"A"}
	require.Equal(t, expectFields, getFieldNames(plainReqs))
}

func TestMakeFxAwareProvides(t *testing.T) {
	provides := provides1{
		First: &firstImpl{},
	}
	expectFields := []string{"First"}
	require.Equal(t, expectFields, getFieldNames(provides))

	// make a struct that adds the fx.Out field
	fxAwareProv := makeFxAwareProvides(reflect.ValueOf(provides), reflect.TypeOf(fxProvides1{}), nil).Interface()
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
	fxOutType, outEmbeds, err := constructFxOutType(reflect.TypeOf(provides))
	require.NoError(t, err)
	expectFields := []string{"Out", "Apple", "B", "Egg"}
	require.Equal(t, expectFields, getFieldNames(fxOutType))

	fxAwareProv := makeFxAwareProvides(reflect.ValueOf(provides), fxOutType, outEmbeds).Interface()
	data, err := json.Marshal(fxAwareProv)
	require.NoError(t, err)
	expectData := `{"Apple":{"D":{"X":5}},"B":{"Color":"yellow"},"Egg":{"Y":6}}`
	require.Equal(t, expectData, string(data))
}

func TestConstructFxAwareStruct(t *testing.T) {
	typ, _, err := constructFxAwareStruct(reflect.TypeOf(requires1{}), false)
	require.NoError(t, err)
	expectFields := []string{"In", "First"}
	require.Equal(t, expectFields, getFieldNames(typ))

	typ, _, err = constructFxAwareStruct(reflect.TypeOf(provides1{}), true)
	require.NoError(t, err)
	expectFields = []string{"Out", "First"}
	require.Equal(t, expectFields, getFieldNames(typ))
}

func TestConstructFxAwareStructCompoundResult(t *testing.T) {
	typ, _, err := constructFxAwareStruct(reflect.TypeOf(provides3{}), true)
	require.NoError(t, err)
	expectFields := []string{"Out", "Apple", "B", "Egg"}
	require.Equal(t, expectFields, getFieldNames(typ))
}

func TestConstructFxAwareStructWithScalarField(t *testing.T) {
	typ, _, err := constructFxAwareStruct(reflect.TypeOf(provides4{}), true)
	require.NoError(t, err)
	expectFields := []string{"Out", "Egg", "Z"}
	require.Equal(t, expectFields, getFieldNames(typ))
}

func TestConstructFxAwareStructWithLifecycle(t *testing.T) {
	typ, _, err := constructFxAwareStruct(reflect.TypeOf(requiresLc{}), false)
	require.NoError(t, err)
	expectFields := []string{"In", "Lc"}
	require.Equal(t, expectFields, getFieldNames(typ))
	// ensure the fx-aware struct uses fx types
	require.Equal(t, typ.Field(0).Type, reflect.TypeOf(fx.In{}))
	require.Equal(t, typ.Field(1).Type, reflect.TypeOf((*fx.Lifecycle)(nil)).Elem())
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
	NewAgentComponent := func(first FirstComp) SecondComp {
		return &secondImpl{First: first}
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
	// both Egg and int are available because their containing struct uses compDef.Out
	start := func(one Egg, two int) {
		require.Equal(t, 4, one.Y)
		require.Equal(t, 5, two)
	}
	module := Component(ProvideComponentConstructor(NewAgentComponent))
	Test[Egg](t, fx.Invoke(start), module)
}

// type that fx App can use Lifecycle hooks
func TestFxLifecycle(t *testing.T) {
	counter := 0
	NewAgentComponent := func(reqs requiresLc) providesService {
		reqs.Lc.Append(compDef.Hook{
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

func assertNoCtorError(t *testing.T, arg fx.Option) {
	app := fx.New(arg)
	err := app.Err()
	if err != nil {
		t.Fatalf("expected no error, instead got %s", err)
	}
}

func assertIsSingleError(t *testing.T, arg fx.Option, errMsg string) {
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
	compDef.Out
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
	compDef.Out
	Z int
}

// requires1 requires 1 component using a composite struct

type requires1 struct {
	First FirstComp
}

// requires2 requires a different component

type requires2 struct {
	Second SecondComp
}

// requiresLc uses Lifecycles

type requiresLc struct {
	Lc compDef.Lifecycle
}

type providesService struct {
	First FirstComp
}

// fxAwareReqs is an fx-aware requires struct

type fxAwareReqs struct {
	fx.In
	A Apple
}
