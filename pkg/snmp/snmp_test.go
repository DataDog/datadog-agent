// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package snmp

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestBuildSNMPParams(t *testing.T) {
	config := Config{
		Network: "192.168.0.0/24",
	}
	_, err := config.BuildSNMPParams("192.168.0.1")
	assert.Equal(t, "No authentication mechanism specified", err.Error())

	config = Config{
		Network: "192.168.0.0/24",
		User:    "admin",
		Version: "4",
	}
	_, err = config.BuildSNMPParams("192.168.0.1")
	assert.Equal(t, "SNMP version not supported: 4", err.Error())

	config = Config{
		Network:   "192.168.0.0/24",
		Community: "public",
	}
	params, _ := config.BuildSNMPParams("192.168.0.1")
	assert.Equal(t, gosnmp.Version2c, params.Version)
	assert.Equal(t, "192.168.0.1", params.Target)

	config = Config{
		Network: "192.168.0.0/24",
		User:    "admin",
	}
	params, _ = config.BuildSNMPParams("192.168.0.2")
	assert.Equal(t, gosnmp.Version3, params.Version)
	assert.Equal(t, gosnmp.NoAuthNoPriv, params.MsgFlags)
	assert.Equal(t, "192.168.0.2", params.Target)

	for _, authProto := range []string{"", "md5", "sha", "sha224", "sha256", "sha384", "sha512"} {
		config = Config{
			Network:      "192.168.0.0/24",
			User:         "admin",
			AuthProtocol: authProto,
		}
		_, err = config.BuildSNMPParams("192.168.0.1")
		assert.NoError(t, err)
		assert.Equal(t, authProto, config.AuthProtocol)
	}

	for _, privProto := range []string{"", "des", "aes", "aes192", "aes192c", "aes256", "aes256c"} {
		config = Config{
			Network:      "192.168.0.0/24",
			User:         "admin",
			PrivProtocol: privProto,
		}
		_, err = config.BuildSNMPParams("192.168.0.1")
		assert.NoError(t, err)
		assert.Equal(t, privProto, config.PrivProtocol)
	}

	config = Config{
		Network:      "192.168.0.0/24",
		User:         "admin",
		AuthProtocol: "foo",
	}
	_, err = config.BuildSNMPParams("192.168.0.1")
	assert.Equal(t, "unsupported authentication protocol: foo", err.Error())

	config = Config{
		Network:      "192.168.0.0/24",
		User:         "admin",
		PrivProtocol: "bar",
	}
	_, err = config.BuildSNMPParams("192.168.0.1")
	assert.Equal(t, "unsupported privacy protocol: bar", err.Error())
}

func TestNewListenerConfig(t *testing.T) {
	configmock.NewFromYAML(t, `
snmp_listener:
  configs:
   - network: 127.0.0.1/30
   - network: 127.0.0.2/30
     collect_device_metadata: true
   - network: 127.0.0.3/30
     collect_device_metadata: false
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, true, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)

	// collect_device_metadata: false
	configmock.NewFromYAML(t, `
snmp_listener:
  collect_device_metadata: false
  configs:
   - network: 127.0.0.1/30
   - network: 127.0.0.2/30
     collect_device_metadata: true
   - network: 127.0.0.3/30
     collect_device_metadata: false
`)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, false, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)

	// collect_device_metadata: true
	configmock.NewFromYAML(t, `
snmp_listener:
  collect_device_metadata: true
  configs:
   - network: 127.0.0.1/30
   - network: 127.0.0.2/30
     collect_device_metadata: true
   - network: 127.0.0.3/30
     collect_device_metadata: false
`)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, true, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)
}

func TestNewNetworkDevicesListenerConfig(t *testing.T) {
	pkgconfigsetup.Datadog().SetConfigType("yaml")

	// default collect_device_metadata should be true
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
     - network: 127.0.0.1/30
     - network: 127.0.0.2/30
       collect_device_metadata: true
     - network: 127.0.0.3/30
       collect_device_metadata: false
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, true, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)

	// collect_device_metadata: false
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    collect_device_metadata: false
    configs:
     - network: 127.0.0.1/30
     - network: 127.0.0.2/30
       collect_device_metadata: true
     - network: 127.0.0.3/30
       collect_device_metadata: false
`)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, false, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)

	// collect_device_metadata: true
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    collect_device_metadata: true
    configs:
     - network: 127.0.0.1/30
     - network: 127.0.0.2/30
       collect_device_metadata: true
     - network: 127.0.0.3/30
       collect_device_metadata: false
`)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, true, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)
}

func TestBothListenersConfig(t *testing.T) {
	pkgconfigsetup.Datadog().SetConfigType("yaml")

	// check that network_devices config override the snmp_listener config
	configmock.NewFromYAML(t, `
snmp_listener:
  collect_device_metadata: true
  configs:
   - network: 127.0.0.1/30
   - network: 127.0.0.2/30
     collect_device_metadata: true
   - network: 127.0.0.3/30
     collect_device_metadata: false
network_devices:
  autodiscovery:
    collect_device_metadata: false
    configs:
     - network: 127.0.0.4/30
     - network: 127.0.0.5/30
       collect_device_metadata: false
     - network: 127.0.0.6/30
       collect_device_metadata: true
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, 3, len(conf.Configs))
	assert.Equal(t, "127.0.0.4/30", conf.Configs[0].Network)
	assert.Equal(t, false, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.5/30", conf.Configs[1].Network)
	assert.Equal(t, false, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.6/30", conf.Configs[2].Network)
	assert.Equal(t, true, conf.Configs[2].CollectDeviceMetadata)

	// incorrect snmp_listener config and correct network_devices config
	configmock.NewFromYAML(t, `
snmp_listener:
  configs:
   - foo: bar
network_devices:
  autodiscovery:
    collect_device_metadata: false
    configs:
     - network: 127.0.0.4/30
     - network: 127.0.0.5/30
       collect_device_metadata: false
     - network: 127.0.0.6/30
       collect_device_metadata: true
`)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)
	assert.Equal(t, 3, len(conf.Configs))
	assert.Equal(t, "127.0.0.4/30", conf.Configs[0].Network)
	assert.Equal(t, false, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.5/30", conf.Configs[1].Network)
	assert.Equal(t, false, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.6/30", conf.Configs[2].Network)
	assert.Equal(t, true, conf.Configs[2].CollectDeviceMetadata)

	// incorrect snmp_listener config and correct network_devices config
	configmock.NewFromYAML(t, `
snmp_listener:
  configs:
  - network: 127.0.0.4/30
  - network: 127.0.0.5/30
    collect_device_metadata: false
  - network: 127.0.0.6/30
    collect_device_metadata: true
network_devices:
  autodiscovery:
    - foo: bar
`)

	conf, err = NewListenerConfig()
	assert.Error(t, err)
}

func Test_LoaderConfig(t *testing.T) {
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
     - network: 127.1.0.0/30
     - network: 127.2.0.0/30
       loader: core
     - network: 127.3.0.0/30
       loader: python
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "", conf.Configs[0].Loader)
	assert.Equal(t, "core", conf.Configs[1].Loader)
	assert.Equal(t, "python", conf.Configs[2].Loader)

	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    loader: core
    configs:
     - network: 127.1.0.0/30
     - network: 127.2.0.0/30
       loader: core
     - network: 127.3.0.0/30
       loader: python
`)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "core", conf.Configs[0].Loader)
	assert.Equal(t, "core", conf.Configs[1].Loader)
	assert.Equal(t, "python", conf.Configs[2].Loader)

}

func Test_MinCollectionInterval(t *testing.T) {
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    min_collection_interval: 60
    configs:
     - network: 127.1.0.0/30
       min_collection_interval: 30
     - network: 127.2.0.0/30
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, uint(30), conf.Configs[0].MinCollectionInterval)
	assert.Equal(t, uint(60), conf.Configs[1].MinCollectionInterval)
}

func Test_Configs(t *testing.T) {
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    workers: 10
    discovery_interval: 11
    discovery_allowed_failures: 5
    loader: core
    collect_device_metadata: true
    configs:
     - authProtocol: someAuthProtocol
       authKey: someAuthKey
       privProtocol: somePrivProtocol
       privKey: somePrivKey
       community_string: someCommunityString
       snmp_version: someSnmpVersion
       network_address: 127.1.0.0/30
       interface_configs:
         '127.1.0.1':
           - match_field: "name"
             match_value: "eth0"
             in_speed: 50
             out_speed: 25
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	networkConf := conf.Configs[0]
	assert.Equal(t, 10, conf.Workers)
	assert.Equal(t, 11, conf.DiscoveryInterval)
	assert.Equal(t, 5, conf.AllowedFailures)
	assert.Equal(t, true, conf.CollectDeviceMetadata)
	assert.Equal(t, false, conf.UseDeviceISAsHostname)
	assert.Equal(t, "core", conf.Loader)
	assert.Equal(t, "someAuthProtocol", networkConf.AuthProtocol)
	assert.Equal(t, "someAuthKey", networkConf.AuthKey)
	assert.Equal(t, "somePrivProtocol", networkConf.PrivProtocol)
	assert.Equal(t, "somePrivKey", networkConf.PrivKey)
	assert.Equal(t, "someCommunityString", networkConf.Community)
	assert.Equal(t, "someSnmpVersion", networkConf.Version)
	assert.Equal(t, "127.1.0.0/30", networkConf.Network)
	assert.Equal(t, map[string][]snmpintegration.InterfaceConfig{
		"127.1.0.1": {
			{
				MatchField: "name",
				MatchValue: "eth0",
				InSpeed:    50,
				OutSpeed:   25,
			},
		},
	}, networkConf.InterfaceConfigs)

	/////////////////
	// legacy configs
	/////////////////
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    allowed_failures: 15
    configs:
     - authentication_protocol: legacyAuthProtocol
       authentication_key: legacyAuthKey
       privacy_protocol: legacyPrivProtocol
       privacy_key: legacyPrivKey
       community: legacyCommunityString
       version: legacySnmpVersion
       network: 127.2.0.0/30
`)
	conf, err = NewListenerConfig()
	assert.NoError(t, err)
	legacyConfig := conf.Configs[0]

	assert.Equal(t, 15, conf.AllowedFailures)
	assert.Equal(t, "legacyAuthProtocol", legacyConfig.AuthProtocol)
	assert.Equal(t, "legacyAuthKey", legacyConfig.AuthKey)
	assert.Equal(t, "legacyPrivProtocol", legacyConfig.PrivProtocol)
	assert.Equal(t, "legacyPrivKey", legacyConfig.PrivKey)
	assert.Equal(t, "legacyCommunityString", legacyConfig.Community)
	assert.Equal(t, "legacySnmpVersion", legacyConfig.Version)
	assert.Equal(t, "127.2.0.0/30", legacyConfig.Network)
}

func Test_NamespaceConfig(t *testing.T) {
	// Default Namespace
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
     - community_string: someCommunityString
       network_address: 127.1.0.0/30
`)
	conf, err := NewListenerConfig()
	assert.NoError(t, err)
	networkConf := conf.Configs[0]
	assert.Equal(t, "default", networkConf.Namespace)

	// Custom Namespace in network_devices
	configmock.NewFromYAML(t, `
network_devices:
  namespace: ponyo
  autodiscovery:
    configs:
    - community_string: someCommunityString
      network_address: 127.1.0.0/30
`)
	conf, err = NewListenerConfig()
	assert.NoError(t, err)
	networkConf = conf.Configs[0]
	assert.Equal(t, "ponyo", networkConf.Namespace)

	// Custom Namespace in snmp_listener
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    namespace: totoro
    configs:
    - community_string: someCommunityString
      network_address: 127.1.0.0/30
    - community_string: someCommunityString
      network_address: 127.2.0.0/30
      namespace: mononoke
`)
	conf, err = NewListenerConfig()
	assert.NoError(t, err)
	assert.Equal(t, "totoro", conf.Configs[0].Namespace)
	assert.Equal(t, "mononoke", conf.Configs[1].Namespace)
}

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, firstNonEmpty(), "")
	assert.Equal(t, firstNonEmpty("totoro"), "totoro")
	assert.Equal(t, firstNonEmpty("", "mononoke"), "mononoke")
	assert.Equal(t, firstNonEmpty("", "mononoke", "ponyo"), "mononoke")
	assert.Equal(t, firstNonEmpty("", "", "ponyo"), "ponyo")
	assert.Equal(t, firstNonEmpty("", "", ""), "")
}

func Test_UseDeviceIDAsHostname(t *testing.T) {
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    use_device_id_as_hostname: true
    configs:
     - network: 127.1.0.0/30
       use_device_id_as_hostname: false
     - network: 127.2.0.0/30
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, false, conf.Configs[0].UseDeviceIDAsHostname)
	assert.Equal(t, true, conf.Configs[1].UseDeviceIDAsHostname)
}

func Test_CollectTopology_withRootCollectTopologyFalse(t *testing.T) {
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    collect_topology: false
    configs:
     - network: 127.1.0.0/30
       collect_topology: true
     - network: 127.2.0.0/30
       collect_topology: false
     - network: 127.3.0.0/30
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, true, conf.Configs[0].CollectTopology)
	assert.Equal(t, false, conf.Configs[1].CollectTopology)
	assert.Equal(t, false, conf.Configs[2].CollectTopology)
}

func Test_CollectTopology_withRootCollectTopologyTrue(t *testing.T) {
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    collect_topology: true
    configs:
     - network: 127.1.0.0/30
       collect_topology: true
     - network: 127.2.0.0/30
       collect_topology: false
     - network: 127.3.0.0/30
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, true, conf.Configs[0].CollectTopology)
	assert.Equal(t, false, conf.Configs[1].CollectTopology)
	assert.Equal(t, true, conf.Configs[2].CollectTopology)
}

func Test_CollectTopology_withRootCollectTopologyUnset(t *testing.T) {
	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
     - network: 127.1.0.0/30
       collect_topology: true
     - network: 127.2.0.0/30
       collect_topology: false
     - network: 127.3.0.0/30
`)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, true, conf.Configs[0].CollectTopology)
	assert.Equal(t, false, conf.Configs[1].CollectTopology)
	assert.Equal(t, true, conf.Configs[2].CollectTopology)
}

func TestConfig_Digest(t *testing.T) {
	tests := []struct {
		name         string
		configA      Config
		configB      Config
		ipAddressA   string
		ipAddressB   string
		isSameDigest bool
	}{
		{
			name:         "same ipaddress",
			ipAddressA:   "1.2.3.4",
			ipAddressB:   "1.2.3.4",
			isSameDigest: true,
		},
		{
			name:       "test different ipaddress",
			ipAddressA: "1.2.3.4",
			ipAddressB: "1.2.3.5",
		},
		{
			name:       "test port",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{Port: 123},
			configB:    Config{Port: 124},
		},
		{
			name:       "test version",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{Version: "1"},
			configB:    Config{Version: "2"},
		},
		{
			name:       "test community",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{Community: "something"},
			configB:    Config{Community: "somethingElse"},
		},
		{
			name:       "test user",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{User: "myuser"},
			configB:    Config{User: "myuser2"},
		},
		{
			name:       "test AuthKey",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{AuthKey: "my-AuthKey"},
			configB:    Config{AuthKey: "my-AuthKey2"},
		},
		{
			name:       "test AuthProtocol",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{AuthProtocol: "sha"},
			configB:    Config{AuthProtocol: "md5"},
		},
		{
			name:       "test PrivKey",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{PrivKey: "abc"},
			configB:    Config{PrivKey: "123"},
		},
		{
			name:       "test PrivProtocol",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{PrivProtocol: "AES"},
			configB:    Config{PrivProtocol: "DES"},
		},
		{
			name:       "test ContextEngineID",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{ContextEngineID: "engineID"},
			configB:    Config{ContextEngineID: "engineID2"},
		},
		{
			name:       "test ContextName",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{ContextName: "someContextName"},
			configB:    Config{ContextName: "someContextName2"},
		},
		{
			name:       "test Loader",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{Loader: "core"},
			configB:    Config{Loader: "python"},
		},
		{
			name:       "test Namespace",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{Namespace: "ns1"},
			configB:    Config{Namespace: "ns2"},
		},
		{
			name:       "test different IgnoredIPAddresses",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{IgnoredIPAddresses: map[string]bool{"1.2.3.3": true}},
			configB:    Config{IgnoredIPAddresses: map[string]bool{"1.2.3.4": true}},
		},
		{
			name:       "test empty IgnoredIPAddresses",
			ipAddressA: "1.2.3.5",
			ipAddressB: "1.2.3.5",
			configA:    Config{IgnoredIPAddresses: map[string]bool{}},
			configB:    Config{IgnoredIPAddresses: map[string]bool{"1.2.3.4": true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digestA := tt.configA.Digest(tt.ipAddressA)
			digestB := tt.configB.Digest(tt.ipAddressB)
			if tt.isSameDigest {
				assert.Equal(t, digestA, digestB)
			} else {
				assert.NotEqual(t, digestA, digestB)
			}
		})
	}
}
