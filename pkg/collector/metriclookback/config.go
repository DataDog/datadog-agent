// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package metriclookback contains helpers for 1Hz check metric lookback.
package metriclookback

import (
	"slices"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// ShadowIDSuffix is appended to the source check ID for lookback shadow checks.
const ShadowIDSuffix = ":shadow"

// goCheckLoaderName matches the existing core check loader name.
const goCheckLoaderName = "core"

// Options controls which source check instances get shadow configs.
type Options struct {
	Enabled    bool
	CheckNames []string
}

// ShadowConfig describes a source check instance selected for shadow execution.
type ShadowConfig struct {
	// SourceConfig preserves the full source config. Scheduler integration must
	// pass Instance and InstanceIndex to the loader instead of iterating
	// SourceConfig.Instances again.
	SourceConfig       integration.Config
	Instance           integration.Data
	InstanceIndex      int
	SourceConfigDigest string
	SourceCheckID      checkid.ID
	ShadowCheckID      checkid.ID
}

type loaderConfig struct {
	LoaderName string `yaml:"loader"`
}

// DeriveShadowConfigs selects check instances that should run in the lookback
// shadow path. The returned source configs preserve their original bytes so
// scheduler integration can reuse the normal Go/core loader path with the
// selected Instance and InstanceIndex, then map the normal check ID to the
// shadow ID at sender/scheduler boundaries.
func DeriveShadowConfigs(configs []integration.Config, opts Options) []ShadowConfig {
	shadowConfigs := []ShadowConfig{}
	for _, config := range configs {
		if !isSupportedCheckConfig(config, opts) {
			continue
		}

		initLoader := selectedInitLoader(config.InitConfig)
		for instanceIndex, instance := range config.Instances {
			instanceEnabled, hasInstanceSetting := instanceLookbackEnabled(instance)
			if !opts.Enabled && !instanceEnabled {
				continue
			}
			if opts.Enabled && hasInstanceSetting && !instanceEnabled {
				continue
			}
			if check.IsJMXInstance(config.Name, instance, config.InitConfig) {
				continue
			}
			if !isCoreLoaderSelected(initLoader, instance) {
				continue
			}

			sourceCheckID := checkid.BuildID(config.Name, config.FastDigest(), instance, config.InitConfig)
			shadowConfigs = append(shadowConfigs, ShadowConfig{
				SourceConfig:       cloneConfig(config),
				Instance:           cloneData(instance),
				InstanceIndex:      instanceIndex,
				SourceConfigDigest: config.Digest(),
				SourceCheckID:      sourceCheckID,
				ShadowCheckID:      checkid.ID(string(sourceCheckID) + ShadowIDSuffix),
			})
		}
	}

	return shadowConfigs
}

func isSupportedCheckConfig(config integration.Config, opts Options) bool {
	if !config.IsCheckConfig() || config.HasFilter(workloadfilter.MetricsFilter) {
		return false
	}
	if len(opts.CheckNames) > 0 && !slices.Contains(opts.CheckNames, config.Name) {
		return false
	}
	return true
}

func selectedInitLoader(initConfig integration.Data) string {
	var cfg loaderConfig
	if err := yaml.Unmarshal(initConfig, &cfg); err != nil {
		return ""
	}
	return cfg.LoaderName
}

func isCoreLoaderSelected(initLoader string, instance integration.Data) bool {
	selectedLoader := initLoader

	var cfg loaderConfig
	if err := yaml.Unmarshal(instance, &cfg); err == nil && cfg.LoaderName != "" {
		selectedLoader = cfg.LoaderName
	}

	return selectedLoader == "" || selectedLoader == goCheckLoaderName
}

func instanceLookbackEnabled(instance integration.Data) (enabled bool, found bool) {
	var raw integration.RawMap
	if err := yaml.Unmarshal(instance, &raw); err != nil {
		return false, false
	}

	value, found := raw["metric_lookback"]
	if !found {
		return false, false
	}

	switch typedValue := value.(type) {
	case integration.RawMap:
		enabledValue, ok := typedValue["enabled"]
		if !ok {
			return false, true
		}
		enabled, ok := enabledValue.(bool)
		return enabled && ok, true
	case map[interface{}]interface{}:
		enabledValue, ok := typedValue["enabled"]
		if !ok {
			return false, true
		}
		enabled, ok := enabledValue.(bool)
		return enabled && ok, true
	default:
		return false, true
	}
}

func cloneConfig(config integration.Config) integration.Config {
	config.Instances = slices.Clone(config.Instances)
	for i := range config.Instances {
		config.Instances[i] = cloneData(config.Instances[i])
	}
	config.InitConfig = cloneData(config.InitConfig)
	config.MetricConfig = cloneData(config.MetricConfig)
	config.LogsConfig = cloneData(config.LogsConfig)
	config.ADIdentifiers = slices.Clone(config.ADIdentifiers)
	config.AdvancedADIdentifiers = slices.Clone(config.AdvancedADIdentifiers)
	return config
}

func cloneData(data integration.Data) integration.Data {
	return append(integration.Data(nil), data...)
}
