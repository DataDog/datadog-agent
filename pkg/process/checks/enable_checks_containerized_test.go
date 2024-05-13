// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Containers will never be enabled on environments other than linux or windows, so
// we must make sure that the build tags in this file match.

func TestContainerCheck(t *testing.T) {
	deps := createDeps(t)
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)

	// Make sure the container check can be enabled if the process check is disabled
	t.Run("containers enabled; rt enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		cfg.SetWithoutSource("process_config.disable_realtime_checks", false)
		config.SetFeatures(t, config.Docker)

		enabledChecks := getEnabledChecks(t, cfg, config.MockSystemProbe(t), deps.WMeta)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertContainsCheck(t, enabledChecks, RTContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure that disabling RT disables the rt container check
	t.Run("containers enabled; rt disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		cfg.SetWithoutSource("process_config.disable_realtime_checks", true)
		config.SetFeatures(t, config.Docker)

		enabledChecks := getEnabledChecks(t, cfg, config.MockSystemProbe(t), deps.WMeta)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure the container check cannot be enabled if we cannot access containers
	t.Run("cannot access containers", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)

		enabledChecks := getEnabledChecks(t, cfg, config.MockSystemProbe(t), deps.WMeta)

		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure the container and process check are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", true)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		config.SetFeatures(t, config.Docker)

		enabledChecks := getEnabledChecks(t, cfg, config.MockSystemProbe(t), deps.WMeta)
		assertContainsCheck(t, enabledChecks, ProcessCheckName)
		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure container checks run on the core agent only
	// when run in core agent mode is enabled
	t.Run("run in core agent", func(t *testing.T) {
		deps := createDeps(t)
		cfg, scfg := config.Mock(t), config.MockSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		cfg.SetWithoutSource("process_config.run_in_core_agent.enabled", true)
		config.SetFeatures(t, config.Docker)

		flavor.SetFlavor("process_agent")
		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta)
		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)

		flavor.SetFlavor("agent")
		enabledChecks = getEnabledChecks(t, cfg, scfg, deps.WMeta)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertContainsCheck(t, enabledChecks, RTContainerCheckName)
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
	deps := createDeps(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)

			mockConfig := config.Mock(t)
			mockConfig.SetWithoutSource("process_config.disable_realtime_checks", tc.disableRealtime)
			mockConfig.SetWithoutSource("process_config.process_discovery.enabled", false) // Not an RT check so we don't care
			config.SetFeatures(t, config.Docker)

			enabledChecks := getEnabledChecks(t, mockConfig, config.MockSystemProbe(t), deps.WMeta)
			assert.EqualValues(tc.expectedChecks, enabledChecks)
		})
	}
}

type deps struct {
	fx.In
	WMeta workloadmeta.Component
}

func createDeps(t *testing.T) deps {
	return fxutil.Test[deps](t, core.MockBundle(), workloadmeta.MockModule(), fx.Supply(workloadmeta.NewParams()))
}
