// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

// MetadataDeviceResource is the device resource name
const MetadataDeviceResource = "device"

// MetadataConfig holds configs per resource type
type MetadataConfig map[string]MetadataResourceConfig

// MetadataResourceConfig holds configs for a metadata resource
type MetadataResourceConfig struct {
	Fields map[string]MetadataField `yaml:"fields" json:"fields"`
	IDTags MetricTagConfigList      `yaml:"id_tags" json:"id_tags"`
}

// MetadataField holds configs for a metadata field
type MetadataField struct {
	Symbol  SymbolConfig   `yaml:"symbol" json:"symbol"`
	Symbols []SymbolConfig `yaml:"symbols" json:"symbols"`
	Value   string         `yaml:"value" json:"value"`
}

// NewMetadataResourceConfig returns a new metadata resource config
func NewMetadataResourceConfig() MetadataResourceConfig {
	return MetadataResourceConfig{}
}

// IsMetadataResourceWithScalarOids returns true if the resource is based on scalar OIDs
// at the moment, we only expect "device" resource to be based on scalar OIDs
func IsMetadataResourceWithScalarOids(resource string) bool {
	return resource == MetadataDeviceResource
}
