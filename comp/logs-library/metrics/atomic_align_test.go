// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"reflect"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

const uberAtomicPkg = "go.uber.org/atomic"

// layout32 returns the gc size and alignment of t on a 32-bit target (arm, 386).
func layout32(t reflect.Type) (size, align uintptr) {
	switch t.Kind() {
	case reflect.Bool, reflect.Int8, reflect.Uint8:
		return 1, 1
	case reflect.Int16, reflect.Uint16:
		return 2, 2
	case reflect.Int32, reflect.Uint32, reflect.Float32, reflect.Int, reflect.Uint, reflect.Uintptr,
		reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return 4, 4
	case reflect.Int64, reflect.Uint64, reflect.Float64, reflect.Complex64:
		return 8, 4
	case reflect.Complex128:
		return 16, 4
	case reflect.String, reflect.Interface:
		return 8, 4
	case reflect.Slice:
		return 12, 4
	case reflect.Array:
		s, a := layout32(t.Elem())
		return s * uintptr(t.Len()), a
	case reflect.Struct:
		size, align = 0, 1
		for i := 0; i < t.NumField(); i++ {
			s, a := layout32(t.Field(i).Type)
			size = alignUp(size, a) + s
			align = max(align, a)
		}
		return alignUp(size, align), align
	}
	panic("layout32: unhandled kind " + t.Kind().String())
}

func alignUp(n, a uintptr) uintptr {
	return (n + a - 1) / a * a
}

// misalignedAtomics32 returns paths of by-value go.uber.org/atomic 64-bit words not 8-aligned on 32-bit.
func misalignedAtomics32(t reflect.Type) []string {
	var out []string
	var walk func(t reflect.Type, off uintptr, path string, inUber bool)
	walk = func(t reflect.Type, off uintptr, path string, inUber bool) {
		switch t.Kind() {
		case reflect.Int64, reflect.Uint64, reflect.Float64:
			if inUber && off%8 != 0 {
				out = append(out, path)
			}
		case reflect.Struct:
			inUber = inUber || t.PkgPath() == uberAtomicPkg
			var fieldOff uintptr
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				s, a := layout32(f.Type)
				fieldOff = alignUp(fieldOff, a)
				p := f.Name
				if path != "" {
					p = path + "." + f.Name
				}
				walk(f.Type, off+fieldOff, p, inUber)
				fieldOff += s
			}
		}
	}
	walk(t, 0, "", false)
	return out
}

func TestMisalignedAtomics32Fixtures(t *testing.T) {
	type bad struct {
		A atomic.Int64
		B atomic.Bool
		C atomic.Float64
	}
	type good struct {
		A atomic.Int64
		C atomic.Float64
		B atomic.Bool
	}
	type ptr struct {
		B atomic.Bool
		C *atomic.Float64
	}
	type nested struct {
		S string
		X struct{ N atomic.Uint64 }
	}
	type afterTime struct {
		TS time.Time
		N  atomic.Int64
	}
	type plain struct {
		B bool
		N int64
	}

	assert.Equal(t, []string{"C.v.v"}, misalignedAtomics32(reflect.TypeOf(bad{})))
	assert.Empty(t, misalignedAtomics32(reflect.TypeOf(good{})))
	assert.Empty(t, misalignedAtomics32(reflect.TypeOf(ptr{})))
	assert.Empty(t, misalignedAtomics32(reflect.TypeOf(nested{})))
	assert.Equal(t, []string{"N.v"}, misalignedAtomics32(reflect.TypeOf(afterTime{})))
	assert.Empty(t, misalignedAtomics32(reflect.TypeOf(plain{})), "only go.uber.org/atomic values are checked")

	size, align := layout32(reflect.TypeOf(time.Time{}))
	assert.Equal(t, [2]uintptr{20, 4}, [2]uintptr{size, align})
}

func TestTelemetryUtilizationMonitorAtomicsAligned32(t *testing.T) {
	assert.Empty(t, misalignedAtomics32(reflect.TypeOf(TelemetryUtilizationMonitor{})))
}

func TestNewTelemetryUtilizationMonitorAllocatesAvg(t *testing.T) {
	um := newTelemetryUtilizationMonitorWithSampleRateAndClock("n", "i", time.Second, clock.NewMock(), nil)
	require.NotNil(t, um.avg)
	assert.Zero(t, um.avg.Load())
}
