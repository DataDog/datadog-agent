// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
)

var ErrNoFieldCollected = errors.New("no field was collected")
var ErrArgNotStruct = errors.New("argument is not a struct")

// NotCollectedError represents a value which is not collected on a specific OS / Arch.
type NotCollectedError struct {
	PkgName   string
	ValueName string
}

// NewNotCollectedError returns a new NotCollectedError
func NewNotCollectedError(pkgName, valueName string) *NotCollectedError {
	return &NotCollectedError{
		pkgName, valueName,
	}
}

func (err *NotCollectedError) Error() string {
	var valuePath string
	if err.PkgName == "" {
		valuePath = err.ValueName
	} else {
		valuePath = fmt.Sprintf("%s.%s", err.PkgName, err.ValueName)
	}
	return fmt.Sprintf("%s is not collected on %s %s", valuePath, runtime.GOOS, runtime.GOARCH)
}

// fieldIsValue returns whether the field has type Value[T] for some T.
//
// It actually checks whether the type has the correct name, and has functions Value and NewErrorValue
// with correct number and types of argument and output
func fieldIsValue(fieldTy reflect.StructField) bool {
	// check that the type is Value[T] for some T
	// cannot really do better than checking the name as long as
	// https://github.com/golang/go/issues/54393 is not implemented
	if !strings.HasPrefix(fieldTy.Type.Name(), "Value[") {
		return false
	}

	// check that a pointer to the field has a Value method
	valueMethod, ok := reflect.PtrTo(fieldTy.Type).MethodByName("Value")
	if !ok || valueMethod.Type.NumIn() != 1 || valueMethod.Type.NumOut() != 2 {
		return false
	}

	// check that the second return value is not an error
	reflErrorInterface := reflect.TypeOf((*error)(nil)).Elem()
	secondRetType := valueMethod.Type.Out(1)
	if secondRetType.Kind() != reflect.Interface || !secondRetType.Implements(reflErrorInterface) {
		return false
	}

	// check that the type has a NewErrorValue method
	newErrorValueMethod, ok := fieldTy.Type.MethodByName("NewErrorValue")
	if !ok || newErrorValueMethod.Type.NumIn() != 2 || newErrorValueMethod.Type.NumOut() != 1 {
		return false
	}

	// check that the returned value has the same type as the field
	if newErrorValueMethod.Type.Out(0) != fieldTy.Type {
		return false
	}

	return true
}

// fieldIsExportable checks if a field can be exported by AsJSON.
//
// A field can be exported if it has type Value, it is exported by the struct and has a json tag.
// The function the json tag, the suffix tag and whether the field can be exported.
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
// the lists of errors for fields which could not be collected, and an error if it failed or if no
// information was collected in the Info struct.
//
// If useDefault is true, fields which failed to be collected will be included in the marshal-able object
// as their default value, otherwise they are ignored.
//
// If the error is non-nil, the first two parameters are unspecified.
//
// Fields which are not exported, don't have a json tag or are not of type Value[T] are ignored.
func AsJSON[T any](info *T, useDefault bool) (interface{}, []error, error) {
	reflVal := reflect.ValueOf(info).Elem()
	reflType := reflect.TypeOf(info).Elem()

	// info has to be a struct
	if reflVal.Kind() != reflect.Struct {
		return nil, nil, ErrArgNotStruct
	}

	values := make(map[string]interface{})
	errors := []error{}

	for i := 0; i < reflVal.NumField(); i++ {
		fieldName, suffix, isExportable := fieldIsExportable(reflType.Field(i))
		if !isExportable {
			continue
		}

		// Value is a method on *Value[T] so we get a pointer to the value
		fieldPtr := reflVal.Field(i).Addr()
		valueMethod, _ := fieldPtr.Type().MethodByName("Value")
		ret := valueMethod.Func.Call([]reflect.Value{fieldPtr})
		retValue := ret[0]
		if ret[1].Interface() != nil {
			err := ret[1].Interface().(error)
			errors = append(errors, err)
			if !useDefault {
				continue
			}

			// use the default value of the type
			retValue = reflect.Zero(retValue.Type())
		}

		renderedValue, ok := renderValue(retValue, suffix)
		if !ok {
			continue
		}

		values[fieldName] = renderedValue
	}

	if len(values) == 0 {
		return nil, errors, ErrNoFieldCollected
	}

	return values, errors, nil
}

// GetPkgName returns the package name from the given package path
func GetPkgName(pkgPath string) string {
	fmt.Fprintln(os.Stderr, pkgPath)
	pkg := strings.Split(pkgPath, "/")
	if len(pkg) == 0 {
		return pkgPath
	}
	return pkg[len(pkg)-1]
}

// Initialize sets the value of exportable fields to a NotCollectedError.
//
// A field is exportable if it has type Value, has a json tag and is exported by the struct.
func Initialize[T any](info *T) error {
	reflVal := reflect.ValueOf(info).Elem()
	reflType := reflect.TypeOf(info).Elem()
	pkgName := GetPkgName(reflType.PkgPath())

	// info has to be a struct
	if reflVal.Kind() != reflect.Struct {
		return ErrArgNotStruct
	}

	for i := 0; i < reflVal.NumField(); i++ {
		fieldName, _, isExportable := fieldIsExportable(reflType.Field(i))
		if !isExportable {
			continue
		}

		err := reflect.ValueOf(NewNotCollectedError(pkgName, fieldName))
		fieldVal := reflVal.Field(i)
		newErrorValueMethod, _ := fieldVal.Type().MethodByName("NewErrorValue")
		ret := newErrorValueMethod.Func.Call([]reflect.Value{fieldVal, err})

		fieldVal.Set(ret[0])
	}

	return nil
}
