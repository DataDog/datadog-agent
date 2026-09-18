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

type blockingPipelineMonitor struct {
	NoopPipelineMonitor
	snaps   []ComponentSnapshot
	started chan struct{}
	release chan struct{}
}

func (m *blockingPipelineMonitor) Snapshots() []ComponentSnapshot {
	close(m.started)
	<-m.release
	return m.snaps
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
		name         string
		comps        []ComponentBackpressure
		wantState    string
		wantMember   string
		wantInstance string
	}{
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
			// Ratio alone does not drive the state: nothing saturated is healthy.
			name:      "a high ratio with no saturation reads healthy",
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
		{
			name: "equal ratios break on component name",
			comps: []ComponentBackpressure{
				{Component: "worker", AvgRatio: 0.9, CurrentlySaturated: true},
				{Component: "processor", AvgRatio: 0.9, CurrentlySaturated: true},
			},
			wantState:  BackpressureSaturated,
			wantMember: "processor",
		},
		{
			name: "equal ratios and names break on instance",
			comps: []ComponentBackpressure{
				{Component: "worker", Instance: "3", AvgRatio: 0.9, CurrentlySaturated: true},
				{Component: "worker", Instance: "1", AvgRatio: 0.9, CurrentlySaturated: true},
			},
			wantState:    BackpressureSaturated,
			wantMember:   "worker",
			wantInstance: "1",
		},
		{
			name: "equal 1m saturation breaks on component name",
			comps: []ComponentBackpressure{
				{Component: "worker", Saturated1mSeconds: 30, Saturated30mSeconds: 60},
				{Component: "processor", Saturated1mSeconds: 30, Saturated30mSeconds: 900},
			},
			wantState:  BackpressureWarning,
			wantMember: "processor",
		},
		{
			name: "equal 30m saturation breaks on component name",
			comps: []ComponentBackpressure{
				{Component: "worker", Saturated30mSeconds: 900},
				{Component: "processor", Saturated30mSeconds: 900},
			},
			wantState:  BackpressureWarning,
			wantMember: "processor",
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
			if tc.wantInstance != "" {
				assert.Equal(t, tc.wantInstance, bottleneck.Instance)
			}
		})
	}
}

// Snapshots arrive in map order, so the same measurements must not blame different stages.
func TestSelectBottleneckIsOrderIndependent(t *testing.T) {
	comps := []ComponentBackpressure{
		{Component: "worker", Instance: "0", AvgRatio: 0.9, CurrentlySaturated: true},
		{Component: "processor", Instance: "0", AvgRatio: 0.9, CurrentlySaturated: true},
		{Component: "strategy", Instance: "0", AvgRatio: 0.9, CurrentlySaturated: true},
	}

	for i := range comps {
		shuffled := append(comps[i:len(comps):len(comps)], comps[:i]...)
		state, bottleneck := SelectBottleneck(shuffled)
		assert.Equal(t, BackpressureSaturated, state)
		require.NotNil(t, bottleneck)
		assert.Equal(t, "processor", bottleneck.Component, "rotation %d blamed a different stage", i)
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

func TestDeriveBackpressureUnmeasuredIsUnknownNotHealthy(t *testing.T) {
	for name, snaps := range map[string][]ComponentSnapshot{
		"no snapshots":   nil,
		"only sender":    {saturatedSnapshot(SenderTlmName, 0, 0, 0, false)},
		"empty registry": {},
	} {
		t.Run(name, func(t *testing.T) {
			summary := DeriveBackpressure(snaps)
			assert.Empty(t, summary.State, "a monitor that measured nothing must not claim HEALTHY")
			assert.Nil(t, summary.Bottleneck)
			assert.Empty(t, summary.Components)
		})
	}
}

// A registered monitor that measures nothing must stay distinguishable from a healthy one:
// the issue text turns NoBottleneck into "the pipeline was keeping up".
func TestCurrentBottleneckComponentNoopMonitorIsUnknown(t *testing.T) {
	RegisterPipelineMonitor(NewNoopPipelineMonitor(""))
	t.Cleanup(ResetPipelineMonitorForTest)

	assert.Empty(t, BackpressureSnapshot().State)
	assert.Empty(t, currentBottleneckComponent(time.Now().Add(-time.Minute)), "unmeasured must not be recorded as NoBottleneck")
}

// A component saturated right now with no 30m history sorts last on duration, so it only
// survives a truncating caller if the bottleneck key outranks the duration key.
func TestDeriveBackpressureKeepsBottleneckFirst(t *testing.T) {
	snaps := []ComponentSnapshot{saturatedSnapshot("zzz_now", 0.99, 0, 0, true)}
	for i := 0; i < 12; i++ {
		s := saturatedSnapshot("aaa_history", 0.5, 0, 10*time.Minute, false)
		s.Instance = string(rune('a' + i))
		snaps = append(snaps, s)
	}

	summary := DeriveBackpressure(snaps)

	require.NotNil(t, summary.Bottleneck)
	assert.Equal(t, "zzz_now", summary.Bottleneck.Component)
	assert.Equal(t, BackpressureSaturated, summary.State)
	require.Len(t, summary.Components, 13)
	assert.Equal(t, "zzz_now", summary.Components[0].Component,
		"the bottleneck must be row 0 or a truncating caller drops it")
}

func TestBackpressureSnapshotUnregisteredIsUnknownNotHealthy(t *testing.T) {
	ResetPipelineMonitorForTest()

	summary := BackpressureSnapshot()
	assert.Empty(t, summary.State, "an unread pipeline must not claim to be healthy")
	assert.Nil(t, summary.Bottleneck)
	assert.Empty(t, currentBottleneckComponent(time.Now().Add(-time.Minute)))
}

func TestCurrentBottleneckComponent(t *testing.T) {
	ResetPipelineMonitorForTest()
	t.Cleanup(ResetPipelineMonitorForTest)

	RegisterPipelineMonitor(&stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("destination_reliable_0", 0.97, 0, 30*time.Minute, true)},
	})
	assert.Equal(t, "destination_reliable_0", currentBottleneckComponent(time.Now().Add(-time.Minute)))
}

// Nothing saturated anywhere in the window is the one case that can claim the pipeline kept up:
// it means rotation outran close_timeout rather than the pipeline's throughput.
func TestCurrentBottleneckComponentHealthyIsNoBottleneck(t *testing.T) {
	ResetPipelineMonitorForTest()
	t.Cleanup(ResetPipelineMonitorForTest)

	RegisterPipelineMonitor(&stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("processor", 0.2, 0, 0, false)},
	})
	assert.Equal(t, NoBottleneck, currentBottleneckComponent(time.Now().Add(-time.Minute)))
}

// Only saturation after the rotation is causal, even when both samples are inside the
// trailing-minute aggregate.
func TestCurrentBottleneckComponentUsesActualLossWindow(t *testing.T) {
	ResetPipelineMonitorForTest()
	t.Cleanup(ResetPipelineMonitorForTest)
	now := time.Now()
	snapshot := saturatedSnapshot("strategy", 0.9, 30*time.Second, 20*time.Minute, false)
	snapshot.Windows.HasLastSaturated = true

	snapshot.Windows.LastSaturatedAt = now.Add(-30 * time.Second)
	RegisterPipelineMonitor(&stubPipelineMonitor{snaps: []ComponentSnapshot{snapshot}})
	require.Equal(t, BackpressureWarning, BackpressureSnapshot().State, "the snapshot still carries the history")
	assert.Empty(t, currentBottleneckComponent(now.Add(-5*time.Second)),
		"saturation that ended before the loss is neither the cause nor proof of health")

	// CurrentlySaturated is debounced and may remain true after the last saturated sample.
	// The timestamp still wins when that sample predates the rotation.
	snapshot.Windows.CurrentlySaturated = true
	RegisterPipelineMonitor(&stubPipelineMonitor{snaps: []ComponentSnapshot{snapshot}})
	assert.Empty(t, currentBottleneckComponent(now.Add(-5*time.Second)))

	snapshot.Windows.LastSaturatedAt = now.Add(-2 * time.Second)
	RegisterPipelineMonitor(&stubPipelineMonitor{snaps: []ComponentSnapshot{snapshot}})
	assert.Equal(t, "strategy", currentBottleneckComponent(now.Add(-5*time.Second)),
		"recovered saturation inside the post-rotation window remains attributable")
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
	lossWindowStartedAt := clk.Now().Add(-time.Minute)

	for i := 0; i < 100; i++ {
		require.Equal(t, "worker", currentBottleneckComponent(lossWindowStartedAt))
	}
	assert.Equal(t, 1, stub.reads, "a rotation storm must not re-derive the bottleneck per rotation")

	clk.Add(bottleneckCacheTTL)
	require.Equal(t, "worker", currentBottleneckComponent(lossWindowStartedAt))
	assert.Equal(t, 2, stub.reads, "the cache must expire so a recovered pipeline stops being blamed")
}

// A transport switch builds a new pipeline; the previous one's bottleneck is stale.
func TestRegisterPipelineMonitorInvalidatesCache(t *testing.T) {
	ResetPipelineMonitorForTest()
	t.Cleanup(ResetPipelineMonitorForTest)

	RegisterPipelineMonitor(&stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("strategy", 0.95, 0, time.Minute, true)},
	})
	lossWindowStartedAt := time.Now().Add(-time.Minute)
	require.Equal(t, "strategy", currentBottleneckComponent(lossWindowStartedAt))

	RegisterPipelineMonitor(&stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("processor", 0.99, 0, time.Minute, true)},
	})
	assert.Equal(t, "processor", currentBottleneckComponent(lossWindowStartedAt))
}

// A transport restart can replace the registered monitor while a tailer is deriving a
// snapshot. The old result must neither reach that tailer nor repopulate the invalidated cache.
func TestRegisterPipelineMonitorDuringSnapshotRetriesWithNewMonitor(t *testing.T) {
	ResetPipelineMonitorForTest()
	t.Cleanup(ResetPipelineMonitorForTest)

	oldMonitor := &blockingPipelineMonitor{
		snaps:   []ComponentSnapshot{saturatedSnapshot("strategy", 0.95, 0, time.Minute, true)},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	RegisterPipelineMonitor(oldMonitor)

	result := make(chan string, 1)
	go func() {
		result <- currentBottleneckComponent(time.Now().Add(-time.Minute))
	}()

	select {
	case <-oldMonitor.started:
	case <-time.After(time.Second):
		t.Fatal("old monitor snapshot did not start")
	}

	newMonitor := &stubPipelineMonitor{
		snaps: []ComponentSnapshot{saturatedSnapshot("processor", 0.99, 0, time.Minute, true)},
	}
	RegisterPipelineMonitor(newMonitor)
	close(oldMonitor.release)

	select {
	case component := <-result:
		assert.Equal(t, "processor", component)
	case <-time.After(time.Second):
		t.Fatal("bottleneck lookup did not retry after monitor replacement")
	}

	assert.Equal(t, "processor", currentBottleneckComponent(time.Now().Add(-time.Minute)))
	assert.Equal(t, 1, newMonitor.reads, "the replacement snapshot should be cached")
}
