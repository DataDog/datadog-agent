// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package autodiscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestDiscoverComponentsFromConfigForSnmp(t *testing.T) {
	pkgconfigsetup.Datadog().SetConfigType("yaml")

	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
      - network: 127.0.0.1/30
`)
	_, configListeners := DiscoverComponentsFromConfig()
	require.Len(t, configListeners, 1)
	assert.Equal(t, "snmp", configListeners[0].Name)

	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
`)
	_, configListeners = DiscoverComponentsFromConfig()
	assert.Empty(t, len(configListeners))

	configmock.NewFromYAML(t, `
snmp_listener:
  configs:
    - network: 127.0.0.1/30
`)
	_, configListeners = DiscoverComponentsFromConfig()
	require.Len(t, configListeners, 1)
	assert.Equal(t, "snmp", configListeners[0].Name)

	configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
      - network_address: 127.0.0.1/30
        ignored_ip_addresses:
          - 127.0.0.3
`)
	_, configListeners = DiscoverComponentsFromConfig()
	require.Len(t, configListeners, 1)
	assert.Equal(t, "snmp", configListeners[0].Name)
}
