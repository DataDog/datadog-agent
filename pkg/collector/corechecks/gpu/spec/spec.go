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

	"regexp"

	"go.yaml.in/yaml/v2"
)

const (
	metricsSpecFile       = "gpu_metrics.yaml"
	architecturesSpecFile = "architectures.yaml"
	tagsSpecFile          = "tags.yaml"
)

//go:embed gpu_metrics.yaml architectures.yaml tags.yaml
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
	Tags []string `yaml:"tags"`
}

// MetricSpec is a metric definition without the name (name is the map key).
type MetricSpec struct {
	Tagsets    []string          `yaml:"tagsets"`
	CustomTags []string          `yaml:"custom_tags,omitempty"`
	Support    MetricSupportSpec `yaml:"support"`
	Validator  *MetricValidator  `yaml:"validator,omitempty"`
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
	UnsupportedArchitectures []string            `yaml:"unsupported_architectures"`
	DeviceModes              map[DeviceMode]bool `yaml:"device_modes"`
}

// ArchitecturesSpec is the YAML architecture capability specification.
type ArchitecturesSpec struct {
	Architectures map[string]ArchitectureSpec `yaml:"architectures"`
}

// Specs bundles all GPU spec files used by validation and tests.
type Specs struct {
	Metrics       *MetricsSpec
	Tags          *TagsSpec
	Architectures *ArchitecturesSpec
}

// ArchitectureCapabilities defines capabilities and unsupported fields.
type ArchitectureCapabilities struct {
	GPM                           bool                                `yaml:"gpm"`
	UnsupportedFieldsByDeviceMode []UnsupportedFieldsByDeviceModeSpec `yaml:"unsupported_fields_by_device_mode"`
}

// UnsupportedFieldsByDeviceModeSpec groups unsupported fields by device modes.
type UnsupportedFieldsByDeviceModeSpec struct {
	DeviceModes []DeviceMode `yaml:"device_modes"`
	Fields      []string     `yaml:"fields"`
}

// ArchitectureSpec defines architecture capabilities and unsupported device modes.
type ArchitectureSpec struct {
	Capabilities           ArchitectureCapabilities `yaml:"capabilities"`
	UnsupportedDeviceModes []DeviceMode             `yaml:"unsupported_device_modes"`
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

	return &Specs{
		Metrics:       metrics,
		Tags:          tags,
		Architectures: architectures,
	}, nil
}
