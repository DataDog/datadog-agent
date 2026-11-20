// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	t time.Time
}

func (c *fakeClock) now() time.Time { return c.t }

func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestRecorder() (*Recorder, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return NewRecorder(clock.t, clock.now), clock
}

func TestStartupWithoutReadyMarker(t *testing.T) {
	r, clock := newTestRecorder()

	var got []Startup
	r.OnReady(func(s Startup) { got = append(got, s) })

	clock.advance(100 * time.Millisecond)
	r.BeforeFxNew()
	clock.advance(time.Second)
	r.RecordComponent(StageConstruct, "fast", 10*time.Millisecond)
	r.RecordComponent(StageConstruct, "slow", 900*time.Millisecond)
	r.FxConstructed()

	startBegin := clock.t
	clock.advance(2 * time.Second)
	r.RecordComponent(StageOnStart, "slow-hook", 2*time.Second)
	r.FxStarted(startBegin)

	require.Len(t, got, 1, "the process is ready once Fx has started")
	assert.Equal(t, map[string]float64{
		PhaseInit:        0.1,
		PhaseFxConstruct: 1,
		PhaseFxStart:     2,
		PhaseTotal:       3.1,
	}, roundPhases(got[0].Phases))
	assert.Equal(t, []Timing{
		{Name: "slow-hook", Stage: StageOnStart, Seconds: 2},
		{Name: "slow", Stage: StageConstruct, Seconds: 0.9},
	}, got[0].SlowComponents)

	// Callbacks registered after the startup are called right away, and markers are only taken
	// into account once.
	r.MarkReady()
	r.FxStarted(clock.t)
	r.OnReady(func(s Startup) { got = append(got, s) })
	require.Len(t, got, 2)
	assert.Equal(t, got[0], got[1])
}

func TestStartupWithReadyMarker(t *testing.T) {
	r, clock := newTestRecorder()
	r.ExpectReadyMarker()

	var got *Startup
	r.OnReady(func(s Startup) { got = &s })

	r.BeforeFxNew()
	r.FxConstructed()
	startBegin := clock.t
	clock.advance(time.Second)
	r.FxStarted(startBegin)
	assert.Nil(t, got, "the process is not ready until MarkReady is called")

	clock.advance(5 * time.Second)
	r.RecordComponent(StageModuleStart, "network_tracer", 4*time.Second)
	r.MarkReady()

	require.NotNil(t, got)
	assert.InDelta(t, 5, got.Phases[PhaseAppStart], 1e-9)
	assert.InDelta(t, 6, got.Phases[PhaseTotal], 1e-9)
	assert.Equal(t, []Timing{{Name: "network_tracer", Stage: StageModuleStart, Seconds: 4}}, got.SlowComponents)
}

func TestSlowComponentsAreBounded(t *testing.T) {
	r, _ := newTestRecorder()
	for i := 1; i <= 2*MaxSlowComponents; i++ {
		r.RecordComponent(StageConstruct, "c", time.Duration(i)*time.Second)
	}
	r.RecordComponent(StageConstruct, "under-threshold", DefaultSlowComponentThreshold-1)

	var got Startup
	r.OnReady(func(s Startup) { got = s })
	r.MarkReady()

	require.Len(t, got.SlowComponents, MaxSlowComponents)
	for i, c := range got.SlowComponents {
		assert.InDelta(t, float64(2*MaxSlowComponents-i), c.Seconds, 1e-9)
	}
}

func TestCleanShutdownIsPersisted(t *testing.T) {
	r, clock := newTestRecorder()
	path := filepath.Join(t.TempDir(), "lifecycle-agent.json")
	r.Persist(path, func(err error) { t.Error(err) })

	rec := readRecord(t, path)
	assert.Equal(t, OutcomeDiedDuringStartup, rec.Outcome())

	r.MarkReady()
	assert.Equal(t, OutcomeDiedWhileRunning, readRecord(t, path).Outcome())

	clock.advance(time.Minute)
	r.BeginStop(TriggerSignal)

	done := r.StartShutdownPhase(PhaseAppStop)
	clock.advance(3 * time.Second)
	done()

	done = r.StartShutdownPhase(PhaseFxStop)
	r.ComponentStopping("comp/a.stop")
	clock.advance(time.Second)
	r.RecordComponent(StageOnStop, "comp/a.stop", time.Second)
	done()
	r.Finish()

	rec = readRecord(t, path)
	assert.Equal(t, OutcomeClean, rec.Outcome())
	assert.Equal(t, TriggerSignal, rec.Trigger)
	assert.Empty(t, rec.CurrentPhase)
	assert.Empty(t, rec.CurrentComponent)
	assert.Equal(t, map[string]float64{PhaseAppStop: 3, PhaseFxStop: 1, PhaseTotal: 4}, roundPhases(rec.Phases))
	assert.Equal(t, []Timing{{Name: "comp/a.stop", Stage: StageOnStop, Seconds: 1}}, rec.SlowComponents)
	total, ok := rec.ShutdownSeconds()
	assert.True(t, ok)
	assert.InDelta(t, 4, total, 1e-9)
}

func TestKilledDuringShutdown(t *testing.T) {
	r, clock := newTestRecorder()
	path := filepath.Join(t.TempDir(), "lifecycle-system-probe.json")
	r.Persist(path, nil)
	r.MarkReady()

	r.BeginStop(TriggerSignal)
	clock.advance(time.Second)
	_ = r.StartShutdownPhase(PhaseAppStop)
	clock.advance(2 * time.Second)
	r.ComponentStopping("event_monitor")
	// The process is killed here: the phase never completes.

	rec := readRecord(t, path)
	assert.Equal(t, OutcomeKilledDuringShutdown, rec.Outcome())
	assert.Equal(t, PhaseAppStop, rec.CurrentPhase)
	assert.Equal(t, "event_monitor", rec.CurrentComponent)
	total, ok := rec.ShutdownSeconds()
	assert.True(t, ok)
	assert.InDelta(t, 3, total, 1e-9, "lower bound of the shutdown duration")
}

func TestImplicitShutdown(t *testing.T) {
	r, clock := newTestRecorder()
	// Finish without a shutdown does nothing, e.g. for one-shot commands.
	r.Finish()

	done := r.StartShutdownPhase(PhaseFxStop)
	clock.advance(time.Second)
	done()
	r.Finish()
	assert.Equal(t, TriggerUnknown, r.record.Trigger)
	assert.Equal(t, StateStopped, r.record.State)

	// Markers after the end of the shutdown are ignored.
	r.BeginStop(TriggerSignal)
	r.StartShutdownPhase(PhaseAppStop)()
	assert.Equal(t, TriggerUnknown, r.record.Trigger)
	assert.NotContains(t, r.record.Phases, PhaseAppStop)
}

func TestLoadPrevious(t *testing.T) {
	dir := t.TempDir()

	_, found, err := LoadPrevious(filepath.Join(dir, "missing.json"))
	assert.NoError(t, err)
	assert.False(t, found)

	path := filepath.Join(dir, "lifecycle-agent.json")
	require.NoError(t, writeRecord(path, Record{Version: recordVersion, State: StateStopped, Phases: map[string]float64{PhaseTotal: 2}}))
	rec, found, err := LoadPrevious(path)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, OutcomeClean, rec.Outcome())
	assert.NoFileExists(t, path, "the record is reported once")

	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))
	_, found, err = LoadPrevious(path)
	assert.Error(t, err)
	assert.False(t, found)
	assert.NoFileExists(t, path, "an invalid record is removed")

	require.NoError(t, writeRecord(path, Record{Version: recordVersion + 1}))
	_, found, err = LoadPrevious(path)
	assert.Error(t, err)
	assert.False(t, found)
}

func TestPersistErrors(t *testing.T) {
	r, _ := newTestRecorder()
	var errs []error
	r.Persist(filepath.Join(t.TempDir(), "missing-dir", "lifecycle-agent.json"), func(err error) { errs = append(errs, err) })
	assert.NotEmpty(t, errs)
}

func readRecord(t *testing.T, path string) Record {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	tmp := filepath.Join(t.TempDir(), "copy.json")
	require.NoError(t, os.WriteFile(tmp, data, 0o600))
	rec, found, err := LoadPrevious(tmp)
	require.NoError(t, err)
	require.True(t, found)
	return rec
}

func roundPhases(phases map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(phases))
	for k, v := range phases {
		out[k] = float64(time.Duration(v*float64(time.Second)).Round(time.Millisecond)) / float64(time.Second)
	}
	return out
}
