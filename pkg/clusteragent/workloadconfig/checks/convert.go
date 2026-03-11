// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package checks

import (
	"encoding/json"
	"fmt"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/workloadconfig"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

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

// configMapKey returns the ConfigMap data key for a check within a DatadogWorkloadConfig CR.
func configMapKey(namespace, crName, checkName string) string {
	return fmt.Sprintf("%s_%s_%s.yaml", namespace, crName, checkName)
}

// convertCR converts a DatadogWorkloadConfig CR into a set of ConfigMap entries (key -> YAML).
// Each CheckConfig in the CR produces one entry.
func convertCR(dwc *datadoghq.DatadogWorkloadConfig) (map[string]string, error) {
	celRules := workloadconfig.BuildCELSelector(dwc)

	entries := make(map[string]string, len(dwc.Spec.Config.Checks))
	for i, check := range dwc.Spec.Config.Checks {
		yamlBytes, err := convertCheckToADConfig(check, celRules)
		if err != nil {
			return nil, fmt.Errorf("check[%d] %q: %w", i, check.Integration, err)
		}
		key := configMapKey(dwc.Namespace, dwc.Name, check.Integration)
		entries[key] = string(yamlBytes)
	}
	return entries, nil
}

// convertCheckToADConfig converts a single CheckConfig into AD-compatible YAML.
func convertCheckToADConfig(check datadoghq.CheckConfig, celRules workloadfilter.Rules) ([]byte, error) {
	cfg := adConfig{
		ADIdentifiers: check.ContainerImage,
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
