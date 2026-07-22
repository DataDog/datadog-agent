// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"slices"
	"time"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	enabledConfigKey       = "metric_lookback.enabled"
	enabledChecksConfigKey = "metric_lookback.enabled_checks"
	collectionIntervalKey  = "metric_lookback.collection_interval"
	instanceConfigKey      = "metric_lookback"
	instanceEnabledKey     = "enabled"

	// defaultShadowCheckInterval is the collection interval for selected metric
	// lookback shadow checks and the minimum recurring interval accepted by the
	// scheduler.
	defaultShadowCheckInterval = time.Second
)

// ShadowPolicyOptions controls which source check instances get shadow candidates.
type ShadowPolicyOptions struct {
	ShadowChecksEnabled bool
	ChecksToShadow      []string
	ShadowInterval      time.Duration
}

// ShadowPolicyOptionsFromConfig reads metric lookback policy options from Agent config.
func ShadowPolicyOptionsFromConfig(cfg model.Reader) ShadowPolicyOptions {
	return ShadowPolicyOptions{
		ShadowChecksEnabled: cfg.GetBool(enabledConfigKey),
		ChecksToShadow:      cfg.GetStringSlice(enabledChecksConfigKey),
		ShadowInterval:      normalizeShadowInterval(cfg.GetDuration(collectionIntervalKey)),
	}
}

// ShadowCandidate describes a source check instance selected for shadow execution.
type ShadowCandidate struct {
	SourceConfig       integration.Config
	Instance           integration.Data
	InstanceIndex      int
	SourceConfigDigest string
	ShadowInterval     time.Duration
}

// SelectShadowCandidates returns copied shadow candidates for selected config instances.
// Loading, scheduling, and execution routing are handled by the caller.
func SelectShadowCandidates(configs []integration.Config, opts ShadowPolicyOptions) []ShadowCandidate {
	candidates := []ShadowCandidate{}
	for _, config := range configs {
		if !isSupportedCheckConfig(config, opts) {
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

			shadowInstance, err := WithShadowExecutionMode(instance)
			if err != nil {
				continue
			}
			sourceConfig := cloneConfig(config)
			sourceConfig.LogsConfig = nil
			candidates = append(candidates, ShadowCandidate{
				SourceConfig:       sourceConfig,
				Instance:           shadowInstance,
				InstanceIndex:      instanceIndex,
				SourceConfigDigest: config.Digest(),
				ShadowInterval:     opts.ShadowInterval,
			})
		}
	}
	return candidates
}

func normalizeShadowInterval(interval time.Duration) time.Duration {
	// The default is also the scheduler's minimum recurring check interval.
	if interval < defaultShadowCheckInterval {
		return defaultShadowCheckInterval
	}
	return interval
}

func isSupportedCheckConfig(config integration.Config, opts ShadowPolicyOptions) bool {
	if !config.IsCheckConfig() || config.HasFilter(workloadfilter.MetricsFilter) {
		return false
	}
	if len(opts.ChecksToShadow) > 0 && !slices.Contains(opts.ChecksToShadow, config.Name) {
		// ChecksToShadow is a config-level allowlist and intentionally gates
		// per-instance opt-in.
		return false
	}
	return true
}

func instanceLookbackEnabled(instance integration.Data) (enabled bool, validOverride bool) {
	var raw integration.RawMap
	if err := yaml.Unmarshal(instance, &raw); err != nil {
		return false, false
	}

	value, found := raw[instanceConfigKey]
	if !found {
		return false, false
	}

	switch typedValue := value.(type) {
	case integration.RawMap:
		enabledValue, ok := typedValue[instanceEnabledKey]
		return boolSetting(enabledValue, ok)
	case map[interface{}]interface{}:
		enabledValue, ok := typedValue[instanceEnabledKey]
		return boolSetting(enabledValue, ok)
	default:
		return false, false
	}
}

func boolSetting(value interface{}, found bool) (bool, bool) {
	if !found {
		return false, false
	}
	enabled, ok := value.(bool)
	return enabled, ok
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
	config.CELSelector = cloneCELSelector(config.CELSelector)
	return config
}

func cloneCELSelector(selector workloadfilter.Rules) workloadfilter.Rules {
	selector.Containers = slices.Clone(selector.Containers)
	selector.Processes = slices.Clone(selector.Processes)
	selector.Pods = slices.Clone(selector.Pods)
	selector.KubeServices = slices.Clone(selector.KubeServices)
	selector.KubeEndpoints = slices.Clone(selector.KubeEndpoints)
	return selector
}
