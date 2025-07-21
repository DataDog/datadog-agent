// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package buildprofile

import (
	"fmt"
	"maps"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/gosnmp/gosnmp"
)

// DefaultMetadataConfig holds default configs per resource type
type DefaultMetadataConfig profiledefinition.ListMap[DefaultMetadataResourceConfig]

// DefaultMetadataResourceConfig holds default configs for a metadata resource
type DefaultMetadataResourceConfig struct {
	ShouldMergeMetadata func(sess session.Session, validConnection bool, config *checkconfig.CheckConfig) bool
	Fields              profiledefinition.ListMap[profiledefinition.MetadataField]
	IDTags              profiledefinition.MetricTagConfigList
}

// ToMetadataConfig converts DefaultMetadataConfig to profiledefinition.MetadataConfig
func (dmc DefaultMetadataConfig) ToMetadataConfig() profiledefinition.MetadataConfig {
	metadataConfig := make(profiledefinition.MetadataConfig)
	for resourceName, resourceConfig := range dmc {
		metadataConfig[resourceName] = resourceConfig.ToMetadataResourceConfig()
	}
	return metadataConfig
}

// ToMetadataResourceConfig converts DefaultMetadataResourceConfig to profiledefinition.MetadataResourceConfig
func (dmrc DefaultMetadataResourceConfig) ToMetadataResourceConfig() profiledefinition.MetadataResourceConfig {
	return profiledefinition.MetadataResourceConfig{
		Fields: dmrc.Fields,
		IDTags: dmrc.IDTags,
	}
}

// DefaultMetadataConfigs contains the default metadata collected on a device
var DefaultMetadataConfigs = []DefaultMetadataConfig{
	// LegacyMetadataConfig contains metadata config used for backward compatibility
	// When users have their own copy of _base.yaml and _generic_if.yaml files
	// they won't have the new profile based metadata definitions for device and interface resources
	// The LegacyMetadataConfig is used as fallback to provide metadata definitions for those resources.
	{
		"device": {
			ShouldMergeMetadata: func(_ session.Session, _ bool, _ *checkconfig.CheckConfig) bool {
				return true
			},
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
			ShouldMergeMetadata: func(_ session.Session, _ bool, _ *checkconfig.CheckConfig) bool {
				return true
			},
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
					Symbol: profiledefinition.SymbolConfigCompat{
						OID:  "1.3.6.1.2.1.31.1.1.1.1",
						Name: "ifName",
					},
				},
			},
		},
		"ip_addresses": {
			ShouldMergeMetadata: func(_ session.Session, _ bool, _ *checkconfig.CheckConfig) bool {
				return true
			},
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
	},

	// Topology metadata
	{
		"lldp_remote": {
			ShouldMergeMetadata: func(_ session.Session, _ bool, config *checkconfig.CheckConfig) bool {
				return config.CollectTopology
			},
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
			ShouldMergeMetadata: func(_ session.Session, _ bool, config *checkconfig.CheckConfig) bool {
				return config.CollectTopology
			},
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
			ShouldMergeMetadata: func(_ session.Session, _ bool, config *checkconfig.CheckConfig) bool {
				return config.CollectTopology
			},
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
			ShouldMergeMetadata: func(_ session.Session, _ bool, config *checkconfig.CheckConfig) bool {
				return config.CollectTopology
			},
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
				"device_secondary_address_type": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.21",
						Name: "cdpCacheSecondaryMgmtAddrType",
					},
				},
				"device_secondary_address": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.22",
						Name: "cdpCacheSecondaryMgmtAddr",
					},
				},
				"device_cache_address_type": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.3",
						Name: "cdpCacheAddressType",
					},
				},
				"device_cache_address": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.9.9.23.1.2.1.1.4",
						Name: "cdpCacheAddress",
					},
				},
			},
		},
	},

	// VPN tunnels metadata
	{
		"cisco_ipsec_tunnel": {
			ShouldMergeMetadata: func(sess session.Session, validConnection bool, config *checkconfig.CheckConfig) bool {
				return config.CollectVPN &&
					checkOidIfConnectedOrSkip(sess, validConnection, "1.3.6.1.4.1.9.9.171.1.3.2.1.4")
			},
			Fields: map[string]profiledefinition.MetadataField{
				"local_outside_ip": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.9.9.171.1.3.2.1.4",
						Name: "cipSecTunLocalAddr",
					},
				},
				"remote_outside_ip": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.4.1.9.9.171.1.3.2.1.5",
						Name: "cipSecTunRemoteAddr",
					},
				},
			},
		},
	},

	// Route table metadata needed for VPN tunnels
	{
		"ipforward_deprecated": {
			ShouldMergeMetadata: func(sess session.Session, validConnection bool, config *checkconfig.CheckConfig) bool {
				return config.CollectVPN &&
					checkOidIfConnectedOrSkip(sess, validConnection, "1.3.6.1.2.1.4.24.4.1.5")
			},
			Fields: map[string]profiledefinition.MetadataField{
				"if_index": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.4.24.4.1.5",
						Name: "ipCidrRouteIfIndex",
					},
				},
				"route_status": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.4.24.4.1.16",
						Name: "ipCidrRouteStatus",
					},
				},
			},
		},
		"ipforward": {
			ShouldMergeMetadata: func(sess session.Session, validConnection bool, config *checkconfig.CheckConfig) bool {
				return config.CollectVPN &&
					checkOidIfConnectedOrSkip(sess, validConnection, "1.3.6.1.2.1.4.24.7.1.7")
			},
			Fields: map[string]profiledefinition.MetadataField{
				"if_index": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.4.24.7.1.7",
						Name: "inetCidrRouteIfIndex",
					},
				},
				"route_status": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.4.24.7.1.17",
						Name: "inetCidrRouteStatus",
					},
				},
			},
		},
	},

	// Tunnel metadata needed for VPN tunnels
	{
		"tunnel_config_deprecated": {
			ShouldMergeMetadata: func(sess session.Session, validConnection bool, config *checkconfig.CheckConfig) bool {
				return config.CollectVPN &&
					checkOidIfConnectedOrSkip(sess, validConnection, "1.3.6.1.2.1.10.131.1.1.2.1.5")
			},
			Fields: map[string]profiledefinition.MetadataField{
				"if_index": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.10.131.1.1.2.1.5",
						Name: "tunnelConfigIfIndex",
					},
				},
			},
		},
		"tunnel_config": {
			ShouldMergeMetadata: func(sess session.Session, validConnection bool, config *checkconfig.CheckConfig) bool {
				return config.CollectVPN &&
					checkOidIfConnectedOrSkip(sess, validConnection, "1.3.6.1.2.1.10.131.1.1.3.1.6")
			},
			Fields: map[string]profiledefinition.MetadataField{
				"if_index": {
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.10.131.1.1.3.1.6",
						Name: "tunnelInetConfigIfIndex",
					},
				},
			},
		},
	},
}

// checkOidIfConnectedOrSkip checks whether a given OID is present on the device only when validConnection is true
// If validConnection is false, it returns true by default
func checkOidIfConnectedOrSkip(sess session.Session, validConnection bool, oid string) bool {
	if !validConnection {
		return true
	}
	return checkOid(sess, oid)
}

// checkOid checks whether a given OID is present on the device
func checkOid(sess session.Session, oid string) bool {
	result, err := sess.GetNext([]string{oid})
	if err != nil {
		return false
	}

	if len(result.Variables) != 1 {
		return false
	}

	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("RESULT")
	fmt.Println(result)
	fmt.Println("VARIABLES")
	fmt.Println(result.Variables)
	fmt.Println("LEN VARIABLES")
	fmt.Println(len(result.Variables))
	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("========================")

	snmpPDU := result.Variables[0]
	if snmpPDU.Type == gosnmp.NoSuchObject ||
		snmpPDU.Type == gosnmp.NoSuchInstance ||
		snmpPDU.Type == gosnmp.EndOfMibView {
		return false
	}

	// We check that the returned full OID is equal or starts with the parameter OID because
	// SNMP GETNEXT command returns the next OID whether it's from the same OID or not
	nextOID := strings.TrimPrefix(snmpPDU.Name, ".")
	return nextOID == oid || strings.HasPrefix(nextOID, oid+".")
}

// updateMetadataDefinitionWithDefaults will add metadata config for resources that does not have metadata definitions
func updateMetadataDefinitionWithDefaults(metadataConfig profiledefinition.MetadataConfig, sess session.Session, validConnection bool, config *checkconfig.CheckConfig) profiledefinition.MetadataConfig {
	newMetadataConfig := maps.Clone(metadataConfig)
	if newMetadataConfig == nil {
		newMetadataConfig = make(profiledefinition.MetadataConfig)
	}

	for _, defaultMetadataConfig := range DefaultMetadataConfigs {
		mergeMetadata(newMetadataConfig, defaultMetadataConfig, sess, validConnection, config)
	}

	return newMetadataConfig
}

func mergeMetadata(metadataConfig profiledefinition.MetadataConfig, extraMetadata DefaultMetadataConfig, sess session.Session, validConnection bool, config *checkconfig.CheckConfig) {
	for resourceName, resourceConfig := range extraMetadata {
		if resourceConfig.ShouldMergeMetadata == nil {
			// This is to force always having to specify the ShouldMergeMetadata function, else the Agent will panic
			panic(fmt.Sprintf("function ShouldMergeMetadata needs to be specified in default metadata config for resource: %s",
				resourceName))
		}

		if !resourceConfig.ShouldMergeMetadata(sess, validConnection, config) {
			continue
		}

		if _, exists := metadataConfig[resourceName]; !exists {
			metadataConfig[resourceName] = resourceConfig.ToMetadataResourceConfig()
		}
	}
}
