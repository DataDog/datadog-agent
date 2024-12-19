// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package profile

import (
	"path/filepath"
	"regexp"

	"github.com/mohae/deepcopy"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// CopyProfileDefinition copies a profile, it's used for testing
func CopyProfileDefinition(profileDef profiledefinition.ProfileDefinition) profiledefinition.ProfileDefinition {
	return deepcopy.Copy(profileDef).(profiledefinition.ProfileDefinition)
}

// SetConfdPathAndCleanProfiles is used for testing only
func SetConfdPathAndCleanProfiles() {
	SetGlobalProfileConfigMap(nil) // make sure from the new confd path will be reloaded
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "test", "conf.d"))
	}
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join(".", "internal", "test", "conf.d"))
	}
	pkgconfigsetup.Datadog().SetWithoutSource("confd_path", file)
}

// FixtureProfileDefinitionMap returns a fixture of ProfileConfigMap with `f5-big-ip` profile
func FixtureProfileDefinitionMap() ProfileConfigMap {
	metrics := []profiledefinition.MetricsConfig{
		{MIB: "F5-BIGIP-SYSTEM-MIB", Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", Name: "sysStatMemoryTotal", ScaleFactor: 2}, MetricType: profiledefinition.ProfileMetricTypeGauge},
		{MIB: "F5-BIGIP-SYSTEM-MIB", Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.999", Name: "oldSyntax"}},
		{
			MIB: "IF-MIB",
			Table: profiledefinition.SymbolConfig{
				OID:  "1.3.6.1.2.1.2.2",
				Name: "ifTable",
			},
			MetricType: profiledefinition.ProfileMetricTypeMonotonicCount,
			Symbols: []profiledefinition.SymbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors", ScaleFactor: 0.5},
				{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
			},
			MetricTags: []profiledefinition.MetricTagConfig{
				{Tag: "interface", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
				{Tag: "interface_alias", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
				{Tag: "mac_address", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.2.2.1.6", Name: "ifPhysAddress", Format: "mac_address"}},
			},
			StaticTags: []string{"table_static_tag:val"},
		},
		{MIB: "SOME-MIB", Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
	}
	return ProfileConfigMap{
		"f5-big-ip": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics:      metrics,
				Extends:      []string{"_base.yaml", "_generic-if.yaml"},
				Device:       profiledefinition.DeviceMeta{Vendor: "f5"},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
				StaticTags:   []string{"static_tag:from_profile_root", "static_tag:from_base_profile"},
				MetricTags: []profiledefinition.MetricTagConfig{
					{
						Symbol:  profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
						Match:   "(\\w)(\\w+)",
						Pattern: regexp.MustCompile(`(\w)(\w+)`),
						Tags: map[string]string{
							"some_tag": "some_tag_value",
							"prefix":   "\\1",
							"suffix":   "\\2",
						},
					},
					{Tag: "snmp_host", Index: 0x0, Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
				},
				Metadata: profiledefinition.MetadataConfig{
					Device: profiledefinition.DeviceMetadata{
						Fields: profiledefinition.DeviceMetadataFields{
							Name: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
							Description: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
							Vendor: profiledefinition.MetadataField{
								Value: "f5",
							},
							SerialNumber: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.3375.2.1.3.3.3.0",
									Name: "sysGeneralChassisSerialNum",
								},
							},
							Type: profiledefinition.MetadataField{
								Value: "load_balancer",
							},
							SysObjectId: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.2.0",
									Name: "sysObjectID",
								},
							},
						},
					},
					Interface: profiledefinition.InterfaceMetadata{
						Fields: profiledefinition.InterfaceMetadataFields{
							Name: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
							Alias: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1.1.18",
									Name: "ifAlias",
								},
							},
							Description: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:                  "1.3.6.1.2.1.31.1.1.1.1",
									Name:                 "ifName",
									ExtractValue:         "(Row\\d)",
									ExtractValueCompiled: regexp.MustCompile(`(Row\d)`),
								},
							},
							MacAddress: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:    "1.3.6.1.2.1.2.2.1.6",
									Name:   "ifPhysAddress",
									Format: "mac_address",
								},
							},
							AdminStatus: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2.1.7",
									Name: "ifAdminStatus",
								},
							},
							OperStatus: profiledefinition.MetadataField{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
								},
							},
						},
						IDTags: profiledefinition.MetricTagConfigList{
							{
								Tag: "custom-tag",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifAlias",
								},
							},
							{
								Tag: "interface",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
						},
					},
				},
			},
			IsUserProfile: true,
		},
		"another_profile": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.32473.1.1"},
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.999.0", Name: "anotherMetric"}, MetricType: ""},
				},
				MetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "snmp_host2", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
					{Tag: "unknown_symbol", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.999.0", Name: "unknownSymbol"}},
				},
				Metadata: profiledefinition.MetadataConfig{},
			},
			IsUserProfile: true,
		},
	}
}
