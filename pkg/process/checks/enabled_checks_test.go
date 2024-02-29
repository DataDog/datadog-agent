// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func assertContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.Contains(t, checks, name)
}

func assertNotContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.NotContains(t, checks, name)
}

func getEnabledChecks(t *testing.T, cfg, sysprobeYamlConfig config.ReaderWriter) []string {
	sysprobeConfigStruct, err := sysconfig.New("")
	require.NoError(t, err)

	var enabledChecks []string
	for _, check := range All(cfg, sysprobeYamlConfig, sysprobeConfigStruct) {
		if check.IsEnabled() {
			enabledChecks = append(enabledChecks, check.Name())
		}
	}
	return enabledChecks
}

func TestProcessDiscovery(t *testing.T) {
	// Make sure the process_discovery check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg, sysprobeCfg := config.Mock(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_discovery.enabled", true)
		enabledChecks := getEnabledChecks(t, cfg, sysprobeCfg)
		assertContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})

	// Make sure the process_discovery check can be disabled
	t.Run("disabled", func(t *testing.T) {
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_discovery.enabled", false)
		enabledChecks := getEnabledChecks(t, cfg, scfg)
		assertNotContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})

	// Make sure the process and process_discovery checks are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_discovery.enabled", true)
		cfg.SetWithoutSource("process_config.process_collection.enabled", true)
		enabledChecks := getEnabledChecks(t, cfg, scfg)
		assertNotContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})
}

func TestProcessCheck(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		enabledChecks := getEnabledChecks(t, cfg, scfg)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure the process check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", true)
		enabledChecks := getEnabledChecks(t, cfg, scfg)
		assertContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure the process check is disabled on the process agent
	// when core-agent mode is on
	t.Run("core agent mode", func(t *testing.T) {
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", true)
		cfg.SetWithoutSource("process_config.run_in_core_agent.enabled", true)
		flavor.SetFlavor("process_agent")
		enabledChecks := getEnabledChecks(t, cfg, scfg)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})
}

func TestConnectionsCheck(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		scfg.SetWithoutSource("network_config.enabled", true)
		scfg.SetWithoutSource("system_probe_config.enabled", true)
		flavor.SetFlavor("process_agent")

		enabledChecks := getEnabledChecks(t, cfg, scfg)
		if runtime.GOOS == "darwin" {
			assertNotContainsCheck(t, enabledChecks, ConnectionsCheckName)
		} else {
			assertContainsCheck(t, enabledChecks, ConnectionsCheckName)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		scfg.SetWithoutSource("network_config.enabled", false)

		enabledChecks := getEnabledChecks(t, cfg, scfg)
		assertNotContainsCheck(t, enabledChecks, ConnectionsCheckName)
	})

	// Make sure the connections check is disabled on the core agent
	// and enabled in the process agent when process checks run in core agent
	t.Run("core agent mode", func(t *testing.T) {
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", true)
		cfg.SetWithoutSource("process_config.run_in_core_agent.enabled", true)
		scfg.SetWithoutSource("network_config.enabled", true)
		scfg.SetWithoutSource("system_probe_config.enabled", true)

		flavor.SetFlavor("agent")
		enabledChecks := getEnabledChecks(t, cfg, scfg)
		assertNotContainsCheck(t, enabledChecks, ConnectionsCheckName)

		flavor.SetFlavor("process_agent")
		enabledChecks = getEnabledChecks(t, cfg, scfg)
		assertContainsCheck(t, enabledChecks, ConnectionsCheckName)
	})
}
