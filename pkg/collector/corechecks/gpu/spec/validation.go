// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package spec

import (
	"fmt"
	"slices"
	"strings"
)

// GPUConfig identifies a GPU configuration used for spec validation.
type GPUConfig struct {
	Architecture    string                   `json:"architecture"`
	DeviceMode      DeviceMode               `json:"device_mode"`
	Capabilities    ArchitectureCapabilities `json:"capabilities,omitempty"`
	NVLinkLinkCount int                      `json:"nvlink_link_count,omitempty"`
}

// ValidationOptions controls which spec failures should be enforced.
type ValidationOptions struct {
	WorkloadActive bool `json:"workload_active"`
	// IgnoreMetrics is a list of metric names that should be ignored during validation.
	IgnoreMetrics map[string]bool `json:"ignore_metrics,omitempty"`
	// ConfigFeatures contains enabled Agent configuration features. Metrics that
	// require a disabled feature are not expected.
	ConfigFeatures map[ConfigFeature]bool `json:"config_features,omitempty"`
	// WorkloadTagsets contains workload-only tagsets required for active
	// workloads. It lets callers distinguish bare processes from workloads with
	// additional metadata, such as Kubernetes containers.
	WorkloadTagsets map[string]bool `json:"workload_tagsets,omitempty"`
	// NvidiaSMIValues contains normalized nvidia-smi values keyed by spec metric name.
	// A non-nil map enables nvidia-smi validation for marked metrics.
	NvidiaSMIValues map[string]*float64 `json:"-"`
	// CalibratedWorkloadValues contains calibrated workload values keyed by spec
	// metric name. A non-nil map enables workload validation for marked metrics.
	CalibratedWorkloadValues map[string]*float64 `json:"-"`
}

// Equals checks if two GPU configs are equal.
func (c *GPUConfig) Equals(other GPUConfig) bool {
	return c.Architecture == other.Architecture &&
		c.DeviceMode == other.DeviceMode &&
		c.Capabilities.GPM == other.Capabilities.GPM &&
		c.Capabilities.NVLink == other.Capabilities.NVLink &&
		c.Capabilities.C2C == other.Capabilities.C2C &&
		c.NVLinkLinkCount == other.NVLinkLinkCount
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
	Name       string
	MetricType string
	Tags       []string
	Value      *float64
}

const maxInvalidValueSamplesPerMetric = 5

type MetricStatus struct {
	Missing             int                    `json:"missing"`
	Unknown             int                    `json:"unknown"`
	Unsupported         int                    `json:"unsupported"`
	WrongType           int                    `json:"wrong_type"`
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

// HasFailures returns true when the metric status contains metric-level or tag-level failures.
func (s *MetricStatus) HasFailures() bool {
	if s == nil {
		return false
	}

	if s.Missing+s.Unknown+s.Unsupported+s.WrongType+s.InvalidValue > 0 {
		return true
	}

	for _, tagResult := range s.TagResults {
		if tagResult.Missing > 0 || tagResult.Unknown > 0 || tagResult.InvalidValue > 0 {
			return true
		}
	}

	return false
}

// HasFailures returns true when the result contains metric-level or tag-level failures.
func (r *ValidationResult) HasFailures() bool {
	for _, status := range r.Metrics {
		if status.HasFailures() {
			return true
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

func validateMetricType(expectedMetricType, observedMetricType string) error {
	if expectedMetricType == "" || observedMetricType == "" || expectedMetricType == observedMetricType {
		return nil
	}

	return fmt.Errorf("metric type %q does not match expected %q", observedMetricType, expectedMetricType)
}

// KnownGPUConfigs returns all supported architecture + mode combinations.
func KnownGPUConfigs(specs *Specs) []GPUConfig {
	configs := make([]GPUConfig, 0, len(specs.Architectures.Architectures)*3)
	for archName, archSpec := range specs.Architectures.Architectures {
		for _, mode := range AllDeviceModes {
			if !IsModeSupportedByArchitecture(archSpec, mode) {
				continue
			}
			capabilities := archSpec.EffectiveCapabilities(mode)
			nvlinkLinkCount := 0
			// Use a nonzero mock link count whenever the spec says this mode is NVLink-capable.
			if capabilities.NVLink > 0 {
				nvlinkLinkCount = 2
			}
			configs = append(configs, GPUConfig{
				Architecture:    strings.ToLower(archName),
				DeviceMode:      mode,
				Capabilities:    capabilities,
				NVLinkLinkCount: nvlinkLinkCount,
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
		if suppressInactiveNVLinkMetric(metricName, config) {
			continue
		}
		if !metricSpec.SupportsCapabilities(config.Capabilities) {
			continue
		}
		if metricSpec.WorkloadOnly && !options.WorkloadActive {
			continue
		}
		if !hasRequiredConfigFeatures(metricSpec, options.ConfigFeatures) {
			continue
		}
		if options.IgnoreMetrics[metricName] {
			continue
		}
		expected[metricName] = metricSpec
	}
	return expected
}

func hasRequiredConfigFeatures(metricSpec MetricSpec, enabled map[ConfigFeature]bool) bool {
	for _, feature := range metricSpec.ConfigRequired {
		if !enabled[feature] {
			return false
		}
	}
	return true
}

// AllConfigFeatures returns every configuration feature known by the spec.
func AllConfigFeatures() map[ConfigFeature]bool {
	return map[ConfigFeature]bool{
		ConfigFeatureSystemProbeEBPF: true,
		ConfigFeatureSystemProbePRM:  true,
	}
}

// AllWorkloadTagsets returns every workload-only tagset defined by the spec.
func AllWorkloadTagsets(tagsSpec *TagsSpec) map[string]bool {
	tagsets := make(map[string]bool)
	for name, tagset := range tagsSpec.Tagsets {
		if tagset.WorkloadOnly {
			tagsets[name] = true
		}
	}
	return tagsets
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

// RequiredTagsForMetricWithOptions returns tags required for a metric in the
// supplied validation context.
func RequiredTagsForMetricWithOptions(tagsSpec *TagsSpec, metricSpec MetricSpec, options ValidationOptions) (map[string]TagSpec, error) {
	requiredTags, workloadOnlyTags, err := RequiredTagsForMetric(tagsSpec, metricSpec)
	if err != nil {
		return nil, err
	}
	if !options.WorkloadActive {
		return requiredTags, nil
	}
	for _, tagsetName := range metricSpec.Tagsets {
		tagsetSpec := tagsSpec.Tagsets[tagsetName]
		if !tagsetSpec.WorkloadOnly || !options.WorkloadTagsets[tagsetName] {
			continue
		}
		for _, tagName := range tagsetSpec.Tags {
			requiredTags[tagName] = workloadOnlyTags[tagName]
		}
	}
	return requiredTags, nil
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
		for _, tagsetName := range metricSpec.Tagsets {
			tagsetSpec := spec.Tags.Tagsets[tagsetName]
			if !tagsetSpec.WorkloadOnly || !options.WorkloadTagsets[tagsetName] {
				continue
			}
			for _, tagName := range tagsetSpec.Tags {
				requiredTags[tagName] = workloadOnlyTags[tagName]
			}
		}
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
	if err := validateValidationOptions(specs, options); err != nil {
		return results, err
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
			continue
		}

		if suppressInactiveNVLinkMetric(metricName, config) || !metricSpec.SupportsCapabilities(config.Capabilities) {
			results.getMetricStatus(metricName).Unsupported++
		}
	}

	// Now, for all metrics that we would expect, validate each of them individually.
	expectedMetrics := ExpectedMetricsForConfig(specs, config, options)
	for metricName, metricSpec := range expectedMetrics {
		metricSamples, found := emittedMetrics[metricName]
		if !found {
			if !metricSpec.Optional {
				results.getMetricStatus(metricName).Missing++
			}
			continue
		}

		tagResults, err := validateMetricTagsAgainstSpec(specs, metricSpec, metricSamples, knownTagValues, options)
		if err != nil {
			return results, fmt.Errorf("validate metric tags for %s: %w", metricName, err)
		}

		metricStatus := results.getMetricStatus(metricName)
		metricStatus.TagResults = tagResults

		for _, sample := range metricSamples {
			if metricSpec.Metadata != nil {
				if err := validateMetricType(metricSpec.Metadata.MetricType, sample.MetricType); err != nil {
					results.getMetricStatus(metricName).WrongType++
				}
			}

			if metricSpec.Validator.HasStaticValueValidation() && sample.Value != nil {
				if err := metricSpec.Validator.ValidateStaticValue(*sample.Value); err != nil {
					results.addInvalidValue(metricName, err.Error())
				}
			}
		}

		if options.NvidiaSMIValues != nil && metricSpec.Validator != nil && metricSpec.Validator.NvidiaSMI {
			validateMetricAgainstKnownGood(&results, metricName, metricSpec.Validator, metricSamples, options.NvidiaSMIValues)
		}
		if options.CalibratedWorkloadValues != nil && metricSpec.Validator != nil && metricSpec.Validator.CalibratedWorkload {
			validateMetricAgainstKnownGood(&results, metricName, metricSpec.Validator, metricSamples, options.CalibratedWorkloadValues)
		}
	}

	return results, nil
}

func validateValidationOptions(specs *Specs, options ValidationOptions) error {
	for tagsetName, enabled := range options.WorkloadTagsets {
		if !enabled {
			continue
		}
		tagsetSpec, found := specs.Tags.Tagsets[tagsetName]
		if !found {
			return fmt.Errorf("unknown enabled workload tagset %q", tagsetName)
		}
		if !tagsetSpec.WorkloadOnly {
			return fmt.Errorf("enabled workload tagset %q is not workload-only", tagsetName)
		}
	}
	return nil
}

func validateMetricAgainstKnownGood(results *ValidationResult, metricName string, validator *MetricValidator, observations []MetricObservation, knownGoodValues map[string]*float64) {
	if len(observations) == 0 {
		results.addInvalidValue(metricName, "emitted metric observation is missing")
		return
	}

	latest := observations[len(observations)-1]
	if latest.Value == nil {
		results.addInvalidValue(metricName, "emitted metric value is missing")
		return
	}
	if err := validator.ValidateKnownGoodValue(*latest.Value, knownGoodValues[metricName]); err != nil {
		results.addInvalidValue(metricName, err.Error())
	}
}

func suppressInactiveNVLinkMetric(metricName string, config GPUConfig) bool {
	return strings.HasPrefix(metricName, "nvlink.") &&
		// NVSwitch connectivity can be reported as zero even when no active NVLink ports are present.
		metricName != "nvlink.nvswitch_connected" &&
		config.NVLinkLinkCount == 0
}
