// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unsafe"

	"go.uber.org/atomic"
)

// Reporter reports stats, extracting values fron a struct and providing them
// in a map from its Report method.
type Reporter struct {
	// stats maps field names to the struct index containing their value
	stats map[string]int

	// value is the struct to be reported on
	value reflect.Value
}

const statsTag = "stats"

// NewReporter create a new Reporter.  Pass a struct containing fields tagged
// with `stats`.  Fields pointing to types in the `go.uber.org/atomic` package
// will be read atomically automatically.
func NewReporter(v interface{}) (Reporter, error) {
	r := Reporter{
		stats: map[string]int{},
		value: reflect.ValueOf(v).Elem(),
	}

	tv := r.value.Type()
	for f := 0; f < tv.NumField(); f++ {
		field := tv.Field(f)
		if _, ok := field.Tag.Lookup(statsTag); ok {
			if !isTypeSupported(field) {
				return Reporter{}, fmt.Errorf("unsupported type %+v for field %s", field.Type.Kind(), field.Name)
			}

			r.stats[toSnakeCase(field.Name)] = f
		}
	}

	return r, nil
}

func qualifiedTypeName(ty reflect.Type) string {
	return ty.PkgPath() + "." + ty.Name()
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

// Report reports stats from the stats object
func (r Reporter) Report() map[string]interface{} {
	stats := make(map[string]interface{}, len(r.stats))
	for name, idx := range r.stats {
		var v interface{}
		f := r.value.Field(idx)

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

// from https://gist.github.com/stoewer/fbe273b711e6a06315d19552dd4d33e6
var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
