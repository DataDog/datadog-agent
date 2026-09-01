// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experiment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWatcher records what it was asked to watch and lets a test inject exits.
type fakeWatcher struct {
	exits chan ExitEvent
	// watched is the last non-empty set of labels Watch was called with.
	watched []string
	// stops counts the Watch(nil) calls, which is how the supervisor says "stop watching".
	stops int
	err   error
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{exits: make(chan ExitEvent, 8)}
}

func (w *fakeWatcher) Watch(_ context.Context, labels []string) error {
	if len(labels) == 0 {
		w.stops++
	} else {
		w.watched = labels
	}
	return w.err
}
func (w *fakeWatcher) Exits() <-chan ExitEvent { return w.exits }
func (w *fakeWatcher) Close() error            { return nil }

type recordedRevert struct {
	version string
	reason  string
}

func newSupervisor(t *testing.T) (*Supervisor, *Deadline, *fakeWatcher, *[]recordedRevert, *time.Time) {
	t.Helper()
	deadline := NewDeadlineAt(filepath.Join(t.TempDir(), deadlineFileName))
	watcher := newFakeWatcher()
	var reverts []recordedRevert
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	supervisor := NewSupervisor(deadline, watcher, []string{"com.datadoghq.agent-exp"}, func(_ context.Context, version string, reason string) error {
		reverts = append(reverts, recordedRevert{version: version, reason: reason})
		return nil
	})
	supervisor.now = func() time.Time { return now }
	return supervisor, deadline, watcher, &reverts, &now
}

// TestArmPersistsTheDeadlineAndStartsWatching is the basic contract. The deadline has to be on
// disk, not in the struct, because the process holding the struct is itself replaced by the
// experiment.
func TestArmPersistsTheDeadlineAndStartsWatching(t *testing.T) {
	supervisor, deadline, watcher, _, now := newSupervisor(t)

	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", 30*time.Minute))

	version, expiresAt, set, err := deadline.Get()
	require.NoError(t, err)
	require.True(t, set)
	assert.Equal(t, "7.99.0", version)
	assert.Equal(t, now.Add(30*time.Minute), expiresAt)
	assert.Equal(t, []string{"com.datadoghq.agent-exp"}, watcher.watched)
}

// TestArmClampsTheDuration pins the host-side clamp. The duration arrives from a remote task, and
// an unbounded or absurd one would defeat the only thing standing between a bad version and a host
// that never comes back.
func TestArmClampsTheDuration(t *testing.T) {
	for name, testCase := range map[string]struct {
		requested time.Duration
		want      time.Duration
	}{
		"unset gets the default":    {requested: 0, want: DefaultDuration},
		"negative gets the default": {requested: -time.Hour, want: DefaultDuration},
		"too short is raised":       {requested: time.Second, want: MinDuration},
		"too long is capped":        {requested: 30 * 24 * time.Hour, want: MaxDuration},
		"reasonable is honoured":    {requested: 2 * time.Hour, want: 2 * time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			supervisor, deadline, _, _, now := newSupervisor(t)
			require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", testCase.requested))
			_, expiresAt, _, err := deadline.Get()
			require.NoError(t, err)
			assert.Equal(t, now.Add(testCase.want), expiresAt)
		})
	}
}

// TestTickDoesNothingBeforeTheDeadline is the case that runs on almost every tick of almost every
// host, so it is the one that must not revert.
func TestTickDoesNothingBeforeTheDeadline(t *testing.T) {
	supervisor, _, _, reverts, _ := newSupervisor(t)
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", time.Hour))

	require.NoError(t, supervisor.Tick(context.Background()))
	assert.Empty(t, *reverts)
}

// TestTickRevertsAtTheDeadline is what the deadline is for: the backend said nothing, and the
// experiment ends anyway.
func TestTickRevertsAtTheDeadline(t *testing.T) {
	supervisor, deadline, _, reverts, now := newSupervisor(t)
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", time.Hour))

	*now = now.Add(time.Hour)
	require.NoError(t, supervisor.Tick(context.Background()))

	require.Len(t, *reverts, 1)
	assert.Equal(t, "7.99.0", (*reverts)[0].version)
	assert.Contains(t, (*reverts)[0].reason, "deadline")

	_, _, set, err := deadline.Get()
	require.NoError(t, err)
	assert.False(t, set, "the deadline was not cleared after the revert")
}

// TestTickRevertsOnAnExperimentJobExit is the fast path. An -exp job carries no KeepAlive, so its
// exit is terminal and means the version does not work here.
func TestTickRevertsOnAnExperimentJobExit(t *testing.T) {
	supervisor, _, watcher, reverts, _ := newSupervisor(t)
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", time.Hour))

	watcher.exits <- ExitEvent{Label: "com.datadoghq.agent-exp", PID: 4242, ExitStatus: 2}
	require.NoError(t, supervisor.Tick(context.Background()))

	require.Len(t, *reverts, 1)
	assert.Contains(t, (*reverts)[0].reason, "com.datadoghq.agent-exp")
	assert.Contains(t, (*reverts)[0].reason, "status 2")
}

// TestRevertStopsWatchingBeforeReverting is load-bearing: the revert boots out the experiment
// jobs, and their exits are a consequence of the revert. Reading them as a reason would revert a
// second time, against whatever is running by then.
func TestRevertStopsWatchingBeforeReverting(t *testing.T) {
	deadline := NewDeadlineAt(filepath.Join(t.TempDir(), deadlineFileName))
	watcher := newFakeWatcher()
	stopsAtRevert := -1
	supervisor := NewSupervisor(deadline, watcher, []string{"com.datadoghq.agent-exp"}, func(_ context.Context, _ string, _ string) error {
		stopsAtRevert = watcher.stops
		// The revert boots out the jobs, which the kernel reports as exits.
		watcher.exits <- ExitEvent{Label: "com.datadoghq.agent-exp", PID: 4242}
		return nil
	})
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	supervisor.now = func() time.Time { return now }

	// MinDuration, not a nanosecond: the clamp raises anything shorter, so the clock has to
	// be moved rather than the duration made small.
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", MinDuration))
	now = now.Add(MinDuration + time.Second)
	require.NoError(t, supervisor.Tick(context.Background()))
	assert.Positive(t, stopsAtRevert, "the watch was still running when the revert started")

	// And the exit the revert itself caused must not drive a second revert.
	reverts := 0
	supervisor.revert = func(_ context.Context, _ string, _ string) error { reverts++; return nil }
	require.NoError(t, supervisor.Tick(context.Background()))
	assert.Zero(t, reverts, "the revert's own fallout caused a second revert")
}

// TestAFailedRevertKeepsTheDeadlineSoTheNextTickRetries is why the deadline is cleared last. A
// transient failure -- launchctl busy, a filesystem hiccup -- must not consume the only record
// that the host is on an experiment.
func TestAFailedRevertKeepsTheDeadlineSoTheNextTickRetries(t *testing.T) {
	deadline := NewDeadlineAt(filepath.Join(t.TempDir(), deadlineFileName))
	attempts := 0
	supervisor := NewSupervisor(deadline, newFakeWatcher(), []string{"com.datadoghq.agent-exp"}, func(_ context.Context, _ string, _ string) error {
		attempts++
		if attempts == 1 {
			return errors.New("launchctl was busy")
		}
		return nil
	})
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	supervisor.now = func() time.Time { return now }

	// MinDuration, not a nanosecond: the clamp raises anything shorter, so the clock has to
	// be moved rather than the duration made small.
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", MinDuration))
	now = now.Add(MinDuration + time.Second)

	require.Error(t, supervisor.Tick(context.Background()))
	_, _, set, err := deadline.Get()
	require.NoError(t, err)
	require.True(t, set, "a failed revert cleared the deadline, leaving the experiment unsupervised")

	require.NoError(t, supervisor.Tick(context.Background()))
	assert.Equal(t, 2, attempts)
	_, _, set, err = deadline.Get()
	require.NoError(t, err)
	assert.False(t, set)
}

// TestDisarmClearsTheDeadline covers promote and stop, which end the experiment deliberately.
func TestDisarmClearsTheDeadline(t *testing.T) {
	supervisor, deadline, watcher, reverts, _ := newSupervisor(t)
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", time.Hour))

	require.NoError(t, supervisor.Disarm(context.Background()))
	_, _, set, err := deadline.Get()
	require.NoError(t, err)
	assert.False(t, set)
	assert.Positive(t, watcher.stops)

	// And nothing reverts afterwards, however long the host runs.
	require.NoError(t, supervisor.Tick(context.Background()))
	assert.Empty(t, *reverts)
}

// TestDisarmOnAnIdleHostIsANoOp is the property the promote and stop hooks rest on: they call
// Disarm unconditionally, and a host that never ran an experiment must not see an error.
func TestDisarmOnAnIdleHostIsANoOp(t *testing.T) {
	supervisor, _, _, reverts, _ := newSupervisor(t)
	require.NoError(t, supervisor.Disarm(context.Background()))
	require.NoError(t, supervisor.Disarm(context.Background()))
	assert.Empty(t, *reverts)
}

// TestReconcileOnAnIdleHostDoesNothing is what almost every daemon start hits.
func TestReconcileOnAnIdleHostDoesNothing(t *testing.T) {
	supervisor, _, watcher, reverts, _ := newSupervisor(t)

	require.NoError(t, supervisor.Reconcile(context.Background()))
	assert.Empty(t, *reverts)
	assert.Empty(t, watcher.watched)
}

// TestReconcileResumesAnUnexpiredExperiment covers the daemon being restarted by the experiment it
// is supervising -- which is the normal case, because the installer is part of the payload.
func TestReconcileResumesAnUnexpiredExperiment(t *testing.T) {
	supervisor, _, watcher, reverts, now := newSupervisor(t)
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", time.Hour))

	// A fresh supervisor over the same deadline file: the process restarted.
	restarted := NewSupervisor(supervisor.deadline, watcher, supervisor.labels, supervisor.revert)
	restarted.now = func() time.Time { return now.Add(10 * time.Minute) }

	require.NoError(t, restarted.Reconcile(context.Background()))
	assert.Empty(t, *reverts)
	assert.Equal(t, []string{"com.datadoghq.agent-exp"}, watcher.watched)
}

// TestReconcileRevertsAnExperimentThatOutlivedTheDaemon is the case the persisted deadline exists
// for. The host rebooted or the daemon was down, the experiment kept running unsupervised, and the
// bound is enforced as soon as anything is watching again.
func TestReconcileRevertsAnExperimentThatOutlivedTheDaemon(t *testing.T) {
	supervisor, deadline, watcher, _, now := newSupervisor(t)
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", time.Hour))

	var reverts []recordedRevert
	restarted := NewSupervisor(deadline, watcher, supervisor.labels, func(_ context.Context, version string, reason string) error {
		reverts = append(reverts, recordedRevert{version: version, reason: reason})
		return nil
	})
	restarted.now = func() time.Time { return now.Add(4 * time.Hour) }

	require.NoError(t, restarted.Reconcile(context.Background()))
	require.Len(t, reverts, 1)
	assert.Equal(t, "7.99.0", reverts[0].version)
	assert.Contains(t, reverts[0].reason, "while the installer was not running")

	_, _, set, err := deadline.Get()
	require.NoError(t, err)
	assert.False(t, set)
}

// TestAnUnreadableDeadlineIsAnErrorNotAnIdleHost is the fail-loud case. Reading a corrupt marker
// as "no experiment is running" would strand a running experiment with nothing left to revert it,
// which is silently the worst outcome available.
func TestAnUnreadableDeadlineIsAnErrorNotAnIdleHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), deadlineFileName)
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0644))
	deadline := NewDeadlineAt(path)

	_, _, _, err := deadline.Get()
	require.Error(t, err)

	supervisor := NewSupervisor(deadline, newFakeWatcher(), nil, func(context.Context, string, string) error { return nil })
	assert.Error(t, supervisor.Tick(context.Background()))
	assert.Error(t, supervisor.Reconcile(context.Background()))
}

// TestAnIncompleteDeadlineIsAnError covers a file that parses but says nothing usable, which is
// what a truncated write would look like if the write were not atomic.
func TestAnIncompleteDeadlineIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), deadlineFileName)
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"7.99.0"}`), 0644))

	_, _, _, err := NewDeadlineAt(path).Get()
	assert.Error(t, err)
}

// TestSetRequiresAVersion keeps a deadline from being recorded against no version, which would
// leave the revert with nothing to report.
func TestSetRequiresAVersion(t *testing.T) {
	deadline := NewDeadlineAt(filepath.Join(t.TempDir(), deadlineFileName))
	assert.Error(t, deadline.Set("", time.Now()))
}

// TestSetIsAtomicUnderReplacement pins that a second Set replaces the first rather than appending
// to it, which is what the temporary-file-and-rename is there for.
func TestSetIsAtomicUnderReplacement(t *testing.T) {
	deadline := NewDeadlineAt(filepath.Join(t.TempDir(), deadlineFileName))
	first := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, deadline.Set("7.99.0", first))
	require.NoError(t, deadline.Set("7.99.1", first.Add(time.Hour)))

	version, expiresAt, set, err := deadline.Get()
	require.NoError(t, err)
	require.True(t, set)
	assert.Equal(t, "7.99.1", version)
	assert.Equal(t, first.Add(time.Hour), expiresAt)

	// And no temporary file was left in the directory.
	entries, err := os.ReadDir(filepath.Dir(deadline.Path()))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// TestSupervisorWithoutAWatcherStillHonoursTheDeadline pins the degraded mode: if the kqueue
// watcher cannot be opened, the experiment is still bounded.
func TestSupervisorWithoutAWatcherStillHonoursTheDeadline(t *testing.T) {
	deadline := NewDeadlineAt(filepath.Join(t.TempDir(), deadlineFileName))
	reverts := 0
	supervisor := NewSupervisor(deadline, nil, nil, func(context.Context, string, string) error { reverts++; return nil })
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	supervisor.now = func() time.Time { return now }

	// MinDuration, not a nanosecond: the clamp raises anything shorter, so the clock has to
	// be moved rather than the duration made small.
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", MinDuration))
	now = now.Add(MinDuration + time.Second)
	require.NoError(t, supervisor.Tick(context.Background()))
	assert.Equal(t, 1, reverts)
}

// TestNoRevertConfiguredIsAnError guards against a supervisor wired up without its one callback,
// which would look like a working supervisor that never reverts anything.
func TestNoRevertConfiguredIsAnError(t *testing.T) {
	deadline := NewDeadlineAt(filepath.Join(t.TempDir(), deadlineFileName))
	supervisor := NewSupervisor(deadline, nil, nil, nil)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	supervisor.now = func() time.Time { return now }

	// MinDuration, not a nanosecond: the clamp raises anything shorter, so the clock has to
	// be moved rather than the duration made small.
	require.NoError(t, supervisor.Arm(context.Background(), "7.99.0", MinDuration))
	now = now.Add(MinDuration + time.Second)
	assert.Error(t, supervisor.Tick(context.Background()))
}
