// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	_ "embed"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typesSource is parsed by TestDeepCopyCoversAllEntities to find the entity
// types that TestEntityDeepCopy must cover.
//
//go:embed types.go
var typesSource string

// newEntities returns one zero entity of each type implementing Entity. Add new
// entity types here: TestDeepCopyCoversAllEntities fails until they are.
func newEntities() []Entity {
	return []Entity{
		&Container{},
		&ContainerImageMetadata{},
		&CRD{},
		&ECSTask{},
		&GPU{},
		&HostTags{},
		&KubeCapabilities{},
		&Kubelet{},
		&KubeletMetrics{},
		&KubernetesAutoscaler{},
		&KubernetesDeployment{},
		&KubernetesKAIPodGroup{},
		&KubernetesKueueQueue{},
		&KubernetesKueueResourceFlavor{},
		&KubernetesKueueWorkload{},
		&KubernetesMetadata{},
		&KubernetesNode{},
		&KubernetesPod{},
		&Process{},
	}
}

// newFilledEntities returns newEntities with all their exported fields set to
// non-zero values.
func newFilledEntities() []Entity {
	entities := newEntities()
	for _, entity := range entities {
		fillValue(reflect.ValueOf(entity).Elem(), nil)
	}

	return entities
}

// fillValue sets v, and every exported field, element and map entry reachable
// from it, to a non-zero value. ancestors holds the types being filled above
// v: a type that is already one of them is left zero, so that recursive types
// are filled down to a finite depth. Interfaces, functions and channels are
// left nil.
func fillValue(v reflect.Value, ancestors []reflect.Type) {
	if slices.Contains(ancestors, v.Type()) {
		return
	}
	ancestors = append(ancestors, v.Type())

	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	case reflect.String:
		v.SetString("value")
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		fillValue(p.Elem(), ancestors)
		v.Set(p)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillValue(s.Index(0), ancestors)
		v.Set(s)
	case reflect.Array:
		for i := range v.Len() {
			fillValue(v.Index(i), ancestors)
		}
	case reflect.Map:
		key := reflect.New(v.Type().Key()).Elem()
		fillValue(key, ancestors)
		elem := reflect.New(v.Type().Elem()).Elem()
		fillValue(elem, ancestors)
		m := reflect.MakeMap(v.Type())
		m.SetMapIndex(key, elem)
		v.Set(m)
	case reflect.Struct:
		if v.Type() == timeType {
			v.Set(reflect.ValueOf(time.Unix(1, 0).UTC()))
			return
		}
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				fillValue(v.Field(i), ancestors)
			}
		}
	}
}

// mutateValue changes v, and every exported field, element and map entry
// reachable from it, and adds an entry to every map, so that any of them
// shared with another value shows up as a change in that other value.
func mutateValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(v.Uint() + 1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(v.Float() + 1)
	case reflect.String:
		v.SetString(v.String() + "-mutated")
	case reflect.Pointer:
		if !v.IsNil() {
			mutateValue(v.Elem())
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			mutateValue(v.Index(i))
		}
	case reflect.Map:
		// fillValue leaves maps nil where it stops on a recursive type.
		if v.IsNil() {
			return
		}
		for _, key := range v.MapKeys() {
			elem := reflect.New(v.Type().Elem()).Elem()
			elem.Set(v.MapIndex(key))
			mutateValue(elem)
			v.SetMapIndex(key, elem)
		}
		// Changing the values isn't enough for maps whose values can't
		// change, like sets (languagemodels.LanguageSet): also add an entry,
		// with a zero key, which differs from the key fillValue sets for the
		// key types entities use.
		v.SetMapIndex(reflect.Zero(v.Type().Key()), reflect.Zero(v.Type().Elem()))
	case reflect.Struct:
		if v.Type() == timeType {
			v.Set(reflect.ValueOf(v.Interface().(time.Time).Add(time.Hour)))
			return
		}
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				mutateValue(v.Field(i))
			}
		}
	}
}

func TestEntityDeepCopy(t *testing.T) {
	originals := newFilledEntities()
	// Identical to originals, never touched: what the originals must still
	// be equal to once their copies are mutated.
	references := newFilledEntities()

	for i, original := range originals {
		t.Run(reflect.TypeOf(original).Elem().Name(), func(t *testing.T) {
			var cp Entity
			require.NotPanics(t, func() { cp = original.DeepCopy() })
			require.IsType(t, original, cp)
			require.NotSame(t, original, cp)
			require.Equal(t, original, cp)

			mutateValue(reflect.ValueOf(cp).Elem())
			require.NotEqual(t, original, cp)
			assert.Equal(t, references[i], original, "mutating the copy changed the original")
		})
	}
}

// TestDeepCopyCoversAllEntities checks that newEntities returns every
// type of types.go that has a DeepCopy() Entity method.
func TestDeepCopyCoversAllEntities(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "types.go", typesSource, parser.SkipObjectResolution)
	require.NoError(t, err)

	var expected []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "DeepCopy" || fn.Type.Results.NumFields() != 1 {
			continue
		}
		if result, ok := fn.Type.Results.List[0].Type.(*ast.Ident); !ok || result.Name != "Entity" {
			continue
		}

		recv := fn.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			recv = star.X
		}
		ident, ok := recv.(*ast.Ident)
		require.True(t, ok, "unexpected receiver type %T for a DeepCopy method", recv)
		expected = append(expected, ident.Name)
	}

	var covered []string
	for _, entity := range newEntities() {
		covered = append(covered, reflect.TypeOf(entity).Elem().Name())
	}

	assert.ElementsMatch(t, expected, covered, "newEntities must return one entity of each type of types.go implementing Entity")
}
