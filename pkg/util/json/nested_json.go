// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package json implements helper functions to interact with json
package json

// GetNestedValue returns the value in the map specified by the array keys,
// where each value is another depth level in the map.
// Returns nil if the map doesn't contain the nested key.
func GetNestedValue(inputMap map[string]interface{}, keys ...string) interface{} {
	val, exists := inputMap[keys[0]]
	if !exists {
		return nil
	}
	if len(keys) == 1 {
		return val
	}
	innerMap, ok := val.(map[string]interface{})
	if !ok {
		return nil
	}
	return GetNestedValue(innerMap, keys[1:]...)
}
