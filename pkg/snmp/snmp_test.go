// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package snmp

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"

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

	config = Config{
		Network:      "192.168.0.0/24",
		User:         "admin",
		AuthProtocol: "foo",
	}
	_, err = config.BuildSNMPParams("192.168.0.1")
	assert.Equal(t, "Unsupported authentication protocol: foo", err.Error())

	config = Config{
		Network:      "192.168.0.0/24",
		User:         "admin",
		PrivProtocol: "bar",
	}
	_, err = config.BuildSNMPParams("192.168.0.1")
	assert.Equal(t, "Unsupported privacy protocol: bar", err.Error())
}

func TestNewListenerConfig(t *testing.T) {
	config.Datadog.SetConfigType("yaml")

	// default collect_device_metadata should be true
	err := config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  configs:
   - network: 127.0.0.1/30
   - network: 127.0.0.2/30
     collect_device_metadata: true
   - network: 127.0.0.3/30
     collect_device_metadata: false
`))
	assert.NoError(t, err)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, true, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)

	// collect_device_metadata: false
	err = config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  collect_device_metadata: false
  configs:
   - network: 127.0.0.1/30
   - network: 127.0.0.2/30
     collect_device_metadata: true
   - network: 127.0.0.3/30
     collect_device_metadata: false
`))
	assert.NoError(t, err)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, false, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)

	// collect_device_metadata: true
	err = config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  collect_device_metadata: true
  configs:
   - network: 127.0.0.1/30
   - network: 127.0.0.2/30
     collect_device_metadata: true
   - network: 127.0.0.3/30
     collect_device_metadata: false
`))
	assert.NoError(t, err)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1/30", conf.Configs[0].Network)
	assert.Equal(t, true, conf.Configs[0].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.2/30", conf.Configs[1].Network)
	assert.Equal(t, true, conf.Configs[1].CollectDeviceMetadata)
	assert.Equal(t, "127.0.0.3/30", conf.Configs[2].Network)
	assert.Equal(t, false, conf.Configs[2].CollectDeviceMetadata)
}

func Test_LoaderConfig(t *testing.T) {
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  configs:
   - network: 127.1.0.0/30
   - network: 127.2.0.0/30
     loader: core
   - network: 127.3.0.0/30
     loader: python
`))
	assert.NoError(t, err)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "", conf.Configs[0].Loader)
	assert.Equal(t, "core", conf.Configs[1].Loader)
	assert.Equal(t, "python", conf.Configs[2].Loader)

	err = config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  loader: core
  configs:
   - network: 127.1.0.0/30
   - network: 127.2.0.0/30
     loader: core
   - network: 127.3.0.0/30
     loader: python
`))
	assert.NoError(t, err)

	conf, err = NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, "core", conf.Configs[0].Loader)
	assert.Equal(t, "core", conf.Configs[1].Loader)
	assert.Equal(t, "python", conf.Configs[2].Loader)

}

func Test_MinCollectionInterval(t *testing.T) {
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  min_collection_interval: 60
  configs:
   - network: 127.1.0.0/30
     min_collection_interval: 30
   - network: 127.2.0.0/30
`))
	assert.NoError(t, err)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, uint(30), conf.Configs[0].MinCollectionInterval)
	assert.Equal(t, uint(60), conf.Configs[1].MinCollectionInterval)
}

func Test_Configs(t *testing.T) {
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
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
`))
	assert.NoError(t, err)

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

	/////////////////
	// legacy configs
	/////////////////
	err = config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  allowed_failures: 15
  configs:
   - authentication_protocol: legacyAuthProtocol
     authentication_key: legacyAuthKey
     privacy_protocol: legacyPrivProtocol
     privacy_key: legacyPrivKey
     community: legacyCommunityString
     version: legacySnmpVersion
     network: 127.2.0.0/30
`))
	assert.NoError(t, err)
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
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  configs:
   - community_string: someCommunityString
     network_address: 127.1.0.0/30
`))
	assert.NoError(t, err)
	conf, err := NewListenerConfig()
	assert.NoError(t, err)
	networkConf := conf.Configs[0]
	assert.Equal(t, "default", networkConf.Namespace)

	// Custom Namespace in network_devices
	config.Datadog.SetConfigType("yaml")
	err = config.Datadog.ReadConfig(strings.NewReader(`
network_devices:
  namespace: ponyo
snmp_listener:
  configs:
  - community_string: someCommunityString
    network_address: 127.1.0.0/30
`))
	assert.NoError(t, err)
	conf, err = NewListenerConfig()
	assert.NoError(t, err)
	networkConf = conf.Configs[0]
	assert.Equal(t, "ponyo", networkConf.Namespace)

	// Custom Namespace in snmp_listener
	config.Datadog.SetConfigType("yaml")
	err = config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  namespace: totoro
  configs:
  - community_string: someCommunityString
    network_address: 127.1.0.0/30
  - community_string: someCommunityString
    network_address: 127.2.0.0/30
    namespace: mononoke
`))
	assert.NoError(t, err)
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
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(`
snmp_listener:
  use_device_id_as_hostname: true
  configs:
   - network: 127.1.0.0/30
     use_device_id_as_hostname: false
   - network: 127.2.0.0/30
`))
	assert.NoError(t, err)

	conf, err := NewListenerConfig()
	assert.NoError(t, err)

	assert.Equal(t, false, conf.Configs[0].UseDeviceIDAsHostname)
	assert.Equal(t, true, conf.Configs[1].UseDeviceIDAsHostname)
}
