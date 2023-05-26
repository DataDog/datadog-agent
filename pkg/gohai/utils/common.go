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
var ErrNotExportable = errors.New("cannot be exported")
var ErrNotCollectable = fmt.Errorf("cannot be collected on %s %s", runtime.GOOS, runtime.GOARCH)

// fieldIsValue returns whether the field has type Value[T] for some T.
//
// # It actually checks whether the type has a function Value with correct number and types of argument and output
//
// Since we don't know the specific T, we can't case to an interface, and reflect doesn't have any way to check
// a generic type as long as https://github.com/golang/go/issues/54393 is not implemented
func fieldIsValue(fieldTy reflect.StructField) bool {
	// check that a pointer to the field has a Value method
	valueMethod, ok := reflect.PtrTo(fieldTy.Type).MethodByName("Value")
	if !ok || valueMethod.Type.NumIn() != 1 || valueMethod.Type.NumOut() != 2 {
		return false
	}

	// check that the second return value is an error
	reflErrorInterface := reflect.TypeOf((*error)(nil)).Elem()
	secondRetType := valueMethod.Type.Out(1)
	return secondRetType.Implements(reflErrorInterface)
}

// fieldIsExportable checks if a field can be exported by AsJSON.
//
// A field can be exported if it has type Value, its inner type can be rendered,
// it is exported by the struct and has a json tag.
// The function returns the json tag, the suffix tag and whether the field can be exported.
//
// The returned strings are only valid if the field is exportable.
func fieldIsExportable(fieldTy reflect.StructField) (string, string, bool) {
	// check if field has type Value
	if !fieldIsValue(fieldTy) {
		return "", "", false
	}

	// check if field is exported
	if !fieldTy.IsExported() {
		return "", "", false
	}

	// check that the T of Value[T] can be rendered
	valueMethod, _ := reflect.PtrTo(fieldTy.Type).MethodByName("Value")
	value := reflect.Zero(valueMethod.Type.Out(0))
	_, supported := renderValue(value, "")
	if !supported {
		return "", "", false
	}

	// check if field has a json tag
	fieldName, ok := fieldTy.Tag.Lookup("json")
	if !ok {
		return "", "", false
	}

	// Get returns an empty string if the key does not exists
	suffix := fieldTy.Tag.Get("suffix")

	return fieldName, suffix, true
}

// renderValue converts the given value to a string, and return a boolean indicating
// whether it succeeded
//
// Supported types are int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, string
func renderValue(value reflect.Value, suffix string) (string, bool) {
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

	return fmt.Sprintf("%s%s", rendered, suffix), true
}

// AsJSON takes an Info struct and returns a marshal-able object representing the fields of the struct,
// the lists of errors for fields for which the collection failed, and an error if it failed.
//
// If useDefault is true, fields which failed to be collected will be included in the marshal-able object
// as their default value, otherwise they are ignored.
//
// If the error is non-nil, the first two parameters are unspecified.
//
// Fields which are not exported, don't have a json tag or are not of type Value[T] for a T which can
// be rendered cause the function to return an error.
func AsJSON[T any](info *T, useDefault bool) (interface{}, []error, error) {
	reflVal := reflect.ValueOf(info).Elem()
	reflType := reflect.TypeOf(info).Elem()

	// info has to be a struct
	if reflVal.Kind() != reflect.Struct {
		return nil, nil, ErrArgNotStruct
	}

	values := make(map[string]interface{})
	warns := []error{}

	for i := 0; i < reflVal.NumField(); i++ {
		field := reflType.Field(i)
		fieldName, suffix, isExportable := fieldIsExportable(field)
		if !isExportable {
			return nil, nil, fmt.Errorf("%s: %w", field.Name, ErrNotExportable)
		}

		// Value is a method on *Value[T] so we get a pointer to the value
		fieldPtr := reflVal.Field(i).Addr()
		valueMethod, _ := fieldPtr.Type().MethodByName("Value")
		ret := valueMethod.Func.Call([]reflect.Value{fieldPtr})
		retValue := ret[0]
		if ret[1].Interface() != nil {
			err := ret[1].Interface().(error)

			if !errors.Is(err, ErrNotCollectable) {
				warns = append(warns, err)
			}

			if !useDefault {
				continue
			}

			// use the default value of the type
			retValue = reflect.Zero(retValue.Type())
		}

		renderedValue, ok := renderValue(retValue, suffix)
		if !ok {
			return nil, nil, fmt.Errorf("%s: %w", field.Name, ErrNotExportable)
		}

		values[fieldName] = renderedValue
	}

	if len(values) == 0 {
		return nil, warns, ErrNoFieldCollected
	}

	return values, warns, nil
}
