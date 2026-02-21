// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package basic contains helpers related to basic types
package basic

import (
	"reflect"
	"runtime"
	"strings"
)

// TODO: Callers that are using SetInTest improperly, need to be fixed
var allowlistCaller = []string{
	// Fixing this test by updating its use of SetInTest causes other failures, needs investigation
	"comp/core/autodiscovery/listeners/snmp_test.go",

	// TestNewConfig has an expectedConfig, which has embedded structs pathteststore.Config and connfilter.Config
	"comp/networkpath/npcollector/npcollectorimpl/config_test.go",
	"comp/networkpath/npcollector/npcollectorimpl/npcollector_testutils.go",

	// TestFullConfig assigns an object usersV3, which is a list of structs
	"comp/snmptraps/config/config_test.go",
}

// ValidateBasicTypes returns true if the argument is made of only basic types
func ValidateBasicTypes(value interface{}) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	if validate(v) {
		return true
	}

	// Allow existing callers that are using SetInTest. Fix these later
	for _, stackSkip := range []int{2, 3, 4} {
		_, absfile, _, _ := runtime.Caller(stackSkip)
		for _, allowSource := range allowlistCaller {
			if strings.HasSuffix(absfile, allowSource) {
				return true
			}
		}
	}

	return false
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
