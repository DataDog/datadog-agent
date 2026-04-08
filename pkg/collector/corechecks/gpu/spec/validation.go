// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package spec

import (
	"errors"
	"fmt"
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
	Value *float64
}

const (
	ErrorMissing      = "missing"
	ErrorUnknown      = "unknown"
	ErrorUnsupported  = "unsupported"
	ErrorInvalidValue = "invalid value"
)

type MetricStatus struct {
	Errors     []string               `json:"errors"`
	TagResults map[string]*TagSummary `json:"tag_results"`
}

type TagSummary struct {
	Found        int `json:"found"`
	Missing      int `json:"missing"`
	Unknown      int `json:"unknown"`
	InvalidValue int `json:"invalid_value"`
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
func KnownGPUConfigs(architectures *ArchitecturesSpec) []GPUConfig {
	if architectures == nil {
		return nil
	}

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
func ExpectedMetricsForConfig(metricsSpec *MetricsSpec, config GPUConfig) map[string]MetricSpec {
	expected := make(map[string]MetricSpec)
	for metricName, metricSpec := range metricsSpec.Metrics {
		if !metricSpec.SupportsConfig(config) {
			continue
		}
		expected[metricName] = metricSpec
	}

	return expected
}

// PrefixedMetricName adds the spec metric prefix to a metric name if needed.
func PrefixedMetricName(metricsSpec *MetricsSpec, metricName string) string {
	if metricsSpec == nil || metricsSpec.MetricPrefix == "" {
		return metricName
	}

	if strings.HasPrefix(metricName, metricsSpec.MetricPrefix+".") {
		return metricName
	}

	return metricsSpec.MetricPrefix + "." + metricName
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
func RequiredTagsForMetric(metricsSpec *MetricsSpec, metricSpec MetricSpec) (map[string]struct{}, error) {
	if metricsSpec == nil {
		return nil, errors.New("metrics spec is nil")
	}

	requiredTags := make(map[string]struct{})
	for _, tagsetName := range metricSpec.Tagsets {
		tagsetSpec, ok := metricsSpec.Tagsets[tagsetName]
		if !ok {
			return nil, fmt.Errorf("unknown tagset %q", tagsetName)
		}

		for _, tag := range tagsetSpec.Tags {
			requiredTags[tag] = struct{}{}
		}
	}

	for _, tag := range metricSpec.CustomTags {
		requiredTags[tag] = struct{}{}
	}

	return requiredTags, nil
}

// RequiredTagsByMetric returns the required tags for each metric in the provided set.
func RequiredTagsByMetric(metricsSpec *MetricsSpec, metrics map[string]MetricSpec) (map[string]map[string]struct{}, error) {
	result := make(map[string]map[string]struct{}, len(metrics))
	for metricName, metricSpec := range metrics {
		requiredTags, err := RequiredTagsForMetric(metricsSpec, metricSpec)
		if err != nil {
			return nil, fmt.Errorf("required tags for %s: %w", metricName, err)
		}
		result[metricName] = requiredTags
	}
	return result, nil
}

// ValidateMetricTagsAgainstSpec validates emitted tags against the spec for a metric.
// If knownTagValues is provided, matching keys are additionally checked for exact values.
func ValidateMetricTagsAgainstSpec(spec *MetricsSpec, metricSpec MetricSpec, metricSamples []MetricObservation, knownTagValues map[string]string) (map[string]*TagSummary, error) {
	tagResults := make(map[string]*TagSummary)

	requiredTags, err := RequiredTagsForMetric(spec, metricSpec)
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
				if value == "" {
					getTagSummary(tag).InvalidValue++
					continue
				}
				if expectedValue, ok := knownTagValues[tag]; ok && value != expectedValue {
					getTagSummary(tag).InvalidValue++
				}
			}
		}
	}

	return tagResults, nil
}

// ValidateEmittedMetricsAgainstSpec validates emitted metrics against the spec for a given GPU config.
func ValidateEmittedMetricsAgainstSpec(metricsSpec *MetricsSpec, config GPUConfig, emittedMetrics map[string][]MetricObservation, knownTagValues map[string]string) (ValidationResult, error) {
	results := ValidationResult{
		Metrics: make(map[string]*MetricStatus),
	}

	for metricName := range emittedMetrics {
		metricSpec, found := metricsSpec.Metrics[metricName]
		if !found {
			results.addError(metricName, ErrorUnknown)
			continue
		}

		if !metricSpec.SupportsConfig(config) {
			results.addError(metricName, ErrorUnsupported)
		}
	}

	expectedMetrics := ExpectedMetricsForConfig(metricsSpec, config)
	for metricName, metricSpec := range expectedMetrics {
		metricSamples, found := emittedMetrics[metricName]
		if !found {
			results.addError(metricName, ErrorMissing)
			continue
		}

		tagResults, err := ValidateMetricTagsAgainstSpec(metricsSpec, metricSpec, metricSamples, knownTagValues)
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
				metricStatus.Errors = append(metricStatus.Errors, fmt.Sprintf("%s: %v", ErrorInvalidValue, err))
			}
		}
	}

	return results, nil
}
