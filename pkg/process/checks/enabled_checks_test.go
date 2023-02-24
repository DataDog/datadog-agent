// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

func assertContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.Contains(t, checks, name)
}

func assertNotContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.NotContains(t, checks, name)
}

func getEnabledChecks(t *testing.T, scfg *sysconfig.Config) []string {
	t.Helper()

	var enabledChecks []string
	for _, check := range All(scfg) {
		if check.IsEnabled() {
			enabledChecks = append(enabledChecks, check.Name())
		}
	}
	return enabledChecks
}

func TestProcessDiscovery(t *testing.T) {
	scfg := &sysconfig.Config{}

	// Make sure the process_discovery check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_discovery.enabled", true)
		enabledChecks := getEnabledChecks(t, scfg)
		assertContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})

	// Make sure the process_discovery check can be disabled
	t.Run("disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_discovery.enabled", false)
		enabledChecks := getEnabledChecks(t, scfg)
		assertNotContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})

	// Make sure the process and process_discovery checks are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_discovery.enabled", true)
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getEnabledChecks(t, scfg)
		assertNotContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})
}

func TestContainerCheck(t *testing.T) {
	scfg := &sysconfig.Config{}

	// Make sure the container check can be enabled if the process check is disabled
	t.Run("containers enabled; rt enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", false)

		enabledChecks := getEnabledChecks(t, scfg)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertContainsCheck(t, enabledChecks, RTContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure that disabling RT disables the rt container check
	t.Run("containers enabled; rt disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", true)

		enabledChecks := getEnabledChecks(t, scfg)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure the container check cannot be enabled if we cannot access containers
	t.Run("cannot access containers", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getEnabledChecks(t, scfg)
		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure the container and process check are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", true)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getEnabledChecks(t, scfg)
		assertContainsCheck(t, enabledChecks, ProcessCheckName)
		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})
}

func TestProcessCheck(t *testing.T) {
	cfg := config.Mock(t)

	scfg, err := sysconfig.New("")
	assert.NoError(t, err)

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		enabledChecks := getEnabledChecks(t, scfg)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure the process check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getEnabledChecks(t, scfg)
		assertContainsCheck(t, enabledChecks, ProcessCheckName)
	})
}

func TestConnectionsCheck(t *testing.T) {
	syscfg := config.MockSystemProbe(t)
	syscfg.Set("system_probe_config.enabled", true)

	t.Run("enabled", func(t *testing.T) {
		syscfg.Set("network_config.enabled", true)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getEnabledChecks(t, scfg)
		assertContainsCheck(t, enabledChecks, ConnectionsCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		syscfg.Set("network_config.enabled", false)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getEnabledChecks(t, scfg)
		assertNotContainsCheck(t, enabledChecks, ConnectionsCheckName)
	})
}

func TestPodCheck(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{config.Kubernetes: {}})
	defer config.SetDetectedFeatures(nil)

	t.Run("enabled", func(t *testing.T) {
		// Resets the cluster name so that it isn't cached during the call to `getEnabledChecks()`
		clustername.ResetClusterName()
		defer clustername.ResetClusterName()

		cfg := config.Mock(t)
		cfg.Set("orchestrator_explorer.enabled", true)
		cfg.Set("cluster_name", "test")

		enabledChecks := getEnabledChecks(t, &sysconfig.Config{})
		assertContainsCheck(t, enabledChecks, PodCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		clustername.ResetClusterName()
		defer clustername.ResetClusterName()

		cfg := config.Mock(t)
		cfg.Set("orchestrator_explorer.enabled", false)

		enabledChecks := getEnabledChecks(t, &sysconfig.Config{})
		assertNotContainsCheck(t, enabledChecks, PodCheckName)
	})
}

func TestProcessEventsCheckEnabled(t *testing.T) {
	scfg := &sysconfig.Config{}
	cfg := config.Mock(t)

	t.Run("default", func(t *testing.T) {
		enabledChecks := getEnabledChecks(t, scfg)
		assertNotContainsCheck(t, enabledChecks, ProcessEventsCheckName)
	})

	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.event_collection.enabled", true)
		enabledChecks := getEnabledChecks(t, scfg)
		assertContainsCheck(t, enabledChecks, ProcessEventsCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.event_collection.enabled", false)
		enabledChecks := getEnabledChecks(t, scfg)
		assertNotContainsCheck(t, enabledChecks, ProcessEventsCheckName)
	})
}

func TestDisableRealTime(t *testing.T) {
	tests := []struct {
		name            string
		disableRealtime bool
		expectedChecks  []string
	}{
		{
			name:            "true",
			disableRealtime: true,
			expectedChecks:  []string{ContainerCheckName},
		},
		{
			name:            "false",
			disableRealtime: false,
			expectedChecks:  []string{ContainerCheckName, RTContainerCheckName},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)

			mockConfig := config.Mock(t)
			mockConfig.Set("process_config.disable_realtime_checks", tc.disableRealtime)
			mockConfig.Set("process_config.process_discovery.enabled", false) // Not an RT check so we don't care

			enabledChecks := getEnabledChecks(t, &sysconfig.Config{})
			assert.EqualValues(tc.expectedChecks, enabledChecks)
		})
	}
}
