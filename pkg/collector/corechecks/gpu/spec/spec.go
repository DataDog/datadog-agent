// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package spec holds structures to parse the metric specification for the GPU check.
package spec

import (
	"embed"
	"errors"
	"fmt"
	"math"
	"slices"

	"regexp"

	"go.yaml.in/yaml/v2"
)

const (
	metricsSpecFile       = "gpu_metrics.yaml"
	architecturesSpecFile = "architectures.yaml"
	tagsSpecFile          = "tags.yaml"
	aggregationsSpecFile  = "aggregations.yaml"
)

//go:embed gpu_metrics.yaml architectures.yaml tags.yaml aggregations.yaml
var embeddedSpecs embed.FS

// DeviceMode identifies the GPU device operating mode in the spec.
type DeviceMode string

const (
	DeviceModePhysical DeviceMode = "physical"
	DeviceModeMIG      DeviceMode = "mig"
	DeviceModeVGPU     DeviceMode = "vgpu"
)

// AllDeviceModes lists every device mode modeled by the GPU spec.
var AllDeviceModes = []DeviceMode{
	DeviceModePhysical,
	DeviceModeMIG,
	DeviceModeVGPU,
}

// MetricsSpec is the YAML metric specification.
type MetricsSpec struct {
	MetricPrefix string                `yaml:"metric_prefix"`
	Metrics      map[string]MetricSpec `yaml:"metrics"`
}

// TagsSpec is the YAML tags specification.
type TagsSpec struct {
	Tags    map[string]TagSpec    `yaml:"tags"`
	Tagsets map[string]TagsetSpec `yaml:"tagsets"`
}

// AggregationsSpec is the YAML aggregation specification.
type AggregationsSpec struct {
	Aggregations map[string]AggregationSpec `yaml:"aggregations"`
}

// AggregationSpec defines aggregation behavior metadata.
type AggregationSpec struct {
	Description           string `yaml:"description"`
	TimeAggregator        string `yaml:"time_aggregator"`
	GroupAggregator       string `yaml:"group_aggregator"`
	GranularityAggregator string `yaml:"granularity_aggregator"`
}

// TagSpec defines validation metadata for a reusable tag.
type TagSpec struct {
	Regex *regexp.Regexp `yaml:"-"`
}

// UnmarshalYAML compiles the optional regex when the tag spec is decoded.
func (s *TagSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw struct {
		Regex string `yaml:"regex,omitempty"`
	}

	if err := unmarshal(&raw); err != nil {
		return fmt.Errorf("unmarshal tag spec: %w", err)
	}

	if raw.Regex == "" {
		s.Regex = nil
		return nil
	}

	compiled, err := regexp.Compile(raw.Regex)
	if err != nil {
		return fmt.Errorf("compile tag regex %q: %w", raw.Regex, err)
	}

	s.Regex = compiled
	return nil
}

// TagsetSpec defines a reusable tagset.
type TagsetSpec struct {
	Tags         []string `yaml:"tags"`
	WorkloadOnly bool     `yaml:"workload_only,omitempty"`
}

// MetricMetadataSpec defines metadata used to generate integrations metadata.csv rows.
type MetricMetadataSpec struct {
	MetricType string `yaml:"metric_type,omitempty"`
	Unit       string `yaml:"unit,omitempty"`
	// UsedInDDUI marks metrics surfaced in Datadog UI defaults.
	UsedInDDUI  bool   `yaml:"used_in_dd_ui,omitempty"`
	Description string `yaml:"description,omitempty"`
	Aggregation string `yaml:"aggregation,omitempty"`
}

// UnmarshalYAML validates metric metadata values while decoding.
func (m *MetricMetadataSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain MetricMetadataSpec

	var decoded plain
	if err := unmarshal(&decoded); err != nil {
		return fmt.Errorf("unmarshal metric metadata: %w", err)
	}

	switch decoded.MetricType {
	case "", "gauge", "counter", "histogram":
		*m = MetricMetadataSpec(decoded)
		return nil
	default:
		return fmt.Errorf("invalid metric_type %q: must be one of [gauge, counter, histogram]", decoded.MetricType)
	}
}

// MetricSpec is a metric definition without the name (name is the map key).
type MetricSpec struct {
	Metadata     *MetricMetadataSpec `yaml:"metadata,omitempty"`
	Tagsets      []string            `yaml:"tagsets"`
	CustomTags   []string            `yaml:"custom_tags,omitempty"`
	WorkloadOnly bool                `yaml:"workload_only,omitempty"`
	Support      MetricSupportSpec   `yaml:"support"`
	Validator    *MetricValidator    `yaml:"validator,omitempty"`
}

// MetricValidatorRange defines an inclusive numeric range validator.
type MetricValidatorRange struct {
	Min *float64 `yaml:"min"`
	Max *float64 `yaml:"max"`
}

// MetricValidator validates emitted metric values against the spec.
type MetricValidator struct {
	Range  *MetricValidatorRange `yaml:"range,omitempty"`
	Values []float64             `yaml:"values,omitempty"`
}

// UnmarshalYAML parses the supported validator shapes while keeping the internal fields private.
func (v *MetricValidator) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain MetricValidator

	var decoded plain
	if err := unmarshal(&decoded); err != nil {
		return fmt.Errorf("unmarshal metric validator: %w", err)
	}

	*v = MetricValidator(decoded)
	return v.validateDefinition()
}

// Validate checks whether the metric value matches the validator.
func (v *MetricValidator) Validate(value float64) error {
	if v == nil {
		return nil
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%v not finite", value)
	}
	if v.Range != nil {
		if value < *v.Range.Min || value > *v.Range.Max {
			return fmt.Errorf("%v not in range [%v, %v]", value, *v.Range.Min, *v.Range.Max)
		}
		return nil
	}

	for _, allowedValue := range v.Values {
		if value == allowedValue {
			return nil
		}
	}
	return fmt.Errorf("%v not in %v", value, v.Values)
}

func (v *MetricValidator) validateDefinition() error {
	if v == nil {
		return nil
	}

	hasRange := v.Range != nil
	hasValues := len(v.Values) > 0

	switch {
	case hasRange && hasValues:
		return errors.New("metric validator must define exactly one of range or values")
	case !hasRange && !hasValues:
		return errors.New("metric validator must define exactly one of range or values")
	}

	if hasRange {
		if v.Range.Min == nil || v.Range.Max == nil {
			return errors.New("metric validator range must define both min and max")
		}
		if math.IsNaN(*v.Range.Min) || math.IsInf(*v.Range.Min, 0) {
			return fmt.Errorf("metric validator range min must be finite, got %v", *v.Range.Min)
		}
		if math.IsNaN(*v.Range.Max) || math.IsInf(*v.Range.Max, 0) {
			return fmt.Errorf("metric validator range max must be finite, got %v", *v.Range.Max)
		}
		if *v.Range.Min > *v.Range.Max {
			return fmt.Errorf("metric validator range min %v must be less than or equal to max %v", *v.Range.Min, *v.Range.Max)
		}
		return nil
	}

	for _, allowedValue := range v.Values {
		if math.IsNaN(allowedValue) || math.IsInf(allowedValue, 0) {
			return fmt.Errorf("metric validator values must be finite, got %v", allowedValue)
		}
	}

	return nil
}

// MetricSupportSpec defines where a metric is supported.
type MetricSupportSpec struct {
	UnsupportedArchitectures []string                   `yaml:"unsupported_architectures"`
	DeviceModes              map[DeviceMode]bool        `yaml:"device_modes"`
	CapabilitiesRequired     MetricCapabilitiesRequired `yaml:"capabilities_required,omitempty"`
}

// MetricCapabilitiesRequired defines hardware capabilities required for a metric.
type MetricCapabilitiesRequired struct {
	NVLink *int  `yaml:"nvlink,omitempty"`
	C2C    *bool `yaml:"c2c,omitempty"`
}

// ArchitecturesSpec is the YAML architecture capability specification.
type ArchitecturesSpec struct {
	NVLinkGenerations map[int]FieldSupportSpec    `yaml:"nvlink_generations"`
	C2C               FieldSupportSpec            `yaml:"c2c"`
	Architectures     map[string]ArchitectureSpec `yaml:"architectures"`
}

// Specs bundles all GPU spec files used by validation and tests.
type Specs struct {
	Metrics       *MetricsSpec
	Tags          *TagsSpec
	Architectures *ArchitecturesSpec
	Aggregations  *AggregationsSpec
}

// ArchitectureCapabilities defines capabilities and unsupported fields.
type ArchitectureCapabilities struct {
	GPM                           bool                                `yaml:"gpm"`
	NVLink                        int                                 `yaml:"nvlink"`
	C2C                           bool                                `yaml:"c2c"`
	UnsupportedFieldsByDeviceMode []UnsupportedFieldsByDeviceModeSpec `yaml:"unsupported_fields_by_device_mode"`
}

// ArchitectureCapabilitiesOverride defines mode-specific capability overrides.
type ArchitectureCapabilitiesOverride struct {
	GPM    *bool `yaml:"gpm,omitempty"`
	NVLink *int  `yaml:"nvlink,omitempty"`
	C2C    *bool `yaml:"c2c,omitempty"`
}

// UnsupportedFieldsByDeviceModeSpec groups unsupported fields by device modes.
type UnsupportedFieldsByDeviceModeSpec struct {
	DeviceModes []DeviceMode `yaml:"device_modes"`
	Fields      []string     `yaml:"fields"`
}

// FieldSupportSpec defines fields made available by a capability.
type FieldSupportSpec struct {
	SupportedFields []string `yaml:"supported_fields"`
}

// ArchitectureSupportSpec defines architecture support for a set of device modes.
type ArchitectureSupportSpec struct {
	// DeviceModes lists the device modes covered by this support entry.
	DeviceModes []DeviceMode `yaml:"device_modes"`
	// Capabilities describes which hardware/API capabilities are available for those modes.
	Capabilities ArchitectureCapabilitiesOverride `yaml:"capabilities"`
	// UnsupportedFields lists additional GetFieldValues fields unsupported for those modes.
	UnsupportedFields []string `yaml:"unsupported_fields"`
}

// ArchitectureSpec defines architecture capabilities and unsupported device modes.
type ArchitectureSpec struct {
	// Capabilities is the default capability set used when no mode-specific support entry matches.
	Capabilities ArchitectureCapabilities `yaml:"capabilities"`
	// Support contains mode-specific capability and unsupported-field overrides.
	Support []ArchitectureSupportSpec `yaml:"support"`
	// UnsupportedDeviceModes lists device modes that should not be validated for this architecture.
	UnsupportedDeviceModes []DeviceMode `yaml:"unsupported_device_modes"`
}

// EffectiveCapabilities returns the capabilities for a device mode.
func (a ArchitectureSpec) EffectiveCapabilities(mode DeviceMode) ArchitectureCapabilities {
	capabilities := a.Capabilities
	for _, support := range a.Support {
		if slices.Contains(support.DeviceModes, mode) {
			capabilities.applyOverride(support.Capabilities)
			return capabilities
		}
	}
	return capabilities
}

func (c *ArchitectureCapabilities) applyOverride(override ArchitectureCapabilitiesOverride) {
	if override.GPM != nil {
		c.GPM = *override.GPM
	}
	if override.NVLink != nil {
		c.NVLink = *override.NVLink
	}
	if override.C2C != nil {
		c.C2C = *override.C2C
	}
}

// UnsupportedFieldsForMode returns fields unsupported for a device mode and capability set.
func (a ArchitectureSpec) UnsupportedFieldsForMode(mode DeviceMode, archSpecs *ArchitecturesSpec) []string {
	unsupported := make(map[string]struct{})
	capabilities := a.EffectiveCapabilities(mode)
	for _, support := range a.Support {
		if !slices.Contains(support.DeviceModes, mode) {
			continue
		}
		for _, field := range support.UnsupportedFields {
			unsupported[field] = struct{}{}
		}
	}
	for _, group := range a.Capabilities.UnsupportedFieldsByDeviceMode {
		if len(group.DeviceModes) > 0 && !slices.Contains(group.DeviceModes, mode) {
			continue
		}
		for _, name := range group.Fields {
			unsupported[name] = struct{}{}
		}
	}
	if archSpecs != nil {
		for generation, support := range archSpecs.NVLinkGenerations {
			if generation <= capabilities.NVLink {
				continue
			}
			for _, field := range support.SupportedFields {
				unsupported[field] = struct{}{}
			}
		}
		if !capabilities.C2C {
			for _, field := range archSpecs.C2C.SupportedFields {
				unsupported[field] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(unsupported))
	for name := range unsupported {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// SupportedNVLinkGeneration returns the highest NVLink generation this architecture can support.
func (a ArchitectureSpec) SupportedNVLinkGeneration() int {
	maxGeneration := a.Capabilities.NVLink
	for _, support := range a.Support {
		if support.Capabilities.NVLink != nil {
			maxGeneration = max(maxGeneration, *support.Capabilities.NVLink)
		}
	}
	return maxGeneration
}

// SupportsArchitecture returns true if the metric is supported on this architecture.
func (m MetricSpec) SupportsArchitecture(arch string) bool {
	for _, unsupportedArch := range m.Support.UnsupportedArchitectures {
		if unsupportedArch == arch {
			return false
		}
	}
	return true
}

// SupportsDeviceMode returns true if the metric's device_modes explicitly allows the mode.
// device_modes values are expected to be booleans; missing means unsupported.
func (m MetricSpec) SupportsDeviceMode(mode DeviceMode) bool {
	if m.Support.DeviceModes == nil {
		return false
	}
	v, ok := m.Support.DeviceModes[mode]
	return ok && v
}

// SupportsConfig returns true if the metric is supported for the given GPU config.
func (m MetricSpec) SupportsConfig(config GPUConfig) bool {
	return m.SupportsArchitecture(config.Architecture) && m.SupportsDeviceMode(config.DeviceMode)
}

// SupportsCapabilities returns true if the metric's capability requirements are met.
func (m MetricSpec) SupportsCapabilities(capabilities ArchitectureCapabilities) bool {
	if m.Support.CapabilitiesRequired.NVLink != nil && capabilities.NVLink < *m.Support.CapabilitiesRequired.NVLink {
		return false
	}
	if m.Support.CapabilitiesRequired.C2C != nil && capabilities.C2C != *m.Support.CapabilitiesRequired.C2C {
		return false
	}
	return true
}

// IsModeSupportedByArchitecture returns true when the architecture supports the device mode.
func IsModeSupportedByArchitecture(archSpec ArchitectureSpec, mode DeviceMode) bool {
	for _, unsupportedMode := range archSpec.UnsupportedDeviceModes {
		if unsupportedMode == mode {
			return false
		}
	}
	return true
}

// LoadMetricsSpec loads the canonical GPU metrics specification file.
func LoadMetricsSpec() (*MetricsSpec, error) {
	data, err := embeddedSpecs.ReadFile(metricsSpecFile)
	if err != nil {
		return nil, fmt.Errorf("read metrics spec %q: %w", metricsSpecFile, err)
	}

	var parsed MetricsSpec
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal metrics spec %q: %w", metricsSpecFile, err)
	}

	return &parsed, nil
}

// LoadTagsSpec loads the canonical GPU tags specification file.
func LoadTagsSpec() (*TagsSpec, error) {
	data, err := embeddedSpecs.ReadFile(tagsSpecFile)
	if err != nil {
		return nil, fmt.Errorf("read tags spec %q: %w", tagsSpecFile, err)
	}

	var parsed TagsSpec
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal tags spec %q: %w", tagsSpecFile, err)
	}

	return &parsed, nil
}

// LoadArchitecturesSpec loads the canonical GPU architectures specification file.
func LoadArchitecturesSpec() (*ArchitecturesSpec, error) {
	data, err := embeddedSpecs.ReadFile(architecturesSpecFile)
	if err != nil {
		return nil, fmt.Errorf("read architectures spec %q: %w", architecturesSpecFile, err)
	}

	var parsed ArchitecturesSpec
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal architectures spec %q: %w", architecturesSpecFile, err)
	}

	return &parsed, nil
}

// LoadAggregationsSpec loads the canonical GPU aggregations specification file.
func LoadAggregationsSpec() (*AggregationsSpec, error) {
	data, err := embeddedSpecs.ReadFile(aggregationsSpecFile)
	if err != nil {
		return nil, fmt.Errorf("read aggregations spec %q: %w", aggregationsSpecFile, err)
	}

	var parsed AggregationsSpec
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal aggregations spec %q: %w", aggregationsSpecFile, err)
	}

	return &parsed, nil
}

// LoadSpecs loads all canonical GPU specification files.
func LoadSpecs() (*Specs, error) {
	metrics, err := LoadMetricsSpec()
	if err != nil {
		return nil, fmt.Errorf("load metrics spec: %w", err)
	}

	tags, err := LoadTagsSpec()
	if err != nil {
		return nil, fmt.Errorf("load tags spec: %w", err)
	}

	architectures, err := LoadArchitecturesSpec()
	if err != nil {
		return nil, fmt.Errorf("load architectures spec: %w", err)
	}

	aggregations, err := LoadAggregationsSpec()
	if err != nil {
		return nil, fmt.Errorf("load aggregations spec: %w", err)
	}

	for metricName, metricSpec := range metrics.Metrics {
		if metricSpec.Metadata == nil || metricSpec.Metadata.Aggregation == "" {
			continue
		}
		if _, found := aggregations.Aggregations[metricSpec.Metadata.Aggregation]; !found {
			return nil, fmt.Errorf("metric %q references unknown aggregation %q", metricName, metricSpec.Metadata.Aggregation)
		}
	}

	return &Specs{
		Metrics:       metrics,
		Tags:          tags,
		Architectures: architectures,
		Aggregations:  aggregations,
	}, nil
}
