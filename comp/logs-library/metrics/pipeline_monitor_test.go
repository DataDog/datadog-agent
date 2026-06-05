// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

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

// TestGlobalComponentSnapshotsAt_IdleDecay is the registry-level regression for the
// stale-EWMA bug. A component is recorded as saturated, then no further samples arrive
// (the component went idle — utilization is sampled only on Start/Stop). Reading the
// snapshots against a later clock must recompute the window stats so CurrentlySaturated
// decays to false, rather than freezing at the last sampled value.
func TestGlobalComponentSnapshotsAt_IdleDecay(t *testing.T) {
	ClearComponentSnapshots()
	defer ClearComponentSnapshots()

	at := time.Unix(7200, 0)
	h := newRollingHistory()
	h.add(at, 0.95) // saturated sample
	setComponentUtilization("processor", "0", 0.95, 0.95, h)

	// Read at the sample time: still saturated.
	fresh := globalComponentSnapshotsAt(at)
	require.Len(t, fresh, 1)
	assert.True(t, fresh[0].Windows.CurrentlySaturated, "fresh read must report saturation")
	assert.Nil(t, fresh[0].history, "history pointer must not escape to callers")

	// Read long after, with no new samples: window stats recompute against the live clock.
	stale := globalComponentSnapshotsAt(at.Add(time.Hour))
	require.Len(t, stale, 1)
	assert.False(t, stale[0].Windows.CurrentlySaturated,
		"an idle component's saturation must decay when read against a later clock")
	assert.InDelta(t, 0.95, stale[0].AvgRatio, 0.0001,
		"AvgRatio is the last-known EWMA and is expected to stay frozen; only window stats decay")
}
