// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package viperconfig

import (
	"reflect"
)

// ValdiateBasicTypes returns true if the argument is made of only basic types
func ValidateBasicTypes(value interface{}) bool {
	v := reflect.ValueOf(value)
	return validate(v)
}

func validate(v reflect.Value) bool {
	if v.Kind() == reflect.Pointer {
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
