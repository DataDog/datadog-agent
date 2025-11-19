// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package viperconfig provides a viper-based implementation of the config interface.
package viperconfig

import (
	"reflect"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// GetViperCombine returns the value at this key, with all layers combined
// meant to be used by viper as a work-around for .Get(key) not working
func GetViperCombine(cfg model.Reader, key string) interface{} {
	rawval := cfg.Get(key)

	// If the setting has a scalar non-nil value, return it
	fields := cfg.GetSubfields(key)
	if !IsNilValue(rawval) && len(fields) == 0 {
		return rawval
	}

	// If the setting is a map, copy to the tree (return value)
	tree := make(map[string]interface{})
	if mapval, ok := rawval.(map[string]interface{}); ok {
		for k, v := range mapval {
			tree[k] = v
		}
	}

	// Iterate the subfields of this setting (will find env vars this way)
	for _, f := range fields {
		setting := strings.Join([]string{key, f}, ".")
		inner := GetViperCombine(cfg, setting)
		if inner == nil {
			continue
		}
		if IsNilValue(inner) {
			var empty map[string]interface{}
			inner = empty
		}
		tree[f] = inner
	}

	if len(tree) == 0 {
		// If tree is empty, return nil so the type is interface{}, not map[string]interface{}
		return nil
	}
	return tree
}

// valid kinds to call IsNil on
var nillableKinds = []reflect.Kind{reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Slice}

// IsNilValue returns true if a is nil, or a is an interface with nil data
func IsNilValue(a interface{}) bool {
	if a == nil {
		return true
	}
	rv := reflect.ValueOf(a)
	// check if IsNil may be called in order to avoid a panic
	if slices.Contains(nillableKinds, rv.Kind()) {
		return reflect.ValueOf(a).IsNil()
	}
	return false
}
