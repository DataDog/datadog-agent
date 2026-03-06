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
)

func TestDiscoverComponentsFromConfigForSnmp(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")

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

func TestDiscoverComponentsFromConfigPrometheus(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")

	configmock.NewFromYAML(t, `
prometheus_scrape:
  enabled: true
`)
	providers, _ := DiscoverComponentsFromConfig()
	found := false
	for _, p := range providers {
		if p.Name == "prometheus_pods" || p.Name == "prometheus_services" {
			found = true
			assert.True(t, p.Polling)
		}
	}
	assert.True(t, found, "expected a prometheus provider to be added")
}

func TestDiscoverComponentsFromConfigDBMAurora(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")

	configmock.NewFromYAML(t, `
database_monitoring:
  autodiscovery:
    aurora:
      enabled: true
`)
	_, listeners := DiscoverComponentsFromConfig()
	found := false
	for _, l := range listeners {
		if l.Name == "database-monitoring-aurora" {
			found = true
		}
	}
	assert.True(t, found, "expected aurora listener to be added")
}

func TestDiscoverComponentsFromConfigDBMRds(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")

	configmock.NewFromYAML(t, `
database_monitoring:
  autodiscovery:
    rds:
      enabled: true
`)
	_, listeners := DiscoverComponentsFromConfig()
	found := false
	for _, l := range listeners {
		if l.Name == "database-monitoring-rds" {
			found = true
		}
	}
	assert.True(t, found, "expected rds listener to be added")
}

func TestDiscoverComponentsFromEnvDefault(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")
	configmock.NewFromYAML(t, ``)

	providers, listeners := DiscoverComponentsFromEnv()

	// Should always include environment and static config listeners
	listenerNames := make([]string, 0, len(listeners))
	for _, l := range listeners {
		listenerNames = append(listenerNames, l.Name)
	}
	assert.Contains(t, listenerNames, "environment")
	assert.Contains(t, listenerNames, "static config")

	// Without container features, no container providers should be added
	for _, p := range providers {
		assert.NotEqual(t, "container", p.Name)
	}
}
