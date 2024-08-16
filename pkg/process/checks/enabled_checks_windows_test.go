// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestProcessCheckWindows(t *testing.T) {
	deps := createDeps(t)
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)

	// Make sure process checks run on the process agent even
	// when run in core agent mode is enabled
	t.Run("run in core agent ignored", func(t *testing.T) {
		cfg, scfg := configmock.New(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", true)
		cfg.SetWithoutSource("process_config.run_in_core_agent.enabled", true)

		flavor.SetFlavor("process_agent")
		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.NpCollector)
		assertContainsCheck(t, enabledChecks, ProcessCheckName)
	})
}

func TestProcessDiscoveryWindows(t *testing.T) {
	deps := createDeps(t)
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)

	// Make sure process discovery checks run on the process agent even
	// when run in core agent mode is enabled
	t.Run("run in core agent ignored", func(t *testing.T) {
		cfg, scfg := configmock.New(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.process_discovery.enabled", true)
		cfg.SetWithoutSource("process_config.run_in_core_agent.enabled", true)

		flavor.SetFlavor("process_agent")
		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.NpCollector)
		assertContainsCheck(t, enabledChecks, DiscoveryCheckName)
	})
}

func TestContainerCheckWindows(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)

	// Make sure container checks run on the process agent even
	// when run in core agent mode is enabled
	t.Run("run in core agent ignored", func(t *testing.T) {
		deps := createDeps(t)
		cfg, scfg := configmock.New(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		cfg.SetWithoutSource("process_config.run_in_core_agent.enabled", true)
		config.SetFeatures(t, config.Docker)

		flavor.SetFlavor("process_agent")
		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.NpCollector)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertContainsCheck(t, enabledChecks, RTContainerCheckName)
	})
}
