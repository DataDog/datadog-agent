// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import "fmt"

func isList(i interface{}) bool {
	_, ok := i.([]interface{})
	return ok
}

func isMap(i interface{}) bool {
	_, ok := i.(map[interface{}]interface{})
	return ok
}

func isScalar(i interface{}) bool {
	return !isList(i) && !isMap(i)
}

// merge merges two layers into a single layer
//
// The override layer takes precedence over the base layer. The values are merged as follows:
// - Scalars: the override value is used
// - Lists: the override list is used
// - Maps: the override map is recursively merged into the base map
func merge(base interface{}, override interface{}) (interface{}, error) {
	if base == nil {
		return override, nil
	}
	if override == nil {
		// this allows to override a value with nil
		return nil, nil
	}
	if isScalar(base) && isScalar(override) {
		return override, nil
	}
	if isList(base) && isList(override) {
		return override, nil
	}
	if isMap(base) && isMap(override) {
		return mergeMap(base.(map[interface{}]interface{}), override.(map[interface{}]interface{}))
	}
	return nil, fmt.Errorf("could not merge %T with %T", base, override)
}

func mergeMap(base, override map[interface{}]interface{}) (map[interface{}]interface{}, error) {
	merged := make(map[interface{}]interface{})
	for k, v := range base {
		merged[k] = v
	}
	for k := range override {
		v, err := merge(base[k], override[k])
		if err != nil {
			return nil, fmt.Errorf("could not merge key %v: %w", k, err)
		}
		merged[k] = v
	}
	return merged, nil
}
