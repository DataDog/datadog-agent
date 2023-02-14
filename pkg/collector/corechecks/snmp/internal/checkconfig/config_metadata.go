// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"

// LegacyMetadataConfig contains metadata config used for backward compatibility
// When users have their own copy of _base.yaml and _generic_if.yaml files
// they won't have the new profile based metadata definitions for device and interface resources
// The LegacyMetadataConfig is used as fallback to provide metadata definitions for those resources.
var LegacyMetadataConfig = MetadataConfig{
	"device": {
		Fields: map[string]MetadataField{
			"description": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.1.1.0",
					Name: "sysDescr",
				},
			},
			"name": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.1.5.0",
					Name: "sysName",
				},
			},
			"sys_object_id": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.1.2.0",
					Name: "sysObjectID",
				},
			},
		},
	},
	"interface": {
		Fields: map[string]MetadataField{
			"name": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.31.1.1.1.1",
					Name: "ifName",
				},
			},
			"description": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2.1.2",
					Name: "ifDescr",
				},
			},
			"admin_status": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2.1.7",
					Name: "ifAdminStatus",
				},
			},
			"oper_status": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2.1.8",
					Name: "ifOperStatus",
				},
			},
			"alias": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.31.1.1.1.18",
					Name: "ifAlias",
				},
			},
			"mac_address": {
				Symbol: SymbolConfig{
					OID:    "1.3.6.1.2.1.2.2.1.6",
					Name:   "ifPhysAddress",
					Format: "mac_address",
				},
			},
		},
		IDTags: MetricTagConfigList{
			{
				Tag: "interface",
				Column: SymbolConfig{
					OID:  "1.3.6.1.2.1.31.1.1.1.1",
					Name: "ifName",
				},
			},
		},
	},
	"ip_addresses": {
		Fields: map[string]MetadataField{
			"if_index": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.4.20.1.2",
					Name: "ipAdEntIfIndex",
				},
			},
			"netmask": {
				Symbol: SymbolConfig{
					OID:  "1.3.6.1.2.1.4.20.1.3",
					Name: "ipAdEntNetMask",
				},
			},
		},
	},
}

var TopologyMetadataConfig = MetadataConfig{
	"lldp_remote": {
		Fields: map[string]MetadataField{
			"chassis_id_type": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.4",
					Name: "lldpRemChassisIdSubtype",
				},
			},
			"chassis_id": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.5",
					Name: "lldpRemChassisId",
				},
			},
			"interface_id_type": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.6",
					Name: "lldpRemPortIdSubtype",
				},
			},
			"interface_id": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.7",
					Name: "lldpRemPortId",
				},
			},
			"interface_desc": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.8",
					Name: "lldpRemPortDesc",
				},
			},
			"device_name": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.9",
					Name: "lldpRemSysName",
				},
			},
			"device_desc": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.10",
					Name: "lldpRemSysDesc",
				},
			},
			// TODO: Implement later lldpRemSysCapSupported and lldpRemSysCapEnabled
			//   - 1.0.8802.1.1.2.1.4.1.1.11 lldpRemSysCapSupported
			//   - 1.0.8802.1.1.2.1.4.1.1.12  lldpRemSysCapEnabled
		},
	},
	"lldp_remote_management": {
		Fields: map[string]MetadataField{
			"interface_id_type": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.2.1.3",
					Name: "lldpRemManAddrIfSubtype",
				},
			},
		},
	},
	"lldp_local": {
		Fields: map[string]MetadataField{
			"interface_id_type": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.3.7.1.2",
					Name: "lldpLocPortIdSubtype",
				},
			},
			"interface_id": {
				Symbol: SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.3.7.1.3",
					Name: "lldpLocPortID",
				},
			},
		},
	},
}

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

// newMetadataResourceConfig returns a new metadata resource config
func newMetadataResourceConfig() MetadataResourceConfig {
	return MetadataResourceConfig{}
}

// IsMetadataResourceWithScalarOids returns true if the resource is based on scalar OIDs
// at the moment, we only expect "device" resource to be based on scalar OIDs
func IsMetadataResourceWithScalarOids(resource string) bool {
	return resource == common.MetadataDeviceResource
}

// updateMetadataDefinitionWithDefaults will add metadata config for resources
// that does not have metadata definitions
func updateMetadataDefinitionWithDefaults(metadataConfig MetadataConfig, collectTopology bool) MetadataConfig {
	newConfig := make(MetadataConfig)
	mergeMetadata(newConfig, metadataConfig)
	mergeMetadata(newConfig, LegacyMetadataConfig)
	if collectTopology {
		mergeMetadata(newConfig, TopologyMetadataConfig)
	}
	return newConfig
}

func mergeMetadata(metadataConfig MetadataConfig, extraMetadata MetadataConfig) {
	for resourceName, resourceConfig := range extraMetadata {
		if _, ok := metadataConfig[resourceName]; !ok {
			metadataConfig[resourceName] = resourceConfig
		}
	}
}
