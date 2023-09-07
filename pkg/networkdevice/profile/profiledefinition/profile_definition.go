// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

// DeviceMeta holds device related static metadata
// DEPRECATED in favour of profile metadata syntax
type DeviceMeta struct {
	// deprecated in favour of new `ProfileDefinition.Metadata` syntax
	Vendor string `yaml:"vendor" json:"vendor"`
}

type ProfileDefinition struct {
	Name         string            `yaml:"name" json:"name"`
	SysObjectIds StringArray       `yaml:"sysobjectid" json:"sysobjectid"`
	Device       DeviceMeta        `yaml:"device" json:"device"` // DEPRECATED
	Metrics      []MetricsConfig   `yaml:"metrics" json:"metrics"`
	Metadata     MetadataConfig    `yaml:"metadata" json:"metadata"`
	MetricTags   []MetricTagConfig `yaml:"metric_tags" json:"metric_tags"`
	StaticTags   []string          `yaml:"static_tags" json:"static_tags"`
	Extends      []string          `yaml:"extends" json:"extends"`
}

// DeviceProfileRcConfig represent the profile stored in remote config.
type DeviceProfileRcConfig struct {
	Profile ProfileDefinition `json:"profile_definition"`
}

func NewProfileDefinition() *ProfileDefinition {
	p := &ProfileDefinition{}
	p.Metadata = make(MetadataConfig)
	return p
}
