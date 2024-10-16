// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package autodiscovery

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	snmplistener "github.com/DataDog/datadog-agent/pkg/snmp"
)

func TestDiscoverComponentsFromConfigForSnmp(t *testing.T) {
	pkgconfigsetup.Datadog().SetConfigType("yaml")

	err := pkgconfigsetup.Datadog().ReadConfig(strings.NewReader(`
network_devices:
  autodiscovery:
    configs:
      - network: 127.0.0.1/30
`))
	assert.NoError(t, err)
	_, configListeners := DiscoverComponentsFromConfig()
	assert.Len(t, configListeners, 1)
	assert.Equal(t, "snmp", configListeners[0].Name)

	err = pkgconfigsetup.Datadog().ReadConfig(strings.NewReader(`
network_devices:
  autodiscovery:
    configs:
`))
	assert.NoError(t, err)
	_, configListeners = DiscoverComponentsFromConfig()
	assert.Empty(t, len(configListeners))

	err = pkgconfigsetup.Datadog().ReadConfig(strings.NewReader(`
snmp_listener:
  configs:
    - network: 127.0.0.1/30
`))
	assert.NoError(t, err)
	_, configListeners = DiscoverComponentsFromConfig()
	assert.Empty(t, len(configListeners))
}

func TestNewListenerConfigForSnmp(t *testing.T) {
	pkgconfigsetup.Datadog().SetConfigType("yaml")

	err := pkgconfigsetup.Datadog().ReadConfig(strings.NewReader(`
network_devices:
  autodiscovery:
    configs:
      - network_address: 127.0.0.1/30
        ignored_ip_addresses:
          - 127.0.0.2
`))
	assert.NoError(t, err)

	snmpConfig, err := snmplistener.NewListenerConfig()
	assert.NoError(t, err)

	assert.Len(t, snmpConfig.Configs, 1)
	assert.Equal(t, "127.0.0.1/30", snmpConfig.Configs[0].Network)
	assert.Equal(t, 1, len(snmpConfig.Configs[0].IgnoredIPAddresses))
	assert.Contains(t, snmpConfig.Configs[0].IgnoredIPAddresses, "127.0.0.2")
}
