// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package collectorimpl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/collector/collector/impl/internal/middleware"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestGPUCheckReadiness(t *testing.T) {
	c := &collectorImpl{
		config: config.NewMockWithOverrides(t, map[string]interface{}{"gpu.enabled": true}),
		checks: make(map[checkid.ID]*middleware.CheckWrapper),
	}
	gpu := NewCheckUnique("gpu:readiness", "gpu")
	t.Cleanup(func() { expvars.RemoveCheckStats(gpu.ID()) })
	addRun := func(err error) {
		expvars.AddCheckStats(gpu, time.Millisecond, err, nil, stats.SenderStats{}, haagentmock.NewMockHaAgent())
	}

	assert.False(t, c.gpuCheckReady(time.Now()), "enabled but missing check")
	c.checks[gpu.ID()] = middleware.NewCheckWrapper(gpu, nil, option.None[agenttelemetry.Component](), option.None[healthplatform.Component]())
	assert.False(t, c.gpuCheckReady(time.Now()), "loaded but never ran")

	addRun(errors.New("failed to initialize NVML"))
	assert.False(t, c.gpuCheckReady(time.Now()), "initialization failed")
	addRun(nil)
	assert.True(t, c.gpuCheckReady(time.Now()), "successful collection, including idle GPUs")
	assert.True(t, c.gpuCheckReady(time.Now().Add(90*time.Second)), "respect the one-minute collection interval")
	assert.False(t, c.gpuCheckReady(time.Now().Add(3*time.Minute)), "previously healthy check stopped making progress")

	addRun(errors.New("check panicked: unexpected nil device"))
	assert.False(t, c.gpuCheckReady(time.Now()), "runner-recovered panic")
	addRun(nil)
	assert.True(t, c.gpuCheckReady(time.Now()), "recovers after a successful run")

	expvars.AddCheckStats(gpu, time.Millisecond, nil, []error{errors.New("optional metric unavailable")}, stats.SenderStats{}, haagentmock.NewMockHaAgent())
	assert.True(t, c.gpuCheckReady(time.Now()), "warnings do not block readiness")

	delete(c.checks, gpu.ID())
	assert.False(t, c.gpuCheckReady(time.Now()), "unloaded check must not reuse old successful stats")
}

func TestGPUReadinessAcknowledgement(t *testing.T) {
	c := &collectorImpl{
		config: config.NewMockWithOverrides(t, map[string]interface{}{"gpu.enabled": true}),
	}
	pings := make(chan time.Time, 2)
	pings <- time.Now()
	pings <- time.Now()
	handle := &health.Handle{C: pings}
	c.ackGPUReadiness(handle, time.Now())
	assert.Len(t, pings, 2, "a missing GPU check must leave the watchdog unanswered")

	c.config.SetInTest("gpu.enabled", false)
	c.ackGPUReadiness(handle, time.Now())
	assert.Empty(t, pings, "disabling GPU monitoring releases the readiness gate")
}

func TestGPUReadinessLifecycle(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			lc := compdef.NewTestLifecycle(t)
			c := &collectorImpl{config: config.NewMockWithOverrides(t, map[string]interface{}{"gpu.enabled": enabled})}
			c.registerGPUReadiness(lc)
			if enabled {
				assert.Contains(t, health.GetReady().Unhealthy, "gpu-check", "registered before startup")
			} else {
				assert.NotContains(t, health.GetReady().Unhealthy, "gpu-check")
			}
			assert.NotContains(t, health.GetLive().Unhealthy, "gpu-check", "GPU failures must not trigger liveness restarts")
			require.NoError(t, lc.Start(context.Background()))
			require.NoError(t, lc.Stop(context.Background()))
			assert.NotContains(t, health.GetReady().Unhealthy, "gpu-check", "shutdown deregisters the health check")
		})
	}
}
