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

// ProfileDefinition is the root profile structure. The ProfileDefinition is currently used in:
// 1/ SNMP Integration: the profiles are in yaml profiles. Yaml profiles include default datadog profiles and user custom profiles.
// The serialisation of yaml profiles are defined by the yaml annotation and few custom unmarshaller (see yaml_utils.go).
// 2/ Datadog backend: the profiles are in json format, they are used to store profiles created via UI.
// The serialisation of json profiles are defined by the json annotation.
type ProfileDefinition struct {
	Name         string            `yaml:"name" json:"name"`
	Description  string            `yaml:"description,omitempty" json:"description,omitempty"`
	SysObjectIds StringArray       `yaml:"sysobjectid,omitempty" json:"sysobjectid,omitempty"`
	Extends      []string          `yaml:"extends,omitempty" json:"extends,omitempty"`
	Metadata     MetadataConfig    `yaml:"metadata,omitempty" json:"metadata,omitempty" jsonschema:"-"`
	MetricTags   []MetricTagConfig `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`
	StaticTags   []string          `yaml:"static_tags,omitempty" json:"static_tags,omitempty"`
	Metrics      []MetricsConfig   `yaml:"metrics,omitempty" json:"metrics,omitempty"`

	// Used previously to pass device vendor field (has been replaced by Metadata).
	// Used in RC for passing device vendor field.
	Device DeviceMeta `yaml:"device,omitempty" json:"device,omitempty" jsonschema:"device,omitempty"` // DEPRECATED
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
