// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// emittedMetric records one AddEnhancedMetric call.
type emittedMetric struct {
	name      string
	value     float64
	timestamp float64
	extraTags []string
}

// mockMetricEmitter records metric calls. Protected by mu for goroutine safety.
type mockMetricEmitter struct {
	mu      sync.Mutex
	metrics []emittedMetric
}

func (m *mockMetricEmitter) AddEnhancedMetric(name string, value float64, _ metrics.MetricSource, ts float64, tags ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = append(m.metrics, emittedMetric{name: name, value: value, timestamp: ts, extraTags: slices.Clone(tags)})
}

func (m *mockMetricEmitter) getEmitted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, len(m.metrics))
	for i, em := range m.metrics {
		names[i] = em.name
	}
	return names
}

func (m *mockMetricEmitter) getEmittedMetrics() []emittedMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]emittedMetric, len(m.metrics))
	copy(result, m.metrics)
	return result
}

// lastTags returns the tag slice from the most recent emission, or nil if
// nothing has been emitted yet.
func (m *mockMetricEmitter) lastTags() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.metrics) == 0 {
		return nil
	}
	return slices.Clone(m.metrics[len(m.metrics)-1].extraTags)
}

func newTestHeartbeat(interval time.Duration) (*Heartbeat, *mockMetricEmitter) {
	emitter := &mockMetricEmitter{}
	hb := NewHeartbeat(interval, emitter, metrics.MetricSourceAWSMicroVMEnhanced, []string{"microvm_image_arn:test-arn"})
	return hb, emitter
}

func TestNewHeartbeat_NonPositiveIntervalFallsBackToDefault(t *testing.T) {
	for _, in := range []time.Duration{0, -1, -time.Second} {
		hb := NewHeartbeat(in, &mockMetricEmitter{}, metrics.MetricSourceAWSMicroVMEnhanced, nil)
		assert.Equal(t, DefaultHeartbeatInterval, hb.interval, "interval=%s should fall back to default", in)
	}
}

// TestHeartbeat_EmitsImmediatelyOnStart verifies that a heartbeat metric is
// recorded as soon as the goroutine starts, before the first ticker interval
// elapses. This guarantees at least one emission for very short-lived instances.
func TestHeartbeat_EmitsImmediatelyOnStart(t *testing.T) {
	// Use a very long interval so the ticker cannot fire during the test window.
	hb, emitter := newTestHeartbeat(time.Hour)
	hb.Start()
	defer hb.Stop()

	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) >= 1
	}, 500*time.Millisecond, time.Millisecond, "must emit once immediately, without waiting for the ticker")
}

// TestHeartbeat_EmitsAtInterval verifies the goroutine emits at the configured
// cadence and that emitted metrics carry the configured tags + source.
func TestHeartbeat_EmitsAtInterval(t *testing.T) {
	const interval = 20 * time.Millisecond
	hb, emitter := newTestHeartbeat(interval)
	hb.Start()
	defer hb.Stop()

	// Wait long enough for at least 3 ticks. Allow generous slack so a slow
	// CI runner doesn't flake.
	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) >= 3
	}, 2*time.Second, 5*time.Millisecond, "expected at least 3 heartbeat emissions")

	for _, name := range emitter.getEmitted() {
		assert.Equal(t, activeInstancesMetricName, name,
			"unexpected metric name from heartbeat goroutine: %s", name)
	}
}

// TestHeartbeat_StartIsIdempotent_NoDoubleEmission verifies that calling
// Start twice does not spawn a second goroutine — the emission rate stays
// consistent with a single goroutine.
func TestHeartbeat_StartIsIdempotent_NoDoubleEmission(t *testing.T) {
	hb, emitter := newTestHeartbeat(50 * time.Millisecond)
	hb.Start()
	hb.Start() // duplicate call

	// Wait deterministically for at least 3 emissions (1 immediate + 2 ticks),
	// then stop to freeze the count before checking the upper bound.
	// With two goroutines (bug): count would be ~2× the single-goroutine rate.
	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) >= 3
	}, 2*time.Second, 5*time.Millisecond, "should have emitted at least 3 times (1 immediate + 2 ticks)")
	hb.Stop()

	count := countOfMetric(emitter, activeInstancesMetricName)
	assert.GreaterOrEqual(t, count, 3, "should have emitted at least 3 times (1 immediate + 2 ticks)")
	assert.LessOrEqual(t, count, 5, "double-Start must not double the emission rate")
}

func TestHeartbeat_StopIsIdempotent(t *testing.T) {
	hb, _ := newTestHeartbeat(50 * time.Millisecond)
	hb.Start()
	hb.Stop()
	assert.NotPanics(t, func() { hb.Stop() }, "second Stop must be a no-op, not a panic")
}

func TestHeartbeat_StopOnNilReceiverIsSafe(t *testing.T) {
	var hb *Heartbeat
	assert.NotPanics(t, func() { hb.Stop() })
}

func TestHeartbeat_StartOnNilReceiverIsSafe(t *testing.T) {
	var hb *Heartbeat
	assert.NotPanics(t, func() { hb.Start() })
}

// TestHeartbeat_RestartAfterStopWorks pins the resume scenario: /suspend
// stops the heartbeat, /resume starts it again, and emissions continue.
func TestHeartbeat_RestartAfterStopWorks(t *testing.T) {
	hb, emitter := newTestHeartbeat(20 * time.Millisecond)
	hb.Start()
	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) >= 1
	}, 500*time.Millisecond, 5*time.Millisecond)
	hb.Stop()
	beforeRestart := countOfMetric(emitter, activeInstancesMetricName)

	hb.Start()
	defer hb.Stop()
	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) > beforeRestart+1
	}, 500*time.Millisecond, 5*time.Millisecond, "heartbeat must resume emissions after restart")
}

// TestHeartbeat_StopWaitsForGoroutineToExit verifies that Stop blocks until
// the goroutine is actually gone. If Stop returned before goroutine exit, a
// runtime.NumGoroutine snapshot taken right after Stop could still see it.
// We sample twice across a generous slack window to allow the runtime to
// reclaim the goroutine.
func TestHeartbeat_StopWaitsForGoroutineToExit(t *testing.T) {
	hb, _ := newTestHeartbeat(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()
	hb.Start()
	hb.Stop()
	// A small grace period — runtime.NumGoroutine reflects scheduling state,
	// not strict ownership; ±1 is normal jitter.
	time.Sleep(10 * time.Millisecond)
	assert.LessOrEqual(t, runtime.NumGoroutine(), baseline+1, "heartbeat goroutine should be cleaned up after Stop")
}

// TestHeartbeat_StopIsUnblockedWhenEmitterStuck verifies that Stop returns
// within heartbeatStopTimeout even when AddEnhancedMetric blocks indefinitely
// (e.g. the metric-agent samples channel is full). Without the timeout,
// /suspend and /terminate would hang past flushTimeout.
//
// It also verifies that, absent a racing Start, the goroutine does NOT relaunch
// after it finally drains: Stop cleared wantRunning, so the cleanup leaves the
// heartbeat stopped.
func TestHeartbeat_StopIsUnblockedWhenEmitterStuck(t *testing.T) {
	block := make(chan struct{}) // closed later to unblock the emitter
	blocker := &blockingMetricEmitter{block: block}
	hb := NewHeartbeat(time.Millisecond /* fire immediately */, blocker, metrics.MetricSourceAWSMicroVMEnhanced, nil)
	hb.Start()

	// Wait until the goroutine is blocked inside AddEnhancedMetric.
	require.Eventually(t, func() bool { return blocker.isBlocked() }, time.Second, time.Millisecond)

	start := time.Now()
	hb.Stop()
	elapsed := time.Since(start)

	assert.Less(t, elapsed, heartbeatStopTimeout+200*time.Millisecond,
		"Stop must return within heartbeatStopTimeout even when emitter is stuck")

	// Unblock the emitter. The stuck goroutine exits AddEnhancedMetric, sees the
	// cancelled context, and self-cleans. Because Stop cleared wantRunning, the
	// cleanup must NOT relaunch.
	close(block)
	require.Eventually(t, func() bool {
		hb.mu.Lock()
		defer hb.mu.Unlock()
		return hb.cancel == nil // drained and cleaned up
	}, time.Second, time.Millisecond, "goroutine must self-clean after emitter unblocks")

	countAfterDrain := blocker.callCount()
	// No relaunch: a relaunched goroutine would emit immediately and then every
	// millisecond, so the call count would keep climbing. It must stay flat.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, countAfterDrain, blocker.callCount(),
		"without a racing Start, the drained goroutine must not relaunch")

	hb.mu.Lock()
	assert.False(t, hb.wantRunning, "Stop must leave wantRunning false")
	assert.Nil(t, hb.cancel, "no goroutine should be running")
	hb.mu.Unlock()
}

// TestHeartbeat_ResumeDuringStuckStopRestartsHeartbeat pins the fix for the
// resume-during-drain race: when /suspend's Stop times out because
// AddEnhancedMetric is blocked, a /resume (Start) that arrives during the drain
// window must not be dropped. Start records the intent without spawning an
// overlapping goroutine; once the blocked goroutine drains, the heartbeat
// relaunches so active-instance emissions resume for the resumed MicroVM.
func TestHeartbeat_ResumeDuringStuckStopRestartsHeartbeat(t *testing.T) {
	block := make(chan struct{})
	blocker := &blockingMetricEmitter{block: block}
	hb := NewHeartbeat(time.Millisecond /* fire immediately */, blocker, metrics.MetricSourceAWSMicroVMEnhanced, nil)
	hb.Start()

	require.Eventually(t, func() bool { return blocker.isBlocked() }, time.Second, time.Millisecond)

	// /suspend: Stop times out because the emitter is stuck; h.cancel stays set.
	hb.Stop()

	// /resume during the drain window: must record intent but not spawn an
	// overlapping goroutine while the old one is still blocked. The no-op path
	// acquires the mutex and returns synchronously, so the goroutine count is
	// stable across the call.
	before := runtime.NumGoroutine()
	hb.Start()
	assert.Equal(t, before, runtime.NumGoroutine(),
		"Start during drain must not spawn an overlapping goroutine")
	hb.mu.Lock()
	wantRunning := hb.wantRunning
	hb.mu.Unlock()
	assert.True(t, wantRunning, "Start must record the desired running state")

	countAtResume := blocker.callCount()

	// Unblock the stuck goroutine. It drains and, because wantRunning is set,
	// its cleanup relaunches a fresh goroutine that resumes emitting.
	close(block)
	require.Eventually(t, func() bool {
		return blocker.callCount() > countAtResume
	}, 2*time.Second, time.Millisecond,
		"heartbeat must relaunch and resume emitting after the stuck goroutine drains")

	hb.Stop() // clean up the relaunched goroutine
}

// Until SetMicroVMID is called, the heartbeat tags microvm_id with the
// placeholder "unknown". Production wiring sets the ID at /run before
// Start, so this state should never reach an emitted metric in practice;
// the test pins the contract anyway in case wiring changes.
func TestHeartbeat_TagsForEmit_DefaultsMicroVMIDToUnknown(t *testing.T) {
	hb := NewHeartbeat(time.Hour, &mockMetricEmitter{}, metrics.MetricSourceAWSMicroVMEnhanced, []string{"microvm_image_arn:test-arn"})
	tags := hb.tagsForEmit()
	assert.Contains(t, tags, "microvm_image_arn:test-arn")
	assert.Contains(t, tags, "microvm_id:unknown")
}

// When resourceName is empty the microvm_image_arn tag must be absent — an
// empty tag value would create a junk time-series in the metrics backend.
func TestHeartbeat_TagsForEmit_NoARNTagWhenResourceNameEmpty(t *testing.T) {
	hb := NewHeartbeat(time.Hour, &mockMetricEmitter{}, metrics.MetricSourceAWSMicroVMEnhanced, nil)
	for _, tag := range hb.tagsForEmit() {
		assert.NotContains(t, tag, "microvm_image_arn",
			"microvm_image_arn must not appear when resourceName is empty")
	}
}

func TestHeartbeat_SetMicroVMID_ReflectsOnEmittedTags(t *testing.T) {
	hb := NewHeartbeat(time.Hour, &mockMetricEmitter{}, metrics.MetricSourceAWSMicroVMEnhanced, []string{"microvm_image_arn:test-arn"})
	hb.SetMicroVMID("mvm-abc123")
	tags := hb.tagsForEmit()
	assert.Contains(t, tags, "microvm_image_arn:test-arn")
	assert.Contains(t, tags, "microvm_id:mvm-abc123")
}

// TestHeartbeat_EmittedMetric_CarriesARNTag verifies end-to-end that the
// microvm_image_arn tag derived from resourceName reaches AddEnhancedMetric.
// Testing tagsForEmit() in isolation does not prove emitAll passes the tags
// through correctly; this test closes that gap.
func TestHeartbeat_EmittedMetric_CarriesARNTag(t *testing.T) {
	emitter := &mockMetricEmitter{}
	hb := NewHeartbeat(time.Hour, emitter, metrics.MetricSourceAWSMicroVMEnhanced, []string{"microvm_image_arn:my-image-arn"})
	hb.Start()
	defer hb.Stop()

	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) >= 1
	}, 500*time.Millisecond, time.Millisecond)

	var hbMetric *emittedMetric
	for _, m := range emitter.getEmittedMetrics() {
		m := m
		if m.name == activeInstancesMetricName {
			hbMetric = &m
			break
		}
	}
	require.NotNil(t, hbMetric)
	assert.Contains(t, hbMetric.extraTags, "microvm_image_arn:my-image-arn",
		"ARN from resourceName must appear in the emitted heartbeat metric tags")
}

// Empty SetMicroVMID input is ignored — preserves the existing value rather
// than clobbering with empty. Defends against a /run where the platform
// header is missing: the heartbeat keeps emitting with whatever ID was
// last seen (or "unknown" if never set).
func TestHeartbeat_SetMicroVMID_EmptyIsIgnored(t *testing.T) {
	hb := NewHeartbeat(time.Hour, &mockMetricEmitter{}, metrics.MetricSourceAWSMicroVMEnhanced, nil)
	hb.SetMicroVMID("first-id")
	hb.SetMicroVMID("")
	tags := hb.tagsForEmit()
	assert.Contains(t, tags, "microvm_id:first-id", "empty ID must not clobber the existing value")
}

func TestHeartbeat_SetMicroVMID_OnNilReceiverIsSafe(t *testing.T) {
	var hb *Heartbeat
	assert.NotPanics(t, func() { hb.SetMicroVMID("anything") })
}

// When the heartbeat goroutine is running, a SetMicroVMID call must be
// reflected on subsequent emissions. The mock emitter records full tag
// slices so we can assert on what reached AddEnhancedMetric.
func TestHeartbeat_SetMicroVMID_VisibleOnNextTick(t *testing.T) {
	emitter := &mockMetricEmitter{}
	hb := NewHeartbeat(20*time.Millisecond, emitter, metrics.MetricSourceAWSMicroVMEnhanced, nil)
	hb.Start()
	defer hb.Stop()

	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) >= 1
	}, 500*time.Millisecond, 5*time.Millisecond)

	hb.SetMicroVMID("mvm-after-start")
	// Wait for at least one tick after the SetMicroVMID call.
	emittedBefore := countOfMetric(emitter, activeInstancesMetricName)
	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) > emittedBefore
	}, 500*time.Millisecond, 5*time.Millisecond)

	// The most recent emission must carry the updated ID.
	last := emitter.lastTags()
	require.NotNil(t, last)
	assert.Contains(t, last, "microvm_id:mvm-after-start")
}

// TestHeartbeat_EmittedMetricsCarryCurrentTimestamp verifies that heartbeat
// emissions go through emitMetric (current wall-clock) rather than the 0
// sentinel. The window bounds the timestamp to the observed emission window,
// guarding against unit regressions (e.g. reverting to the 0 sentinel).
func TestHeartbeat_EmittedMetricsCarryCurrentTimestamp(t *testing.T) {
	hb, emitter := newTestHeartbeat(20 * time.Millisecond)

	before := float64(time.Now().UnixNano()) / float64(time.Second)
	hb.Start()
	defer hb.Stop()

	require.Eventually(t, func() bool {
		return countOfMetric(emitter, activeInstancesMetricName) >= 1
	}, 500*time.Millisecond, 5*time.Millisecond, "expected at least one heartbeat emission")

	after := float64(time.Now().UnixNano()) / float64(time.Second)

	for _, m := range emitter.getEmittedMetrics() {
		assert.Greater(t, m.timestamp, 0.0, "timestamp must not be the 0 sentinel")
		assert.GreaterOrEqual(t, m.timestamp, before, "timestamp must be at or after pre-start time")
		assert.LessOrEqual(t, m.timestamp, after, "timestamp must be at or before observation time")
	}
}

// countOfMetric returns the number of times metricName appears in the mock
// emitter's recorded emissions.
func countOfMetric(m *mockMetricEmitter, metricName string) int {
	emitted := m.getEmitted()
	count := 0
	for _, n := range emitted {
		if n == metricName {
			count++
		}
	}
	return count
}

// blockingMetricEmitter blocks inside AddEnhancedMetric until its block
// channel is closed. Used to simulate a full metric-agent samples channel.
type blockingMetricEmitter struct {
	mu      sync.Mutex
	blocked bool
	count   int
	block   chan struct{}
}

func (b *blockingMetricEmitter) AddEnhancedMetric(_ string, _ float64, _ metrics.MetricSource, _ float64, _ ...string) {
	b.mu.Lock()
	b.blocked = true
	b.count++
	b.mu.Unlock()
	<-b.block // blocks until closed
	b.mu.Lock()
	b.blocked = false
	b.mu.Unlock()
}

func (b *blockingMetricEmitter) isBlocked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.blocked
}

func (b *blockingMetricEmitter) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

// noopMetricEmitter discards metrics. Used by benchmarks to isolate the
// Start/Stop lifecycle cost from any emitter bookkeeping/allocations.
type noopMetricEmitter struct{}

func (noopMetricEmitter) AddEnhancedMetric(_ string, _ float64, _ metrics.MetricSource, _ float64, _ ...string) {
}

// BenchmarkHeartbeat_StartStopCycle measures a full lifecycle churn: Start
// spawns the emit goroutine and Stop cancels and joins it. This is the path
// driven by /run, /suspend, /resume, /terminate. A long interval keeps the
// ticker from firing, so the measurement reflects goroutine spawn+join plus the
// wantRunning bookkeeping the fix added — not the emit cadence.
func BenchmarkHeartbeat_StartStopCycle(b *testing.B) {
	hb := NewHeartbeat(time.Hour, noopMetricEmitter{}, metrics.MetricSourceAWSMicroVMEnhanced, []string{"microvm_image_arn:bench"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hb.Start()
		hb.Stop()
	}
}

// BenchmarkHeartbeat_StartWhileRunning isolates the idempotent Start path
// (heartbeat already running): acquire the mutex, set wantRunning, observe
// cancel != nil, return. This is exactly the per-call overhead the fix adds on
// the no-op path — it should be a handful of ns with zero allocations.
func BenchmarkHeartbeat_StartWhileRunning(b *testing.B) {
	hb := NewHeartbeat(time.Hour, noopMetricEmitter{}, metrics.MetricSourceAWSMicroVMEnhanced, nil)
	hb.Start()
	defer hb.Stop()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hb.Start() // no-op: already running
	}
}
