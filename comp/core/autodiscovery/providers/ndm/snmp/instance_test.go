// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
)

func ptr[T any](v T) *T { return &v }

func TestKeyConfigParsesTheBackendPayloadVerbatim(t *testing.T) {
	raw := []byte(`{
		"init_config": {
			"namespace": "prod",
			"device_tags_source": "both",
			"oid_batch_size": 20,
			"bulk_max_repetitions": 30,
			"min_collection_interval": 60,
			"use_remote_config_profiles": true,
			"collect_topology": false,
			"collect_vpn": true,
			"global_metrics": [{"MIB": "IF-MIB", "symbol": {"OID": "1.3.6.1.2.1.1.3.0", "name": "sysUpTimeInstance"}}],
			"ping": {"enabled": true, "count": 3, "interval_ms": 25, "timeout_ms": 4000, "linux": {"use_raw_socket": true}}
		},
		"instances": [
			{
				"ip_address": "10.0.0.1",
				"cred": {"id": "cred-1", "name": "core-switches"},
				"port": 1161,
				"timeout_sec": 5,
				"retries": 4,
				"oid_batch_size": 10,
				"bulk_max_repetitions": 15,
				"min_collection_interval": 30,
				"profile": "cisco-nexus",
				"use_remote_config_profiles": false,
				"use_global_metrics": false,
				"collect_topology": true,
				"collect_vpn": false,
				"namespace": "edge",
				"tags": ["site:paris"],
				"device_tags_source": "agent",
				"metrics": [{"MIB": "IF-MIB", "symbol": {"OID": "1.3.6.1.2.1.2.1.0", "name": "ifNumber"}}],
				"metric_tags": [{"tag": "snmp_host", "symbol": {"OID": "1.3.6.1.2.1.1.5.0", "name": "sysName"}}],
				"ping": {"enabled": false, "count": 1, "interval_ms": 10, "timeout_ms": 1000, "linux": {"use_raw_socket": false}},
				"interface_configs": [{"match_field": "name", "match_value": "eth0", "in_speed": 25, "out_speed": 10, "tags": ["role:uplink"], "disabled": true}]
			},
			{"ip_address": "10.0.0.2", "cred": {"id": "cred-2", "name": "routers"}}
		]
	}`)

	var got keyConfig
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, initConfig{
		Namespace:             "prod",
		DeviceTagsSource:      "both",
		OIDBatchSize:          20,
		BulkMaxRepetitions:    30,
		MinCollectionInterval: 60,
		UseRCProfiles:         ptr(true),
		CollectTopology:       ptr(false),
		CollectVPN:            ptr(true),
		GlobalMetrics: []profiledefinition.MetricsConfig{{
			MIB:    "IF-MIB",
			Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"},
		}},
		Ping: pingConfig{
			Enabled:    ptr(true),
			Count:      3,
			IntervalMS: 25,
			TimeoutMS:  4000,
			Linux:      pingLinuxConfig{UseRawSocket: ptr(true)},
		},
	}, got.InitConfig)

	require.Len(t, got.Instances, 2)
	assert.Equal(t, documentInstance{
		IPAddress:             "10.0.0.1",
		Cred:                  credentialRef{ID: "cred-1", Name: "core-switches"},
		Port:                  1161,
		TimeoutSec:            5,
		Retries:               4,
		OIDBatchSize:          10,
		BulkMaxRepetitions:    15,
		MinCollectionInterval: 30,
		Profile:               "cisco-nexus",
		UseRCProfiles:         ptr(false),
		UseGlobalMetrics:      ptr(false),
		CollectTopology:       ptr(true),
		CollectVPN:            ptr(false),
		Namespace:             "edge",
		Tags:                  []string{"site:paris"},
		DeviceTagsSource:      "agent",
		Metrics: []profiledefinition.MetricsConfig{{
			MIB:    "IF-MIB",
			Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.1.0", Name: "ifNumber"},
		}},
		MetricTags: []profiledefinition.MetricTagConfig{{
			Tag:    "snmp_host",
			Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
		}},
		Ping: pingConfig{
			Enabled:    ptr(false),
			Count:      1,
			IntervalMS: 10,
			TimeoutMS:  1000,
			Linux:      pingLinuxConfig{UseRawSocket: ptr(false)},
		},
		InterfaceConfigs: []snmpintegration.InterfaceConfig{{
			MatchField: "name",
			MatchValue: "eth0",
			InSpeed:    25,
			OutSpeed:   10,
			Tags:       []string{"role:uplink"},
			Disabled:   true,
		}},
	}, got.Instances[0])

	assert.Equal(t, documentInstance{
		IPAddress: "10.0.0.2",
		Cred:      credentialRef{ID: "cred-2", Name: "routers"},
	}, got.Instances[1])
}

func TestRenderInitConfigCarriesEverySetting(t *testing.T) {
	got, err := renderInitConfig(initConfig{
		Namespace:             "prod",
		DeviceTagsSource:      "both",
		OIDBatchSize:          20,
		BulkMaxRepetitions:    30,
		MinCollectionInterval: 60,
		UseRCProfiles:         ptr(true),
		CollectTopology:       ptr(false),
		CollectVPN:            ptr(true),
		GlobalMetrics: []profiledefinition.MetricsConfig{{
			MIB:    "IF-MIB",
			Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"},
		}},
		Ping: pingConfig{
			Enabled:    ptr(true),
			Count:      3,
			IntervalMS: 25,
			TimeoutMS:  4000,
			Linux:      pingLinuxConfig{UseRawSocket: ptr(true)},
		},
	})
	require.NoError(t, err)

	assert.YAMLEq(t, `
namespace: prod
device_tags_source: both
oid_batch_size: 20
bulk_max_repetitions: 30
min_collection_interval: 60
use_remote_config_profiles: true
collect_topology: false
collect_vpn: true
global_metrics:
  - MIB: IF-MIB
    symbol:
      OID: 1.3.6.1.2.1.1.3.0
      name: sysUpTimeInstance
ping:
  enabled: true
  count: 3
  interval: 25
  timeout: 4000
  linux:
    use_raw_socket: true
`, string(got))
}

func TestRenderInitConfigOfAnEmptyBlockIsEmpty(t *testing.T) {
	got, err := renderInitConfig(initConfig{})
	require.NoError(t, err)
	assert.YAMLEq(t, "{}", string(got))
}

func TestRenderInstanceForV2C(t *testing.T) {
	got, err := renderInstance(
		documentInstance{IPAddress: "10.0.0.1", Cred: credentialRef{ID: "cred-1", Name: "v2c-public"}},
		credentials.Credential{ID: "cred-1", Name: "v2c-public", SNMPVersion: "2c", CommunityString: "public"},
	)
	require.NoError(t, err)

	assert.YAMLEq(t, "ip_address: 10.0.0.1\nsnmp_version: \"2c\"\ncommunity_string: public\n", string(got))
	assert.NotContains(t, string(got), "port")
	assert.NotContains(t, string(got), "timeout")
	assert.NotContains(t, string(got), "retries")
	assert.NotContains(t, string(got), "authProtocol")
	assert.NotContains(t, string(got), "user")
}

func TestRenderInstanceForV1(t *testing.T) {
	got, err := renderInstance(
		documentInstance{IPAddress: "10.0.0.9"},
		credentials.Credential{Name: "v1", SNMPVersion: "1", CommunityString: "public"},
	)
	require.NoError(t, err)
	assert.YAMLEq(t, "ip_address: 10.0.0.9\nsnmp_version: \"1\"\ncommunity_string: public\n", string(got))
}

func TestRenderInstanceForV3(t *testing.T) {
	got, err := renderInstance(
		documentInstance{IPAddress: "10.0.0.2"},
		credentials.Credential{
			Name:            "v3-full",
			SNMPVersion:     "3",
			User:            "test-user",
			AuthProtocol:    "SHA",
			AuthKey:         "test-auth-key",
			PrivProtocol:    "AES",
			PrivKey:         "test-priv-key",
			ContextName:     "test-context",
			ContextEngineID: "test-engine-id",
		},
	)
	require.NoError(t, err)

	assert.YAMLEq(t, `
ip_address: 10.0.0.2
snmp_version: "3"
user: test-user
authProtocol: SHA
authKey: test-auth-key
privProtocol: AES
privKey: test-priv-key
context_name: test-context
context_engine_id: test-engine-id
`, string(got))
	assert.NotContains(t, string(got), "community_string")
}

func TestRenderInstanceCarriesEveryPerDeviceSetting(t *testing.T) {
	got, err := renderInstance(documentInstance{
		IPAddress:             "10.0.0.1",
		Cred:                  credentialRef{ID: "cred-1", Name: "core-switches"},
		Port:                  1161,
		TimeoutSec:            5,
		Retries:               4,
		OIDBatchSize:          10,
		BulkMaxRepetitions:    15,
		MinCollectionInterval: 30,
		Profile:               "cisco-nexus",
		UseRCProfiles:         ptr(false),
		UseGlobalMetrics:      ptr(false),
		CollectTopology:       ptr(true),
		CollectVPN:            ptr(false),
		Namespace:             "edge",
		Tags:                  []string{"site:paris"},
		DeviceTagsSource:      "agent",
		Metrics: []profiledefinition.MetricsConfig{{
			MIB:    "IF-MIB",
			Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.1.0", Name: "ifNumber"},
		}},
		MetricTags: []profiledefinition.MetricTagConfig{{
			Tag:    "snmp_host",
			Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
		}},
		Ping: pingConfig{
			Enabled:    ptr(false),
			Count:      1,
			IntervalMS: 10,
			TimeoutMS:  1000,
			Linux:      pingLinuxConfig{UseRawSocket: ptr(false)},
		},
		InterfaceConfigs: []snmpintegration.InterfaceConfig{{
			MatchField: "name",
			MatchValue: "eth0",
			InSpeed:    25,
			OutSpeed:   10,
			Tags:       []string{"role:uplink"},
			Disabled:   true,
		}},
	}, credentials.Credential{ID: "cred-1", Name: "core-switches", SNMPVersion: "2c", CommunityString: "public"})
	require.NoError(t, err)

	assert.YAMLEq(t, `
ip_address: 10.0.0.1
snmp_version: "2c"
community_string: public
port: 1161
timeout: 5
retries: 4
oid_batch_size: 10
bulk_max_repetitions: 15
min_collection_interval: 30
profile: cisco-nexus
use_remote_config_profiles: false
use_global_metrics: false
collect_topology: true
collect_vpn: false
namespace: edge
tags:
  - site:paris
device_tags_source: agent
metrics:
  - MIB: IF-MIB
    symbol:
      OID: 1.3.6.1.2.1.2.1.0
      name: ifNumber
metric_tags:
  - tag: snmp_host
    symbol:
      OID: 1.3.6.1.2.1.1.5.0
      name: sysName
ping:
  enabled: false
  count: 1
  interval: 10
  timeout: 1000
  linux:
    use_raw_socket: false
interface_configs:
  - match_field: name
    match_value: eth0
    in_speed: 25
    out_speed: 10
    tags:
      - role:uplink
    disabled: true
`, string(got))
}

func TestRenderInstanceKeepsTheVersionAString(t *testing.T) {
	// The snmp check reads snmp_version as a string, so it must stay quoted.
	got, err := renderInstance(documentInstance{IPAddress: "10.0.0.3"}, credentials.Credential{Name: "v3", SNMPVersion: "3", User: "u"})
	require.NoError(t, err)
	assert.Contains(t, string(got), `snmp_version: "3"`)
}

func TestRenderPingOmitsTheBlockTheDocumentDidNotSet(t *testing.T) {
	assert.Nil(t, renderPing(pingConfig{}))
}

func TestRenderPingKeepsAnExplicitDisable(t *testing.T) {
	assert.Equal(t, &checkPingBlock{Enabled: ptr(false)}, renderPing(pingConfig{Enabled: ptr(false)}))
}

func TestRenderPingMapsTheMillisecondNames(t *testing.T) {
	got := renderPing(pingConfig{IntervalMS: 25, TimeoutMS: 4000})
	require.NotNil(t, got)
	assert.Equal(t, 25, got.Interval)
	assert.Equal(t, 4000, got.Timeout)
}
