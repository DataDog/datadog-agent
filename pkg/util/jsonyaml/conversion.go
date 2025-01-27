// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package jsonyaml offers helpers to convert JSON to YAML and vice versa.
package jsonyaml

import (
	"encoding/json"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// YAMLToJSON converts YAML data to JSON.
func YAMLToJSON(data []byte) ([]byte, error) {
	var obj interface{}
	var err error

	if err = yaml.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	// YAML objects can have keys that are not strings, but JSON objects cannot.
	if obj, err = stringifyMapKeys(obj); err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

// JSONToYAML converts JSON data to YAML.
func JSONToYAML(data []byte) ([]byte, error) {
	var obj interface{}
	// We are using yaml.Unmarshal here (instead of json.Unmarshal) because the
	// Go JSON library doesn't try to pick the right number type (int, float,
	// etc.) when unmarshalling to interface{}, it just picks float64
	// universally. go-yaml does go through the effort of picking the right
	// number type, so we can preserve number type throughout this process.
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return yaml.Marshal(obj)
}

func stringifyMapKeys(obj interface{}) (interface{}, error) {
	switch obj := obj.(type) {

	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(obj))
		for k, v := range obj {
			var keyString string
			switch typedKey := k.(type) {

			case string:
				keyString = typedKey
			case int:
				keyString = strconv.Itoa(typedKey)
			case int64:
				keyString = strconv.FormatInt(typedKey, 10)
			case float64:
				keyString = strconv.FormatFloat(typedKey, 'f', -1, 64)
			case bool:
				keyString = strconv.FormatBool(typedKey)
			default:
				return nil, fmt.Errorf("unsupported map key of type: %T, key: %+#v, value: %+#v", k, k, v)
			}
			var err error
			m[keyString], err = stringifyMapKeys(v)
			if err != nil {
				return nil, err
			}
		}
		return m, nil

	case []interface{}:
		for i, v := range obj {
			var err error
			obj[i], err = stringifyMapKeys(v)
			if err != nil {
				return nil, err
			}
		}
	}

	return obj, nil
}
