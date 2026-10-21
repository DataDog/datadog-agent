// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && linux && nvml

package run

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/commonchecks"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestCommandGPUEnabled(t *testing.T) {
	t.Setenv("DD_GPU_ENABLED", "true")

	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run"},
		run,
		func(cfg config.Component, tagger tagger.Component, wmeta workloadmeta.Component, filterStore workloadfilter.Component, readiness *gpu.Readiness) {
			require.NotNil(t, readiness)
			t.Cleanup(func() { readiness.CleanupForTest(t) })

			// This callback runs during Fx construction, before lifecycle hooks
			// can start the health probe or configure the GPU check.
			require.Contains(t, health.GetReady().Unhealthy, gpu.CheckName)
			require.NotContains(t, health.GetLive().Unhealthy, gpu.CheckName)

			commonchecks.RegisterChecks(wmeta, filterStore, tagger, cfg, telemetryimpl.NewMockComponent(), nil, nil, nil, nil, option.None[networkconfigmanagement.Component](), readiness)
			loader, err := corecheckLoader.NewGoCheckLoader()
			require.NoError(t, err)
			loadedCheck, err := loader.Load(mocksender.CreateDefaultDemultiplexer(t), integration.Config{Name: gpu.CheckName}, integration.Data{}, 0)
			require.NoError(t, err)
			t.Cleanup(loadedCheck.Cancel)
			gpuCheck, ok := loadedCheck.(*gpu.Check)
			require.True(t, ok)
			gpuCheck.AssertReadinessForTest(t, readiness)
		})
}
