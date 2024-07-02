// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package autodiscovery

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestDiscoverComponentsFromConfigForSnmp(t *testing.T) {
	config.Datadog().SetConfigType("yaml")

	err := config.Datadog().ReadConfig(strings.NewReader(`
network_devices:
  autodiscovery:
    configs:
      - network: 127.0.0.1/30
`))
	assert.NoError(t, err)
	_, configListeners := DiscoverComponentsFromConfig()
	assert.Len(t, configListeners, 1)
	assert.Equal(t, "snmp", configListeners[0].Name)

	err = config.Datadog().ReadConfig(strings.NewReader(`
network_devices:
  autodiscovery:
    configs:
`))
	assert.NoError(t, err)
	_, configListeners = DiscoverComponentsFromConfig()
	assert.Empty(t, len(configListeners))

	err = config.Datadog().ReadConfig(strings.NewReader(`
snmp_listener:
  configs:
    - network: 127.0.0.1/30
`))
	assert.NoError(t, err)
	_, configListeners = DiscoverComponentsFromConfig()
	assert.Empty(t, len(configListeners))
}
