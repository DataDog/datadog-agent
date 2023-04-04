// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checkconfig

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestConfigurations(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	aggregator.NewBufferedAggregator(nil, nil, "", 1*time.Hour)

	// language=yaml
	rawInstanceConfig := []byte(`
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
  forced_type: flag_stream
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
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
  f5-big-ip:
    definition_file: f5-big-ip.yaml
global_metrics:
- symbol:
    OID: 1.2.3.4
    name: aGlobalMetric
oid_batch_size: 10
bulk_max_repetitions: 20
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)

	assert.Nil(t, err)
	assert.Equal(t, 10, config.OidBatchSize)
	assert.Equal(t, uint32(20), config.BulkMaxRepetitions)
	assert.Equal(t, "1.2.3.4", config.IPAddress)
	assert.Equal(t, uint16(1161), config.Port)
	assert.Equal(t, 7, config.Timeout)
	assert.Equal(t, 5, config.Retries)
	assert.Equal(t, "2c", config.SnmpVersion)
	assert.Equal(t, "my-user", config.User)
	assert.Equal(t, "sha", config.AuthProtocol)
	assert.Equal(t, "my-authKey", config.AuthKey)
	assert.Equal(t, "aes", config.PrivProtocol)
	assert.Equal(t, "my-privKey", config.PrivKey)
	assert.Equal(t, "my-contextName", config.ContextName)
	assert.Equal(t, []string{"device_namespace:default", "snmp_device:1.2.3.4"}, config.GetStaticTags())
	expectedMetrics := []MetricsConfig{
		{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "ifNumber"}},
		{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.2.2", Name: "ifNumber2"}, MetricTags: MetricTagConfigList{
			{symbolTag: "mytag1"},
			{symbolTag: "mytag2"},
		}},
		{Symbol: SymbolConfig{OID: "1.3.6.1.4.1.318.1.1.1.11.1.1.0", Name: "upsBasicStateOutputState", ScaleFactor: 10}, ForcedType: "flag_stream", Options: MetricsConfigOption{Placement: 5, MetricSuffix: "ReplaceBattery"}},
		{
			Symbols: []SymbolConfig{
				// ifInErrors defined in instance config with a different set of metric tags from the one defined
				// in the imported profile
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
				{OID: "1.3.6.1.2.1.2.2.1.20", Name: "ifOutErrors", ScaleFactor: 3},
			},
			MetricTags: []MetricTagConfig{
				{Tag: "if_index", Index: 1},
				{Tag: "if_desc", Column: SymbolConfig{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr"},
					IndexTransform: []MetricIndexTransform{
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
					Column: SymbolConfig{OID: "1.3.6.1.2.1.2.2.1.3", Name: "ifType"},
					Mapping: map[string]string{
						"1":  "other",
						"2":  "regular1822",
						"3":  "hdh1822",
						"4":  "ddn-x25",
						"29": "ultra",
					}},
				{
					Column: SymbolConfig{
						Name: "cpiPduName",
						OID:  "1.2.3.4.8.1.2",
					},
					Match:   "(\\w)(\\w+)",
					pattern: regexp.MustCompile("(\\w)(\\w+)"),
					Tags: map[string]string{
						"prefix": "\\1",
						"suffix": "\\2",
					}},
			},
		},
		{Symbol: SymbolConfig{OID: "1.2.3.4", Name: "aGlobalMetric"}},
	}
	expectedMetrics = append(expectedMetrics, MetricsConfig{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}})
	expectedMetrics = append(expectedMetrics, fixtureProfileDefinitionMap()["f5-big-ip"].Metrics...)

	expectedMetricTags := []MetricTagConfig{
		{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol"},
		{Tag: "my_symbol_mapped", OID: "1.2.3", Name: "mySymbol", Mapping: map[string]string{"1": "one", "2": "two"}},
		{
			OID:     "1.2.3",
			Name:    "mySymbol",
			Match:   "(\\w)(\\w+)",
			pattern: regexp.MustCompile("(\\w)(\\w+)"),
			Tags: map[string]string{
				"prefix": "\\1",
				"suffix": "\\2",
			},
		},
		{
			OID:     "1.3.6.1.2.1.1.5.0",
			Name:    "sysName",
			Match:   "(\\w)(\\w+)",
			pattern: regexp.MustCompile("(\\w)(\\w+)"),
			Tags: map[string]string{
				"some_tag": "some_tag_value",
				"prefix":   "\\1",
				"suffix":   "\\2",
			},
		},
		{Tag: "snmp_host", OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
	}

	assert.Equal(t, expectedMetrics, config.Metrics)
	assert.Equal(t, expectedMetricTags, config.MetricTags)
	assert.Equal(t, []string{"snmp_profile:f5-big-ip", "device_vendor:f5", "static_tag:from_profile_root", "static_tag:from_base_profile"}, config.ProfileTags)
	assert.Equal(t, 1, len(config.Profiles))
	assert.Equal(t, "default:1.2.3.4", config.DeviceID)
	assert.Equal(t, []string{"device_namespace:default", "snmp_device:1.2.3.4"}, config.DeviceIDTags)
	assert.Equal(t, "127.0.0.0/30", config.ResolvedSubnetName)
	assert.Equal(t, false, config.AutodetectProfile)
}

func TestDiscoveryConfigurations(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`
network_address: 127.0.0.0/24
ignored_ip_addresses:
  - 127.0.0.9
  - 127.0.0.8
discovery_interval: 5
discovery_allowed_failures: 15
discovery_workers: 20
workers: 30
`)
	// language=yaml
	rawInitConfig := []byte(`
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)

	assert.Nil(t, err)
	assert.Equal(t, "127.0.0.0/24", config.Network)
	assert.Equal(t, 5, config.DiscoveryInterval)
	assert.Equal(t, 15, config.DiscoveryAllowedFailures)
	assert.Equal(t, 20, config.DiscoveryWorkers)
	assert.Equal(t, 30, config.Workers)
	assert.Equal(t, map[string]bool{
		"127.0.0.8": true,
		"127.0.0.9": true,
	}, config.IgnoredIPAddresses)
}

func TestInlineProfileConfiguration(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	aggregator.NewBufferedAggregator(nil, nil, "", 1*time.Hour)

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
snmp_version: 2c
profile: inline-profile
community_string: '123'
`)
	// language=yaml
	rawInitConfig := []byte(`
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
          forced_type: gauge
          symbol:
            OID: 1.4.5
            name: myMetric
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)

	assert.Nil(t, err)
	assert.Equal(t, []string{"device_namespace:default", "snmp_device:1.2.3.4"}, config.GetStaticTags())
	metrics := []MetricsConfig{
		{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
		{Symbol: SymbolConfig{OID: "1.4.5", Name: "myMetric"}, ForcedType: "gauge"},
	}

	metricsTags := []MetricTagConfig{
		{Tag: "snmp_host", OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
	}

	assert.Equal(t, "123", config.CommunityString)
	assert.Equal(t, metrics, config.Metrics)
	assert.Equal(t, metricsTags, config.MetricTags)
	assert.Equal(t, 2, len(config.Profiles))
	assert.Equal(t, "default:1.2.3.4", config.DeviceID)
	assert.Equal(t, []string{"device_namespace:default", "snmp_device:1.2.3.4"}, config.DeviceIDTags)
	assert.Equal(t, false, config.AutodetectProfile)
	assert.Equal(t, 3600, config.DiscoveryInterval)
	assert.Equal(t, 3, config.DiscoveryAllowedFailures)
	assert.Equal(t, 5, config.DiscoveryWorkers)
	assert.Equal(t, 5, config.Workers)
}

func TestDefaultConfigurations(t *testing.T) {
	SetConfdPathAndCleanProfiles()

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	// language=yaml
	rawInitConfig := []byte(``)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)

	assert.Nil(t, err)
	assert.Equal(t, "default", config.Namespace)
	assert.Equal(t, "1.2.3.4", config.IPAddress)
	assert.Equal(t, uint16(161), config.Port)
	assert.Equal(t, 2, config.Timeout)
	assert.Equal(t, 3, config.Retries)
	metrics := []MetricsConfig{{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}}}

	var metricsTags []MetricTagConfig

	assert.Equal(t, metrics, config.Metrics)
	assert.Equal(t, metricsTags, config.MetricTags)
	assert.Equal(t, 2, len(config.Profiles))
	assert.Equal(t, fixtureProfileDefinitionMap()["f5-big-ip"].Metrics, config.Profiles["f5-big-ip"].Metrics)
}

func TestPortConfiguration(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	// TEST Default port
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	config, err := NewCheckConfig(rawInstanceConfig, []byte(``))
	assert.Nil(t, err)
	assert.Equal(t, uint16(161), config.Port)

	// TEST Custom port
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
port: 1234
community_string: abc
`)
	config, err = NewCheckConfig(rawInstanceConfig, []byte(``))
	assert.Nil(t, err)
	assert.Equal(t, uint16(1234), config.Port)
}

func TestBatchSizeConfiguration(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	// TEST Default batch size
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	config, err := NewCheckConfig(rawInstanceConfig, []byte(``))
	assert.Nil(t, err)
	assert.Equal(t, 5, config.OidBatchSize)

	// TEST Instance config batch size
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
oid_batch_size: 10
`)
	config, err = NewCheckConfig(rawInstanceConfig, []byte(``))
	assert.Nil(t, err)
	assert.Equal(t, 10, config.OidBatchSize)

	// TEST Init config batch size
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	// language=yaml
	rawInitConfig := []byte(`
oid_batch_size: 15
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, 15, config.OidBatchSize)

	// TEST Instance & Init config batch size
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
oid_batch_size: 20
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 15
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, 20, config.OidBatchSize)
}

func TestBulkMaxRepetitionConfiguration(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	// TEST Default batch size
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	config, err := NewCheckConfig(rawInstanceConfig, []byte(``))
	assert.Nil(t, err)
	assert.Equal(t, uint32(10), config.BulkMaxRepetitions)

	// TEST Instance config batch size
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
bulk_max_repetitions: 10
`)
	config, err = NewCheckConfig(rawInstanceConfig, []byte(``))
	assert.Nil(t, err)
	assert.Equal(t, uint32(10), config.BulkMaxRepetitions)

	// TEST Init config batch size
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	// language=yaml
	rawInitConfig := []byte(`
bulk_max_repetitions: 15
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, uint32(15), config.BulkMaxRepetitions)

	// TEST Instance & Init config batch size
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
bulk_max_repetitions: 20
`)
	// language=yaml
	rawInitConfig = []byte(`
bulk_max_repetitions: 15
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, uint32(20), config.BulkMaxRepetitions)

	// TEST invalid value
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
bulk_max_repetitions: -5
`)
	// language=yaml
	rawInitConfig = []byte(``)
	_, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.EqualError(t, err, "bulk max repetition must be a positive integer. Invalid value: -5")
}

func TestGlobalMetricsConfigurations(t *testing.T) {
	SetConfdPathAndCleanProfiles()

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: ifNumber
`)
	// language=yaml
	rawInitConfig := []byte(`
global_metrics:
- symbol:
    OID: 1.2.3.4
    name: aGlobalMetric
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	metrics := []MetricsConfig{
		{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "ifNumber"}},
		{Symbol: SymbolConfig{OID: "1.2.3.4", Name: "aGlobalMetric"}},
		{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
	}
	assert.Equal(t, metrics, config.Metrics)
}

func TestUseGlobalMetricsFalse(t *testing.T) {
	SetConfdPathAndCleanProfiles()

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: aInstanceMetric
use_global_metrics: false
`)
	// language=yaml
	rawInitConfig := []byte(`
global_metrics:
- symbol:
    OID: 1.2.3.4
    name: aGlobalMetric
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	metrics := []MetricsConfig{
		{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "aInstanceMetric"}},
		{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
	}
	assert.Equal(t, metrics, config.Metrics)
}

func Test_NewCheckConfig_errors(t *testing.T) {
	SetConfdPathAndCleanProfiles()

	tests := []struct {
		name              string
		rawInstanceConfig []byte
		rawInitConfig     []byte
		expectedErrors    []string
	}{
		{
			name: "unknown profile",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
profile: does-not-exist
`),
			// language=yaml
			rawInitConfig: []byte(`
profiles:
  f5-big-ip:
    definition_file: f5-big-ip.yaml
`),
			expectedErrors: []string{
				"failed to refresh with profile `does-not-exist`: unknown profile `does-not-exist`",
			},
		},
		{
			name: "validation errors",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
metrics:
- symbol:
    OID: 1.2.3
-
`),
			// language=yaml
			rawInitConfig: []byte(``),
			expectedErrors: []string{
				"validation errors: either a table symbol or a scalar symbol must be provided",
				"either a table symbol or a scalar symbol must be provided",
			},
		},
		{
			name: "both ip_address and network error",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
network_address: 10.0.0.0/24
`),
			// language=yaml
			rawInitConfig: []byte(``),
			expectedErrors: []string{
				"`ip_address` and `network` cannot be used at the same time",
			},
		},
		{
			name: "no ip_address or network error",
			// language=yaml
			rawInstanceConfig: []byte(`
`),
			// language=yaml
			rawInitConfig: []byte(``),
			expectedErrors: []string{
				"`ip_address` or `network` config must be provided",
			},
		},
		{
			name: "invalid subnet cidr",
			// language=yaml
			rawInstanceConfig: []byte(`
network_address: 10.0.0.0/xx
`),
			// language=yaml
			rawInitConfig: []byte(``),
			expectedErrors: []string{
				"couldn't parse SNMP network: invalid CIDR address: 10.0.0.0/xx",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCheckConfig(tt.rawInstanceConfig, tt.rawInitConfig)
			for _, errStr := range tt.expectedErrors {
				assert.Contains(t, err.Error(), errStr)
			}
		})
	}
}

func Test_getProfileForSysObjectID(t *testing.T) {
	mockProfiles := profileDefinitionMap{
		"profile1": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
		},
		"profile2": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.10"},
		},
		"profile3": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.5.*"},
		},
	}
	mockProfilesWithPatternError := profileDefinitionMap{
		"profile1": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.***.*"},
		},
	}
	mockProfilesWithInvalidPatternError := profileDefinitionMap{
		"profile1": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.[.*"},
		},
	}
	mockProfilesWithDuplicateSysobjectid := profileDefinitionMap{
		"profile1": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3"},
		},
		"profile2": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3"},
		},
		"profile3": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.4"},
		},
	}
	tests := []struct {
		name            string
		profiles        profileDefinitionMap
		sysObjectID     string
		expectedProfile string
		expectedError   string
	}{
		{
			name:            "found matching profile",
			profiles:        mockProfiles,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.1",
			expectedProfile: "profile1",
			expectedError:   "",
		},
		{
			name:            "found more precise matching profile",
			profiles:        mockProfiles,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.10",
			expectedProfile: "profile2",
			expectedError:   "",
		},
		{
			name:            "found even more precise matching profile",
			profiles:        mockProfiles,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfile: "profile3",
			expectedError:   "",
		},
		{
			name:            "failed to get most specific profile for sysObjectID",
			profiles:        mockProfilesWithPatternError,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfile: "",
			expectedError:   "failed to get most specific profile for sysObjectID `1.3.6.1.4.1.3375.2.1.3.4.5.11`, for matched oids [1.3.6.1.4.1.3375.2.1.3.***.*]: error parsing part `***` for pattern `1.3.6.1.4.1.3375.2.1.3.***.*`: strconv.Atoi: parsing \"***\": invalid syntax",
		},
		{
			name:            "invalid pattern", // profiles with invalid patterns are skipped, leading to: cannot get most specific oid from empty list of oids
			profiles:        mockProfilesWithInvalidPatternError,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfile: "",
			expectedError:   "failed to get most specific profile for sysObjectID `1.3.6.1.4.1.3375.2.1.3.4.5.11`, for matched oids []: cannot get most specific oid from empty list of oids",
		},
		{
			name:            "duplicate sysobjectid",
			profiles:        mockProfilesWithDuplicateSysobjectid,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3",
			expectedProfile: "",
			expectedError:   "has the same sysObjectID (1.3.6.1.4.1.3375.2.1.3) as",
		},
		{
			name:            "unrelated duplicate sysobjectid should not raise error",
			profiles:        mockProfilesWithDuplicateSysobjectid,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.4",
			expectedProfile: "profile3",
			expectedError:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := GetProfileForSysObjectID(tt.profiles, tt.sysObjectID)
			if tt.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), tt.expectedError)
			}
			assert.Equal(t, tt.expectedProfile, profile)
		})
	}
}

func Test_snmpConfig_toString(t *testing.T) {
	c := CheckConfig{
		CommunityString: "my_communityString",
		AuthProtocol:    "my_authProtocol",
		AuthKey:         "my_authKey",
		PrivProtocol:    "my_privProtocol",
		PrivKey:         "my_privKey",
	}
	assert.NotContains(t, c.ToString(), "my_communityString")
	assert.NotContains(t, c.ToString(), "my_authKey")
	assert.NotContains(t, c.ToString(), "my_privKey")

	assert.Contains(t, c.ToString(), "my_authProtocol")
	assert.Contains(t, c.ToString(), "my_privProtocol")
}

func Test_Configure_invalidYaml(t *testing.T) {
	tests := []struct {
		name              string
		rawInstanceConfig []byte
		rawInitConfig     []byte
		expectedErr       string
	}{
		{
			name: "invalid rawInitConfig",
			// language=yaml
			rawInstanceConfig: []byte(``),
			// language=yaml
			rawInitConfig: []byte(`::x`),
			expectedErr:   "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `::x` into checkconfig.InitConfig",
		},
		{
			name: "invalid rawInstanceConfig",
			// language=yaml
			rawInstanceConfig: []byte(`::x`),
			// language=yaml
			rawInitConfig: []byte(``),
			expectedErr:   "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `::x` into checkconfig.InstanceConfig",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCheckConfig(tt.rawInstanceConfig, tt.rawInitConfig)
			assert.EqualError(t, err, tt.expectedErr)
		})
	}
}

func TestNumberConfigsUsingStrings(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
port: "123"
timeout: "15"
retries: "5"
`)
	config, err := NewCheckConfig(rawInstanceConfig, []byte(``))
	assert.Nil(t, err)
	assert.Equal(t, uint16(123), config.Port)
	assert.Equal(t, 15, config.Timeout)
	assert.Equal(t, 5, config.Retries)

}

func TestExtraTags(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	config, err := NewCheckConfig(rawInstanceConfig, []byte(``))
	assert.Nil(t, err)
	assert.Equal(t, []string{"device_namespace:default", "snmp_device:1.2.3.4"}, config.GetStaticTags())

	// language=yaml
	rawInstanceConfigWithExtraTags := []byte(`
ip_address: 1.2.3.4
community_string: abc
extra_tags: "extratag1:val1,extratag2:val2"
`)
	config, err = NewCheckConfig(rawInstanceConfigWithExtraTags, []byte(``))
	assert.Nil(t, err)
	assert.ElementsMatch(t, []string{"device_namespace:default", "snmp_device:1.2.3.4", "extratag1:val1", "extratag2:val2"}, config.GetStaticTags())
}

func Test_snmpConfig_getDeviceIDTags(t *testing.T) {
	c := &CheckConfig{
		IPAddress:    "1.2.3.4",
		ExtraTags:    []string{"extratag1:val1", "extratag2"},
		InstanceTags: []string{"instancetag1:val1", "instancetag2"},
		Namespace:    "hey",
	}
	actualTags := c.getDeviceIDTags()

	expectedTags := []string{"device_namespace:hey", "snmp_device:1.2.3.4"}
	assert.Equal(t, expectedTags, actualTags)
}

func Test_snmpConfig_refreshWithProfile(t *testing.T) {
	metrics := []MetricsConfig{
		{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
		{
			Symbols: []SymbolConfig{
				{
					OID:  "1.2.3.4.6",
					Name: "abc",
				},
			},
			MetricTags: MetricTagConfigList{
				MetricTagConfig{
					Column: SymbolConfig{
						OID: "1.2.3.4.7",
					},
				},
			},
		},
	}
	profile1 := profileDefinition{
		Device: DeviceMeta{
			Vendor: "a-vendor",
		},
		Metrics: metrics,
		MetricTags: []MetricTagConfig{
			{Tag: "interface", Column: SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
		},
		Metadata: MetadataConfig{
			"device": {
				Fields: map[string]MetadataField{
					"description": {
						Symbol: SymbolConfig{
							OID:  "1.3.6.1.2.1.1.99.3.0",
							Name: "sysDescr",
						},
					},
					"name": {
						Symbols: []SymbolConfig{
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
				Fields: map[string]MetadataField{
					"oper_status": {
						Symbol: SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.99",
							Name: "someIfSymbol",
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
		},
		SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
	}
	mockProfiles := profileDefinitionMap{
		"profile1": profile1,
	}
	c := &CheckConfig{
		IPAddress: "1.2.3.4",
		Profiles:  mockProfiles,
	}
	err := c.RefreshWithProfile("f5")
	assert.EqualError(t, err, "unknown profile `f5`")

	err = c.RefreshWithProfile("profile1")
	assert.NoError(t, err)

	assert.Equal(t, "profile1", c.Profile)
	assert.Equal(t, profile1, *c.ProfileDef)
	assert.Equal(t, metrics, c.Metrics)
	assert.Equal(t, []MetricTagConfig{
		{Tag: "interface", Column: SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
	}, c.MetricTags)
	assert.Equal(t, OidConfig{
		ScalarOids: []string{"1.2.3.4.5"},
		ColumnOids: []string{"1.2.3.4.6", "1.2.3.4.7"},
	}, c.OidConfig)
	assert.Equal(t, []string{"snmp_profile:profile1", "device_vendor:a-vendor"}, c.ProfileTags)

	c = &CheckConfig{
		IPAddress:             "1.2.3.4",
		Profiles:              mockProfiles,
		CollectDeviceMetadata: true,
		CollectTopology:       false,
	}
	err = c.RefreshWithProfile("profile1")
	assert.NoError(t, err)
	assert.Equal(t, OidConfig{
		ScalarOids: []string{
			"1.2.3.4.5",
			"1.3.6.1.2.1.1.99.1.0",
			"1.3.6.1.2.1.1.99.2.0",
			"1.3.6.1.2.1.1.99.3.0",
		},
		ColumnOids: []string{
			"1.2.3.4.6",
			"1.2.3.4.7",
			"1.3.6.1.2.1.2.2.1.99",
			"1.3.6.1.2.1.31.1.1.1.1",
			"1.3.6.1.2.1.4.20.1.2",
			"1.3.6.1.2.1.4.20.1.3",
		},
	}, c.OidConfig)

	// With metadata disabled
	c.CollectDeviceMetadata = false
	err = c.RefreshWithProfile("profile1")
	assert.NoError(t, err)
	assert.Equal(t, OidConfig{
		ScalarOids: []string{
			"1.2.3.4.5",
		},
		ColumnOids: []string{
			"1.2.3.4.6",
			"1.2.3.4.7",
		},
	}, c.OidConfig)
}

func Test_getSubnetFromTags(t *testing.T) {
	subnet, err := getSubnetFromTags([]string{"aa", "bb"})
	assert.Equal(t, "", subnet)
	assert.EqualError(t, err, "subnet not found in tags [aa bb]")

	subnet, err = getSubnetFromTags([]string{"aa", "autodiscovery_subnet:127.0.0.0/30", "bb"})
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.0/30", subnet)

	// make sure we don't panic if the subnet if empty
	subnet, err = getSubnetFromTags([]string{"aa", "autodiscovery_subnet:", "bb"})
	assert.NoError(t, err)
	assert.Equal(t, "", subnet)
}

func Test_buildConfig_collectDeviceMetadata(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig := []byte(`
oid_batch_size: 10
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.CollectDeviceMetadata)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
collect_device_metadata: true
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.CollectDeviceMetadata)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
collect_device_metadata: true
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.CollectDeviceMetadata)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
collect_device_metadata: false
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
collect_device_metadata: true
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, false, config.CollectDeviceMetadata)
}

func Test_buildConfig_collectTopology(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig := []byte(`
oid_batch_size: 10
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, false, config.CollectTopology)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
collect_topology: true
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.CollectTopology)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
collect_topology: true
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.CollectTopology)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
collect_topology: false
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
collect_topology: true
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, false, config.CollectTopology)
}

func Test_buildConfig_namespace(t *testing.T) {
	defer coreconfig.Datadog.Set("network_devices.namespace", "default")

	// Should use namespace defined in instance config
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: "abc"
namespace: my-ns
`)
	rawInitConfig := []byte(``)

	conf, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, "my-ns", conf.Namespace)

	// Should use namespace defined in datadog.yaml network_devices
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	rawInitConfig = []byte(``)
	conf, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, "default", conf.Namespace)

	// Should use namespace defined in init config
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	rawInitConfig = []byte(`
namespace: ns-from-datadog-conf`)
	conf, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, "ns-from-datadog-conf", conf.Namespace)

	// Should use namespace defined in datadog.yaml network_devices
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	rawInitConfig = []byte(``)
	coreconfig.Datadog.Set("network_devices.namespace", "totoro")
	conf, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, "totoro", conf.Namespace)

	// Should use namespace defined in init config
	// when namespace is empty in instance config
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
namespace: ""
`)
	rawInitConfig = []byte(`
namespace: ponyo`)
	conf, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, "ponyo", conf.Namespace)

	// Should use namespace defined in datadog.yaml network_devices
	// when namespace is empty in init config
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	rawInitConfig = []byte(`
namespace: `)
	coreconfig.Datadog.Set("network_devices.namespace", "mononoke")
	conf, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, "mononoke", conf.Namespace)

	// Should throw error when namespace is empty in datadog.yaml network_devices
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	rawInitConfig = []byte(``)
	coreconfig.Datadog.Set("network_devices.namespace", "")
	_, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.EqualError(t, err, "namespace cannot be empty")
}

func Test_buildConfig_UseDeviceIDAsHostname(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig := []byte(`
oid_batch_size: 10
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, false, config.UseDeviceIDAsHostname)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
use_device_id_as_hostname: true
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.UseDeviceIDAsHostname)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
use_device_id_as_hostname: true
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.UseDeviceIDAsHostname)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
use_device_id_as_hostname: false
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
use_device_id_as_hostname: true
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, false, config.UseDeviceIDAsHostname)
}

func Test_buildConfig_DetectMetricsEnabled(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig := []byte(`
oid_batch_size: 10
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, false, config.DetectMetricsEnabled)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
experimental_detect_metrics_enabled: true
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.DetectMetricsEnabled)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
experimental_detect_metrics_enabled: true
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, true, config.DetectMetricsEnabled)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
experimental_detect_metrics_enabled: false
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
experimental_detect_metrics_enabled: true
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, false, config.DetectMetricsEnabled)
}

func Test_buildConfig_DetectMetricsRefreshInterval(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig := []byte(`
oid_batch_size: 10
`)
	config, err := NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, 3600, config.DetectMetricsRefreshInterval)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
experimental_detect_metrics_refresh_interval: 10
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, 10, config.DetectMetricsRefreshInterval)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
experimental_detect_metrics_refresh_interval: 10
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 20
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, 10, config.DetectMetricsRefreshInterval)

	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: "abc"
experimental_detect_metrics_refresh_interval: 20
`)
	// language=yaml
	rawInitConfig = []byte(`
oid_batch_size: 10
experimental_detect_metrics_refresh_interval: 30
`)
	config, err = NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)
	assert.Equal(t, 20, config.DetectMetricsRefreshInterval)
}

func Test_buildConfig_minCollectionInterval(t *testing.T) {
	tests := []struct {
		name              string
		rawInstanceConfig []byte
		rawInitConfig     []byte
		expectedInterval  time.Duration
		expectedErr       string
	}{
		{
			name: "default min collection interval",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`),
			// language=yaml
			rawInitConfig:    []byte(``),
			expectedInterval: 15 * time.Second,
		},
		{
			name: "init min_collection_interval",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
`),
			// language=yaml
			rawInitConfig: []byte(`
min_collection_interval: 20
`),
			expectedInterval: 20 * time.Second,
		},
		{
			name: "instance min_collection_interval",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
min_collection_interval: 25
`),
			// language=yaml
			rawInitConfig: []byte(`
min_collection_interval: 20
`),
			expectedInterval: 25 * time.Second,
		},
		{
			name: "instance extra_min_collection_interval",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
extra_min_collection_interval: 30
`),
			// language=yaml
			rawInitConfig: []byte(`
min_collection_interval: 20
`),
			expectedInterval: 30 * time.Second,
		},
		{
			name: "instance extra_min_collection_interval precedence",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
extra_min_collection_interval: 30
min_collection_interval: 40
`),
			// language=yaml
			rawInitConfig: []byte(`
min_collection_interval: 20
`),
			expectedInterval: 30 * time.Second,
		},
		{
			name: "instance min_collection_interval with extra = 0",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
extra_min_collection_interval: 0
min_collection_interval: 40
`),
			// language=yaml
			rawInitConfig: []byte(`
min_collection_interval: 20
`),
			expectedInterval: 40 * time.Second,
		},
		{
			name: "negative min_collection_interval",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
`),
			// language=yaml
			rawInitConfig: []byte(`
min_collection_interval: -10
`),
			expectedInterval: 0,
			expectedErr:      "min collection interval must be > 0, but got: -10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewCheckConfig(tt.rawInstanceConfig, tt.rawInitConfig)
			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
			} else {
				assert.Equal(t, tt.expectedInterval, config.MinCollectionInterval)
			}
		})
	}
}

func Test_buildConfig_InterfaceConfigs(t *testing.T) {
	tests := []struct {
		name                     string
		rawInstanceConfig        []byte
		rawInitConfig            []byte
		expectedInterfaceConfigs []snmpintegration.InterfaceConfig
		expectedErr              string
	}{
		{
			name: "interface config as yaml",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
interface_configs:
  - match_field: "name"
    match_value: "eth0"
    in_speed: 25
    out_speed: 10
`),
			// language=yaml
			rawInitConfig: []byte(``),
			expectedInterfaceConfigs: []snmpintegration.InterfaceConfig{
				{
					MatchField: "name",
					MatchValue: "eth0",
					InSpeed:    25,
					OutSpeed:   10,
				},
			},
		},
		{
			name: "interface config as json string",
			// language=yaml
			rawInstanceConfig: []byte(`
ip_address: 1.2.3.4
interface_configs: '[{"match_field":"name","match_value":"eth0","in_speed":25,"out_speed":10}]'
`),
			// language=yaml
			rawInitConfig: []byte(``),
			expectedInterfaceConfigs: []snmpintegration.InterfaceConfig{
				{
					MatchField: "name",
					MatchValue: "eth0",
					InSpeed:    25,
					OutSpeed:   10,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewCheckConfig(tt.rawInstanceConfig, tt.rawInitConfig)
			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
			} else {
				assert.Equal(t, tt.expectedInterfaceConfigs, config.InterfaceConfigs)
			}
		})
	}
}

func TestCheckConfig_DiscoveryDigest(t *testing.T) {
	baseCaseHash := DeviceDigest("a1d0f0237ee2fe8f")
	tests := []struct {
		name         string
		config       CheckConfig
		ipAddress    string
		expectedHash DeviceDigest
	}{
		{
			name:      "base case",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "",
			},
			expectedHash: baseCaseHash,
		},
		{
			name:      "different ip",
			ipAddress: "127.0.0.2",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "",
			},
			expectedHash: "82787fd6ceade9e2",
		},
		{
			name:      "different CommunityString",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public2",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "",
			},
			expectedHash: "cdf44b020cb2a9e7",
		},
		{
			name:      "different snmp version",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "3",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "",
			},
			expectedHash: "d41b9d5601104294",
		},
		{
			name:      "different user",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "user2",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "",
			},
			expectedHash: "992c6d4145819fd2",
		},
		{
			name:      "different AuthProtocol",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "md5",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "",
			},
			expectedHash: "16e9c29b483c41ad",
		},
		{
			name:      "different AuthKey",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "1234",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "",
			},
			expectedHash: "ea3fce2535709f4d",
		},
		{
			name:      "different PrivProtocol",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "aes",
				PrivKey:         "123",
				ContextName:     "",
			},
			expectedHash: "a1dbe4237eecf26e",
		},
		{
			name:      "different PrivKey",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "1234",
				ContextName:     "",
			},
			expectedHash: "3942f94fb039bddd",
		},
		{
			name:      "different context name",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "123",
			},
			expectedHash: "36e5cb68e8ad58d1",
		},
		{
			name:      "different ignored ips",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:               161,
				CommunityString:    "public",
				SnmpVersion:        "2",
				User:               "123",
				AuthProtocol:       "sha",
				AuthKey:            "123",
				PrivProtocol:       "des",
				PrivKey:            "123",
				ContextName:        "123",
				IgnoredIPAddresses: map[string]bool{"127.0.0.3": true},
			},
			expectedHash: "20714cf5b9e95cfe",
		},
		{
			name:      "different other fields lead to same hash",
			ipAddress: "127.0.0.1",
			config: CheckConfig{
				Port:            161,
				CommunityString: "public",
				SnmpVersion:     "2",
				User:            "123",
				AuthProtocol:    "sha",
				AuthKey:         "123",
				PrivProtocol:    "des",
				PrivKey:         "123",
				ContextName:     "",
				Retries:         999,
			},
			expectedHash: baseCaseHash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedHash, tt.config.DeviceDigest(tt.ipAddress))
		})
	}
}

func assertNotSameButEqualElements(t *testing.T, item1 interface{}, item2 interface{}) {
	assert.NotEqual(t, fmt.Sprintf("%p", item1), fmt.Sprintf("%p", item2))
	assert.Equal(t, fmt.Sprintf("%p", item1), fmt.Sprintf("%p", item1))
	assert.Equal(t, fmt.Sprintf("%p", item2), fmt.Sprintf("%p", item2))
	assert.ElementsMatch(t, item1, item2)
}

func TestCheckConfig_Copy(t *testing.T) {
	config := CheckConfig{
		Network:         "127.0.0.0/30",
		IPAddress:       "127.0.0.5",
		Port:            161,
		CommunityString: "public",
		SnmpVersion:     "2",
		Timeout:         5,
		Retries:         5,
		User:            "123",
		AuthProtocol:    "sha",
		AuthKey:         "123",
		PrivProtocol:    "des",
		PrivKey:         "123",
		ContextName:     "",
		OidConfig: OidConfig{
			ScalarOids: []string{"1.2.3"},
			ColumnOids: []string{"1.2.3", "2.3.4"},
		},
		Metrics: []MetricsConfig{
			{
				Symbol: SymbolConfig{
					OID:  "1.2",
					Name: "abc",
				},
			},
		},
		MetricTags: []MetricTagConfig{
			{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol"},
		},
		OidBatchSize:       10,
		BulkMaxRepetitions: 10,
		Profiles: profileDefinitionMap{"f5-big-ip": profileDefinition{
			Device: DeviceMeta{Vendor: "f5"},
		}},
		ProfileTags: []string{"profile_tag:atag"},
		Profile:     "f5",
		ProfileDef: &profileDefinition{
			Device: DeviceMeta{Vendor: "f5"},
		},
		ExtraTags:             []string{"ExtraTags:tag"},
		InstanceTags:          []string{"InstanceTags:tag"},
		CollectDeviceMetadata: true,
		CollectTopology:       true,
		UseDeviceIDAsHostname: true,
		DeviceID:              "123",
		DeviceIDTags:          []string{"DeviceIDTags:tag"},
		ResolvedSubnetName:    "1.2.3.4/28",
		AutodetectProfile:     true,
		MinCollectionInterval: 120,
	}
	configCopy := config.Copy()

	assert.Equal(t, config.Network, configCopy.Network)
	assert.Equal(t, config.IPAddress, configCopy.IPAddress)
	assert.Equal(t, config.Port, configCopy.Port)
	assert.Equal(t, config.CommunityString, configCopy.CommunityString)
	assert.Equal(t, config.SnmpVersion, configCopy.SnmpVersion)
	assert.Equal(t, config.Timeout, configCopy.Timeout)
	assert.Equal(t, config.Retries, configCopy.Retries)
	assert.Equal(t, config.User, configCopy.User)
	assert.Equal(t, config.AuthProtocol, configCopy.AuthProtocol)
	assert.Equal(t, config.AuthKey, configCopy.AuthKey)
	assert.Equal(t, config.PrivProtocol, configCopy.PrivProtocol)
	assert.Equal(t, config.PrivKey, configCopy.PrivKey)
	assert.Equal(t, config.ContextName, configCopy.ContextName)
	assert.Equal(t, config.OidConfig, configCopy.OidConfig)

	assertNotSameButEqualElements(t, config.Metrics, configCopy.Metrics)
	assertNotSameButEqualElements(t, config.MetricTags, configCopy.MetricTags)

	assert.Equal(t, config.OidBatchSize, configCopy.OidBatchSize)
	assert.Equal(t, config.BulkMaxRepetitions, configCopy.BulkMaxRepetitions)
	assert.Equal(t, config.Profiles, configCopy.Profiles)

	assertNotSameButEqualElements(t, config.ProfileTags, configCopy.ProfileTags)

	assert.Equal(t, config.Profile, configCopy.Profile)
	assert.Equal(t, config.ProfileDef, configCopy.ProfileDef)
	assertNotSameButEqualElements(t, config.ExtraTags, configCopy.ExtraTags)
	assertNotSameButEqualElements(t, config.InstanceTags, configCopy.InstanceTags)
	assert.Equal(t, config.CollectDeviceMetadata, configCopy.CollectDeviceMetadata)
	assert.Equal(t, config.CollectTopology, configCopy.CollectTopology)
	assert.Equal(t, config.UseDeviceIDAsHostname, configCopy.UseDeviceIDAsHostname)
	assert.Equal(t, config.DeviceID, configCopy.DeviceID)
	assertNotSameButEqualElements(t, config.DeviceIDTags, configCopy.DeviceIDTags)
	assert.Equal(t, config.ResolvedSubnetName, configCopy.ResolvedSubnetName)
	assert.Equal(t, config.AutodetectProfile, configCopy.AutodetectProfile)
	assert.Equal(t, config.MinCollectionInterval, configCopy.MinCollectionInterval)
}

func TestCheckConfig_CopyWithNewIP(t *testing.T) {
	config := CheckConfig{
		IPAddress:       "127.0.0.5",
		Port:            161,
		CommunityString: "public",
		InstanceTags:    []string{"tag1:val1"},
	}
	config.UpdateDeviceIDAndTags()

	configCopy := config.CopyWithNewIP("127.0.0.10")

	assert.Equal(t, "127.0.0.10", configCopy.IPAddress)
	assert.Equal(t, config.Port, configCopy.Port)
	assert.Equal(t, config.CommunityString, configCopy.CommunityString)
	assert.NotEqual(t, config.DeviceID, configCopy.DeviceID)
}

func TestCheckConfig_getResolvedSubnetName(t *testing.T) {
	tests := []struct {
		name               string
		network            string
		instanceTags       []string
		expectedSubnetName string
	}{
		{
			name:               "from Network",
			network:            "1.2.0.0/24",
			expectedSubnetName: "1.2.0.0/24",
		},
		{
			name:               "from instance tags",
			instanceTags:       []string{"autodiscovery_subnet:10.10.0.0/25"},
			expectedSubnetName: "10.10.0.0/25",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CheckConfig{
				Network:      tt.network,
				InstanceTags: tt.instanceTags,
			}
			assert.Equal(t, tt.expectedSubnetName, c.getResolvedSubnetName())
		})
	}
}

func TestCheckConfig_GetStaticTags(t *testing.T) {
	coreconfig.Datadog.Set("hostname", "my-hostname")
	tests := []struct {
		name         string
		config       CheckConfig
		expectedTags []string
	}{
		{
			name: "IPAddress",
			config: CheckConfig{
				Namespace: "default",
				IPAddress: "1.2.3.4",
			},
			expectedTags: []string{
				"device_namespace:default",
				"snmp_device:1.2.3.4",
			},
		},
		{
			name: "extraTags",
			config: CheckConfig{
				Namespace: "default",
				IPAddress: "1.2.3.4",
				ExtraTags: []string{
					"extra_tag1:val1",
					"extra_tag2:val2",
				},
			},
			expectedTags: []string{
				"extra_tag1:val1",
				"extra_tag2:val2",
				"device_namespace:default",
				"snmp_device:1.2.3.4",
			},
		},
		{
			name: "Agent Hostname",
			config: CheckConfig{
				Namespace:             "default",
				IPAddress:             "1.2.3.4",
				UseDeviceIDAsHostname: true,
			},
			expectedTags: []string{
				"device_namespace:default",
				"snmp_device:1.2.3.4",
				"agent_host:my-hostname",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatch(t, tt.expectedTags, tt.config.GetStaticTags())
		})
	}
}
