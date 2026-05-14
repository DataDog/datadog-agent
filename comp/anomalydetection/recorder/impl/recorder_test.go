// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package recorderimpl

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// fakeHandle is a minimal observer.Handle for testing — records the calls it receives
// so the test can assert that the recorder forwarded the observation.
type fakeHandle struct {
	metricCalls int
	logCalls    int
}

func (h *fakeHandle) ObserveMetric(_ observerdef.MetricView) { h.metricCalls++ }
func (h *fakeHandle) ObserveLog(_ observerdef.LogView)       { h.logCalls++ }

// fakeLogView is a minimal observer.LogView with caller-controlled fields,
// used to assert that ObserveLog reads the message's own timestamp.
type fakeLogView struct {
	content     []byte
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

func (f *fakeLogView) GetContent() []byte           { return f.content }
func (f *fakeLogView) GetStatus() string            { return f.status }
func (f *fakeLogView) GetTags() []string            { return f.tags }
func (f *fakeLogView) GetHostname() string          { return f.hostname }
func (f *fakeLogView) GetTimestampUnixMilli() int64 { return f.timestampMs }

// TestRecorderDisabledByDefault locks in the most important contract: when
// anomaly_detection.recording.enabled is false (the default), GetHandle returns
// the inner handle unwrapped and no writer goroutines are started. This is the
// production-safe path that every agent uses.
func TestRecorderDisabledByDefault(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled": false,
	})
	lc := compdef.NewTestLifecycle(t)
	goroutinesBefore := runtime.NumGoroutine()

	out := NewComponent(Requires{Lifecycle: lc, Config: cfg})

	require.NotNil(t, out.Comp)
	lc.AssertHooksNumber(0) // no shutdown hook needed; nothing to clean up

	inner := &fakeHandle{}
	wrapped := out.Comp.GetHandle(func(_ string) observerdef.Handle { return inner })("test-source")
	require.Same(t, observerdef.Handle(inner), wrapped, "disabled recorder must return inner handle unwrapped")

	// Verify no recording-side state was constructed: NumGoroutine should not have
	// grown by the four worker goroutines a real writer would have spawned.
	require.LessOrEqual(t, runtime.NumGoroutine(), goroutinesBefore+1,
		"disabled recorder must not start writer goroutines")
}

// TestRecorderMisconfigDoesNotFailStart locks in that bad recording config
// (empty output_dir while enabled=true) logs and silently disables the recorder
// rather than failing fx graph construction. This matters because the recorder
// ships in every agent; a config typo must not brick agent startup.
func TestRecorderMisconfigDoesNotFailStart(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":    true,
		"anomaly_detection.recording.output_dir": "", // misconfigured
	})
	lc := compdef.NewTestLifecycle(t)

	out := NewComponent(Requires{Lifecycle: lc, Config: cfg})

	require.NotNil(t, out.Comp, "constructor must return a usable component even on bad config")
	lc.AssertHooksNumber(0) // no writers => nothing to stop

	// And the returned component must behave as disabled, not panic on use.
	inner := &fakeHandle{}
	wrapped := out.Comp.GetHandle(func(_ string) observerdef.Handle { return inner })("x")
	require.Same(t, observerdef.Handle(inner), wrapped)
}

// TestRecorderLifecycleFlushesAndStops is the most important new test.
// It exercises the full enabled path: writes a metric, runs the OnStop hook,
// and verifies (a) the parquet file is on disk afterwards (final batch flushed,
// not lost) and (b) writer goroutines have terminated (no leak).
func TestRecorderLifecycleFlushesAndStops(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":        true,
		"anomaly_detection.recording.output_dir":     tmpDir,
		"anomaly_detection.recording.flush_interval": time.Hour, // long: ensure flush only happens via OnStop
		"anomaly_detection.recording.retention":      time.Hour,
	})
	lc := compdef.NewTestLifecycle(t)
	goroutinesBefore := runtime.NumGoroutine()

	out := NewComponent(Requires{Lifecycle: lc, Config: cfg})
	require.NotNil(t, out.Comp)
	lc.AssertHooksNumber(1)

	// Writer goroutines should now exist. We started 4 (metric + log flushLoop
	// and cleanupLoop) but only require >0 to avoid flakiness from unrelated
	// concurrent goroutines. The leak-detection happens after Stop, below.
	require.Greater(t, runtime.NumGoroutine(), goroutinesBefore, "writer goroutines should be running")

	// Write directly through the typed writer rather than constructing a full
	// observer handle/MetricView mock — we're testing the lifecycle, not the
	// handle wrapper. The handle wrapper path is exercised by other tests.
	impl := out.Comp.(*recorderImpl)
	impl.metricParquetWriter.WriteMetric("src", "metric.x", 1.5,
		[]string{"env:test"}, 1234567890, false)

	// No file should exist yet — flushInterval is 1h, so only OnStop can flush.
	files, err := filepath.Glob(filepath.Join(tmpDir, "observer-metrics-*.parquet"))
	require.NoError(t, err)
	require.Empty(t, files, "no flush should happen before OnStop with flush_interval=1h")

	// Run the OnStop hook.
	require.NoError(t, lc.Stop(context.Background()))

	// The in-memory batch must have been flushed to disk.
	files, err = filepath.Glob(filepath.Join(tmpDir, "observer-metrics-*.parquet"))
	require.NoError(t, err)
	require.Len(t, files, 1, "OnStop must flush the pending batch to disk")
	st, err := os.Stat(files[0])
	require.NoError(t, err)
	require.Greater(t, st.Size(), int64(minParquetFileSize), "flushed file should be a non-trivial parquet")

	// Allow goroutines to wind down; flushLoop returns after the final flush, cleanupLoop
	// after seeing stopCh close. They should be gone within a short window.
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= goroutinesBefore+1
	}, 2*time.Second, 20*time.Millisecond,
		"writer goroutines must terminate after OnStop (leak check)")

	// Subsequent OnStop must be idempotent and not panic.
	require.NoError(t, impl.metricParquetWriter.Close())
}

// TestMetricTimestampUnitConsistency locks in that MetricData.TimestampMs is
// reported in the same unit as LogData.TimestampMs (milliseconds), so consumers
// can join the two streams on time without unit conversion. The bug class this
// guards against: the underlying parquet schema stores ms internally, but if the
// reader/writer/exposed field aren't all consistent on ms-vs-seconds, joins are
// silently off by 1000x.
func TestMetricTimestampUnitConsistency(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a metric with timestamp passed in seconds (as the observer handle path does),
	// then read it back through the public MetricData interface and confirm the
	// returned TimestampMs is in milliseconds — same unit as LogData.TimestampMs.
	w, err := newMetricParquetWriter(tmpDir, time.Hour, 0)
	require.NoError(t, err)
	const tsSec = int64(1234567890)
	w.WriteMetric("src", "m", 1.0, []string{"env:test"}, tsSec, false)
	require.NoError(t, w.Close())

	// Disabled recorder is fine here — we only need ReadAllMetrics, which doesn't
	// require recording-enabled state.
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled": false,
	})
	out := NewComponent(Requires{Lifecycle: compdef.NewTestLifecycle(t), Config: cfg})
	metrics, err := out.Comp.ReadAllMetrics(tmpDir)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, tsSec*1000, metrics[0].TimestampMs,
		"MetricData.TimestampMs must be in milliseconds (same unit as LogData.TimestampMs); writer received %d seconds, expected %d ms on read", tsSec, tsSec*1000)
}

// TestRecorderNegativeFlushIntervalFallsBackToDefault locks in the Codex #1
// fix: a negative anomaly_detection.recording.flush_interval must not be
// passed through to time.NewTicker (which panics on non-positive durations).
// The recorder should fall back to the 60s default instead.
func TestRecorderNegativeFlushIntervalFallsBackToDefault(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":        true,
		"anomaly_detection.recording.output_dir":     t.TempDir(),
		"anomaly_detection.recording.flush_interval": -1 * time.Second,
		"anomaly_detection.recording.retention":      time.Hour,
	})
	lc := compdef.NewTestLifecycle(t)

	out := NewComponent(Requires{Lifecycle: lc, Config: cfg})
	require.NotNil(t, out.Comp)

	impl := out.Comp.(*recorderImpl)
	require.NotNil(t, impl.metricParquetWriter, "writer should be constructed (recorder not disabled)")
	require.Equal(t, 60*time.Second, impl.metricParquetWriter.flushInterval,
		"negative flush_interval must fall back to 60s default")
	require.NoError(t, lc.Stop(context.Background()))
}

// TestRecordingHandle_PreservesLogTimestamp locks in the Codex #3 fix:
// ObserveLog must use the message's own GetTimestampUnixMilli() instead of
// wall-clock time, so replayed/delayed/buffered logs are recorded with their
// original event time.
func TestRecordingHandle_PreservesLogTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":        true,
		"anomaly_detection.recording.output_dir":     tmpDir,
		"anomaly_detection.recording.flush_interval": time.Hour, // only flush on Stop
		"anomaly_detection.recording.retention":      time.Hour,
	})
	lc := compdef.NewTestLifecycle(t)

	out := NewComponent(Requires{Lifecycle: lc, Config: cfg})
	require.NotNil(t, out.Comp)

	inner := &fakeHandle{}
	handle := out.Comp.GetHandle(func(_ string) observerdef.Handle { return inner })("test-source")

	// Pick an event time that is unambiguously not "now".
	const eventTimeMs = int64(1_700_000_000_000)
	handle.ObserveLog(&fakeLogView{
		content:     []byte("hello"),
		status:      "info",
		hostname:    "host-x",
		tags:        []string{"env:test"},
		timestampMs: eventTimeMs,
	})

	require.NoError(t, lc.Stop(context.Background()))

	logs, err := out.Comp.ReadAllLogs(tmpDir)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, eventTimeMs, logs[0].TimestampMs,
		"recorded log timestamp must come from LogView.GetTimestampUnixMilli, not wall clock")
}
