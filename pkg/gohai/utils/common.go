// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
)

var ErrNoFieldCollected = errors.New("no field was collected")
var ErrArgNotStruct = errors.New("argument is not a struct")
var ErrNotCollectable = fmt.Errorf("cannot be collected on %s %s", runtime.GOOS, runtime.GOARCH)
var ErrNotExported = errors.New("field not exported by the struct")
var ErrCannotRender = errors.New("field inner type cannot be rendered")
var ErrNoValueMethod = errors.New("field doesn't have the expected Value method")
var ErrNoJSONTag = errors.New("field doesn't have a json tag")

// reflectValueToString converts the given value to a string, and returns a boolean indicating
// whether it succeeded
//
// Supported types are int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, string
func reflectValueToString(value reflect.Value) (string, bool) {
	var rendered string
	switch value.Kind() {
	// case reflect.Bool:
	case reflect.Int:
		rendered = fmt.Sprintf("%d", value.Interface().(int))
	case reflect.Int8:
		rendered = fmt.Sprintf("%d", value.Interface().(int8))
	case reflect.Int16:
		rendered = fmt.Sprintf("%d", value.Interface().(int16))
	case reflect.Int32:
		rendered = fmt.Sprintf("%d", value.Interface().(int32))
	case reflect.Int64:
		rendered = fmt.Sprintf("%d", value.Interface().(int64))
	case reflect.Uint:
		rendered = fmt.Sprintf("%d", value.Interface().(uint))
	case reflect.Uint8:
		rendered = fmt.Sprintf("%d", value.Interface().(uint8))
	case reflect.Uint16:
		rendered = fmt.Sprintf("%d", value.Interface().(uint16))
	case reflect.Uint32:
		rendered = fmt.Sprintf("%d", value.Interface().(uint32))
	case reflect.Uint64:
		rendered = fmt.Sprintf("%d", value.Interface().(uint64))
	// case reflect.Uintptr:
	case reflect.Float32:
		rendered = fmt.Sprintf("%f", value.Interface().(float32))
	case reflect.Float64:
		rendered = fmt.Sprintf("%f", value.Interface().(float64))
	// case reflect.Complex64:
	// case reflect.Complex128:
	// case reflect.Array:
	// case reflect.Chan:
	// case reflect.Func:
	// case reflect.Interface:
	// case reflect.Map:
	// case reflect.Pointer:
	// case reflect.Slice:
	case reflect.String:
		rendered = value.String()
	// case reflect.Struct:
	// case reflect.UnsafePointer:
	default:
		return "", false
	}

	return rendered, true
}

// getValueMethod returns whether the field has a Value function with the correct arguments and return types.
//
// Since we don't know the specific T, we can't cast to an interface, and reflect doesn't have any way to check
// a generic type as long as https://github.com/golang/go/issues/54393 is not implemented.
func getValueMethod(fieldTy reflect.StructField) (reflect.Method, bool) {
	// check that a pointer to the field type has a Value method
	// (Value is a method on *Value[T])
	valueMethod, ok := reflect.PtrTo(fieldTy.Type).MethodByName("Value")
	if !ok || valueMethod.Type.NumIn() != 1 || valueMethod.Type.NumOut() != 2 {
		return reflect.Method{}, false
	}

	// check that the second return value is an error
	reflErrorInterface := reflect.TypeOf((*error)(nil)).Elem()
	secondRetType := valueMethod.Type.Out(1)
	if !secondRetType.Implements(reflErrorInterface) {
		return reflect.Method{}, false
	}

	return valueMethod, true
}

// AsJSON takes a structure and returns a marshal-able object representing the fields of the struct,
// the lists of errors for fields for which the collection failed, and an error if it failed.
//
// If useDefault is true, fields which failed to be collected will be included in the marshal-able object
// as their default value, otherwise they are ignored.
//
// Fields which are not exported, don't have a json tag or are not of type Value[T] for a T which can
// be rendered cause the function to return an error.
//
// The string list contain errors of fields which failed to be collected. Fields which are not
// collected on the platform are ignored.
//
// If the error is non-nil, the first two parameters are unspecified.
func AsJSON[T any](info *T, useDefault bool) (interface{}, []string, error) {
	reflVal := reflect.ValueOf(info).Elem()
	reflType := reflect.TypeOf(info).Elem()

	// info has to be a struct
	if reflVal.Kind() != reflect.Struct {
		return nil, nil, ErrArgNotStruct
	}

	values := make(map[string]interface{})
	warns := []string{}

	for i := 0; i < reflVal.NumField(); i++ {
		fieldTy := reflType.Field(i)
		fieldName := fieldTy.Name

		// check if field is exported
		if !fieldTy.IsExported() {
			return nil, nil, fmt.Errorf("%s: %w", fieldName, ErrNotExported)
		}

		// check if field has a json tag
		jsonName, ok := fieldTy.Tag.Lookup("json")
		if !ok {
			return nil, nil, fmt.Errorf("%s: %w", fieldName, ErrNoJSONTag)
		}

		// check that the field has the expected Value method
		valueMethod, ok := getValueMethod(fieldTy)
		if !ok {
			return nil, nil, fmt.Errorf("%s: %w", fieldName, ErrNoValueMethod)
		}

		ret := valueMethod.Func.Call([]reflect.Value{reflVal.Field(i).Addr()})
		retValue := ret[0]
		if ret[1].Interface() != nil {
			// get the error returned by Value
			err := ret[1].Interface().(error)

			// we want errors for fields which failed to collect
			// ErrNotCollectable means that the field is not implemented on the current platform
			// ignore these errors
			if !errors.Is(err, ErrNotCollectable) {
				warns = append(warns, err.Error())
			}

			// if the field is an error and we don't want to print the default value, continue
			if !useDefault {
				continue
			}

			// use the default value of the type
			retValue = reflect.Zero(retValue.Type())
		}

		// try to render the value
		renderedValue, ok := reflectValueToString(retValue)
		if !ok {
			return nil, nil, fmt.Errorf("%s: %w", fieldName, ErrCannotRender)
		}

		// Get returns an empty string if the key does not exist so no need for particular error handling
		unit := fieldTy.Tag.Get("unit")

		values[jsonName] = fmt.Sprintf("%s%s", renderedValue, unit)
	}

	// return an error if no field was successfully collected
	if len(values) == 0 {
		return nil, warns, ErrNoFieldCollected
	}

	return values, warns, nil
}
