// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

// DeviceMeta holds device related static metadata
// DEPRECATED in favour of profile metadata syntax
type DeviceMeta struct {
	// deprecated in favour of new `ProfileDefinition.Metadata` syntax
	Vendor string `yaml:"vendor"`
}

type ProfileDefinition struct {
	Metrics      []MetricsConfig   `yaml:"metrics"`
	Metadata     MetadataConfig    `yaml:"metadata"`
	MetricTags   []MetricTagConfig `yaml:"metric_tags"`
	StaticTags   []string          `yaml:"static_tags"`
	Extends      []string          `yaml:"extends"`
	Device       DeviceMeta        `yaml:"device"`
	SysObjectIds StringArray       `yaml:"sysobjectid"`
}

func NewProfileDefinition() *ProfileDefinition {
	p := &ProfileDefinition{}
	p.Metadata = make(MetadataConfig)
	return p
}
