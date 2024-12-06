// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"reflect"
	"slices"
)

// GetAndFilterGroup filters 'zero' values from an FX group.
//
// A 'zero' value, nil in most cases, can be injected into a group when a component declares returning a element for
// that group but don't actually creates the element. This is common pattern with component that can be disabled or
// partially enabled.
//
// This should be called in every component's constructor that requires an FX group as a dependency.
func GetAndFilterGroup[S ~[]E, E any](group S) S {
	return slices.DeleteFunc(group, func(item E) bool {
		// if item is an untyped nil, aka interface{}(nil), we filter them directly
		t := reflect.TypeOf(item)
		if t == nil {
			return true
		}

		switch t.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice, reflect.Func, reflect.Interface:
			return reflect.ValueOf(item).IsNil()
		}
		return false
	})
}
