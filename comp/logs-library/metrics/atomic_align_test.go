// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	stdatomic "sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

// atomicLayoutType preserves value fields and named structs for go/types sizing.
// Reference types need only their size; their contents do not affect struct layout.
func atomicLayoutType(t reflect.Type) types.Type {
	switch t.Kind() {
	case reflect.Struct:
		fields := make([]*types.Var, t.NumField())
		for i := range fields {
			f := t.Field(i)
			fields[i] = types.NewVar(token.NoPos, nil, f.Name, atomicLayoutType(f.Type))
		}
		s := types.NewStruct(fields, nil)
		if t.Name() != "" {
			// Preserve package/name, including sync/atomic's special align64 marker.
			pkg := types.NewPackage(t.PkgPath(), "layout")
			return types.NewNamed(types.NewTypeName(token.NoPos, pkg, t.Name(), nil), s, nil)
		}
		return s
	case reflect.Array:
		return types.NewArray(atomicLayoutType(t.Elem()), int64(t.Len()))
	case reflect.Slice:
		return types.NewSlice(types.Typ[types.Byte])
	case reflect.Interface:
		return types.NewInterfaceType(nil, nil).Complete()
	case reflect.Pointer, reflect.UnsafePointer, reflect.Map, reflect.Chan, reflect.Func:
		return types.Typ[types.UnsafePointer]
	default:
		// reflect and go/types use the same names for numeric, bool and string kinds.
		return types.Universe.Lookup(t.Kind().String()).Type()
	}
}

// misalignedMonitorAtomics checks inline Uber atomic words relative to an
// allocated monitor's 8-byte-aligned base. The constructor test separately checks
// avg's allocation. This models the host's type definitions with ARM sizes;
// it does not replace running the package tests on ARM.
func misalignedMonitorAtomics(t reflect.Type) []string {
	sizes := types.SizesFor("gc", "arm")
	var bad []string
	var walk func(types.Type, int64, string, bool)
	walk = func(t types.Type, offset int64, path string, inAtomic bool) {
		if n, ok := t.(*types.Named); ok {
			inAtomic = inAtomic || n.Obj().Pkg().Path() == "go.uber.org/atomic"
		}
		switch u := t.Underlying().(type) {
		case *types.Struct:
			fields := make([]*types.Var, u.NumFields())
			for i := range fields {
				fields[i] = u.Field(i)
			}
			for i, off := range sizes.Offsetsof(fields) {
				name := fields[i].Name()
				if path != "" {
					name = path + "." + name
				}
				walk(fields[i].Type(), offset+off, name, inAtomic)
			}
		case *types.Array:
			for i := int64(0); i < u.Len(); i++ {
				walk(u.Elem(), offset+i*sizes.Sizeof(u.Elem()), fmt.Sprintf("%s[%d]", path, i), inAtomic)
			}
		case *types.Basic:
			if inAtomic && (u.Kind() == types.Int64 || u.Kind() == types.Uint64 || u.Kind() == types.Float64) && offset%8 != 0 {
				bad = append(bad, path)
			}
		}
	}
	walk(atomicLayoutType(t), 0, "", false)
	return bad
}

func TestTelemetryUtilizationMonitorAtomicAlignment(t *testing.T) {
	assert.Empty(t, misalignedMonitorAtomics(reflect.TypeOf(TelemetryUtilizationMonitor{})),
		"inline 64-bit atomics must be 8-byte aligned on ARM32")
}

func TestMonitorAtomicAlignmentLayouts(t *testing.T) {
	tests := []struct {
		name  string
		value any
		bad   []string
	}{
		{"original avg", struct {
			Busy, Start atomic.Int64
			Started     atomic.Bool
			Avg         atomic.Float64
		}{}, []string{"Avg.v.v"}},
		{"moved bool", struct {
			Busy    atomic.Int64
			Started atomic.Bool
			Start   atomic.Int64
		}{}, []string{"Start.v"}},
		{"new counter", struct {
			Busy, Start atomic.Int64
			Started     atomic.Bool
			Count       atomic.Int64
		}{}, []string{"Count.v"}},
		{"aligned values", struct {
			Busy, Start atomic.Int64
			Avg         atomic.Float64
			Started     atomic.Bool
		}{}, nil},
		{"pointer", struct {
			Started atomic.Bool
			Avg     *atomic.Float64
		}{}, nil},
		{"array stride", [2]struct {
			Count atomic.Int64
			Flag  bool
		}{}, []string{"[1].Count.v"}},
		{"trailing zero sized field", struct {
			Prefix struct {
				N int64
				Z struct{}
			}
			Count atomic.Int64
		}{}, []string{"Count.v"}},
		{"stdlib alignment", struct {
			Flag  bool
			Count stdatomic.Int64
			Uber  atomic.Int64
		}{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.bad, misalignedMonitorAtomics(reflect.TypeOf(tt.value)))
		})
	}
}
