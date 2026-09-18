// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
)

// ValidateRoutingValues rejects unresolved route inputs before image delivery,
// secrets, or chart mutations. Opaque Pod overrides cannot certify routing.
func ValidateRoutingValues(values map[string]interface{}) error {
	var walk func(map[string]interface{}, string) error
	walk = func(m map[string]interface{}, path string) error {
		for key, value := range m {
			p := path + "." + key
			switch key {
			case "apiKey", "apiKeyExistingSecret", "appKey", "appKeyExistingSecret", "site", "dd_url", "skipSslValidation", "envFrom", "volumes", "volumeMounts", "extraContainers", "initContainers", "command", "args", "image", "targetSystem", "fips", "multiRegionFailover":
				return fmt.Errorf("%s: destination, credential or opaque Pod override conflicts with agent.receiver", p)
			case "env":
				entries, ok := value.([]interface{})
				if !ok {
					return fmt.Errorf("%s: expected env list", p)
				}
				seen := map[string]bool{}
				for _, entry := range entries {
					e, ok := entry.(map[string]interface{})
					if !ok {
						return fmt.Errorf("%s: expected env object", p)
					}
					name, ok := e["name"].(string)
					if !ok || name == "" || seen[name] {
						return fmt.Errorf("%s: missing or duplicate env name", p)
					}
					seen[name] = true
					v := e["value"]
					if v == "false" {
						v = false
					}
					if receivers.Owns(receivers.EnvKey(name)) || receivers.Unsupported(receivers.EnvKey(name), v) {
						return fmt.Errorf("%s.%s: owned or unsupported receiver setting", p, name)
					}
				}
			case "envDict":
				entries, ok := value.(map[string]interface{})
				if !ok {
					return fmt.Errorf("%s: expected env mapping", p)
				}
				for name, v := range entries {
					if receivers.Owns(receivers.EnvKey(name)) || receivers.Unsupported(receivers.EnvKey(name), v) {
						return fmt.Errorf("%s.%s: owned or unsupported receiver setting", p, name)
					}
				}
			case "customAgentConfig", "customConfig":
				raw, err := yaml.Marshal(value)
				if err != nil {
					return fmt.Errorf("%s: invalid custom config", p)
				}
				if text, ok := value.(string); ok {
					raw = []byte(text)
				}
				if _, err := receivers.ValidateConfig(string(raw)); err != nil {
					return fmt.Errorf("%s: %w", p, err)
				}
			case "remoteConfiguration":
				return fmt.Errorf("%s: owned by agent.receiver", p)
			case "metricsProvider", "autoscaling", "securityAgent", "networkMonitoring", "serviceMonitoring", "networkDevices", "otelCollector", "otlp", "processDiscovery", "hostProfiler", "privateActionRunner", "kubernetesActions", "agentDataPlane", "operator", "csi":
				if value != false {
					return fmt.Errorf("%s: feature unsupported by explicit receiver routing", p)
				}
			default:
				if nested, ok := value.(map[string]interface{}); ok {
					if err := walk(nested, p); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	return walk(values, "values")
}

// ApplyRoutingValues routes node Agent, DCA and cluster-checks runners from the
// same source-named settings. It does not turn on collection. Native credentials
// are transported in the chart's Kubernetes Secret, not repeated into env lists.
func ApplyRoutingValues(values map[string]interface{}, p receivers.Plan) error {
	if err := p.Validate(); err != nil {
		return err
	}
	settings := p.Settings()
	dd := mapAt(values, "datadog")
	if p.Endpoint != "" {
		dd["dd_url"] = p.Endpoint
	} else {
		dd["site"] = p.Site
	}
	values["remoteConfiguration"] = map[string]interface{}{"enabled": p.RemoteConfig == "native"}
	dd["remoteConfiguration"] = map[string]interface{}{"enabled": p.RemoteConfig == "native"}
	delete(settings, "dd_url")
	delete(settings, "site")
	delete(settings, "remote_configuration.enabled")
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var env []interface{}
	// The pinned chart emits RC per role; do not duplicate its env entries.
	for _, k := range keys {
		env = append(env, map[string]interface{}{"name": receivers.EnvName(k), "value": fmt.Sprint(settings[k])})
	}
	for _, role := range []string{"datadog", "clusterAgent", "clusterChecksRunner"} {
		m := mapAt(values, role)
		merged, err := MergeEnv(m["env"], env)
		if err != nil {
			return err
		}
		m["env"] = merged
	}
	return nil
}
func mapAt(values map[string]interface{}, key string) map[string]interface{} {
	m, ok := values[key].(map[string]interface{})
	if !ok {
		m = map[string]interface{}{}
		values[key] = m
	}
	return m
}

// MergeEnv merges by name, retaining unrelated entries, never whole-list
// replacement. Duplicate input names are rejected rather than shadowed.
func MergeEnv(base, overlay interface{}) ([]interface{}, error) {
	var out []interface{}
	index := map[string]int{}
	for _, list := range []interface{}{base, overlay} {
		if list == nil {
			continue
		}
		entries, ok := list.([]interface{})
		if !ok {
			return nil, fmt.Errorf("env must be a list")
		}
		seen := map[string]bool{}
		for _, item := range entries {
			entry, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("env entry must be an object")
			}
			name, ok := entry["name"].(string)
			if !ok || strings.TrimSpace(name) == "" || seen[name] {
				return nil, fmt.Errorf("env contains missing or duplicate name")
			}
			seen[name] = true
			if i, ok := index[name]; ok {
				out[i] = item
			} else {
				index[name] = len(out)
				out = append(out, item)
			}
		}
	}
	return out, nil
}
