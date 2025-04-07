// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	"gopkg.in/yaml.v3"
)

// ensureMapKey ensures a map key exists with a map value and returns the nested map
func ensureMapKey(m map[string]interface{}, key string) map[string]interface{} {
	if _, ok := m[key]; !ok {
		m[key] = make(map[string]interface{})
	}
	return m[key].(map[string]interface{})
}

func enableOTELAgentonfig(valueYaml string) string {
	var config map[string]interface{}
	if valueYaml == "" {
		config = make(map[string]interface{})
	} else {
		if err := yaml.Unmarshal([]byte(valueYaml), &config); err != nil {
			return valueYaml
		}
	}

	// Initialize the agents.containers.otelAgent structure if it doesn't exist
	agents := ensureMapKey(config, "agents")
	containers := ensureMapKey(agents, "containers")
	otelAgent := ensureMapKey(containers, "otelAgent")

	// Init env if needed and set DD_OTELCOLLECTOR_ENABLED
	if _, ok := otelAgent["env"]; !ok {
		otelAgent["env"] = []interface{}{}
	}

	env := otelAgent["env"].([]interface{})
	found := false
	for _, e := range env {
		if envMap, ok := e.(map[string]interface{}); ok && envMap["name"] == "DD_OTELCOLLECTOR_ENABLED" {
			envMap["value"] = "true"
			found = true
			break
		}
	}

	if !found {
		otelAgent["env"] = append(env, map[string]interface{}{
			"name": "DD_OTELCOLLECTOR_ENABLED", "value": "true",
		})
	}

	// Return updated YAML
	modifiedYaml, err := yaml.Marshal(config)
	if err != nil {
		return valueYaml
	}
	return string(modifiedYaml)
}
