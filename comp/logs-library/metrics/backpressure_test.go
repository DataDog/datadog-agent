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

// stubPipelineMonitor answers Snapshots with a fixed set and counts the reads, so the
// bottleneck cache's TTL is observable.
type stubPipelineMonitor struct {
	NoopPipelineMonitor
	snaps []ComponentSnapshot
	reads int
}

func (s *stubPipelineMonitor) Snapshots() []ComponentSnapshot {
	s.reads++
	return s.snaps
}

func saturatedSnapshot(name string, ratio float64, sat1m, sat30m time.Duration, currently bool) ComponentSnapshot {
	return ComponentSnapshot{
		Name:     name,
		Instance: "0",
		AvgRatio: ratio,
		Windows: WindowStats{
			Saturated1m:        sat1m,
			Saturated30m:       sat30m,
			CurrentlySaturated: currently,
		},
	}
}

func TestSelectBottleneck(t *testing.T) {
	tests := []struct {
		name       string
		comps      []ComponentBackpressure
		wantState  string
		wantMember string
	}{
		{
			name:      "no saturation reads healthy",
			comps:     []ComponentBackpressure{{Component: "processor", AvgRatio: 0.5}},
			wantState: BackpressureHealthy,
		},
		{
			name:       "currently saturated wins",
			comps:      []ComponentBackpressure{{Component: "processor", AvgRatio: 0.95, CurrentlySaturated: true}},
			wantState:  BackpressureSaturated,
			wantMember: "processor",
		},
		{
			// A high EWMA that stopped updating must not read as live saturation.
			name:       "frozen ratio with stale saturation reads warning",
			comps:      []ComponentBackpressure{{Component: "processor", AvgRatio: 0.95, Saturated30mSeconds: 120}},
			wantState:  BackpressureWarning,
			wantMember: "processor",
		},
		{
			name:      "frozen ratio with no saturation reads healthy",
			comps:     []ComponentBackpressure{{Component: "processor", AvgRatio: 0.95}},
			wantState: BackpressureHealthy,
		},
		{
			name: "currently saturated beats a longer 1m saturation elsewhere",
			comps: []ComponentBackpressure{
				{Component: "processor", AvgRatio: 0.3, Saturated1mSeconds: 60, Saturated30mSeconds: 900},
				{Component: "strategy", AvgRatio: 0.91, CurrentlySaturated: true},
			},
			wantState:  BackpressureSaturated,
			wantMember: "strategy",
		},
		{
			name: "among currently saturated the highest ratio wins",
			comps: []ComponentBackpressure{
				{Component: "processor", AvgRatio: 0.85, CurrentlySaturated: true},
				{Component: "worker", AvgRatio: 0.98, CurrentlySaturated: true},
			},
			wantState:  BackpressureSaturated,
			wantMember: "worker",
		},
		{
			name: "1m saturation beats 30m only",
			comps: []ComponentBackpressure{
				{Component: "processor", Saturated30mSeconds: 900},
				{Component: "worker", Saturated1mSeconds: 30, Saturated30mSeconds: 60},
			},
			wantState:  BackpressureWarning,
			wantMember: "worker",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state, bottleneck := SelectBottleneck(tc.comps)
			assert.Equal(t, tc.wantState, state)
			if tc.wantMember == "" {
				assert.Nil(t, bottleneck)
				return
			}
			require.NotNil(t, bottleneck)
			assert.Equal(t, tc.wantMember, bottleneck.Component)
		})
	}
}

// "sender" is a capacity-only aggregation point whose ratio is always 0, so including it
// would put a permanently-healthy row in the breakdown.
func TestDeriveBackpressureExcludesSender(t *testing.T) {
	summary := DeriveBackpressure([]ComponentSnapshot{
		saturatedSnapshot(SenderTlmName, 0, 0, 0, false),
		saturatedSnapshot("processor", 0.4, 0, 0, false),
	})

	require.Len(t, summary.Components, 1)
	assert.Equal(t, "processor", summary.Components[0].Component)
}

func TestDeriveBackpressureRanksWorstFirst(t *testing.T) {
	summary := DeriveBackpressure([]ComponentSnapshot{
		saturatedSnapshot("processor", 0.10, 0, 0, false),
		saturatedSnapshot("worker", 0.99, 0, 30*time.Minute, true),
		saturatedSnapshot("strategy", 0.50, 0, time.Minute, false),
	})

	require.Len(t, summary.Components, 3)
	assert.Equal(t, []string{"worker", "strategy", "processor"},
		[]string{summary.Components[0].Component, summary.Components[1].Component, summary.Components[2].Component})

	require.NotNil(t, summary.Bottleneck)
	assert.Equal(t, "worker", summary.Bottleneck.Component,
		"sorting the components must not move the bottleneck out from under the pointer")
	assert.Equal(t, BackpressureSaturated, summary.State)
}

func TestBackpressureSnapshotUnregisteredIsUnknownNotHealthy(t *testing.T) {
	ResetPipelineMonitorForTest()

	summary := BackpressureSnapshot()
	assert.Empty(t, summary.State, "an unread pipeline must not claim to be healthy")
	assert.Nil(t, summary.Bottleneck)
	assert.Empty(t, currentBottleneckComponent())
}

func TestCurrentBottleneckComponent(t *testing.T) {
	ResetPipelineMonitorForTest()
	t.Cleanup(ResetPipelineMonitorForTest)

	RegisterPipelineMonitor(&stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("destination_reliable_0", 0.97, 0, 30*time.Minute, true)},
	})
	assert.Equal(t, "destination_reliable_0", currentBottleneckComponent())

	// A healthy pipeline is a distinct answer from an unread one: it means rotation outran
	// close_timeout rather than the pipeline's throughput.
	RegisterPipelineMonitor(&stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("processor", 0.2, 0, 0, false)},
	})
	assert.Equal(t, NoBottleneck, currentBottleneckComponent())
}

func TestCurrentBottleneckComponentMemoizes(t *testing.T) {
	ResetPipelineMonitorForTest()
	t.Cleanup(ResetPipelineMonitorForTest)

	clk := clock.NewMock()
	bottleneck = newBottleneckCache(clk)
	t.Cleanup(func() { bottleneck = newBottleneckCache(clock.New()) })

	stub := &stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("worker", 0.95, 0, time.Minute, true)},
	}
	RegisterPipelineMonitor(stub)

	for i := 0; i < 100; i++ {
		require.Equal(t, "worker", currentBottleneckComponent())
	}
	assert.Equal(t, 1, stub.reads, "a rotation storm must not re-derive the bottleneck per rotation")

	clk.Add(bottleneckCacheTTL)
	require.Equal(t, "worker", currentBottleneckComponent())
	assert.Equal(t, 2, stub.reads, "the cache must expire so a recovered pipeline stops being blamed")
}

// A transport switch builds a new pipeline; the previous one's bottleneck is stale.
func TestRegisterPipelineMonitorInvalidatesCache(t *testing.T) {
	ResetPipelineMonitorForTest()
	t.Cleanup(ResetPipelineMonitorForTest)

	RegisterPipelineMonitor(&stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("strategy", 0.95, 0, time.Minute, true)},
	})
	require.Equal(t, "strategy", currentBottleneckComponent())

	RegisterPipelineMonitor(&stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("processor", 0.99, 0, time.Minute, true)},
	})
	assert.Equal(t, "processor", currentBottleneckComponent())
}
