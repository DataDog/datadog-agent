package cprofstruct

import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"

// MetadataConfig holds configs per resource type
type MetadataConfig map[string]MetadataResourceConfig

// MetadataResourceConfig holds configs for a metadata resource
type MetadataResourceConfig struct {
	Fields map[string]MetadataField `yaml:"fields"`
	IDTags MetricTagConfigList      `yaml:"id_tags"`
}

// MetadataField holds configs for a metadata field
type MetadataField struct {
	Symbol  SymbolConfig   `yaml:"symbol"`
	Symbols []SymbolConfig `yaml:"symbols"`
	Value   string         `yaml:"value"`
}

// NewMetadataResourceConfig returns a new metadata resource config
func NewMetadataResourceConfig() MetadataResourceConfig {
	return MetadataResourceConfig{}
}

// IsMetadataResourceWithScalarOids returns true if the resource is based on scalar OIDs
// at the moment, we only expect "device" resource to be based on scalar OIDs
func IsMetadataResourceWithScalarOids(resource string) bool {
	return resource == common.MetadataDeviceResource
}
