// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineMonitorTracksCorrectCapacity(t *testing.T) {
	pm := NewTelemetryPipelineMonitor()

	pm.ReportComponentIngress(mockPayload{count: 1, size: 1}, "1", "test")
	pm.ReportComponentIngress(mockPayload{count: 5, size: 5}, "5", "test")
	pm.ReportComponentIngress(mockPayload{count: 10, size: 10}, "10", "test")

	assert.Equal(t, pm.getMonitor("1", "test").ingress, int64(1))
	assert.Equal(t, pm.getMonitor("1", "test").ingressBytes, int64(1))

	assert.Equal(t, pm.getMonitor("5", "test").ingress, int64(5))
	assert.Equal(t, pm.getMonitor("5", "test").ingressBytes, int64(5))

	assert.Equal(t, pm.getMonitor("10", "test").ingress, int64(10))
	assert.Equal(t, pm.getMonitor("10", "test").ingressBytes, int64(10))

	pm.ReportComponentEgress(mockPayload{count: 1, size: 1}, "1", "test")
	pm.ReportComponentEgress(mockPayload{count: 5, size: 5}, "5", "test")
	pm.ReportComponentEgress(mockPayload{count: 10, size: 10}, "10", "test")

	assert.Equal(t, pm.getMonitor("1", "test").egress, int64(1))
	assert.Equal(t, pm.getMonitor("1", "test").egressBytes, int64(1))

	assert.Equal(t, pm.getMonitor("5", "test").egress, int64(5))
	assert.Equal(t, pm.getMonitor("5", "test").egressBytes, int64(5))

	assert.Equal(t, pm.getMonitor("10", "test").egress, int64(10))
	assert.Equal(t, pm.getMonitor("10", "test").egressBytes, int64(10))

	assert.Equal(t, pm.getMonitor("1", "test").ingress-pm.getMonitor("1", "test").egress, int64(0))
	assert.Equal(t, pm.getMonitor("5", "test").ingress-pm.getMonitor("5", "test").egress, int64(0))
	assert.Equal(t, pm.getMonitor("10", "test").ingress-pm.getMonitor("10", "test").egress, int64(0))
}

// TestTelemetryPipelineMonitor_StartStopLifecycle checks Stop doesn't hang, is safe without Start, and is idempotent.
func TestTelemetryPipelineMonitor_StartStopLifecycle(_ *testing.T) {
	clk := clock.NewMock()

	// Stop without Start must be a no-op, not block on a nil channel.
	pm := newTelemetryPipelineMonitorWithClock(clk, time.Second)
	pm.Stop()

	pm = newTelemetryPipelineMonitorWithClock(clk, time.Second)
	pm.Start()
	pm.Start() // idempotent: must not spawn a second loop
	// Stop blocks until the loop goroutine exits; if it leaked, the test would hang here.
	pm.Stop()
	pm.Stop() // idempotent
}

func TestMakeUtilizationMonitor_ReplacesDuplicateKey(t *testing.T) {
	clk := clock.NewMock()
	pm := newTelemetryPipelineMonitorWithClock(clk, time.Second)

	first := pm.MakeUtilizationMonitor("processor", "0").(*TelemetryUtilizationMonitor)
	second := pm.MakeUtilizationMonitor("processor", "0").(*TelemetryUtilizationMonitor)

	require.Len(t, pm.utilizationMonitors, 1)
	require.NotSame(t, first, second)
	require.Same(t, second, pm.utilizationMonitors["processor:0"])
}

// TestTelemetryPipelineMonitor_TickerSamplesRegisteredMonitor checks the running ticker samples its registered monitors.
func TestTelemetryPipelineMonitor_TickerSamplesRegisteredMonitor(t *testing.T) {
	clk := clock.NewMock()
	pm := newTelemetryPipelineMonitorWithClock(clk, time.Second)
	um := pm.MakeUtilizationMonitor("processor", "0").(*TelemetryUtilizationMonitor)

	pm.Start()
	defer pm.Stop()
	um.Start() // blocked in-use, never Stop

	require.Eventually(t, func() bool {
		clk.Add(time.Second)
		// avg is published atomically, so a subscriber can poll it without locking the hot path.
		return um.avg.Load() > 0
	}, 2*time.Second, 5*time.Millisecond,
		"the pipeline monitor's ticker must sample its registered utilization monitor")
}

// findSnapshot returns the snapshot for name:instance from a slice, or fails the test.
func findSnapshot(t *testing.T, snaps []ComponentSnapshot, name, instance string) ComponentSnapshot {
	t.Helper()
	for _, s := range snaps {
		if s.Name == name && s.Instance == instance {
			return s
		}
	}
	t.Fatalf("no snapshot for %s:%s", name, instance)
	return ComponentSnapshot{}
}

// TestSnapshots_SaturationHoldsThenDecaysToWarning checks CurrentlySaturated holds while blocked, then decays to WARNING once idle.
func TestSnapshots_SaturationHoldsThenDecaysToWarning(t *testing.T) {
	clk := clock.NewMock()
	pm := newTelemetryPipelineMonitorWithClock(clk, time.Second)
	um := pm.MakeUtilizationMonitor("processor", "0").(*TelemetryUtilizationMonitor)

	um.Start() // enter in-use and never Stop: models a goroutine blocked on a full output channel

	// Drive periodic samples while blocked: the EWMA must climb to saturation.
	for i := 0; i < 40; i++ {
		clk.Add(time.Second)
		um.sample(clk.Now())
	}
	require.True(t, findSnapshot(t, pm.registry.at(clk.Now()), "processor", "0").Windows.CurrentlySaturated,
		"a component blocked in-use must read as currently saturated")

	// While still blocked, repeated reads (each a status-page render) must stay saturated.
	for i := 0; i < 10; i++ {
		clk.Add(time.Second)
		um.sample(clk.Now())
		assert.True(t, findSnapshot(t, pm.registry.at(clk.Now()), "processor", "0").Windows.CurrentlySaturated,
			"CurrentlySaturated must not flap while the component remains saturated (read %d)", i)
	}

	// Recover: go idle; continued sampling decays the EWMA and ages saturated samples out of the window.
	um.Stop()
	for i := 0; i < 20; i++ {
		clk.Add(time.Second)
		um.sample(clk.Now())
	}

	final := findSnapshot(t, pm.registry.at(clk.Now()), "processor", "0")
	assert.False(t, final.Windows.CurrentlySaturated,
		"once recovered and idle, CurrentlySaturated must decay to false")
	assert.Greater(t, final.Windows.Saturated30m, time.Duration(0),
		"the past saturation must persist as Saturated30m so the state holds at WARNING, not HEALTHY")
}

// TestRegistryAt_IdleDecay checks that reading against a later clock decays an idle component's CurrentlySaturated.
func TestRegistryAt_IdleDecay(t *testing.T) {
	at := time.Unix(7200, 0)
	h := newRollingHistory()
	h.add(at, 0.95) // saturated sample
	reg := newSnapshotRegistry()
	reg.setUtilization("processor", "0", 0.95, 0.95, h)

	// Read at the sample time: still saturated.
	fresh := reg.at(at)
	require.Len(t, fresh, 1)
	assert.True(t, fresh[0].Windows.CurrentlySaturated, "fresh read must report saturation")
	assert.Nil(t, fresh[0].history, "history pointer must not escape to callers")

	// Read long after, with no new samples: window stats recompute against the live clock.
	stale := reg.at(at.Add(time.Hour))
	require.Len(t, stale, 1)
	assert.False(t, stale[0].Windows.CurrentlySaturated,
		"an idle component's saturation must decay when read against a later clock")
	assert.InDelta(t, 0.95, stale[0].AvgRatio, 0.0001,
		"AvgRatio is the last-known EWMA and is expected to stay frozen; only window stats decay")
}

// TestSnapshots_PipelineMonitorsAreIsolated is the regression guard for the old global registry:
// two independent monitors that share component keys must not see each other's snapshots.
func TestSnapshots_PipelineMonitorsAreIsolated(t *testing.T) {
	clk := clock.NewMock()

	saturated := newTelemetryPipelineMonitorWithClock(clk, time.Second)
	um := saturated.MakeUtilizationMonitor("processor", "0").(*TelemetryUtilizationMonitor)
	um.Start() // blocked in-use, drives EWMA to saturation
	for i := 0; i < 40; i++ {
		clk.Add(time.Second)
		um.sample(clk.Now())
	}

	// A second monitor with the SAME component key but no activity. Under the old global map its
	// keys would have collided with the saturated monitor; with per-instance registries it stays empty.
	idle := newTelemetryPipelineMonitorWithClock(clk, time.Second)
	idle.MakeUtilizationMonitor("processor", "0")

	require.True(t, findSnapshot(t, saturated.registry.at(clk.Now()), "processor", "0").Windows.CurrentlySaturated,
		"the active monitor must report its own saturation")
	assert.Empty(t, idle.registry.at(clk.Now()),
		"a separate monitor must not observe another monitor's snapshots (no global collision)")
}
