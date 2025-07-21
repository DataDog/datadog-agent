// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package buildprofile

import (
	"maps"
	"regexp"
	"testing"
	"time"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildProfile(t *testing.T) {
	metrics := []profiledefinition.MetricsConfig{
		{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
		{
			Symbols: []profiledefinition.SymbolConfig{
				{
					OID:  "1.2.3.4.6",
					Name: "abc",
				},
			},
			MetricTags: profiledefinition.MetricTagConfigList{
				profiledefinition.MetricTagConfig{
					Symbol: profiledefinition.SymbolConfigCompat{
						OID: "1.2.3.4.7",
					},
				},
			},
		},
	}
	profile1 := profiledefinition.ProfileDefinition{
		Name:    "profile1",
		Version: 12,
		Metrics: metrics,
		MetricTags: []profiledefinition.MetricTagConfig{
			{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
		},
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"vendor": {
						Value: "a-vendor",
					},
					"description": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.1.99.3.0",
							Name: "sysDescr",
						},
					},
					"name": {
						Symbols: []profiledefinition.SymbolConfig{
							{
								OID:  "1.3.6.1.2.1.1.99.1.0",
								Name: "symbol1",
							},
							{
								OID:  "1.3.6.1.2.1.1.99.2.0",
								Name: "symbol2",
							},
						},
					},
				},
			},
			"interface": {
				Fields: map[string]profiledefinition.MetadataField{
					"oper_status": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.99",
							Name: "someIfSymbol",
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
		},
		SysObjectIDs: profiledefinition.StringArray{"1.1.1.*"},
	}

	legacyMetadataConfig := DefaultMetadataConfigs[0].ToMetadataConfig()
	profile1MergedMetadata := maps.Clone(profile1.Metadata)
	profile1MergedMetadata["ip_addresses"] = legacyMetadataConfig["ip_addresses"]

	mockProfiles := profile.StaticProvider(profile.ProfileConfigMap{
		"profile1": profile.ProfileConfig{
			Definition: profile1,
		},
	})

	tests := []struct {
		name                   string
		sessionFactory         session.Factory
		config                 *checkconfig.CheckConfig
		sysObjectID            string
		expectedProfileBuilder func() profiledefinition.ProfileDefinition
		expectedError          string
	}{
		{
			name: "inline",
			config: &checkconfig.CheckConfig{
				IPAddress:        "1.2.3.4",
				RequestedMetrics: metrics,
				RequestedMetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
				},
				ProfileName: checkconfig.ProfileNameInline,
			},
			expectedProfileBuilder: func() profiledefinition.ProfileDefinition {
				return profiledefinition.ProfileDefinition{
					Metrics: metrics,
					MetricTags: []profiledefinition.MetricTagConfig{
						{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
					},
					Metadata: legacyMetadataConfig,
				}
			},
		},
		{
			name: "static",
			config: &checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     "profile1",
			},
			expectedProfileBuilder: func() profiledefinition.ProfileDefinition {
				return profiledefinition.ProfileDefinition{
					Name:    "profile1",
					Version: 12,
					Metrics: metrics,
					MetricTags: []profiledefinition.MetricTagConfig{
						{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
					},
					StaticTags: []string{"snmp_profile:profile1"},
					Metadata:   profile1MergedMetadata,
				}
			},
		},
		{
			name: "dynamic",
			config: &checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     checkconfig.ProfileNameAuto,
			},
			sysObjectID: "1.1.1.1",
			expectedProfileBuilder: func() profiledefinition.ProfileDefinition {
				return profiledefinition.ProfileDefinition{
					Name:    "profile1",
					Version: 12,
					Metrics: metrics,
					MetricTags: []profiledefinition.MetricTagConfig{
						{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
					},
					StaticTags: []string{"snmp_profile:profile1"},
					Metadata:   profile1MergedMetadata,
				}
			},
		},
		{
			name: "static with requested metrics",
			config: &checkconfig.CheckConfig{
				IPAddress:             "1.2.3.4",
				ProfileProvider:       mockProfiles,
				CollectDeviceMetadata: true,
				CollectTopology:       false,
				ProfileName:           "profile1",
				RequestedMetrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "3.1", Name: "global-metric"}}},
				RequestedMetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "global-tag", Symbol: profiledefinition.SymbolConfigCompat{OID: "3.2", Name: "globalSymbol"}},
				},
			},
			expectedProfileBuilder: func() profiledefinition.ProfileDefinition {
				return profiledefinition.ProfileDefinition{
					Name:    "profile1",
					Version: 12,
					Metrics: append([]profiledefinition.MetricsConfig{
						{Symbol: profiledefinition.SymbolConfig{OID: "3.1", Name: "global-metric"}}},
						metrics...),
					MetricTags: []profiledefinition.MetricTagConfig{
						{Tag: "global-tag", Symbol: profiledefinition.SymbolConfigCompat{OID: "3.2", Name: "globalSymbol"}},
						{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
					},
					StaticTags: []string{"snmp_profile:profile1"},
					Metadata:   profile1MergedMetadata,
				}
			},
		},
		{
			name: "static unknown",
			config: &checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     "f5",
			},
			expectedError: "unknown profile \"f5\"",
		},
		{
			name: "dynamic unknown",
			config: &checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     checkconfig.ProfileNameAuto,
			},
			sysObjectID: "3.3.3.3",
			expectedError: "failed to get profile for sysObjectID \"3.3.3.3\": no profiles found for sysObjectID \"3." +
				"3.3.3\"",
		},
		{
			name: "VPN tunnels metadata with invalid session",
			config: &checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     "profile1",
				CollectVPN:      true,
			},
			expectedProfileBuilder: func() profiledefinition.ProfileDefinition {
				vpnTunnelsMergedMetadata := maps.Clone(profile1MergedMetadata)
				vpnTunnelsMergedMetadata["cisco_ipsec_tunnel"] = DefaultMetadataConfigs[2].ToMetadataConfig()["cisco_ipsec_tunnel"]
				vpnTunnelsMergedMetadata["ipforward_deprecated"] = DefaultMetadataConfigs[3].ToMetadataConfig()["ipforward_deprecated"]
				vpnTunnelsMergedMetadata["ipforward"] = DefaultMetadataConfigs[3].ToMetadataConfig()["ipforward"]
				vpnTunnelsMergedMetadata["tunnel_config_deprecated"] = DefaultMetadataConfigs[4].ToMetadataConfig()["tunnel_config_deprecated"]
				vpnTunnelsMergedMetadata["tunnel_config"] = DefaultMetadataConfigs[4].ToMetadataConfig()["tunnel_config"]

				return profiledefinition.ProfileDefinition{
					Name:    "profile1",
					Version: 12,
					Metrics: metrics,
					MetricTags: []profiledefinition.MetricTagConfig{
						{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
					},
					StaticTags: []string{"snmp_profile:profile1"},
					Metadata:   vpnTunnelsMergedMetadata,
				}
			},
		},
		{
			name: "VPN tunnels metadata with valid session",
			sessionFactory: func(*checkconfig.CheckConfig) (session.Session, error) {
				sess := session.CreateFakeSession()
				sess.
					SetByte("1.3.6.1.4.1.9.9.171.1.3.2.1.4.2", []byte{0x0A, 0x00, 0x00, 0x01}).
					SetInt("1.3.6.1.2.1.4.24.7.1.7.2.16.255.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.2.0.0.0.0", 2).
					SetInt("1.3.6.1.2.1.10.131.1.1.3.1.6.1.4.10.0.2.91.4.34.230.217.35.1.1", 6)
				return sess, nil
			},
			config: &checkconfig.CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     "profile1",
				CollectVPN:      true,
			},
			expectedProfileBuilder: func() profiledefinition.ProfileDefinition {
				vpnTunnelsMergedMetadata := maps.Clone(profile1MergedMetadata)
				vpnTunnelsMergedMetadata["cisco_ipsec_tunnel"] = DefaultMetadataConfigs[2].ToMetadataConfig()["cisco_ipsec_tunnel"]
				vpnTunnelsMergedMetadata["ipforward"] = DefaultMetadataConfigs[3].ToMetadataConfig()["ipforward"]
				vpnTunnelsMergedMetadata["tunnel_config"] = DefaultMetadataConfigs[4].ToMetadataConfig()["tunnel_config"]

				return profiledefinition.ProfileDefinition{
					Name:    "profile1",
					Version: 12,
					Metrics: metrics,
					MetricTags: []profiledefinition.MetricTagConfig{
						{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
					},
					StaticTags: []string{"snmp_profile:profile1"},
					Metadata:   vpnTunnelsMergedMetadata,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var validConnection bool
			var sess session.Session
			var err error

			if tt.sessionFactory != nil {
				validConnection = true
				sess, err = tt.sessionFactory(tt.config)
				assert.NoError(t, err)
			}

			profileDef, err := BuildProfile(tt.sysObjectID, sess, validConnection, tt.config)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)

				expectedProfile := tt.expectedProfileBuilder()
				if !assert.Equal(t, expectedProfile, profileDef) {
					for k, v := range expectedProfile.Metadata["device"].Fields {
						t.Log(k, v)
					}
					t.Log("===")
					for k, v := range profileDef.Metadata["device"].Fields {
						t.Log(k, v)
					}
				}
			}
		})
	}
}

func TestProfileConfig(t *testing.T) {
	profile.SetConfdPathAndCleanProfiles()
	aggregator.NewBufferedAggregator(nil, nil, nil, nooptagger.NewComponent(), "", 1*time.Hour)

	tests := []struct {
		name               string
		rawInstanceConfig  []byte
		rawInitConfig      []byte
		expectedMetrics    []profiledefinition.MetricsConfig
		expectedMetricTags []profiledefinition.MetricTagConfig
		expectedStaticTags []string
	}{
		{
			name: "TestConfigurations",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
port: 1161
timeout: 7
retries: 5
snmp_version: 2c
user: my-user
authProtocol: sha
authKey: my-authKey
privProtocol: aes
privKey: my-privKey
context_name: my-contextName
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: ifNumber
- OID: 1.3.6.1.2.1.2.2
  name: ifNumber2
  metric_tags:
  - mytag1
  - mytag2
- symbol:
    OID: 1.3.6.1.4.1.318.1.1.1.11.1.1.0
    name: upsBasicStateOutputState
    scale_factor: 10
  metric_type: flag_stream
  options:
    placement: 5
    metric_suffix: ReplaceBattery
- table:
    OID: 1.3.6.1.2.1.2.2
    name: ifTable
  symbols:
  - OID: 1.3.6.1.2.1.2.2.1.14
    name: ifInErrors
  - OID: 1.3.6.1.2.1.2.2.1.20
    name: ifOutErrors
    scale_factor: 3
  metric_tags:
  - tag: if_index
    index: 1
  - tag: if_desc
    column:
      OID: 1.3.6.1.2.1.2.2.1.2
      name: ifDescr
    index_transform:
      - start: 1
        end: 3
      - start: 4
        end: 6
  - index: 1
    tag: ipversion
    mapping:
      0: unknown
      1: ipv4
      2: ipv6
      3: ipv4z
      4: ipv6z
      16: dns
  - tag: if_type
    column:
      OID: 1.3.6.1.2.1.2.2.1.3
      name: ifType
    mapping:
      1: other
      2: regular1822
      3: hdh1822
      4: ddn-x25
      29: ultra
  - column:
      OID: '1.2.3.4.8.1.2'
      name: 'cpiPduName'
    match: '(\w)(\w+)'
    tags:
      prefix: '\1'
      suffix: '\2'
metric_tags:
  - OID: 1.2.3
    symbol: mySymbol
    tag: my_symbol
  - OID: 1.2.3
    symbol: mySymbol
    tag: my_symbol_mapped
    mapping:
      1: one
      2: two
  - OID: 1.2.3
    symbol: mySymbol
    match: '(\w)(\w+)'
    tags:
      prefix: '\1'
      suffix: '\2'
profile: f5-big-ip
tags:
  - tag1
  - tag2:val2
  - autodiscovery_subnet:127.0.0.0/30
`),
			// language=yaml
			rawInitConfig: []byte(`
profiles:
  f5-big-ip:
    definition_file: f5-big-ip.yaml
global_metrics:
- symbol:
    OID: 1.2.3.4
    name: aGlobalMetric
oid_batch_size: 10
bulk_max_repetitions: 20
`),
			expectedMetrics: append(append([]profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "ifNumber"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.2", Name: "ifNumber2"}, MetricTags: profiledefinition.MetricTagConfigList{
					{SymbolTag: "mytag1"},
					{SymbolTag: "mytag2"},
				}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.318.1.1.1.11.1.1.0", Name: "upsBasicStateOutputState", ScaleFactor: 10}, MetricType: profiledefinition.ProfileMetricTypeFlagStream, Options: profiledefinition.MetricsConfigOption{Placement: 5, MetricSuffix: "ReplaceBattery"}},
				{
					Table: profiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.2.2",
						Name: "ifTable",
					},
					Symbols: []profiledefinition.SymbolConfig{
						// ifInErrors defined in instance config with a different set of metric tags from the one defined
						// in the imported profile
						{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
						{OID: "1.3.6.1.2.1.2.2.1.20", Name: "ifOutErrors", ScaleFactor: 3},
					},
					MetricTags: []profiledefinition.MetricTagConfig{
						{Tag: "if_index", Index: 1},
						{Tag: "if_desc", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr"},
							IndexTransform: []profiledefinition.MetricIndexTransform{
								{
									Start: 1,
									End:   3,
								},
								{
									Start: 4,
									End:   6,
								},
							},
						},
						{Tag: "ipversion", Index: 1, Mapping: map[string]string{
							"0":  "unknown",
							"1":  "ipv4",
							"2":  "ipv6",
							"3":  "ipv4z",
							"4":  "ipv6z",
							"16": "dns",
						}},
						{Tag: "if_type",
							Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.2.2.1.3", Name: "ifType"},
							Mapping: map[string]string{
								"1":  "other",
								"2":  "regular1822",
								"3":  "hdh1822",
								"4":  "ddn-x25",
								"29": "ultra",
							}},
						{
							Symbol: profiledefinition.SymbolConfigCompat{
								Name: "cpiPduName",
								OID:  "1.2.3.4.8.1.2",
							},
							Match:   "(\\w)(\\w+)",
							Pattern: regexp.MustCompile(`(\w)(\w+)`),
							Tags: map[string]string{
								"prefix": "\\1",
								"suffix": "\\2",
							}},
					},
				},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4", Name: "aGlobalMetric"}},
			},
				profiledefinition.MetricsConfig{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}}),
				profile.FixtureProfileDefinitionMap()["f5-big-ip"].Definition.Metrics...),
			expectedMetricTags: []profiledefinition.MetricTagConfig{
				{Tag: "my_symbol", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.2.3", Name: "mySymbol"}},
				{Tag: "my_symbol_mapped", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.2.3", Name: "mySymbol"}, Mapping: map[string]string{"1": "one", "2": "two"}},
				{
					Symbol:  profiledefinition.SymbolConfigCompat{OID: "1.2.3", Name: "mySymbol"},
					Match:   "(\\w)(\\w+)",
					Pattern: regexp.MustCompile(`(\w)(\w+)`),
					Tags: map[string]string{
						"prefix": "\\1",
						"suffix": "\\2",
					},
				},
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
				{Tag: "snmp_host", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
			},
			expectedStaticTags: []string{
				"snmp_profile:f5-big-ip",
				"device_vendor:f5",
				"static_tag:from_profile_root",
				"static_tag:from_base_profile"},
		},
		{
			name: "TestProfileNormalizeMetrics",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 172.26.0.2
profile: profile1
community_string: public
`),
			// language=yaml
			rawInitConfig: []byte(`
profiles:
  profile1:
    definition:
      metrics:
        - {OID: 1.3.6.1.2.1.7.1.0, name: IAmACounter32}
        - {OID: 1.3.6.1.2.1.4.31.1.1.6.1, name: IAmACounter64}
        - {OID: 1.3.6.1.2.1.4.24.6.0, name: IAmAGauge32}
        - {OID: 1.3.6.1.2.1.88.1.1.1.0, name: IAmAnInteger}
`),
			expectedMetrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.7.1.0", Name: "IAmACounter32"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.4.31.1.1.6.1", Name: "IAmACounter64"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.4.24.6.0", Name: "IAmAGauge32"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.88.1.1.1.0", Name: "IAmAnInteger"}},
			},
			expectedStaticTags: []string{
				"snmp_profile:profile1",
			},
		},
		{
			name: "TestInlineProfileConfiguration",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
snmp_version: 2c
profile: inline-profile
community_string: '123'
`),
			// language=yaml
			rawInitConfig: []byte(`
profiles:
  f5-big-ip:
    definition_file: f5-big-ip.yaml
  inline-profile:
    definition:
      device:
        vendor: "f5"
      sysobjectid: 1.2.3
      metric_tags:
        - OID: 1.3.6.1.2.1.1.5.0
          symbol: sysName
          tag: snmp_host
      metrics:
        - MIB: MY-PROFILE-MIB
          metric_type: gauge
          symbol:
            OID: 1.4.5
            name: myMetric
`),
			expectedMetrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
				{MIB: "MY-PROFILE-MIB", Symbol: profiledefinition.SymbolConfig{OID: "1.4.5", Name: "myMetric"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
			},
			expectedMetricTags: []profiledefinition.MetricTagConfig{
				{Tag: "snmp_host", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
			},
			expectedStaticTags: []string{
				"snmp_profile:inline-profile",
				"device_vendor:f5",
			},
		},
		{
			name: "TestDefaultConfigurations",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
community_string: abc
`),
			// language=yaml
			rawInitConfig:   []byte(``),
			expectedMetrics: []profiledefinition.MetricsConfig{{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}}},
		},
		{
			name: "TestGlobalMetricsConfigurations",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
community_string: abc
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: ifNumber
`),
			// language=yaml
			rawInitConfig: []byte(`
global_metrics:
- symbol:
    OID: 1.2.3.4
    name: aGlobalMetric
`),
			expectedMetrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "ifNumber"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4", Name: "aGlobalMetric"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
			},
		},
		{
			name: "TestUseGlobalMetricsFalse",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
community_string: abc
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: aInstanceMetric
use_global_metrics: false
`),
			// language=yaml
			rawInitConfig: []byte(`
global_metrics:
- symbol:
    OID: 1.2.3.4
    name: aGlobalMetric
`),
			expectedMetrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "aInstanceMetric"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
			},
		},
		//{
		//	name: "",
		//	// language=yaml
		//	rawInstanceConfig: ,
		//	// language=yaml
		//	rawInitConfig: ,
		//},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := checkconfig.NewCheckConfig(tt.rawInstanceConfig, tt.rawInitConfig, nil)
			require.NoError(t, err)

			profileDef, err := BuildProfile("", nil, false, config)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedMetrics, profileDef.Metrics)
			assert.Equal(t, tt.expectedMetricTags, profileDef.MetricTags)
			assert.Equal(t, tt.expectedStaticTags, profileDef.StaticTags)
		})
	}
}

func TestProfileRCConfig(t *testing.T) {
	tests := []struct {
		name                 string
		rawInstanceConfig    []byte
		rawInitConfig        []byte
		profiles             []profiledefinition.ProfileDefinition
		sysObjectID          string
		expectedProfileName  string
		expectedMetrics      []profiledefinition.MetricsConfig
		expectedErrorMessage string
	}{
		{
			name: "TestExplicitRCConfig",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
profile: profile1`),
			// language=yaml
			rawInitConfig: []byte(`use_remote_config_profiles: true`),
			profiles: []profiledefinition.ProfileDefinition{
				{
					Name: "profile1",
					Metrics: []profiledefinition.MetricsConfig{
						{Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.7.1.0",
							Name: "IAmACounter32",
						}},
					},
				},
			},
			sysObjectID:         "",
			expectedProfileName: "profile1",
			expectedMetrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.7.1.0", Name: "IAmACounter32"}},
			},
		},
		{
			name: "TestDynamicRCConfig",
			// language=yaml
			rawInstanceConfig: []byte(`ip_address: 1.2.3.4`),
			// language=yaml
			rawInitConfig: []byte(`use_remote_config_profiles: true`),
			profiles: []profiledefinition.ProfileDefinition{
				{
					Name:         "profile1",
					SysObjectIDs: []string{"1.2.3.4.*"},
					Metrics: []profiledefinition.MetricsConfig{
						{Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.7.1.0",
							Name: "IAmACounter32",
						}},
					},
				},
			},
			sysObjectID:         "1.2.3.4.5.6",
			expectedProfileName: "profile1",
			expectedMetrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
				{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.7.1.0", Name: "IAmACounter32"}},
			},
		},
		{
			name: "TestRCConflict",
			// language=yaml
			rawInstanceConfig: []byte(`ip_address: 1.2.3.4`),
			// language=yaml
			rawInitConfig: []byte(`use_remote_config_profiles: true`),
			profiles: []profiledefinition.ProfileDefinition{
				{
					Name:         "profile1",
					SysObjectIDs: []string{"1.2.3.4.*"},
					Metrics: []profiledefinition.MetricsConfig{
						{Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.7.1.0",
							Name: "IAmACounter32",
						}},
					},
				}, {
					Name:         "profile2",
					SysObjectIDs: []string{"1.2.3.4.*"},
					Metrics: []profiledefinition.MetricsConfig{
						{Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.7.1.0",
							Name: "IAmACounter32",
						}},
					},
				},
			},
			sysObjectID:          "1.2.3.4.5.6",
			expectedErrorMessage: "has the same sysObjectID (1.2.3.4.*)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := checkconfig.MakeMockClient(tt.profiles)
			defer profile.ResetRCProvider()
			require.NoError(t, err)

			config, err := checkconfig.NewCheckConfig(tt.rawInstanceConfig, tt.rawInitConfig, client)
			require.NoError(t, err)

			profileDef, err := BuildProfile(tt.sysObjectID, nil, false, config)
			if tt.expectedErrorMessage == "" {
				require.NoError(t, err)

				assert.Equal(t, tt.expectedProfileName, profileDef.Name)
				assert.Equal(t, tt.expectedMetrics, profileDef.Metrics)
			} else {
				require.ErrorContains(t, err, tt.expectedErrorMessage)
			}
		})
	}
}
