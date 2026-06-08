// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metriclookback contains helpers for 1Hz check metric lookback.
package metriclookback

import (
	"slices"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/checkloader"
)

// ShadowIDSuffix is appended to the source check ID for lookback shadow checks.
const ShadowIDSuffix = ":shadow"

// goCheckLoaderName matches the existing core check loader name.
const goCheckLoaderName = "core"

// Options controls which source check instances get shadow configs.
type Options struct {
	ShadowChecksEnabled bool
	ChecksToShadow      []string
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
	// SourceCheckID lets later scheduler/sender stages bind shadow work back
	// to the original check ID while reporting with ShadowCheckID.
	SourceCheckID checkid.ID
	ShadowCheckID checkid.ID
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

		initLoader, err := checkloader.InitConfigLoader(config.InitConfig)
		if err != nil {
			continue
		}
		for instanceIndex, instance := range config.Instances {
			instanceEnabled, hasInstanceSetting := instanceLookbackEnabled(instance)
			if !opts.ShadowChecksEnabled && !instanceEnabled {
				continue
			}
			if opts.ShadowChecksEnabled && hasInstanceSetting && !instanceEnabled {
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
	if len(opts.ChecksToShadow) > 0 && !slices.Contains(opts.ChecksToShadow, config.Name) {
		return false
	}
	return true
}

func isCoreLoaderSelected(initLoader string, instance integration.Data) bool {
	selectedLoader, err := checkloader.SelectedInstanceLoader(initLoader, instance)
	if err != nil {
		return false
	}

	// V1 only supports Go/core shadow checks. An empty loader means the normal
	// scheduler tries default loaders in priority order, so do not infer core
	// here; Python may win before core.
	return selectedLoader == goCheckLoaderName
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
			return false, false
		}
		enabled, ok := enabledValue.(bool)
		return enabled && ok, true
	case map[interface{}]interface{}:
		enabledValue, ok := typedValue["enabled"]
		if !ok {
			return false, false
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
