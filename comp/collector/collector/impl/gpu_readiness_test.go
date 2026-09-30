// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package collectorimpl

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

func TestNVMLReadiness(t *testing.T) {
	for _, scenario := range []string{"disabled", "already initialized", "initializes later", "timeout", "stop while waiting"} {
		t.Run(scenario, func(t *testing.T) {
			// The mock clock advances the deadline; synctest waits for goroutines
			// to finish responding without wall-clock sleeps or polling.
			synctest.Test(t, func(t *testing.T) {
				lc := compdef.NewTestLifecycle(t)
				clk := clock.NewMock()
				var initialized atomic.Bool
				initialized.Store(scenario == "already initialized")
				c := &collectorImpl{
					config: config.NewMockWithOverrides(t, map[string]interface{}{"gpu.enabled": scenario != "disabled"}),
					log:    logmock.New(t),
				}
				c.registerNVMLReadiness(lc, initialized.Load, clk)

				assertWaiting := func(want bool) {
					t.Helper()
					if want {
						assert.Contains(t, health.GetReady().Unhealthy, "gpu-nvml")
					} else {
						assert.NotContains(t, health.GetReady().Unhealthy, "gpu-nvml")
					}
					assert.NotContains(t, health.GetLive().Unhealthy, "gpu-nvml")
					assert.NotContains(t, health.GetLive().Healthy, "gpu-nvml")
				}
				assertWaiting(scenario != "disabled")
				require.NoError(t, lc.Start(context.Background()))
				stop := sync.OnceFunc(func() { require.NoError(t, lc.Stop(context.Background())) })
				t.Cleanup(stop)
				synctest.Wait()

				switch scenario {
				case "disabled", "already initialized":
					assertWaiting(false)
				case "initializes later":
					assertWaiting(true)
					clk.Add(time.Minute)
					synctest.Wait()
					assertWaiting(true)
					initialized.Store(true)
					clk.Add(time.Second)
					synctest.Wait()
					assertWaiting(false)

					// This is a startup gate: later failures must not rearm it.
					initialized.Store(false)
					clk.Add(nvmlReadinessTimeout)
					synctest.Wait()
					assertWaiting(false)
				case "timeout":
					clk.Add(nvmlReadinessTimeout - time.Nanosecond)
					synctest.Wait()
					assertWaiting(true)
					clk.Add(time.Nanosecond)
					synctest.Wait()
					assertWaiting(false)
					assert.False(t, initialized.Load(), "timing out does not claim NVML initialized")
				case "stop while waiting":
					assertWaiting(true)
					stop()
					assertWaiting(false)
				}
			})
		})
	}
}
