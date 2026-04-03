// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package spec holds structures to parse the metric specification for the GPU check.
package spec

import (
	"embed"
	"fmt"

	"go.yaml.in/yaml/v2"
)

const (
	metricsSpecFile       = "gpu_metrics.yaml"
	architecturesSpecFile = "architectures.yaml"
)

//go:embed gpu_metrics.yaml architectures.yaml
var embeddedSpecs embed.FS

// DeviceMode identifies the GPU device operating mode in the spec.
type DeviceMode string

const (
	DeviceModePhysical DeviceMode = "physical"
	DeviceModeMIG      DeviceMode = "mig"
	DeviceModeVGPU     DeviceMode = "vgpu"
)

// MetricsSpec is the YAML metric specification.
type MetricsSpec struct {
	MetricPrefix string                `yaml:"metric_prefix"`
	Tagsets      map[string]TagsetSpec `yaml:"tagsets"`
	Metrics      map[string]MetricSpec `yaml:"metrics"`
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

// IsDeviceModeExplicitlyUnsupported returns true if device_modes explicitly disables this mode.
func (m MetricSpec) IsDeviceModeExplicitlyUnsupported(mode DeviceMode) bool {
	if m.Support.DeviceModes == nil {
		return false
	}
	v, ok := m.Support.DeviceModes[mode]
	return ok && !v
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
