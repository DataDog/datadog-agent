// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package processmanager provides installer helpers for interpreting the Agent's
// process_manager settings when running Datadog Agent fleet installer hooks (e.g. DDOT).
// It is not related to the runtime dd-procmgr / COAT client in pkg/procmgr.
package processmanager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// yamlNestedStringKeyMap returns a string-keyed map for a YAML-decoded nested object.
// yaml.v2 stores nested maps as map[interface{}]interface{} when the parent is map[string]any,
// so a plain .(map[string]any) on cfg["process_manager"] misses valid configs.
func yamlNestedStringKeyMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[interface{}]interface{}:
		out := make(map[string]any, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = val
		}
		return out, true
	default:
		return nil, false
	}
}

// EnabledFromDatadogYAML returns whether the process manager should run for DDOT hooks,
// matching pkg/config/setup defaults: default true, overridable by DD_PROCESS_MANAGER_ENABLED, then
// process_manager.enabled in ProgramData datadog.yaml. Missing process_manager section or missing
// enabled key means true (same as BindEnvAndSetDefault("process_manager.enabled", true)).
// If datadog.yaml is missing, treat enabled as true: enableOTelCollectorConfigInDatadogYAML does
// not create the file when absent (fresh install), so this hook can run before any other step
// lays down datadog.yaml — same default as the Agent when process_manager is unset.
func EnabledFromDatadogYAML() (bool, error) {
	if v, ok := os.LookupEnv("DD_PROCESS_MANAGER_ENABLED"); ok && strings.TrimSpace(v) != "" {
		return yamlTruthy(v), nil
	}
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	data, err := os.ReadFile(ddYaml)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	return enabledFromCfgMap(cfg), nil
}

func enabledFromCfgMap(cfg map[string]any) bool {
	pm, ok := yamlNestedStringKeyMap(cfg["process_manager"])
	if !ok {
		return true
	}
	if _, has := pm["enabled"]; !has {
		return true
	}
	return yamlTruthy(pm["enabled"])
}

func yamlTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || x == "1" || strings.EqualFold(x, "yes")
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case uint64:
		return x != 0
	default:
		return false
	}
}
