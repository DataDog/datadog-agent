// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"github.com/DataDog/datadog-agent/pkg/process/runner"
	"testing"

	"github.com/stretchr/testify/assert"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

func getCheckNames(checks []checks.Check) []string {
	names := make([]string, len(checks))
	for idx, ch := range checks {
		names[idx] = ch.Name()
	}
	return names
}

func assertContainsCheck(t *testing.T, checks []checks.Check, name string) {
	t.Helper()
	assert.Contains(t, getCheckNames(checks), name)
}

func assertNotContainsCheck(t *testing.T, checks []checks.Check, name string) {
	t.Helper()
	assert.NotContains(t, getCheckNames(checks), name)
}

func TestProcessDiscovery(t *testing.T) {
	scfg := &sysconfig.Config{}
	cfg := config.Mock(t)

	// Make sure the process_discovery check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", true)
		enabledChecks := getChecks(scfg, false)
		assertContainsCheck(t, enabledChecks, checks.DiscoveryCheckName)
	})

	// Make sure the process_discovery check can be disabled
	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", false)
		enabledChecks := getChecks(scfg, true)
		assertNotContainsCheck(t, enabledChecks, checks.DiscoveryCheckName)
	})

	// Make sure the process and process_discovery checks are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg.Set("process_config.process_discovery.enabled", true)
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getChecks(scfg, true)
		assertNotContainsCheck(t, enabledChecks, checks.DiscoveryCheckName)
	})
}

func TestContainerCheck(t *testing.T) {
	scfg := &sysconfig.Config{}
	cfg := config.Mock(t)

	// Make sure the container check can be enabled if the process check is disabled
	t.Run("containers enabled; rt enabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", false)

		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ContainerCheckName)
		assertContainsCheck(t, enabledChecks, checks.RTContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.ProcessCheckName)
	})

	// Make sure that disabling RT disables the rt container check
	t.Run("containers enabled; rt disabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", true)

		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.RTContainerCheckName)
	})

	// Make sure the container check cannot be enabled if we cannot access containers
	t.Run("cannot access containers", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getChecks(scfg, false)
		assertNotContainsCheck(t, enabledChecks, checks.ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.RTContainerCheckName)
	})

	// Make sure the container and process check are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", true)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ProcessCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, checks.RTContainerCheckName)
	})
}

func TestProcessCheck(t *testing.T) {
	cfg := config.Mock(t)

	scfg, err := sysconfig.New("")
	assert.NoError(t, err)

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		enabledChecks := getChecks(scfg, true)
		assertNotContainsCheck(t, enabledChecks, checks.ProcessCheckName)
	})

	// Make sure the process check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ProcessCheckName)
	})
}

func TestConnectionsCheck(t *testing.T) {
	syscfg := config.MockSystemProbe(t)
	syscfg.Set("system_probe_config.enabled", true)

	t.Run("enabled", func(t *testing.T) {
		syscfg.Set("network_config.enabled", true)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getChecks(scfg, true)
		assertContainsCheck(t, enabledChecks, checks.ConnectionsCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		syscfg.Set("network_config.enabled", false)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getChecks(scfg, true)
		assertNotContainsCheck(t, enabledChecks, checks.ConnectionsCheckName)
	})
}

func TestPodCheck(t *testing.T) {
	config.SetDetectedFeatures(config.FeatureMap{config.Kubernetes: {}})
	defer config.SetDetectedFeatures(nil)

	t.Run("enabled", func(t *testing.T) {
		// Resets the cluster name so that it isn't cached during the call to `getChecks()`
		clustername.ResetClusterName()
		defer clustername.ResetClusterName()

		cfg := config.Mock(t)
		cfg.Set("orchestrator_explorer.enabled", true)
		cfg.Set("cluster_name", "test")

		enabledChecks := getChecks(&sysconfig.Config{}, true)
		assertContainsCheck(t, enabledChecks, checks.PodCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		clustername.ResetClusterName()
		defer clustername.ResetClusterName()

		cfg := config.Mock(t)
		cfg.Set("orchestrator_explorer.enabled", false)

		enabledChecks := getChecks(&sysconfig.Config{}, true)
		assertNotContainsCheck(t, enabledChecks, checks.PodCheckName)
	})
}

func TestProcessEventsCheck(t *testing.T) {
	scfg := &sysconfig.Config{}
	cfg := config.Mock(t)

	t.Run("default", func(t *testing.T) {
		enabledChecks := getChecks(scfg, false)
		assertNotContainsCheck(t, enabledChecks, checks.ProcessEventsCheckName)
	})

	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.event_collection.enabled", true)
		enabledChecks := getChecks(scfg, false)
		assertContainsCheck(t, enabledChecks, checks.ProcessEventsCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.event_collection.enabled", false)
		enabledChecks := getChecks(scfg, false)
		assertNotContainsCheck(t, enabledChecks, checks.ProcessEventsCheckName)
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
			expectedChecks:  []string{checks.ContainerCheckName},
		},
		{
			name:            "false",
			disableRealtime: false,
			expectedChecks:  []string{checks.ContainerCheckName, checks.RTContainerCheckName},
		},
	}

	assert := assert.New(t)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := config.Mock(t)
			mockConfig.Set("process_config.disable_realtime_checks", tc.disableRealtime)
			mockConfig.Set("process_config.process_discovery.enabled", false) // Not an RT check so we don't care

			enabledChecks := getChecks(&sysconfig.Config{}, true)
			assert.EqualValues(tc.expectedChecks, getCheckNames(enabledChecks))

			c, err := runner.NewCollector(nil, &checks.HostInfo{}, enabledChecks)
			assert.NoError(err)
			assert.Equal(!tc.disableRealtime, c.runRealTime)
		})
	}
}
