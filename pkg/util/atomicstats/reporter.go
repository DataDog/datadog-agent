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

	// fields maps field names to getters and setters for that field
	fields map[string]field
}

type field struct {
	// idx is the index of the field in the struct
	idx int

	// get will get the value from the field
	get func(f reflect.Value) interface{}
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
		// errors here are programming (type) errors that cannot be caught at compile time,
		// and thus are fatal
		panic("NewReporter expects a pointer to a struct")
	}

	structType := ptrType.Elem()

	reporter := Reporter{
		structType: structType,
		fields:     map[string]field{},
	}

	for idx := 0; idx < structType.NumField(); idx++ {
		fieldType := structType.Field(idx)
		if _, ok := fieldType.Tag.Lookup(statsTag); ok {
			fld, err := getFieldFor(fieldType.Type)
			if err != nil {
				panic(err.Error())
			}

			fld.idx = idx
			reporter.fields[toSnakeCase(fieldType.Name)] = fld
		}
	}

	return reporter
}

// Report returns a `map` representation of the stats in the given value.
// All map keys are converted to snake_case.
//
// The value _must_ have the same type as the value passed to NewReporter.
func (r *Reporter) Report(v interface{}) map[string]interface{} {
	stats := make(map[string]interface{}, len(r.fields))

	value := reflect.ValueOf(v)
	r.checkType(value)

	// dereference the pointer
	value = value.Elem()

	for name, fld := range r.fields {
		stats[name] = fld.get(value.Field(fld.idx))
	}

	return stats
}

// checkType checks that the given value is a pointer to r.structType
func (r *Reporter) checkType(value reflect.Value) {
	if value.Type() != reflect.PtrTo(r.structType) {
		panic(fmt.Sprintf("Reporter#Report expects %s; got %s", reflect.PtrTo(r.structType), value.Type()))
	}
}

func getFieldFor(fieldType reflect.Type) (field, error) {
	switch fieldType.Kind() {
	case reflect.Int:
		return field{
			get: func(v reflect.Value) interface{} {
				return int(v.Int())
			}}, nil
	case reflect.Int8:
		return field{
			get: func(v reflect.Value) interface{} {
				return int8(v.Int())
			}}, nil
	case reflect.Int16:
		return field{
			get: func(v reflect.Value) interface{} {
				return int16(v.Int())
			}}, nil
	case reflect.Int32:
		return field{
			get: func(v reflect.Value) interface{} {
				return int32(v.Int())
			}}, nil
	case reflect.Int64:
		return field{
			get: func(v reflect.Value) interface{} {
				return int64(v.Int()) //nolint:unconvert
			}}, nil
	case reflect.Uint:
		return field{
			get: func(v reflect.Value) interface{} {
				return uint(v.Uint())
			}}, nil
	case reflect.Uint8:
		return field{
			get: func(v reflect.Value) interface{} {
				return uint8(v.Uint())
			}}, nil
	case reflect.Uint16:
		return field{
			get: func(v reflect.Value) interface{} {
				return uint16(v.Uint())
			}}, nil
	case reflect.Uint32:
		return field{
			get: func(v reflect.Value) interface{} {
				return uint32(v.Uint())
			}}, nil
	case reflect.Uint64:
		return field{
			get: func(v reflect.Value) interface{} {
				return uint64(v.Uint()) //nolint:unconvert
			}}, nil
	case reflect.Uintptr:
		return field{
			get: func(v reflect.Value) interface{} {
				return *(*uintptr)(unsafe.Pointer(v.UnsafeAddr()))
			}}, nil
	case reflect.Ptr:
		referentType := qualifiedTypeName(fieldType.Elem())
		switch referentType {
		case "go.uber.org/atomic.Bool":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Bool).Load()
				}}, nil
		case "go.uber.org/atomic.Duration":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Duration).Load()
				}}, nil
		case "go.uber.org/atomic.Error":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Error).Load()
				}}, nil
		case "go.uber.org/atomic.Float64":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Float64).Load()
				}}, nil
		case "go.uber.org/atomic.Int32":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Int32).Load()
				}}, nil
		case "go.uber.org/atomic.Int64":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Int64).Load()
				}}, nil
		case "go.uber.org/atomic.String":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.String).Load()
				}}, nil
		case "go.uber.org/atomic.Time":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Time).Load()
				}}, nil
		case "go.uber.org/atomic.Uint32":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Uint32).Load()
				}}, nil
		case "go.uber.org/atomic.Uint64":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Uint64).Load()
				}}, nil
		case "go.uber.org/atomic.Uintptr":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Uintptr).Load()
				}}, nil
		case "go.uber.org/atomic.UnsafePointer":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.UnsafePointer).Load()
				}}, nil
		case "go.uber.org/atomic.Value":
			return field{
				get: func(v reflect.Value) interface{} {
					return getPointer(v).(*atomic.Value).Load()
				}}, nil
		}

		// NOTE: if adding a type here, also update the doc comment for the Reporter type
	}

	// none of the cases above matched..
	return field{}, fmt.Errorf("Unrecognized field type %s", fieldType)
}

// getPointer returns an interface{} representing the data to which pointer v refers.
func getPointer(v reflect.Value) interface{} {
	// This is a "trick" to hide the fact that f is unexported from f.Interface,
	// which (unlike f.Int etc, above) will panic for unexported fields
	v = reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr()))
	// evaluate v
	v = v.Elem()
	// convert to interface{}
	return v.Interface()
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
