// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"strings"
	"testing"

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
	err := config.Datadog.ReadConfig(strings.NewReader(`
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

	conf, err := NewListenerConfig()
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
