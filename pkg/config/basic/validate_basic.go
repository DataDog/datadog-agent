// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package basic contains helpers related to basic types
package basic

import (
	"fmt"
	"reflect"
)

// StructToMap recursively converts a struct to map[string]interface{} using
// mapstructure tags for keys (falling back to field name). This produces only
// basic types that pass ValidateBasicTypes. Zero-value fields are omitted.
func StructToMap(v interface{}) interface{} {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		m := make(map[string]interface{})
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if !f.IsExported() {
				continue
			}
			fv := rv.Field(i)
			if fv.IsZero() {
				continue
			}
			key := f.Tag.Get("mapstructure")
			if key == "" || key == "-" {
				key = f.Name
			}
			m[key] = StructToMap(fv.Interface())
		}
		return m
	case reflect.Slice:
		s := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			s[i] = StructToMap(rv.Index(i).Interface())
		}
		return s
	case reflect.Map:
		m := make(map[string]interface{})
		iter := rv.MapRange()
		for iter.Next() {
			m[fmt.Sprintf("%v", iter.Key().Interface())] = StructToMap(iter.Value().Interface())
		}
		return m
	default:
		return v
	}
}

// ValidateBasicTypes returns true if the argument is made of only basic types
func ValidateBasicTypes(value interface{}) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return validate(v)
}

func validate(v reflect.Value) bool {
	if v.Interface() == nil {
		return true
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() == reflect.Interface {
		// Handle appearances of `interface`` in `[]interface{}`, `map[string]interface{}`, etc
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	case reflect.Struct:
		return false
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !validate(v.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			if !validate(key) {
				return false
			}
			if !validate(iter.Value()) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
