// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"runtime"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	gpusubscriber "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
	gpusubscriberfxmock "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func assertContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.Contains(t, checks, name)
}

func assertNotContainsCheck(t *testing.T, checks []string, name string) {
	t.Helper()
	assert.NotContains(t, checks, name)
}

func getEnabledChecks(t *testing.T, cfg, sysprobeYamlConfig pkgconfigmodel.ReaderWriter, wmeta workloadmeta.Component, gpuSubscriber gpusubscriber.Component, npCollector npcollector.Component, statsd statsd.ClientInterface) []string {
	sysprobeConfigStruct, err := sysconfig.New("", "")
	require.NoError(t, err)

	var enabledChecks []string
	for _, check := range All(cfg, sysprobeYamlConfig, sysprobeConfigStruct, wmeta, gpuSubscriber, npCollector, statsd) {
		if check.IsEnabled() {
			enabledChecks = append(enabledChecks, check.Name())
		}
	}
	return enabledChecks
}

func TestProcessCheck(t *testing.T) {
	deps := createProcessCheckDeps(t)
	t.Run("disabled", func(t *testing.T) {
		cfg, scfg := configmock.New(t), configmock.NewSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", false)
		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure the process check can be enabled
	t.Run("enabled", func(t *testing.T) {
		cfg, scfg := configmock.New(t), configmock.NewSystemProbe(t)
		cfg.SetWithoutSource("process_config.process_collection.enabled", true)
		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd)
		assertContainsCheck(t, enabledChecks, ProcessCheckName)
	})
}

func TestConnectionsCheck(t *testing.T) {
	deps := createProcessCheckDeps(t)
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)

	t.Run("enabled", func(t *testing.T) {
		cfg, scfg := configmock.New(t), configmock.NewSystemProbe(t)
		scfg.SetWithoutSource("network_config.enabled", true)
		scfg.SetWithoutSource("system_probe_config.enabled", true)
		flavor.SetFlavor("process_agent")

		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd)
		if runtime.GOOS == "darwin" {
			assertNotContainsCheck(t, enabledChecks, ConnectionsCheckName)
		} else {
			assertContainsCheck(t, enabledChecks, ConnectionsCheckName)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		cfg, scfg := configmock.New(t), configmock.NewSystemProbe(t)
		scfg.SetWithoutSource("network_config.enabled", false)

		enabledChecks := getEnabledChecks(t, cfg, scfg, deps.WMeta, deps.GpuSubscriber, deps.NpCollector, deps.Statsd)
		assertNotContainsCheck(t, enabledChecks, ConnectionsCheckName)
	})
}

type ProcessCheckDeps struct {
	fx.In
	WMeta         workloadmeta.Component
	NpCollector   npcollector.Component
	GpuSubscriber gpusubscriber.Component
	Statsd        statsd.ClientInterface
}

func createProcessCheckDeps(t *testing.T) ProcessCheckDeps {
	return fxutil.Test[ProcessCheckDeps](t,
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		core.MockBundle(),
		gpusubscriberfxmock.MockModule(),
		npcollectorimpl.MockModule(),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
	)
}
