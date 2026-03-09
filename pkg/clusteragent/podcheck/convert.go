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
	ADIdentifiers []string             `yaml:"ad_identifiers"`
	CELSelector   workloadfilter.Rules `yaml:"cel_selector,omitempty"`
	InitConfig    interface{}          `yaml:"init_config"`
	Instances     []interface{}        `yaml:"instances"`
	Logs          []interface{}        `yaml:"logs,omitempty"`
}

// configMapKey returns the ConfigMap data key for a DatadogPodCheck.
func configMapKey(dpc *datadoghq.DatadogPodCheck) string {
	return fmt.Sprintf("%s_%s_%s.yaml", dpc.Namespace, dpc.Name, dpc.Spec.Check.Name)
}

// convertToADConfig converts a DatadogPodCheck into an AD-compatible YAML config.
func convertToADConfig(dpc *datadoghq.DatadogPodCheck) ([]byte, error) {
	cfg := adConfig{
		ADIdentifiers: []string{dpc.Spec.ContainerImage},
	}

	// init_config
	if dpc.Spec.Check.InitConfig != nil && dpc.Spec.Check.InitConfig.Raw != nil {
		var initConfig interface{}
		if err := json.Unmarshal(dpc.Spec.Check.InitConfig.Raw, &initConfig); err != nil {
			return nil, fmt.Errorf("failed to parse initConfig: %w", err)
		}
		cfg.InitConfig = initConfig
	}

	// instances
	for i, inst := range dpc.Spec.Check.Instances {
		var instance interface{}
		if err := json.Unmarshal(inst.Raw, &instance); err != nil {
			return nil, fmt.Errorf("failed to parse instance[%d]: %w", i, err)
		}
		cfg.Instances = append(cfg.Instances, instance)
	}

	// logs
	for i, logEntry := range dpc.Spec.Logs {
		var entry interface{}
		if err := json.Unmarshal(logEntry.Raw, &entry); err != nil {
			return nil, fmt.Errorf("failed to parse logs[%d]: %w", i, err)
		}
		cfg.Logs = append(cfg.Logs, entry)
	}

	// cel_selector from matchAnnotations
	if dpc.Spec.Selector != nil && len(dpc.Spec.Selector.MatchAnnotations) > 0 {
		rule := buildAnnotationCELRules(dpc.Spec.Selector.MatchAnnotations)
		rule = append(rule, fmt.Sprintf("container.pod.namespace == '%s'", dpc.Namespace))
		combinedRules := strings.Join(rule, " && ")
		cfg.CELSelector = workloadfilter.Rules{Containers: []string{combinedRules}}
	}

	return yaml.Marshal(cfg)
}

// buildAnnotationCELRules generates CEL expressions that match pod annotations.
// Each annotation key-value pair becomes a CEL rule like: pod.annotations["key"] == "value"
func buildAnnotationCELRules(annotations map[string]string) []string {
	rules := make([]string, 0, len(annotations))
	// Sort keys for deterministic output
	keys := make([]string, 0, len(annotations))
	for k := range annotations {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		rules = append(rules, fmt.Sprintf(`container.pod.annotations["%s"] == "%s"`, k, annotations[k]))
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
