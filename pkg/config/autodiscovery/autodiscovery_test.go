// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package autodiscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestDiscoverComponentsFromConfigForHTTPSD(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")
	flavor.SetTestFlavor(t, flavor.ClusterAgent)

	t.Run("legacy single url triggers provider", func(t *testing.T) {
		cfg := configmock.NewFromYAML(t, `
prometheus_http_sd:
  url: http://legacy/sd
  check_template: '{"name":"openmetrics","init_config":{},"instances":[{}]}'
`)
		providers, _ := DiscoverComponentsFromConfig(cfg)
		require.True(t, containsProvider(providers, "prometheus_http_sd"))
	})

	t.Run("configs list triggers provider", func(t *testing.T) {
		cfg := configmock.NewFromYAML(t, `
prometheus_http_sd:
  configs:
    - url: http://a/sd
      check_template: '{"name":"openmetrics","init_config":{},"instances":[{}]}'
`)
		providers, _ := DiscoverComponentsFromConfig(cfg)
		require.True(t, containsProvider(providers, "prometheus_http_sd"))
	})

	t.Run("no http_sd config means no provider", func(t *testing.T) {
		cfg := configmock.NewFromYAML(t, ``)
		providers, _ := DiscoverComponentsFromConfig(cfg)
		require.False(t, containsProvider(providers, "prometheus_http_sd"))
	})
}

func containsProvider(providers []pkgconfigsetup.ConfigurationProviders, name string) bool {
	for _, p := range providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

func containsListener(listeners []pkgconfigsetup.Listeners, name string) bool {
	for _, listener := range listeners {
		if listener.Name == name {
			return true
		}
	}
	return false
}

func TestDiscoverComponentsFromEnvForProcess(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")
	cfg := configmock.NewFromYAML(t, ``)

	t.Run("process feature adds process listener", func(t *testing.T) {
		flavor.SetTestFlavor(t, flavor.DefaultAgent)
		env.SetFeatures(t, env.Process)

		_, listeners := DiscoverComponentsFromEnv(cfg)
		assert.True(t, containsListener(listeners, "process"))
	})

	t.Run("without process feature does not add process listener", func(t *testing.T) {
		flavor.SetTestFlavor(t, flavor.DefaultAgent)
		env.SetFeatures(t)

		_, listeners := DiscoverComponentsFromEnv(cfg)
		assert.False(t, containsListener(listeners, "process"))
	})

	t.Run("process feature keeps container discovery behavior", func(t *testing.T) {
		flavor.SetTestFlavor(t, flavor.DefaultAgent)
		env.SetFeatures(t, env.Process, env.Docker)

		providers, listeners := DiscoverComponentsFromEnv(cfg)
		assert.True(t, containsProvider(providers, "kubernetes-container-allinone"))
		assert.True(t, containsListener(listeners, "container"))
		assert.True(t, containsListener(listeners, "process"))
	})

	t.Run("cluster agent does not add process listener", func(t *testing.T) {
		flavor.SetTestFlavor(t, flavor.ClusterAgent)
		env.SetFeatures(t, env.Process)

		_, listeners := DiscoverComponentsFromEnv(cfg)
		assert.False(t, containsListener(listeners, "process"))
	})
}

func TestDiscoverComponentsFromConfigForDDI(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")

	t.Run("enabled on default agent in kubernetes adds instrumentation_checks provider", func(t *testing.T) {
		flavor.SetTestFlavor(t, flavor.DefaultAgent)
		t.Setenv("KUBERNETES_SERVICE_PORT", "443")

		cfg := configmock.NewFromYAML(t, `
instrumentation_crd_controller:
  enabled: true
`)
		providers, _ := DiscoverComponentsFromConfig(cfg)
		assert.True(t, containsProvider(providers, "instrumentation_checks"))
	})

	t.Run("disabled config does not add instrumentation_checks provider", func(t *testing.T) {
		flavor.SetTestFlavor(t, flavor.DefaultAgent)
		t.Setenv("KUBERNETES_SERVICE_PORT", "443")

		cfg := configmock.NewFromYAML(t, `
instrumentation_crd_controller:
  enabled: false
`)
		providers, _ := DiscoverComponentsFromConfig(cfg)
		assert.False(t, containsProvider(providers, "instrumentation_checks"))
	})

	t.Run("enabled on cluster agent does not add instrumentation_checks provider", func(t *testing.T) {
		flavor.SetTestFlavor(t, flavor.ClusterAgent)
		t.Setenv("KUBERNETES_SERVICE_PORT", "443")

		cfg := configmock.NewFromYAML(t, `
instrumentation_crd_controller:
  enabled: true
`)
		providers, _ := DiscoverComponentsFromConfig(cfg)
		assert.False(t, containsProvider(providers, "instrumentation_checks"))
	})

	t.Run("enabled outside kubernetes does not add instrumentation_checks provider", func(t *testing.T) {
		flavor.SetTestFlavor(t, flavor.DefaultAgent)
		t.Setenv("KUBERNETES_SERVICE_PORT", "")
		t.Setenv("KUBERNETES", "")

		cfg := configmock.NewFromYAML(t, `
instrumentation_crd_controller:
  enabled: true
`)
		providers, _ := DiscoverComponentsFromConfig(cfg)
		assert.False(t, containsProvider(providers, "instrumentation_checks"))
	})
}

func TestDiscoverComponentsFromConfigForSnmp(t *testing.T) {
	configmock.SetDefaultConfigType(t, "yaml")

	// The static config listener is always enabled, independently of any
	// configuration, so it is expected in every result below.
	cfg := configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
      - network: 127.0.0.1/30
`)
	_, configListeners := DiscoverComponentsFromConfig(cfg)
	assert.True(t, containsListener(configListeners, "static config"))
	assert.True(t, containsListener(configListeners, "snmp"))

	cfg = configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
`)
	_, configListeners = DiscoverComponentsFromConfig(cfg)
	assert.True(t, containsListener(configListeners, "static config"))
	assert.False(t, containsListener(configListeners, "snmp"))

	cfg = configmock.NewFromYAML(t, `
snmp_listener:
  configs:
    - network: 127.0.0.1/30
`)
	_, configListeners = DiscoverComponentsFromConfig(cfg)
	assert.True(t, containsListener(configListeners, "static config"))
	assert.True(t, containsListener(configListeners, "snmp"))

	cfg = configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
      - network_address: 127.0.0.1/30
        ignored_ip_addresses:
          - 127.0.0.3
`)
	_, configListeners = DiscoverComponentsFromConfig(cfg)
	assert.True(t, containsListener(configListeners, "static config"))
	assert.True(t, containsListener(configListeners, "snmp"))
}
