// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubPipelineMonitor can gate reads to exercise cache refresh and invalidation.
type stubPipelineMonitor struct {
	NoopPipelineMonitor
	snaps   []ComponentSnapshot
	reads   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (s *stubPipelineMonitor) Snapshots() []ComponentSnapshot {
	s.reads.Add(1)
	if s.started != nil {
		s.started <- struct{}{}
	}
	if s.release != nil {
		<-s.release
	}
	return s.snaps
}

func saturatedSnapshot(name string, ratio float64, sat1m, sat30m time.Duration, currently bool) ComponentSnapshot {
	return ComponentSnapshot{
		Name:     name,
		Instance: "0",
		AvgRatio: ratio,
		Measured: true,
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
			for shift := range tc.comps {
				comps := append(tc.comps[shift:len(tc.comps):len(tc.comps)], tc.comps[:shift]...)
				state, bottleneck := SelectBottleneck(comps)
				assert.Equal(t, tc.wantState, state)
				if tc.wantMember == "" {
					assert.Nil(t, bottleneck)
					continue
				}
				require.NotNil(t, bottleneck)
				assert.Equal(t, tc.wantMember, bottleneck.Component)
				assert.Equal(t, tc.wantInstance, bottleneck.Instance)
			}
		})
	}
}

// A capacity-only component ("sender") has no utilization monitor, so its zero windows are
// unobserved rather than saturation-free; including it would claim a healthy row.
func TestDeriveBackpressureExcludesUnmeasured(t *testing.T) {
	summary := DeriveBackpressure([]ComponentSnapshot{
		{Name: SenderTlmName},
		saturatedSnapshot("processor", 0.4, 0, 0, false),
	})

	require.Len(t, summary.Components, 1)
	assert.Equal(t, "processor", summary.Components[0].Component)
}

func TestBackpressureExcludesNonblockingDestinations(t *testing.T) {
	now := time.Now()
	for _, currently := range []bool{true, false} {
		for _, blockingSaturated := range []bool{true, false} {
			t.Run(fmt.Sprintf("current=%t/blocking_saturated=%t", currently, blockingSaturated), func(t *testing.T) {
				unreliable := saturatedSnapshot("destination_unreliable_0", 0.99, time.Minute, time.Minute, currently)
				unreliable.Windows.HasLastSaturated = true
				unreliable.Windows.LastSaturatedAt = now
				processor := saturatedSnapshot("processor", 0.2, 0, 0, false)
				wantState, wantLoss := BackpressureHealthy, NoBottleneck
				if blockingSaturated {
					processor = saturatedSnapshot("processor", 0.95, time.Second, time.Second, currently)
					processor.Windows.HasLastSaturated = true
					processor.Windows.LastSaturatedAt = now.Add(-time.Second)
					wantState, wantLoss = BackpressureWarning, "processor"
					if currently {
						wantState = BackpressureSaturated
					}
				}

				summary := DeriveBackpressure([]ComponentSnapshot{unreliable, processor})
				assert.Equal(t, wantState, summary.State)
				assert.Equal(t, wantLoss, bottleneckDuringLoss(summary, now.Add(-time.Minute), now))
				if blockingSaturated {
					require.NotNil(t, summary.Bottleneck)
					assert.Equal(t, "processor", summary.Bottleneck.Component)
				} else {
					assert.Nil(t, summary.Bottleneck)
				}
				assert.Len(t, summary.Components, 2, "nonblocking destination measurements remain visible")
			})
		}
	}
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
		"no snapshots":       nil,
		"only capacity-only": {{Name: SenderTlmName}, {Name: "processor"}},
		"empty registry":     {},
	} {
		t.Run(name, func(t *testing.T) {
			summary := DeriveBackpressure(snaps)
			assert.Empty(t, summary.State, "a monitor that measured nothing must not claim HEALTHY")
			assert.Nil(t, summary.Bottleneck)
			assert.Empty(t, summary.Components)
		})
	}
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

func TestCurrentBottleneckComponent(t *testing.T) {
	for _, tc := range []struct {
		name             string
		monitor          PipelineMonitor
		state, component string
	}{
		{name: "unregistered"},
		{name: "noop", monitor: NewNoopPipelineMonitor("")},
		{name: "empty", monitor: &stubPipelineMonitor{}},
		{
			name: "saturated", state: BackpressureSaturated, component: "destination_reliable_0",
			monitor: &stubPipelineMonitor{snaps: []ComponentSnapshot{saturatedSnapshot("destination_reliable_0", 0.97, 0, time.Minute, true)}},
		},
		{
			name: "healthy", state: BackpressureHealthy, component: NoBottleneck,
			monitor: &stubPipelineMonitor{snaps: []ComponentSnapshot{saturatedSnapshot("processor", 0.2, 0, 0, false)}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterPipelineMonitor(tc.monitor)
			t.Cleanup(ResetPipelineMonitorForTest)
			assert.Equal(t, tc.state, BackpressureSnapshot().State)
			assert.Equal(t, tc.component, currentBottleneckComponent(time.Now().Add(-time.Minute)))
		})
	}
}

// Only saturation inside the loss window is attributed, even when both samples are inside the
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

func TestCurrentBottleneckComponentCoalescesConcurrentMisses(t *testing.T) {
	const callers = 32
	clk := clock.NewMock()
	cache := newBottleneckCache(clk)
	monitor := &stubPipelineMonitor{snaps: []ComponentSnapshot{saturatedSnapshot("worker", 0.95, 0, time.Minute, true)}}
	RegisterPipelineMonitor(monitor)
	t.Cleanup(ResetPipelineMonitorForTest)
	window := clk.Now().Add(-time.Minute)

	// Exercise both a cold cache and an expired one with an unchanged monitor.
	for wave := 1; wave <= 2; wave++ {
		monitor.started = make(chan struct{}, callers)
		monitor.release = make(chan struct{})
		start := make(chan struct{})
		results := make(chan string, callers)
		var ready sync.WaitGroup
		ready.Add(callers)
		for i := 0; i < callers; i++ {
			go func() {
				ready.Done()
				<-start
				results <- cache.get(window)
			}()
		}
		ready.Wait()
		close(start)
		<-monitor.started
		close(monitor.release)
		for i := 0; i < callers; i++ {
			assert.Equal(t, "worker", <-results)
		}
		assert.Equal(t, int32(wave), monitor.reads.Load(), "one snapshot per burst")
		clk.Add(bottleneckCacheTTL)
	}
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

	oldMonitor := &stubPipelineMonitor{
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
	assert.Equal(t, int32(1), newMonitor.reads.Load(), "the replacement snapshot should be cached")
}
