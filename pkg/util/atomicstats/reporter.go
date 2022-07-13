// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package atomicstats provides support for "stats" structs containing atomic
// values.
package atomicstats

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unsafe"

	"go.uber.org/atomic"
)

const statsTag = "stats"

// Reporter supports reporting on a specific struct type, containing
// precomputed values to ensure subsequent operations are faster.
//
// Typically a single Reporter is created at startup for each stats struct.
//
// Such structs should tag fields to be included in the stats with `stats:""`.
// Stats fields can be of any of the following types:
//
//     int
//     int8
//     int16
//     int32
//     int64
//     uint
//     uint8
//     uint16
//     uint32
//     uint64
//     uintptr
//     go.uber.org/atomic.Bool
//     go.uber.org/atomic.Duration
//     go.uber.org/atomic.Error
//     go.uber.org/atomic.Float64
//     go.uber.org/atomic.Int32
//     go.uber.org/atomic.Int64
//     go.uber.org/atomic.String
//     go.uber.org/atomic.Time
//     go.uber.org/atomic.Uint32
//     go.uber.org/atomic.Uint64
//     go.uber.org/atomic.Uintptr
//     go.uber.org/atomic.UnsafePointer
//     go.uber.org/atomic.Value
type Reporter struct {
	// structType is the type of value this Reporter reports on
	structType reflect.Type

	// stats maps field names to the struct index containing their value
	stats map[string]int
}

// NewReporter creates a new Reporter to represent the type of the argument.
//
// Typically the argument is a nil pointer of the correct type:
//
//     type foostats struct { .. }
//     statsReporter := atomicstats.NewReporter((*foostats)(nil))
func NewReporter(v interface{}) Reporter {
	ptrType := reflect.TypeOf(v)

	if ptrType.Kind() != reflect.Ptr || ptrType.Elem().Kind() != reflect.Struct {
		panic("NewReporter expects a pointer to a struct")
	}

	structType := ptrType.Elem()

	reporter := Reporter{
		structType: structType,
		stats:      map[string]int{},
	}

	for f := 0; f < structType.NumField(); f++ {
		field := structType.Field(f)
		if _, ok := field.Tag.Lookup(statsTag); ok {
			if !isTypeSupported(field) {
				panic(fmt.Errorf("unsupported type %+v for field %s", field.Type.Kind(), field.Name))
			}

			reporter.stats[toSnakeCase(field.Name)] = f
		}
	}

	return reporter
}

// Report returns a `map` representation of the stats in the given value.
// All map keys are converted to snake_case.
//
// The value _must_ have the same type as the value passed to NewReporter.
func (r *Reporter) Report(v interface{}) map[string]interface{} {
	stats := make(map[string]interface{}, len(r.stats))

	value := reflect.ValueOf(v)
	r.checkType(value)

	// dereference the pointer
	value = value.Elem()

	for name, idx := range r.stats {
		var v interface{}
		f := value.Field(idx)

		switch f.Kind() {
		case reflect.Int:
			v = int(f.Int())
		case reflect.Int8:
			v = int8(f.Int())
		case reflect.Int16:
			v = int16(f.Int())
		case reflect.Int32:
			v = int32(f.Int())
		case reflect.Int64:
			v = int64(f.Int()) //nolint:unconvert
		case reflect.Uint:
			v = uint(f.Uint())
		case reflect.Uint8:
			v = uint8(f.Uint())
		case reflect.Uint16:
			v = uint16(f.Uint())
		case reflect.Uint32:
			v = uint32(f.Uint())
		case reflect.Uint64:
			v = uint64(f.Uint()) //nolint:unconvert
		case reflect.Uintptr:
			v = *(*uintptr)(unsafe.Pointer(f.UnsafeAddr()))
		case reflect.Ptr:
			// This is a "trick" to hide the fact that f is unexported from f.Interface,
			// which (unlike f.Int etc, above) will panic for unexported fields
			f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()

			referentType := qualifiedTypeName(f.Elem().Type())
			switch referentType {
			case "go.uber.org/atomic.Bool":
				v = f.Interface().(*atomic.Bool).Load()
			case "go.uber.org/atomic.Duration":
				v = f.Interface().(*atomic.Duration).Load()
			case "go.uber.org/atomic.Error":
				v = f.Interface().(*atomic.Error).Load()
			case "go.uber.org/atomic.Float64":
				v = f.Interface().(*atomic.Float64).Load()
			case "go.uber.org/atomic.Int32":
				v = f.Interface().(*atomic.Int32).Load()
			case "go.uber.org/atomic.Int64":
				v = f.Interface().(*atomic.Int64).Load()
			case "go.uber.org/atomic.String":
				v = f.Interface().(*atomic.String).Load()
			case "go.uber.org/atomic.Time":
				v = f.Interface().(*atomic.Time).Load()
			case "go.uber.org/atomic.Uint32":
				v = f.Interface().(*atomic.Uint32).Load()
			case "go.uber.org/atomic.Uint64":
				v = f.Interface().(*atomic.Uint64).Load()
			case "go.uber.org/atomic.Uintptr":
				v = f.Interface().(*atomic.Uintptr).Load()
			case "go.uber.org/atomic.UnsafePointer":
				v = f.Interface().(*atomic.UnsafePointer).Load()
			case "go.uber.org/atomic.Value":
				v = f.Interface().(*atomic.Value).Load()
			default:
				panic("Unrecognized pointer to %s" + referentType)
			}
		default:
			panic(fmt.Sprintf("unrecognized kind %#v for field %s", f.Kind(), name))
		}
		stats[name] = v
	}
	return stats
}

// checkType checks that the given value is a pointer to r.structType
func (r *Reporter) checkType(value reflect.Value) {
	if value.Type() != reflect.PtrTo(r.structType) {
		panic(fmt.Sprintf("Reporter#Report expects %s; got %s", reflect.PtrTo(r.structType), value.Type()))
	}
}

func isTypeSupported(f reflect.StructField) bool {
	switch f.Type.Kind() {
	case
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		return true
	case reflect.Ptr:
		referentType := qualifiedTypeName(f.Type.Elem())
		switch referentType {
		case
			"go.uber.org/atomic.Bool",
			"go.uber.org/atomic.Duration",
			"go.uber.org/atomic.Error",
			"go.uber.org/atomic.Float64",
			"go.uber.org/atomic.Int32",
			"go.uber.org/atomic.Int64",
			"go.uber.org/atomic.String",
			"go.uber.org/atomic.Time",
			"go.uber.org/atomic.Uint32",
			"go.uber.org/atomic.Uint64",
			"go.uber.org/atomic.Uintptr",
			"go.uber.org/atomic.UnsafePointer",
			"go.uber.org/atomic.Value":
			return true
		}
	}

	return false
}

func qualifiedTypeName(ty reflect.Type) string {
	return ty.PkgPath() + "." + ty.Name()
}

// from https://gist.github.com/stoewer/fbe273b711e6a06315d19552dd4d33e6
var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
