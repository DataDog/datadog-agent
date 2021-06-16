// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigurations(t *testing.T) {
	setConfdPathAndCleanProfiles()
	aggregator.InitAggregatorWithFlushInterval(nil, nil, "", 1*time.Hour)

	check := Check{session: &snmpSession{}}
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
`)
	err := check.Configure(rawInstanceConfig, rawInitConfig, "test")

	assert.Nil(t, err)
	assert.Equal(t, 10, check.config.oidBatchSize)
	assert.Equal(t, "1.2.3.4", check.config.ipAddress)
	assert.Equal(t, uint16(1161), check.config.port)
	assert.Equal(t, 7, check.config.timeout)
	assert.Equal(t, 5, check.config.retries)
	assert.Equal(t, "2c", check.config.snmpVersion)
	assert.Equal(t, "my-user", check.config.user)
	assert.Equal(t, "sha", check.config.authProtocol)
	assert.Equal(t, "my-authKey", check.config.authKey)
	assert.Equal(t, "aes", check.config.privProtocol)
	assert.Equal(t, "my-privKey", check.config.privKey)
	assert.Equal(t, "my-contextName", check.config.contextName)
	assert.Equal(t, []string{"snmp_device:1.2.3.4"}, check.config.getStaticTags())
	metrics := []metricsConfig{
		{Symbol: symbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "ifNumber"}},
		{Symbol: symbolConfig{OID: "1.3.6.1.2.1.2.2", Name: "ifNumber2"}, MetricTags: metricTagConfigList{
			{symbolTag: "mytag1"},
			{symbolTag: "mytag2"},
		}},
		{Symbol: symbolConfig{OID: "1.3.6.1.4.1.318.1.1.1.11.1.1.0", Name: "upsBasicStateOutputState"}, ForcedType: "flag_stream", Options: metricsConfigOption{Placement: 5, MetricSuffix: "ReplaceBattery"}},
		{
			Symbols: []symbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
				{OID: "1.3.6.1.2.1.2.2.1.20", Name: "ifOutErrors"},
			},
			MetricTags: []metricTagConfig{
				{Tag: "if_index", Index: 1},
				{Tag: "if_desc", Column: symbolConfig{OID: "1.3.6.1.2.1.2.2.1.2", Name: "ifDescr"},
					IndexTransform: []metricIndexTransform{
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
				{
					Column: symbolConfig{
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
		{Symbol: symbolConfig{OID: "1.2.3.4", Name: "aGlobalMetric"}},
	}
	metrics = append(metrics, mockProfilesDefinitions()["f5-big-ip"].Metrics...)
	metrics = append(metrics, metricsConfig{Symbol: symbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}})

	metricsTags := []metricTagConfig{
		{Tag: "my_symbol", OID: "1.2.3", Name: "mySymbol"},
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

	assert.Equal(t, metrics, check.config.metrics)
	assert.Equal(t, metricsTags, check.config.metricTags)
	assert.Equal(t, 1, len(check.config.profiles))
	assert.Equal(t, "780a58c96c908df8", check.config.deviceID)
	assert.Equal(t, []string{"snmp_device:1.2.3.4", "tag1", "tag2:val2"}, check.config.deviceIDTags)
	assert.Equal(t, "127.0.0.0/30", check.config.subnet)
	assert.Equal(t, false, check.config.autodetectProfile)
}

func TestDefaultConfigurations(t *testing.T) {
	setConfdPathAndCleanProfiles()

	check := Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	// language=yaml
	rawInitConfig := []byte(``)
	err := check.Configure(rawInstanceConfig, rawInitConfig, "test")

	assert.Nil(t, err)
	assert.Equal(t, "1.2.3.4", check.config.ipAddress)
	assert.Equal(t, uint16(161), check.config.port)
	assert.Equal(t, 2, check.config.timeout)
	assert.Equal(t, 3, check.config.retries)
	metrics := []metricsConfig{{Symbol: symbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}}}

	var metricsTags []metricTagConfig

	assert.Equal(t, metrics, check.config.metrics)
	assert.Equal(t, metricsTags, check.config.metricTags)
	assert.Equal(t, 1, len(check.config.profiles))
	assert.Equal(t, mockProfilesDefinitions()["f5-big-ip"].Metrics, check.config.profiles["f5-big-ip"].Metrics)
}

func TestIPAddressConfiguration(t *testing.T) {
	setConfdPathAndCleanProfiles()
	// TEST Default port
	check := Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address:
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.EqualError(t, err, "build config failed: ip_address config must be provided")
}

func TestPortConfiguration(t *testing.T) {
	setConfdPathAndCleanProfiles()
	// TEST Default port
	check := Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)
	assert.Equal(t, uint16(161), check.config.port)

	// TEST Custom port
	check = Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
port: 1234
community_string: abc
`)
	err = check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)
	assert.Equal(t, uint16(1234), check.config.port)
}

func TestBatchSizeConfiguration(t *testing.T) {
	setConfdPathAndCleanProfiles()
	// TEST Default batch size
	check := Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)
	assert.Equal(t, 60, check.config.oidBatchSize)

	// TEST Instance config batch size
	check = Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
oid_batch_size: 10
`)
	err = check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)
	assert.Equal(t, 10, check.config.oidBatchSize)

	// TEST Init config batch size
	check = Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig = []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	// language=yaml
	rawInitConfig := []byte(`
oid_batch_size: 15
`)
	err = check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)
	assert.Equal(t, 15, check.config.oidBatchSize)

	// TEST Instance & Init config batch size
	check = Check{session: &snmpSession{}}
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
	err = check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)
	assert.Equal(t, 20, check.config.oidBatchSize)
}

func TestGlobalMetricsConfigurations(t *testing.T) {
	setConfdPathAndCleanProfiles()

	check := Check{session: &snmpSession{}}
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
	err := check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)

	metrics := []metricsConfig{
		{Symbol: symbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "ifNumber"}},
		{Symbol: symbolConfig{OID: "1.2.3.4", Name: "aGlobalMetric"}},
		{Symbol: symbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
	}
	assert.Equal(t, metrics, check.config.metrics)
}

func TestUseGlobalMetricsFalse(t *testing.T) {
	setConfdPathAndCleanProfiles()

	check := Check{session: &snmpSession{}}
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
	err := check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)

	metrics := []metricsConfig{
		{Symbol: symbolConfig{OID: "1.3.6.1.2.1.2.1", Name: "aInstanceMetric"}},
		{Symbol: symbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
	}
	assert.Equal(t, metrics, check.config.metrics)
}

func Test_buildConfig(t *testing.T) {
	setConfdPathAndCleanProfiles()

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
			rawInitConfig: []byte(`
`),
			expectedErrors: []string{
				"validation errors: either a table symbol or a scalar symbol must be provided",
				"either a table symbol or a scalar symbol must be provided",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildConfig(tt.rawInstanceConfig, tt.rawInitConfig)
			for _, errStr := range tt.expectedErrors {
				assert.Contains(t, err.Error(), errStr)
			}
		})
	}
}

func Test_getProfileForSysObjectID(t *testing.T) {
	mockProfiles := profileDefinitionMap{
		"profile1": profileDefinition{
			Metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
		},
		"profile2": profileDefinition{
			Metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.10"},
		},
		"profile3": profileDefinition{
			Metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.5.*"},
		},
	}
	mockProfilesWithPatternError := profileDefinitionMap{
		"profile1": profileDefinition{
			Metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.***.*"},
		},
	}
	mockProfilesWithInvalidPatternError := profileDefinitionMap{
		"profile1": profileDefinition{
			Metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.[.*"},
		},
	}
	mockProfilesWithDuplicateSysobjectid := profileDefinitionMap{
		"profile1": profileDefinition{
			Metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3"},
		},
		"profile2": profileDefinition{
			Metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
			},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3"},
		},
		"profile3": profileDefinition{
			Metrics: []metricsConfig{
				{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
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
			profile, err := getProfileForSysObjectID(tt.profiles, tt.sysObjectID)
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
	c := snmpConfig{
		communityString: "my_communityString",
		authProtocol:    "my_authProtocol",
		authKey:         "my_authKey",
		privProtocol:    "my_privProtocol",
		privKey:         "my_privKey",
	}
	assert.NotContains(t, c.toString(), "my_communityString")
	assert.NotContains(t, c.toString(), "my_authKey")
	assert.NotContains(t, c.toString(), "my_privKey")

	assert.Contains(t, c.toString(), "my_authProtocol")
	assert.Contains(t, c.toString(), "my_privProtocol")
}

func Test_Configure_invalidYaml(t *testing.T) {
	check := Check{session: &snmpSession{}}

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
			expectedErr:   "build config failed: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `::x` into snmp.snmpInitConfig",
		},
		{
			name: "invalid rawInstanceConfig",
			// language=yaml
			rawInstanceConfig: []byte(`::x`),
			// language=yaml
			rawInitConfig: []byte(``),
			expectedErr:   "common configure failed: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `::x` into integration.CommonInstanceConfig",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := check.Configure(tt.rawInstanceConfig, tt.rawInitConfig, "test")
			assert.EqualError(t, err, tt.expectedErr)
		})
	}
}

func TestNumberConfigsUsingStrings(t *testing.T) {
	setConfdPathAndCleanProfiles()
	check := Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
port: "123"
timeout: "15"
retries: "5"
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)
	assert.Equal(t, uint16(123), check.config.port)
	assert.Equal(t, 15, check.config.timeout)
	assert.Equal(t, 5, check.config.retries)

}

func TestExtraTags(t *testing.T) {
	setConfdPathAndCleanProfiles()
	check := Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: abc
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)
	assert.Equal(t, []string{"snmp_device:1.2.3.4"}, check.config.getStaticTags())

	// language=yaml
	rawInstanceConfigWithExtraTags := []byte(`
ip_address: 1.2.3.4
community_string: abc
extra_tags: "extratag1:val1,extratag2:val2"
`)
	err = check.Configure(rawInstanceConfigWithExtraTags, []byte(``), "test")
	assert.Nil(t, err)
	assert.Equal(t, []string{"snmp_device:1.2.3.4", "extratag1:val1", "extratag2:val2"}, check.config.getStaticTags())
}

func Test_snmpConfig_getDeviceIDTags(t *testing.T) {
	c := &snmpConfig{
		ipAddress:    "1.2.3.4",
		extraTags:    []string{"extratag1:val1", "extratag2"},
		instanceTags: []string{"instancetag1:val1", "instancetag2"},
	}
	actualTags := c.getDeviceIDTags()

	expectedTags := []string{"extratag1:val1", "extratag2", "instancetag1:val1", "instancetag2", "snmp_device:1.2.3.4"}
	assert.Equal(t, expectedTags, actualTags)
}

func Test_snmpConfig_refreshWithProfile(t *testing.T) {
	metrics := []metricsConfig{
		{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
		{
			Symbols: []symbolConfig{
				{
					OID:  "1.2.3.4.6",
					Name: "abc",
				},
			},
			MetricTags: metricTagConfigList{
				metricTagConfig{
					Column: symbolConfig{
						OID: "1.2.3.4.7",
					},
				},
			},
		},
	}
	profile1 := profileDefinition{
		Device: deviceMeta{
			Vendor: "a-vendor",
		},
		Metrics: metrics,
		MetricTags: []metricTagConfig{
			{Tag: "interface", Column: symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
		},
		SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
	}
	mockProfiles := profileDefinitionMap{
		"profile1": profile1,
	}
	c := &snmpConfig{
		ipAddress: "1.2.3.4",
		profiles:  mockProfiles,
	}
	err := c.refreshWithProfile("f5")
	assert.EqualError(t, err, "unknown profile `f5`")

	err = c.refreshWithProfile("profile1")
	assert.NoError(t, err)

	assert.Equal(t, "profile1", c.profile)
	assert.Equal(t, profile1, *c.profileDef)
	assert.Equal(t, metrics, c.metrics)
	assert.Equal(t, []metricTagConfig{
		{Tag: "interface", Column: symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
	}, c.metricTags)
	assert.Equal(t, oidConfig{
		scalarOids: []string{"1.2.3.4.5"},
		columnOids: []string{"1.2.3.4.6", "1.2.3.4.7"},
	}, c.oidConfig)
	assert.Equal(t, []string{"snmp_profile:profile1", "device_vendor:a-vendor"}, c.profileTags)

	c = &snmpConfig{
		ipAddress:             "1.2.3.4",
		profiles:              mockProfiles,
		collectDeviceMetadata: true,
	}
	err = c.refreshWithProfile("profile1")
	assert.NoError(t, err)
	assert.Equal(t, oidConfig{
		scalarOids: []string{
			"1.2.3.4.5",
			"1.3.6.1.2.1.1.5.0",
			"1.3.6.1.2.1.1.1.0",
			"1.3.6.1.2.1.1.2.0",
		},
		columnOids: []string{
			"1.2.3.4.6",
			"1.2.3.4.7",
			"1.3.6.1.2.1.31.1.1.1.1",
			"1.3.6.1.2.1.31.1.1.1.18",
			"1.3.6.1.2.1.2.2.1.2",
			"1.3.6.1.2.1.2.2.1.6",
			"1.3.6.1.2.1.2.2.1.7",
			"1.3.6.1.2.1.2.2.1.8",
		},
	}, c.oidConfig)
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
	check := Check{session: &snmpSession{}}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: "abc"
`)
	// language=yaml
	rawInitConfig := []byte(`
oid_batch_size: 10
`)
	err := check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)
	assert.Equal(t, false, check.config.collectDeviceMetadata)

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
	err = check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)
	assert.Equal(t, true, check.config.collectDeviceMetadata)

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
	err = check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)
	assert.Equal(t, true, check.config.collectDeviceMetadata)

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
	err = check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)
	assert.Equal(t, false, check.config.collectDeviceMetadata)
}
