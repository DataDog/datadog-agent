// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

// MetadataDeviceResource is the device resource name
const MetadataDeviceResource = "device"

// MetadataConfig holds metadata config per resource type
type MetadataConfig struct {
	Device    DeviceMetadata    `yaml:"device,omitempty" json:"device,omitempty"`
	Interface InterfaceMetadata `yaml:"interface,omitempty" json:"interface,omitempty"`
}

// DeviceMetadata holds device metadata
type DeviceMetadata struct {
	Fields DeviceMetadataFields `yaml:"fields,omitempty" json:"fields,omitempty"`
}

// DeviceMetadataFields holds device metadata fields
type DeviceMetadataFields struct {
	Name         MetadataField `yaml:"name,omitempty" json:"name,omitempty"`
	Description  MetadataField `yaml:"description,omitempty" json:"description,omitempty"`
	Location     MetadataField `yaml:"location,omitempty" json:"location,omitempty"`
	Vendor       MetadataField `yaml:"vendor,omitempty" json:"vendor,omitempty"`
	SerialNumber MetadataField `yaml:"serial_number,omitempty" json:"serial_number,omitempty"`
	Version      MetadataField `yaml:"version,omitempty" json:"version,omitempty"`
	ProductName  MetadataField `yaml:"product_name,omitempty" json:"product_name,omitempty"`
	Model        MetadataField `yaml:"model,omitempty" json:"model,omitempty"`
	OsName       MetadataField `yaml:"os_name,omitempty" json:"os_name,omitempty"`
	OsVersion    MetadataField `yaml:"os_version,omitempty" json:"os_version,omitempty"`
	OsHostname   MetadataField `yaml:"os_hostname,omitempty" json:"os_hostname,omitempty"`
	DeviceType   MetadataField `yaml:"type,omitempty" json:"type,omitempty"`
}

// InterfaceMetadata holds interface metadata
type InterfaceMetadata struct {
	Fields string              `yaml:"fields,omitempty" json:"fields,omitempty"`
	IDTags MetricTagConfigList `yaml:"id_tags,omitempty" json:"id_tags,omitempty"`
}

// InterfaceMetadataFields holds interface metadata fields
type InterfaceMetadataFields struct {
	Name        MetadataField `yaml:"name,omitempty" json:"name,omitempty"`
	Alias       MetadataField `yaml:"alias,omitempty" json:"alias,omitempty"`
	Description MetadataField `yaml:"description,omitempty" json:"description,omitempty"`
	MacAddress  MetadataField `yaml:"mac_address,omitempty" json:"mac_address,omitempty"` // add mac_Address format
	AdminStatus MetadataField `yaml:"admin_status,omitempty" json:"admin_status,omitempty"`
	OperStatus  MetadataField `yaml:"oper_status,omitempty" json:"oper_status,omitempty"`
}

// MetadataResourceConfig holds configs for a metadata resource
type MetadataResourceConfig struct {
	Fields map[string]MetadataField `yaml:"fields" json:"fields"`
	IDTags MetricTagConfigList      `yaml:"id_tags,omitempty" json:"id_tags,omitempty"`
}

// MetadataField holds configs for a metadata field
type MetadataField struct {
	Symbol  SymbolConfig   `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Symbols []SymbolConfig `yaml:"symbols,omitempty" json:"symbols,omitempty"`
	Value   string         `yaml:"value,omitempty" json:"value,omitempty"`
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
