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
)

func assertContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.Contains(t, checks, name)
}

func assertNotContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.NotContains(t, checks, name)
}

func getEnabledChecks(scfg *sysconfig.Config) []string {
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
		enabledChecks := getEnabledChecks(scfg)
		assertContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})

	// Make sure the process_discovery check can be disabled
	t.Run("disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_discovery.enabled", false)
		enabledChecks := getEnabledChecks(scfg)
		assertNotContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})

	// Make sure the process and process_discovery checks are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_discovery.enabled", true)
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getEnabledChecks(scfg)
		assertNotContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})
}

func TestProcessCheck(t *testing.T) {
	cfg := config.Mock(t)

	scfg, err := sysconfig.New("")
	assert.NoError(t, err)

	t.Run("disabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", false)
		enabledChecks := getEnabledChecks(scfg)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure the process check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg.Set("process_config.process_collection.enabled", true)
		enabledChecks := getEnabledChecks(scfg)
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

		enabledChecks := getEnabledChecks(scfg)
		assertContainsCheck(t, enabledChecks, ConnectionsCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		syscfg.Set("network_config.enabled", false)
		scfg, err := sysconfig.New("")
		assert.NoError(t, err)

		enabledChecks := getEnabledChecks(scfg)
		assertNotContainsCheck(t, enabledChecks, ConnectionsCheckName)
	})
}
