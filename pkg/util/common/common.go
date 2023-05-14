// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"reflect"
	"strings"
)

// StringSet represents a list of uniq strings
type StringSet map[string]struct{}

// NewStringSet returns as new StringSet initialized with initItems
func NewStringSet(initItems ...string) StringSet {
	newSet := StringSet{}
	for _, item := range initItems {
		newSet.Add(item)
	}
	return newSet
}

// Add adds an item to the set
func (s StringSet) Add(item string) {
	s[item] = struct{}{}
}

// GetAll returns all the strings from the set
func (s StringSet) GetAll() []string {
	res := []string{}
	for item := range s {
		res = append(res, item)
	}
	return res
}

// StructToMap converts a struct to a map[string]interface{} based on `json` annotations defaulting to field names
func StructToMap(obj interface{}) map[string]interface{} {
	rt, rv := reflect.TypeOf(obj), reflect.ValueOf(obj)
	if rt != nil && rt.Kind() != reflect.Struct {
		return make(map[string]interface{}, 0)
	}

	out := make(map[string]interface{}, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)

		// Unexported fields, access not allowed
		if field.PkgPath != "" {
			continue
		}

		var fieldName string
		if tagVal, ok := field.Tag.Lookup("json"); ok {
			// Honor the special "-" in json attribute
			if strings.HasPrefix(tagVal, "-") {
				continue
			}
			fieldName = tagVal
		} else {
			fieldName = field.Name
		}

		val := valueToInterface(rv.Field(i))
		if val != nil {
			out[fieldName] = val
		}
	}

	return out
}

func valueToInterface(value reflect.Value) interface{} {
	if !value.IsValid() {
		return nil
	}

	switch value.Type().Kind() {
	case reflect.Struct:
		return StructToMap(value.Interface())

	case reflect.Ptr:
		if !value.IsNil() {
			return valueToInterface(value.Elem())
		}

	case reflect.Array:
	case reflect.Slice:
		arr := make([]interface{}, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			val := valueToInterface(value.Index(i))
			if val != nil {
				arr = append(arr, val)
			}
		}
		return arr

	case reflect.Map:
		m := make(map[string]interface{}, value.Len())
		for _, k := range value.MapKeys() {
			v := value.MapIndex(k)
			m[k.String()] = valueToInterface(v)
		}
		return m

	default:
		return value.Interface()
	}

	return nil
}
