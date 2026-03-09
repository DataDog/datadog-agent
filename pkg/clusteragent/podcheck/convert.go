// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package podcheck

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"gopkg.in/yaml.v2"
)

// adConfig represents an autodiscovery-compatible check configuration YAML.
// The fields match the configFormat struct in comp/core/autodiscovery/providers/config_reader.go.
type adConfig struct {
	ADIdentifiers []string             `yaml:"ad_identifiers,omitempty"`
	CELSelector   workloadfilter.Rules `yaml:"cel_selector,omitempty"`
	InitConfig    interface{}          `yaml:"init_config"`
	Instances     []interface{}        `yaml:"instances"`
	Logs          interface{}          `yaml:"logs,omitempty"`
}

// configMapKey returns the ConfigMap data key for a check within a DatadogPodCheck CR.
func configMapKey(namespace, crName, checkName string) string {
	return fmt.Sprintf("%s_%s_%s.yaml", namespace, crName, checkName)
}

// convertCR converts a DatadogPodCheck CR into a set of ConfigMap entries (key -> YAML).
// Each CheckConfig in the CR produces one entry.
func convertCR(dpc *datadoghq.DatadogPodCheck) (map[string]string, error) {
	celRules := buildCELSelector(dpc)

	entries := make(map[string]string, len(dpc.Spec.Checks))
	for i, check := range dpc.Spec.Checks {
		yamlBytes, err := convertCheckToADConfig(check, celRules)
		if err != nil {
			return nil, fmt.Errorf("check[%d] %q: %w", i, check.Name, err)
		}
		key := configMapKey(dpc.Namespace, dpc.Name, check.Name)
		entries[key] = string(yamlBytes)
	}
	return entries, nil
}

// convertCheckToADConfig converts a single CheckConfig into AD-compatible YAML.
func convertCheckToADConfig(check datadoghq.CheckConfig, celRules workloadfilter.Rules) ([]byte, error) {
	cfg := adConfig{
		ADIdentifiers: check.ADIdentifiers,
		CELSelector:   celRules,
	}

	if check.InitConfig != nil && check.InitConfig.Raw != nil {
		var initConfig interface{}
		if err := json.Unmarshal(check.InitConfig.Raw, &initConfig); err != nil {
			return nil, fmt.Errorf("failed to parse initConfig: %w", err)
		}
		cfg.InitConfig = initConfig
	}

	for i, inst := range check.Instances {
		var instance interface{}
		if err := json.Unmarshal(inst.Raw, &instance); err != nil {
			return nil, fmt.Errorf("failed to parse instance[%d]: %w", i, err)
		}
		cfg.Instances = append(cfg.Instances, instance)
	}

	if check.Logs != nil && check.Logs.Raw != nil {
		var logs interface{}
		if err := json.Unmarshal(check.Logs.Raw, &logs); err != nil {
			return nil, fmt.Errorf("failed to parse logs: %w", err)
		}
		cfg.Logs = logs
	}

	return yaml.Marshal(cfg)
}

// buildCELSelector builds a CEL selector from a DatadogPodCheck's Selector and namespace.
func buildCELSelector(dpc *datadoghq.DatadogPodCheck) workloadfilter.Rules {
	hasAnnotations := len(dpc.Spec.Selector.MatchAnnotations) > 0
	hasLabels := len(dpc.Spec.Selector.MatchLabels) > 0
	var rules []string
	if hasAnnotations {
		rules = append(rules, buildMapCELRules("container.pod.annotations", dpc.Spec.Selector.MatchAnnotations)...)
	}
	if hasLabels {
		rules = append(rules, buildMapCELRules("container.pod.labels", dpc.Spec.Selector.MatchLabels)...)
	}
	rules = append(rules, fmt.Sprintf("container.pod.namespace == '%s'", dpc.Namespace))
	combinedRules := strings.Join(rules, " && ")
	return workloadfilter.Rules{Containers: []string{combinedRules}}
}

// buildMapCELRules generates CEL equality expressions for a map field.
// Each key-value pair becomes: fieldPath["key"] == "value".
// Keys are sorted for deterministic output.
func buildMapCELRules(fieldPath string, m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rules := make([]string, 0, len(m))
	for _, k := range keys {
		rules = append(rules, fmt.Sprintf(`%s["%s"] == "%s"`, fieldPath, k, m[k]))
	}
	return rules
}

// unstructuredToPodCheck converts an unstructured object to a DatadogPodCheck.
func unstructuredToPodCheck(obj interface{}) (*datadoghq.DatadogPodCheck, error) {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("could not cast to Unstructured: %T", obj)
	}
	dpc := &datadoghq.DatadogPodCheck{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), dpc); err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to DatadogPodCheck: %w", err)
	}
	return dpc, nil
}
