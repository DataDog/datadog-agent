// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && linux && nvml

package run

import (
	"testing"
	"testing/synctest"
	"time"

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

func TestGPUReadinessTimeout(t *testing.T) {
	for _, tt := range []struct {
		name    string
		env     string
		yaml    string
		timeout time.Duration
	}{
		{name: "default", timeout: time.Minute},
		{name: "environment override", env: "2m", timeout: 2 * time.Minute},
		{name: "yaml override", yaml: "45s", timeout: 45 * time.Second},
		{name: "zero disables readiness", env: "0s"},
		{name: "negative disables readiness", env: "-1s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("DD_GPU_READINESS_TIMEOUT", tt.env)
			}
			yaml := "gpu:\n  enabled: true\n"
			if tt.yaml != "" {
				yaml += "  readiness_timeout: " + tt.yaml + "\n"
			}
			cfg := config.NewMockFromYAML(t, yaml)
			synctest.Test(t, func(t *testing.T) {
				readiness := newGPUReadiness(cfg)
				if tt.timeout == 0 {
					require.Nil(t, readiness)
					require.NotContains(t, health.GetReady().Unhealthy, gpu.CheckName)
					require.NotContains(t, health.GetReady().Healthy, gpu.CheckName)
					return
				}
				require.NotNil(t, readiness)
				t.Cleanup(func() { readiness.CleanupForTest(t) })
				require.Contains(t, health.GetReady().Unhealthy, gpu.CheckName)

				// The configured deadline applies even if the check never runs.
				synctest.Wait()
				time.Sleep(tt.timeout - time.Second)
				synctest.Wait()
				require.Contains(t, health.GetReady().Unhealthy, gpu.CheckName)
				time.Sleep(16 * time.Second)
				synctest.Wait()
				require.NotContains(t, health.GetReady().Unhealthy, gpu.CheckName)
				require.Contains(t, health.GetReady().Healthy, gpu.CheckName)
			})
		})
	}
}
