// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package viperconfig

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

func (c *safeConfig) addToSchema(name string, val interface{}, envVars []string, noEnv bool, noDefault bool) {
	parts := strings.Split(name, ".")

	curr := c.Schema
	if len(parts) > 1 {
		for i := 0; i < len(parts)-1; i++ {
			p := curr["properties"]
			if section, ok := p.(map[string]interface{})[parts[i]]; ok {
				curr = section.(map[string]interface{})
			} else {
				newSection := map[string]interface{}{
					"type":       "object",
					"node_type":  "section",
					"properties": map[string]interface{}{},
				}
				p.(map[string]interface{})[parts[i]] = newSection
				curr = newSection
			}
		}
	}

	var node map[string]interface{}

	if noDefault {
		node = map[string]interface{}{
			"tags": []string{"TODO:fix-no-default"},
		}
	} else {
		switch v := val.(type) {
		case bool:
			node = map[string]interface{}{
				"type":    "boolean",
				"default": v,
			}
		case int:
			node = map[string]interface{}{
				"type": "number",
			}
			if !noDefault {
				node["default"] = v
			}
		case int64:
			node = map[string]interface{}{
				"type": "number",
			}
			if !noDefault {
				node["default"] = v
			}
		case time.Duration:
			node = map[string]interface{}{
				"type":   "number",
				"format": "duration",
				"tags":   []string{"golang_type:duration"},
			}
			if !noDefault {
				node["default"] = v
			}
		case float64:
			node = map[string]interface{}{
				"type": "number",
				"tags": []string{"golang_type:float64"},
			}
			if !noDefault {
				node["default"] = v
			}
		case string:
			node = map[string]interface{}{
				"type": "string",
			}
			if !noDefault {
				node["default"] = v
			}
		case []string:
			node = map[string]interface{}{
				"type":  "array",
				"items": map[string]string{"type": "string"},
			}
			if !noDefault {
				node["default"] = v
			}
		case []int:
			node = map[string]interface{}{
				"type":  "array",
				"items": map[string]string{"type": "number"},
			}
			if !noDefault {
				node["default"] = v
			}
		case []interface{}:
			node = map[string]interface{}{
				"type": "array",
			}
			if !noDefault {
				node["default"] = v
			}
		case map[string]string:
			node = map[string]interface{}{
				"type":                 "object",
				"additionalProperties": map[string]string{"type": "string"},
			}
			if !noDefault {
				node["default"] = v
			}
		case map[string][]string:
			node = map[string]interface{}{
				"type":                 "object",
				"additionalProperties": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
			}
			if !noDefault {
				node["default"] = v
			}
		case map[string]float64:
			node = map[string]interface{}{
				"type":                 "object",
				"additionalProperties": map[string]string{"type": "number"},
				"tags":                 []string{"golang_type:map[string]float64"},
			}
			if !noDefault {
				node["default"] = v
			}
		case map[string]interface{}:
			node = map[string]interface{}{
				"type": "object",
				"tags": []string{"golang_type:map[string]interface{}"},
			}
			if !noDefault {
				node["default"] = v
			}
		case []map[string]interface{}:
			node = map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
				},
			}
			if !noDefault {
				node["default"] = v
			}
		case []map[string]string:
			node = map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": map[string]string{"type": "string"},
				},
			}
			if !noDefault {
				node["default"] = v
			}
		case nil:
			node = map[string]interface{}{
				"tags": []string{"golang_type:nil", "TODO:fix-missing-type"},
			}
		default:
			fmt.Printf("Error: unknown type for %s: %v\n", name, reflect.TypeOf(val))
			return
		}
	}

	if noEnv || envVars != nil {
		if _, found := node["tags"]; !found {
			node["tags"] = []string{}
		}

		if noEnv {
			node["tags"] = append(node["tags"].([]string), "no-env")
		}
		if envVars != nil {
			node["env_vars"] = envVars
		}
	}
	curr["properties"].(map[string]interface{})[parts[len(parts)-1]] = node
}
