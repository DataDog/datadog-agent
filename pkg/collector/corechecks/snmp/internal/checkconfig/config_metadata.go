// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// LegacyMetadataConfig contains metadata config used for backward compatibility
// When users have their own copy of _base.yaml and _generic_if.yaml files
// they won't have the new profile based metadata definitions for device and interface resources
// The LegacyMetadataConfig is used as fallback to provide metadata definitions for those resources.
var LegacyMetadataConfig = profiledefinition.MetadataConfig{
	"device": {
		Fields: map[string]profiledefinition.MetadataField{
			"description": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.1.0",
					Name: "sysDescr",
				},
			},
			"name": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.5.0",
					Name: "sysName",
				},
			},
			"sys_object_id": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.1.2.0",
					Name: "sysObjectID",
				},
			},
		},
	},
	"interface": {
		Fields: map[string]profiledefinition.MetadataField{
			"name": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.31.1.1.1.1",
					Name: "ifName",
				},
			},
			"description": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2.1.2",
					Name: "ifDescr",
				},
			},
			"admin_status": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2.1.7",
					Name: "ifAdminStatus",
				},
			},
			"oper_status": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2.1.8",
					Name: "ifOperStatus",
				},
			},
			"alias": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.31.1.1.1.18",
					Name: "ifAlias",
				},
			},
			"mac_address": {
				Symbol: profiledefinition.SymbolConfig{
					OID:    "1.3.6.1.2.1.2.2.1.6",
					Name:   "ifPhysAddress",
					Format: "mac_address",
				},
			},
		},
		IDTags: profiledefinition.MetricTagConfigList{
			{
				Tag: "interface",
				Column: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.31.1.1.1.1",
					Name: "ifName",
				},
			},
		},
	},
	"ip_addresses": {
		Fields: map[string]profiledefinition.MetadataField{
			"if_index": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.4.20.1.2",
					Name: "ipAdEntIfIndex",
				},
			},
			"netmask": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.4.20.1.3",
					Name: "ipAdEntNetMask",
				},
			},
		},
	},
}

// TopologyMetadataConfig represent the metadata needed for topology
var TopologyMetadataConfig = profiledefinition.MetadataConfig{
	"lldp_remote": {
		Fields: map[string]profiledefinition.MetadataField{
			"chassis_id_type": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.4",
					Name: "lldpRemChassisIdSubtype",
				},
			},
			"chassis_id": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.5",
					Name: "lldpRemChassisId",
				},
			},
			"interface_id_type": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.6",
					Name: "lldpRemPortIdSubtype",
				},
			},
			"interface_id": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.7",
					Name: "lldpRemPortId",
				},
			},
			"interface_desc": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.8",
					Name: "lldpRemPortDesc",
				},
			},
			"device_name": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.1.1.9",
					Name: "lldpRemSysName",
				},
			},
			"device_desc": {
				Symbol: profiledefinition.SymbolConfig{
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
		Fields: map[string]profiledefinition.MetadataField{
			"interface_id_type": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.4.2.1.3",
					Name: "lldpRemManAddrIfSubtype",
				},
			},
		},
	},
	"lldp_local": {
		Fields: map[string]profiledefinition.MetadataField{
			"interface_id_type": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.3.7.1.2",
					Name: "lldpLocPortIdSubtype",
				},
			},
			"interface_id": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.0.8802.1.1.2.1.3.7.1.3",
					Name: "lldpLocPortID",
				},
			},
		},
	},
	"cdp_remote": {
		Fields: map[string]profiledefinition.MetadataField{
			"device_desc": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.5",
					Name: "cdpCacheVersion",
				},
			},
			"device_id": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.6",
					Name: "cdpCacheDeviceId",
				},
			},
			"interface_id": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.7",
					Name: "cdpCacheDevicePort",
				},
			},
			"device_name": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.17",
					Name: "cdpCacheSysName",
				},
			},
			"device_address_type": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.19",
					Name: "cdpCachePrimaryMgmtAddrType",
				},
			},
			"device_address": {
				Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.20",
					Name: "cdpCachePrimaryMgmtAddr",
				},
			},
			// TODO: Add
			//   - 1.3.6.1.4.1.9.9.23.1.2.1.1.4 cdpCacheAddress
			//   - 1.3.6.1.4.1.9.9.23.1.2.1.1.22  cdpCacheSecondaryMgmtAddrType
			// as backup when cdpCachePrimaryMgmtAddr is not present or cdpCachePrimaryMgmtAddrType is not ip(1)
		},
	},
}

// updateMetadataDefinitionWithDefaults will add metadata config for resources
// that does not have metadata definitions
func updateMetadataDefinitionWithDefaults(metadataConfig profiledefinition.MetadataConfig, collectTopology bool) profiledefinition.MetadataConfig {
	newConfig := make(profiledefinition.MetadataConfig)
	mergeMetadata(newConfig, metadataConfig)
	mergeMetadata(newConfig, LegacyMetadataConfig)
	if collectTopology {
		mergeMetadata(newConfig, TopologyMetadataConfig)
	}
	return newConfig
}

func mergeMetadata(metadataConfig profiledefinition.MetadataConfig, extraMetadata profiledefinition.MetadataConfig) {
	for resourceName, resourceConfig := range extraMetadata {
		if _, ok := metadataConfig[resourceName]; !ok {
			metadataConfig[resourceName] = resourceConfig
		}
	}
}
