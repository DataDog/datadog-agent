// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

// Package utils provides various helper functions used in the library.
package utils

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
)

var (
	// ErrNoFieldCollected means no field could be collected
	ErrNoFieldCollected = errors.New("no field was collected")
	// ErrArgNotStruct means the argument should be a struct but wasn't
	ErrArgNotStruct = errors.New("argument is not a struct")
	// ErrNotCollectable means the value can't be collected on the given platform
	ErrNotCollectable = fmt.Errorf("cannot be collected on %s %s", runtime.GOOS, runtime.GOARCH)
	// ErrNotExported means the struct has an unexported field
	ErrNotExported = errors.New("field not exported by the struct")
	// ErrCannotRender means a field which cannot be rendered
	ErrCannotRender = errors.New("field inner type cannot be rendered")
	// ErrNoValueMethod means a field doesn't have a Value method
	ErrNoValueMethod = errors.New("field doesn't have the expected Value method")
	// ErrNoJSONTag means a field doesn't have a json tag
	ErrNoJSONTag = errors.New("field doesn't have a json tag")
)

// canBeRendered returns whether the given type can be converted to a string properly
func canBeRendered(ty reflect.Kind) bool {
	renderables := map[reflect.Kind]struct{}{
		reflect.Int: {}, reflect.Int8: {}, reflect.Int16: {}, reflect.Int32: {},
		reflect.Int64: {}, reflect.Uint: {}, reflect.Uint8: {}, reflect.Uint16: {},
		reflect.Uint32: {}, reflect.Uint64: {}, reflect.Uintptr: {}, reflect.Float32: {},
		reflect.Float64: {}, reflect.String: {},
	}

	_, ok := renderables[ty]
	return ok
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

// Jsonable represents a type which can be converted to a mashallable object
type Jsonable interface {
	AsJSON() (interface{}, []string, error)
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
	var lastErr error // store an error to return, so that errors.Is can be used

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
				lastErr = err
			}

			// if the field is an error and we don't want to print the default value, continue
			if !useDefault {
				continue
			}

			// use the default value of the type
			retValue = reflect.Zero(retValue.Type())
		}

		if !canBeRendered(retValue.Kind()) {
			return nil, nil, fmt.Errorf("%s: %w", fieldName, ErrCannotRender)
		}

		// Get returns an empty string if the key does not exist so no need for particular error handling
		unit := fieldTy.Tag.Get("unit")

		values[jsonName] = fmt.Sprintf("%v%s", retValue.Interface(), unit)
	}

	// return an error if no field was successfully collected
	if len(values) == 0 {
		// if all field errors are the same then use that as error message on top of the generic "no field was collected"
		if len(warns) != 0 && allEqual(warns) {
			return nil, nil, fmt.Errorf("%w: %w", ErrNoFieldCollected, lastErr)
		}

		return nil, warns, ErrNoFieldCollected
	}

	return values, warns, nil
}

func allEqual[T comparable](values []T) bool {
	if len(values) == 0 {
		return true
	}

	first := values[0]
	for _, val := range values {
		if val != first {
			return false
		}
	}

	return true
}
