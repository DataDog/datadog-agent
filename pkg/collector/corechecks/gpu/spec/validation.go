// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package spec

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// GPUConfig identifies an architecture + device mode pair from the spec.
type GPUConfig struct {
	Architecture string     `json:"architecture"`
	DeviceMode   DeviceMode `json:"device_mode"`
}

// ValidationOptions controls which spec failures should be enforced.
type ValidationOptions struct {
	WorkloadActive bool `json:"workload_active"`
}

// Equals checks if two GPU configs are equal.
func (c *GPUConfig) Equals(other GPUConfig) bool {
	return c.Architecture == other.Architecture && c.DeviceMode == other.DeviceMode
}

// TagFilter returns the Datadog tag filter expression for a GPU config.
func (c *GPUConfig) TagFilter() string {
	// kube_cluster_name:* is required to exclude hosts that are not part of the standard clusters (e.g., workspaces)
	parts := []string{"kube_cluster_name:*", "gpu_architecture:" + c.Architecture}
	switch c.DeviceMode {
	case DeviceModeMIG:
		parts = append(parts, "gpu_slicing_mode:mig")
	case DeviceModeVGPU:
		parts = append(parts, "gpu_virtualization_mode:*vgpu")
	default:
		parts = append(parts, "NOT gpu_virtualization_mode:*vgpu", "NOT gpu_slicing_mode:mig")
	}

	return strings.Join(parts, " AND ")
}

func NewGPUConfigFromTags(architecture, slicingMode, virtualizationMode string) GPUConfig {
	deviceMode := DeviceModePhysical
	if slicingMode == "mig" {
		deviceMode = DeviceModeMIG
	} else if virtualizationMode == "vgpu" {
		deviceMode = DeviceModeVGPU
	}
	return GPUConfig{Architecture: architecture, DeviceMode: deviceMode}
}

// MetricObservation is the normalized observation used by shared validation.
type MetricObservation struct {
	Name  string
	Tags  []string
	Value *float64
}

const maxInvalidValueSamplesPerMetric = 5

type MetricStatus struct {
	Missing             int                    `json:"missing"`
	Unknown             int                    `json:"unknown"`
	Unsupported         int                    `json:"unsupported"`
	InvalidValue        int                    `json:"invalid_value"`
	InvalidValueSamples []string               `json:"invalid_value_samples,omitempty"`
	TagResults          map[string]*TagSummary `json:"tag_results"`
}

type TagSummary struct {
	WorkloadOnly        bool     `json:"workload_only,omitempty"`
	Found               int      `json:"found"`
	Missing             int      `json:"missing"`
	Unknown             int      `json:"unknown"`
	InvalidValue        int      `json:"invalid_value"`
	InvalidValueSamples []string `json:"invalid_value_samples"`
}

func (t *TagSummary) addInvalidValue(value string) {
	t.InvalidValue++

	if slices.Contains(t.InvalidValueSamples, value) || len(t.InvalidValueSamples) >= maxInvalidValueSamplesPerMetric {
		return
	}
	t.InvalidValueSamples = append(t.InvalidValueSamples, value)
}

// ValidationResult holds validation failures derived from spec expectations.
type ValidationResult struct {
	Metrics map[string]*MetricStatus `json:"metrics"`
}

// HasFailures returns true when the result contains metric-level or tag-level failures.
func (r *ValidationResult) HasFailures() bool {
	for _, status := range r.Metrics {
		if status.Missing > 0 || status.Unknown > 0 || status.Unsupported > 0 || status.InvalidValue > 0 {
			return true
		}
		for _, tagResult := range status.TagResults {
			if tagResult.Missing > 0 || tagResult.Unknown > 0 || tagResult.InvalidValue > 0 {
				return true
			}
		}
	}
	return false
}

func (r *ValidationResult) getMetricStatus(metricName string) *MetricStatus {
	if _, found := r.Metrics[metricName]; !found {
		r.Metrics[metricName] = &MetricStatus{
			TagResults: map[string]*TagSummary{},
		}
	}
	return r.Metrics[metricName]
}

func (r *ValidationResult) addInvalidValue(metricName string, sample string) {
	metricStatus := r.getMetricStatus(metricName)
	metricStatus.InvalidValue++
	if len(metricStatus.InvalidValueSamples) < maxInvalidValueSamplesPerMetric {
		metricStatus.InvalidValueSamples = append(metricStatus.InvalidValueSamples, sample)
	}
}

// KnownGPUConfigs returns all supported architecture + mode combinations.
func KnownGPUConfigs(specs *Specs) []GPUConfig {
	configs := make([]GPUConfig, 0, len(specs.Architectures.Architectures)*3)
	for archName, archSpec := range specs.Architectures.Architectures {
		for _, mode := range AllDeviceModes {
			if !IsModeSupportedByArchitecture(archSpec, mode) {
				continue
			}
			configs = append(configs, GPUConfig{
				Architecture: strings.ToLower(archName),
				DeviceMode:   mode,
			})
		}
	}

	return configs
}

// ExpectedMetricsForConfig returns the spec metric names expected for a GPU config.
func ExpectedMetricsForConfig(specs *Specs, config GPUConfig, options ValidationOptions) map[string]MetricSpec {
	expected := make(map[string]MetricSpec)
	for metricName, metricSpec := range specs.Metrics.Metrics {
		if !metricSpec.SupportsConfig(config) {
			continue
		}
		if metricSpec.WorkloadOnly && !options.WorkloadActive {
			continue
		}
		expected[metricName] = metricSpec
	}
	return expected
}

// PrefixedMetricName adds the spec metric prefix to a metric name if needed.
func PrefixedMetricName(specs *Specs, metricName string) string {
	if specs.Metrics.MetricPrefix == "" {
		return metricName
	}
	if strings.HasPrefix(metricName, specs.Metrics.MetricPrefix+".") {
		return metricName
	}

	return specs.Metrics.MetricPrefix + "." + metricName
}

// TagsToKeyValues converts Datadog-style tags to a key -> values map.
func TagsToKeyValues(tags []string) map[string][]string {
	result := make(map[string][]string, len(tags))
	for _, tag := range tags {
		key, value, ok := strings.Cut(tag, ":")
		if !ok || key == "" || value == "" {
			continue
		}
		result[key] = append(result[key], value)
	}
	return result
}

// RequiredTagsForMetric expands the required tags for a metric from tagsets and custom tags.
func RequiredTagsForMetric(tagsSpec *TagsSpec, metricSpec MetricSpec) (map[string]TagSpec, map[string]TagSpec, error) {
	requiredTags := make(map[string]TagSpec)
	workloadOnlyTags := make(map[string]TagSpec)
	for _, tagsetName := range metricSpec.Tagsets {
		tagsetSpec, ok := tagsSpec.Tagsets[tagsetName]
		if !ok {
			return nil, nil, fmt.Errorf("unknown tagset %q", tagsetName)
		}
		targetMap := requiredTags
		if tagsetSpec.WorkloadOnly {
			targetMap = workloadOnlyTags
		}
		for _, tag := range tagsetSpec.Tags {
			tagSpec, found := tagsSpec.Tags[tag]
			if !found {
				return nil, nil, fmt.Errorf("tagset %s references unknown tag %s", tagsetName, tag)
			}
			targetMap[tag] = tagSpec
		}
	}

	for _, tag := range metricSpec.CustomTags {
		tagSpec, found := tagsSpec.Tags[tag]
		if !found {
			return nil, nil, fmt.Errorf("unknown custom tag %q", tag)
		}
		requiredTags[tag] = tagSpec
	}

	return requiredTags, workloadOnlyTags, nil
}

// validateMetricTagsAgainstSpec validates emitted tags against the spec for a metric.
// If knownTagValues is provided, matching keys are additionally checked for exact values.
func validateMetricTagsAgainstSpec(spec *Specs, metricSpec MetricSpec, metricSamples []MetricObservation, knownTagValues map[string]string, options ValidationOptions) (map[string]*TagSummary, error) {
	tagResults := make(map[string]*TagSummary)

	requiredTags, workloadOnlyTags, err := RequiredTagsForMetric(spec.Tags, metricSpec)
	if err != nil {
		return nil, fmt.Errorf("required tags failed: %w", err)
	}

	if options.WorkloadActive {
		maps.Copy(requiredTags, workloadOnlyTags) // include workload tags as required for the tag validation
	}

	getTagSummary := func(tag string) *TagSummary {
		if _, found := tagResults[tag]; !found {
			_, workloadOnly := workloadOnlyTags[tag]
			tagResults[tag] = &TagSummary{WorkloadOnly: workloadOnly}
		}
		return tagResults[tag]
	}

	for _, sample := range metricSamples {
		tagsByKey := TagsToKeyValues(sample.Tags)

		for tag := range requiredTags {
			summary := getTagSummary(tag)
			if values, found := tagsByKey[tag]; !found || len(values) == 0 {
				summary.Missing++
			} else {
				summary.Found++
			}
		}

		for tag, values := range tagsByKey {
			_, allowed := requiredTags[tag]
			if !allowed {
				getTagSummary(tag).Unknown++
				continue
			}

			for _, value := range values {
				expectedValue, hasExpectedValue := knownTagValues[tag]
				tagSpec, hasTagSpec := requiredTags[tag]

				if value == "" || (hasExpectedValue && value != expectedValue) || (hasTagSpec && tagSpec.Regex != nil && !tagSpec.Regex.MatchString(value)) {
					getTagSummary(tag).addInvalidValue(value)
					continue
				}
			}
		}
	}

	return tagResults, nil
}

// ValidateEmittedMetricsAgainstSpec validates emitted metrics against the spec for a given GPU config.
func ValidateEmittedMetricsAgainstSpec(specs *Specs, config GPUConfig, emittedMetrics map[string][]MetricObservation, knownTagValues map[string]string, options ValidationOptions) (ValidationResult, error) {
	results := ValidationResult{
		Metrics: make(map[string]*MetricStatus),
	}

	// First, check that all of the emitted metrics are known to the spec and supported by the given config.
	for metricName := range emittedMetrics {
		metricSpec, found := specs.Metrics.Metrics[metricName]
		if !found {
			results.getMetricStatus(metricName).Unknown++
			continue
		}

		if !metricSpec.SupportsConfig(config) {
			results.getMetricStatus(metricName).Unsupported++
		}
	}

	// Now, for all metrics that we would expect, validate each of them individually.
	expectedMetrics := ExpectedMetricsForConfig(specs, config, options)
	for metricName, metricSpec := range expectedMetrics {
		metricSamples, found := emittedMetrics[metricName]
		if !found {
			results.getMetricStatus(metricName).Missing++
			continue
		}

		tagResults, err := validateMetricTagsAgainstSpec(specs, metricSpec, metricSamples, knownTagValues, options)
		if err != nil {
			return results, fmt.Errorf("validate metric tags for %s: %w", metricName, err)
		}

		metricStatus := results.getMetricStatus(metricName)
		metricStatus.TagResults = tagResults

		if metricSpec.Validator == nil {
			continue
		}

		for _, sample := range metricSamples {
			if sample.Value == nil {
				continue
			}
			if err := metricSpec.Validator.Validate(*sample.Value); err != nil {
				results.addInvalidValue(metricName, err.Error())
			}
		}
	}

	return results, nil
}
