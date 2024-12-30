// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmxfetch

import "github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"

// GetJSONSerializableMap returns a JSON serializable map from a raw map
func GetJSONSerializableMap(m interface{}) interface{} {
	switch x := m.(type) {
	// unbelievably I cannot collapse this into the next (identical) case
	case map[interface{}]interface{}:
		j := integration.JSONMap{}
		for k, v := range x {
			j[k.(string)] = GetJSONSerializableMap(v)
		}
		return j
	case integration.RawMap:
		j := integration.JSONMap{}
		for k, v := range x {
			j[k.(string)] = GetJSONSerializableMap(v)
		}
		return j
	case integration.JSONMap:
		j := integration.JSONMap{}
		for k, v := range x {
			j[k] = GetJSONSerializableMap(v)
		}
		return j
	case []interface{}:
		j := make([]interface{}, len(x))

		for i, v := range x {
			j[i] = GetJSONSerializableMap(v)
		}
		return j
	}
	return m
}
