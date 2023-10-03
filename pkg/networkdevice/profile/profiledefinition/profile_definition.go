// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

// DeviceMeta holds device related static metadata
// DEPRECATED in favour of profile metadata syntax
type DeviceMeta struct {
	// deprecated in favour of new `ProfileDefinition.Metadata` syntax
	Vendor string `yaml:"vendor,omitempty" json:"vendor,omitempty"`
}

// ProfileDefinition is the root profile structure
type ProfileDefinition struct {
	Name         string            `yaml:"name" json:"name"`
	Description  string            `yaml:"description,omitempty" json:"description,omitempty"`
	SysObjectIds StringArray       `yaml:"sysobjectid,omitempty" json:"sysobjectid,omitempty"`
	Extends      []string          `yaml:"extends,omitempty" json:"extends,omitempty"`
	Device       DeviceMeta        `yaml:"device,omitempty" json:"device,omitempty"` // DEPRECATED
	Metadata     MetadataConfig    `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	MetricTags   []MetricTagConfig `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`
	StaticTags   []string          `yaml:"static_tags,omitempty" json:"static_tags,omitempty"`
	Metrics      []MetricsConfig   `yaml:"metrics,omitempty" json:"metrics,omitempty"`
}

// DeviceProfileRcConfig represent the profile stored in remote config.
type DeviceProfileRcConfig struct {
	Profile ProfileDefinition `json:"profile_definition"`
}

// NewProfileDefinition creates a new ProfileDefinition
func NewProfileDefinition() *ProfileDefinition {
	p := &ProfileDefinition{}
	p.Metadata = make(MetadataConfig)
	return p
}
