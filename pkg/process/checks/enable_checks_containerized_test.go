// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package checks

import (
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	gpusubscriber "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
	gpusubscriberfxmock "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Containers will never be enabled on environments other than linux or windows, so
// we must make sure that the build tags in this file match.

func TestContainerCheck(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)

	// Make sure the container check can be enabled if the process check is disabled
	t.Run("containers enabled; rt enabled", func(t *testing.T) {
		deps := createDeps(t)
		cfg := configmock.New(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		cfg.SetWithoutSource("process_config.disable_realtime_checks", false)
		env.SetFeatures(t, env.Docker)

		enabledChecks := getEnabledChecks(t, cfg, configmock.NewSystemProbe(t), deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd, deps.Tagger)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertContainsCheck(t, enabledChecks, RTContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure that disabling RT disables the rt container check
	t.Run("containers enabled; rt disabled", func(t *testing.T) {
		deps := createDeps(t)
		cfg := configmock.New(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		cfg.SetWithoutSource("process_config.disable_realtime_checks", true)
		env.SetFeatures(t, env.Docker)

		enabledChecks := getEnabledChecks(t, cfg, configmock.NewSystemProbe(t), deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd, deps.Tagger)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure the container check cannot be enabled if we cannot access containers
	t.Run("cannot access containers", func(t *testing.T) {
		deps := createDeps(t)
		cfg := configmock.New(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)

		enabledChecks := getEnabledChecks(t, cfg, configmock.NewSystemProbe(t), deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd, deps.Tagger)

		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure the container and process check are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		deps := createDeps(t)
		cfg := configmock.New(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", true)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		env.SetFeatures(t, env.Docker)

		enabledChecks := getEnabledChecks(t, cfg, configmock.NewSystemProbe(t), deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd, deps.Tagger)
		assertContainsCheck(t, enabledChecks, ProcessCheckName)
		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure container checks run on the core agent only
	// when run in core agent mode is enabled
	t.Run("run in core agent", func(t *testing.T) {
		deps := createDeps(t)
		cfg, scfg := configmock.New(t), configmock.NewSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		cfg.SetWithoutSource("process_config.container_collection.enabled", true)
		cfg.SetWithoutSource("process_config.run_in_core_agent.enabled", true)
		env.SetFeatures(t, env.Docker)

		flavor.SetFlavor("process_agent")
		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd, deps.Tagger)
		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)

		flavor.SetFlavor("agent")
		enabledChecks = getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd, deps.Tagger)
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

			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("process_config.disable_realtime_checks", tc.disableRealtime)
			mockConfig.SetWithoutSource("process_config.process_discovery.enabled", false) // Not an RT check so we don't care
			env.SetFeatures(t, env.Docker)

			enabledChecks := getEnabledChecks(t, mockConfig, configmock.NewSystemProbe(t), deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd, deps.Tagger)
			assert.EqualValues(tc.expectedChecks, enabledChecks)
		})
	}
}

type deps struct {
	fx.In
	WMeta         workloadmeta.Component
	NpCollector   npcollector.Component
	GpuSubscriber gpusubscriber.Component
	Statsd        statsd.ClientInterface
	Tagger        tagger.Component
}

func createDeps(t *testing.T) deps {
	return fxutil.Test[deps](t,
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		gpusubscriberfxmock.MockModule(),
		npcollectorimpl.MockModule(),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
		fx.Provide(func(t testing.TB) tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
	)
}

func assertContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.Contains(t, checks, name)
}

func assertNotContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.NotContains(t, checks, name)
}

func getEnabledChecks(t *testing.T, cfg, sysprobeYamlConfig pkgconfigmodel.ReaderWriter, wmeta workloadmeta.Component, gpuSubscriber gpusubscriber.Component, npCollector npcollector.Component, statsd statsd.ClientInterface, tagger tagger.Component) []string {
	sysprobeConfigStruct, err := sysconfig.New("", "")
	require.NoError(t, err)

	var enabledChecks []string
	for _, check := range All(cfg, sysprobeYamlConfig, sysprobeConfigStruct, wmeta, gpuSubscriber, npCollector, statsd, tagger) {
		if check.IsEnabled() {
			enabledChecks = append(enabledChecks, check.Name())
		}
	}
	return enabledChecks
}
