// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package spec

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// GPUConfig identifies an architecture + device mode pair from the spec.
type GPUConfig struct {
	Architecture string     `json:"architecture"`
	DeviceMode   DeviceMode `json:"device_mode"`
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
	Name string
	Tags []string
}

const (
	ErrorMissing      = "missing"
	ErrorUnknown      = "unknown"
	ErrorUnsupported  = "unsupported"
	ErrorInvalidValue = "invalid value"

	maxInvalidValueSamples = 5
)

type MetricStatus struct {
	Errors     []string               `json:"errors"`
	TagResults map[string]*TagSummary `json:"tag_results"`
}

type TagSummary struct {
	Found               int      `json:"found"`
	Missing             int      `json:"missing"`
	Unknown             int      `json:"unknown"`
	InvalidValue        int      `json:"invalid_value"`
	InvalidValueSamples []string `json:"invalid_value_samples"`
}

func (t *TagSummary) addInvalidValue(value string) {
	t.InvalidValue++

	if slices.Contains(t.InvalidValueSamples, value) || len(t.InvalidValueSamples) >= maxInvalidValueSamples {
		return
	}
	t.InvalidValueSamples = append(t.InvalidValueSamples, value)
}

// ValidationResult holds validation failures derived from spec expectations.
type ValidationResult struct {
	Metrics map[string]*MetricStatus `json:"metrics"`
}

// HasFailures returns true when the result contains any missing, unknown, or tag-level failures.
func (r *ValidationResult) HasFailures() bool {
	for _, status := range r.Metrics {
		if len(status.Errors) > 0 {
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
			Errors:     []string{},
			TagResults: map[string]*TagSummary{},
		}
	}
	return r.Metrics[metricName]
}

func (r *ValidationResult) addError(metricName string, err string) {
	r.getMetricStatus(metricName).Errors = append(r.getMetricStatus(metricName).Errors, err)
}

// KnownGPUConfigs returns all supported architecture + mode combinations.
func KnownGPUConfigs(specs *Specs) []GPUConfig {
	if specs == nil || specs.Architectures == nil {
		return nil
	}

	architectures := specs.Architectures
	configs := make([]GPUConfig, 0, len(architectures.Architectures)*3)
	for archName, archSpec := range architectures.Architectures {
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
func ExpectedMetricsForConfig(specs *Specs, config GPUConfig) map[string]MetricSpec {
	expected := make(map[string]MetricSpec)
	for metricName, metricSpec := range specs.Metrics.Metrics {
		if !metricSpec.SupportsConfig(config) {
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
func RequiredTagsForMetric(tagsSpec *TagsSpec, metricSpec MetricSpec) (map[string]TagSpec, error) {
	if tagsSpec == nil {
		return nil, errors.New("tags spec is nil")
	}

	requiredTags := make(map[string]TagSpec)
	for _, tagsetName := range metricSpec.Tagsets {
		tagsetSpec, ok := tagsSpec.Tagsets[tagsetName]
		if !ok {
			return nil, fmt.Errorf("unknown tagset %q", tagsetName)
		}

		for _, tag := range tagsetSpec.Tags {
			tagSpec, found := tagsSpec.Tags[tag]
			if !found {
				return nil, fmt.Errorf("tagset %s references unknown tag %s", tagsetName, tag)
			}
			requiredTags[tag] = tagSpec
		}
	}

	for _, tag := range metricSpec.CustomTags {
		tagSpec, found := tagsSpec.Tags[tag]
		if !found {
			return nil, fmt.Errorf("unknown custom tag %q", tag)
		}
		requiredTags[tag] = tagSpec
	}

	return requiredTags, nil
}

// RequiredTagsByMetric returns the required tags for each metric in the provided set.
func RequiredTagsByMetric(tagsSpec *TagsSpec, metrics map[string]MetricSpec) (map[string]map[string]TagSpec, error) {
	result := make(map[string]map[string]TagSpec, len(metrics))
	for metricName, metricSpec := range metrics {
		requiredTags, err := RequiredTagsForMetric(tagsSpec, metricSpec)
		if err != nil {
			return nil, fmt.Errorf("required tags for %s: %w", metricName, err)
		}
		result[metricName] = requiredTags
	}
	return result, nil
}

// validateMetricTagsAgainstSpec validates emitted tags against the spec for a metric.
// If knownTagValues is provided, matching keys are additionally checked for exact values.
func validateMetricTagsAgainstSpec(tagsSpec *TagsSpec, metricSpec MetricSpec, metricSamples []MetricObservation, knownTagValues map[string]string) (map[string]*TagSummary, error) {
	tagResults := make(map[string]*TagSummary)

	requiredTags, err := RequiredTagsForMetric(tagsSpec, metricSpec)
	if err != nil {
		return nil, fmt.Errorf("required tags failed: %w", err)
	}

	getTagSummary := func(tag string) *TagSummary {
		if _, found := tagResults[tag]; !found {
			tagResults[tag] = &TagSummary{}
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
func ValidateEmittedMetricsAgainstSpec(specs *Specs, config GPUConfig, emittedMetrics map[string][]MetricObservation, knownTagValues map[string]string) (ValidationResult, error) {
	results := ValidationResult{
		Metrics: make(map[string]*MetricStatus),
	}

	// First, check that all of the emitted metrics are known to the spec and supported by the given config.
	for metricName := range emittedMetrics {
		metricSpec, found := specs.Metrics.Metrics[metricName]
		if !found {
			results.addError(metricName, ErrorUnknown)
			continue
		}

		if !metricSpec.SupportsConfig(config) {
			results.addError(metricName, ErrorUnsupported)
		}
	}

	// Now, for all metrics that we would expect, validate each of them individually.
	expectedMetrics := ExpectedMetricsForConfig(specs, config)
	for metricName, metricSpec := range expectedMetrics {
		if _, found := emittedMetrics[metricName]; !found {
			results.addError(metricName, ErrorMissing)
			continue
		}

		tagResults, err := validateMetricTagsAgainstSpec(specs.Tags, metricSpec, emittedMetrics[metricName], knownTagValues)
		if err != nil {
			return results, fmt.Errorf("validate metric tags for %s: %w", metricName, err)
		}

		results.getMetricStatus(metricName).TagResults = tagResults
	}

	return results, nil
}
