// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package spec

import (
	"fmt"
	"strings"
)

// GPUConfig identifies an architecture + device mode pair from the spec.
type GPUConfig struct {
	Architecture string
	DeviceMode   DeviceMode
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
)

type MetricStatus struct {
	Errors    []string            `json:"errors"`
	TagErrors map[string][]string `json:"tag_errors"`
}

// ValidationResult holds validation failures derived from spec expectations.
type ValidationResult struct {
	Metrics map[string]*MetricStatus `json:"metrics"`
}

func (r *ValidationResult) getMetricStatus(metricName string) *MetricStatus {
	if _, found := r.Metrics[metricName]; !found {
		r.Metrics[metricName] = &MetricStatus{
			Errors:    []string{},
			TagErrors: map[string][]string{},
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

// ExpectedMetricsForConfig returns the fully-prefixed metrics expected for an architecture + mode.
func ExpectedMetricsForConfig(metricsSpec *MetricsSpec, arch string, mode DeviceMode) map[string]MetricSpec {
	expected := make(map[string]MetricSpec)
	if metricsSpec == nil {
		return expected
	}

	for metricName, metricSpec := range metricsSpec.Metrics {
		if !metricSpec.SupportsArchitecture(arch) || !metricSpec.SupportsDeviceMode(mode) {
			continue
		}
		expected[metricsSpec.MetricPrefix+"."+metricName] = metricSpec
	}

	return expected
}

// UnprefixedMetricName strips the spec metric prefix from an emitted metric name.
func UnprefixedMetricName(metricsSpec *MetricsSpec, metricName string) string {
	if metricsSpec == nil || metricsSpec.MetricPrefix == "" {
		return metricName
	}

	return strings.TrimPrefix(metricName, metricsSpec.MetricPrefix+".")
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
		return nil, fmt.Errorf("metrics spec is nil")
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
func ValidateMetricTagsAgainstSpec(spec *MetricsSpec, metricName string, metricSpec MetricSpec, metricSamples []MetricObservation, knownTagValues map[string]string) (map[string][]string, error) {
	tagErrors := make(map[string][]string)

	requiredTags, err := RequiredTagsForMetric(spec, metricSpec)
	if err != nil {
		return nil, fmt.Errorf("required tags for %s: %w", metricName, err)
	}

	for _, sample := range metricSamples {
		tagsByKey := TagsToKeyValues(sample.Tags)

		for tag := range requiredTags {
			if _, found := tagsByKey[tag]; !found {
				tagErrors[tag] = append(tagErrors[tag], ErrorMissing)
			}
		}

		for tag, values := range tagsByKey {
			_, allowed := requiredTags[tag]
			if !allowed {
				tagErrors[tag] = append(tagErrors[tag], ErrorUnknown)
				continue
			}

			for _, value := range values {
				if value == "" {
					tagErrors[tag] = append(tagErrors[tag], ErrorInvalidValue)
					continue
				}
				if expectedValue, ok := knownTagValues[tag]; ok && value != expectedValue {
					tagErrors[tag] = append(tagErrors[tag], ErrorInvalidValue)
				}
			}
		}
	}

	return tagErrors, nil
}

func ValidateEmittedMetricsAgainstSpec(metricsSpec *MetricsSpec, archName string, mode DeviceMode, emittedMetrics map[string][]MetricObservation, knownTagValues map[string]string) (ValidationResult, error) {
	results := ValidationResult{
		Metrics: make(map[string]*MetricStatus),
	}

	for metricName := range emittedMetrics {
		metricSpec, found := metricsSpec.Metrics[metricName]
		if !found {
			results.addError(metricName, ErrorUnknown)
			continue
		}

		if !metricSpec.SupportsArchitecture(archName) || !metricSpec.SupportsDeviceMode(mode) {
			results.addError(metricName, ErrorUnsupported)
		}
	}

	for metricName, metricSpec := range metricsSpec.Metrics {
		if !metricSpec.SupportsArchitecture(archName) || !metricSpec.SupportsDeviceMode(mode) {
			continue
		}

		if _, found := emittedMetrics[metricName]; !found {
			results.addError(metricName, ErrorMissing)
			continue
		}

		tagErrors, err := ValidateMetricTagsAgainstSpec(metricsSpec, metricName, metricSpec, emittedMetrics[metricName], knownTagValues)
		if err != nil {
			return results, fmt.Errorf("validate metric tags for %s: %w", metricName, err)
		}

		results.getMetricStatus(metricName).TagErrors = tagErrors
	}

	return results, nil
}
