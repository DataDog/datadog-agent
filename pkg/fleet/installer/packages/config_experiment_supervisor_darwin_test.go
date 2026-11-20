// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// runSupervisor runs s's main loop in the background and returns where its result arrives.
func runSupervisor(ctx context.Context, s *experimentSupervisor) <-chan error {
	done := make(chan error, 1)
	go func() { done <- s.run(ctx) }()
	return done
}

func waitFor(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the supervisor should have returned")
		return nil
	}
}

// stubStopOtherWatchers replaces W2's stop of the other watchers with a recorder.
func stubStopOtherWatchers(t *testing.T) *int {
	t.Helper()

	calls := new(int)
	original := stopOtherWatchers
	stopOtherWatchers = func(context.Context) error {
		*calls++
		return nil
	}
	t.Cleanup(func() { stopOtherWatchers = original })
	return calls
}

// shortOverlap shortens how long a watcher defers a failure it does not own.
func shortOverlap(t *testing.T, overlap time.Duration) {
	t.Helper()

	originalOverlap, originalStop, originalPoll := watcherOverlap, watcherStopTimeout, deferPoll
	watcherOverlap, watcherStopTimeout, deferPoll = overlap, overlap, 5*time.Millisecond
	t.Cleanup(func() { watcherOverlap, watcherStopTimeout, deferPoll = originalOverlap, originalStop, originalPoll })
}

// secondSupervisor is W2's state for the current deadline.
func secondSupervisor(t *testing.T) *experimentSupervisor {
	t.Helper()

	s := newTestSupervisor(t)
	s.role = secondWatcher
	return s
}

// exitOf records pid as label's armed pid and returns a failing exit event for it.
func exitOf(s *experimentSupervisor, pid int, label string) launchd.ExitEvent {
	s.pids[pid] = label
	return launchd.ExitEvent{Pid: pid, Status: 1 << 8}
}

// expectEvent waits for one change on the state watch.
func expectEvent(t *testing.T, changes <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-changes:
	case <-time.After(5 * time.Second):
		t.Fatalf("no change was seen when the deadline was %s", what)
	}
}

// TestWatchExperimentStateSeesEveryDeadlineChange pins the end-of-experiment signal: writing the
// deadline, marking it as reverting and clearing it each change the directory, and each is seen.
func TestWatchExperimentStateSeesEveryDeadlineChange(t *testing.T) {
	stubDeadlinePath(t)
	original := stateWatchPoll
	stateWatchPoll = 10 * time.Millisecond
	t.Cleanup(func() { stateWatchPoll = original })
	changes, stop, err := watchExperimentState()
	require.NoError(t, err)
	t.Cleanup(stop)

	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	expectEvent(t, changes, "written")
	require.NoError(t, experimentDeadline().MarkReverting())
	expectEvent(t, changes, "marked as reverting")
	require.NoError(t, experimentDeadline().Clear())
	expectEvent(t, changes, "cleared")
	stop()
	stop()
}

// TestSecondWatcherStopsTheOtherWatchersOnceArmed pins W2's start: it arms on every experiment job,
// then stops every other watcher, W1 included, and owns the experiment daemon's exit once they are
// gone.
func TestSecondWatcherStopsTheOtherWatchersOnceArmed(t *testing.T) {
	runningExperiment(t)
	stopped := stubStopOtherWatchers(t)
	s := secondSupervisor(t)
	t.Cleanup(s.release)

	require.NoError(t, s.takeOver(context.Background()))
	assert.False(t, s.finished)
	assert.NotNil(t, s.agentObserver, "W2 should be armed on every experiment job")
	require.Eventually(t, func() bool { return s.predecessorGone.Load() }, 5*time.Second, time.Millisecond)
	assert.Equal(t, 1, *stopped)
}

// TestSecondWatcherDoesNotStopTheFirstWhenItCannotArm pins that W2 leaves W1 in place until it can
// supervise itself.
func TestSecondWatcherDoesNotStopTheFirstWhenItCannotArm(t *testing.T) {
	l, _, _ := runningExperiment(t)
	l.failPrint["system/"+agentJob+"-exp"] = true
	stopped := stubStopOtherWatchers(t)
	s := secondSupervisor(t)
	t.Cleanup(s.release)

	require.Error(t, s.takeOver(context.Background()))
	assert.Zero(t, *stopped, "W1 must not be stopped by a W2 that could not arm")
}

// TestFirstWatcherDefersAnAgentFailureToARunningSecondWatcher pins (b1): with W2 running, an agent
// job failing is W2's to revert; W1 waits, sees W2 end the experiment, and does nothing.
func TestFirstWatcherDefersAnAgentFailureToARunningSecondWatcher(t *testing.T) {
	l, _, _ := runningExperiment(t)
	shortOverlap(t, 5*time.Second)
	stubWatchers(t, true)
	s := newTestSupervisor(t)
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = experimentDeadline().MarkFailed()
	}()

	done, err := s.onExit(context.Background(), exitOf(s, 4242, agentJob))
	require.NoError(t, err)
	assert.False(t, done)
	for _, label := range agentJobs {
		assert.True(t, l.isLoaded(label+"-exp"), "W1 must leave %s to W2", label+"-exp")
	}
}

// TestFirstWatcherRevertsAnAgentFailureOnceNoSecondWatcherIsLeft covers W2 dying with a failure it
// owned still pending: W1 takes it back and reverts.
func TestFirstWatcherRevertsAnAgentFailureOnceNoSecondWatcherIsLeft(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	shortOverlap(t, 5*time.Second)
	setRunning := stubWatchers(t, true)
	s := newTestSupervisor(t)
	go func() {
		time.Sleep(50 * time.Millisecond)
		setRunning(false)
	}()

	done, err := s.onExit(context.Background(), exitOf(s, 4242, agentJob))
	require.NoError(t, err)
	assert.True(t, done)
	assertReverted(t, l, experimentPath)
}

// TestFirstWatcherAlwaysRevertsTheExperimentDaemonExit pins that the experiment daemon's exit is
// W1's to revert for as long as W1 runs, even with W2 up: W2 may not have started at all.
func TestFirstWatcherAlwaysRevertsTheExperimentDaemonExit(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	stubWatchers(t, true)
	s := newTestSupervisor(t)

	done, err := s.onExit(context.Background(), exitOf(s, 4242, installerJob))
	require.NoError(t, err)
	assert.True(t, done)
	assertReverted(t, l, experimentPath)
}

// TestSecondWatcherLeavesTheDaemonExitToARunningFirstWatcher is the other side: while W1 runs, W2
// waits for it to act on the experiment daemon's exit.
func TestSecondWatcherLeavesTheDaemonExitToARunningFirstWatcher(t *testing.T) {
	l, _, _ := runningExperiment(t)
	shortOverlap(t, 5*time.Second)
	stubWatchers(t, true)
	s := secondSupervisor(t)
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = experimentDeadline().MarkFailed()
	}()

	done, err := s.onExit(context.Background(), exitOf(s, 4242, installerJob))
	require.NoError(t, err)
	assert.False(t, done)
	for _, label := range agentJobs {
		assert.True(t, l.isLoaded(label+"-exp"), "W2 must leave %s to W1", label+"-exp")
	}
}

// TestSecondWatcherTakesTheDaemonExitOnceTheFirstIsGone pins that the experiment daemon's exit is
// W2's once W1 has been stopped.
func TestSecondWatcherTakesTheDaemonExitOnceTheFirstIsGone(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	stubWatchers(t, true)
	s := secondSupervisor(t)
	s.predecessorGone.Store(true)

	done, err := s.onExit(context.Background(), exitOf(s, 4242, installerJob))
	require.NoError(t, err)
	assert.True(t, done)
	assertReverted(t, l, experimentPath)
}

// TestSecondWatcherRevertsAnAgentFailure pins that every failure other than the experiment daemon's
// exit is W2's, W1 running or not.
func TestSecondWatcherRevertsAnAgentFailure(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	stubWatchers(t, true)
	s := secondSupervisor(t)

	done, err := s.onExit(context.Background(), exitOf(s, 4242, agentJob))
	require.NoError(t, err)
	assert.True(t, done)
	assertReverted(t, l, experimentPath)
}

// TestSupervisorHandsTheDaemonBackOnceTheDeadlineIsCleared pins the end of an experiment: the stop
// or promote hook clears the deadline once it has handed the Agent back, the supervisor sees the
// change, hands the installer daemon back and exits.
func TestSupervisorHandsTheDaemonBackOnceTheDeadlineIsCleared(t *testing.T) {
	l := stubLiveLaunchd(t, installerJob+"-exp")
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := secondSupervisor(t)
	s.watchState()
	t.Cleanup(s.release)
	require.NotNil(t, s.stateChanges, "the state directory should be watched")
	done := runSupervisor(context.Background(), s)

	require.NoError(t, experimentDeadline().Clear())
	require.NoError(t, waitFor(t, done))
	assert.True(t, l.isLoaded(installerJob), "the stable daemon should be back")
	assert.False(t, l.isLoaded(installerJob+"-exp"), "the experiment daemon should be gone")
}

// TestSupervisorHandsTheDaemonBackOnItsTick covers the same end with the directory unwatched: the
// next tick sees the deadline cleared and hands the daemon back.
func TestSupervisorHandsTheDaemonBackOnItsTick(t *testing.T) {
	original := deadlineTickInterval
	deadlineTickInterval = 5 * time.Millisecond
	t.Cleanup(func() { deadlineTickInterval = original })
	l := stubLiveLaunchd(t, installerJob+"-exp")
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := secondSupervisor(t)
	done := runSupervisor(context.Background(), s)
	require.NoError(t, experimentDeadline().Clear())

	require.NoError(t, waitFor(t, done))
	assert.True(t, l.isLoaded(installerJob), "the stable daemon should be back")
}

// TestFirstWatcherLeavesTheHandBackToTheSecond pins that W1, with W2 running, does not hand the
// daemon back when the deadline is cleared: W2 does.
func TestFirstWatcherLeavesTheHandBackToTheSecond(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubWatchers(t, true)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := newTestSupervisor(t)
	require.NoError(t, experimentDeadline().Clear())

	readFailures, revertingTicks := 0, 0
	done, err := s.onDeadline(context.Background(), false, &readFailures, &revertingTicks)
	require.NoError(t, err)
	assert.False(t, done)
	assert.Empty(t, launchctlCalls(*calls, "bootout"), "W1 must leave the hand-back to W2")
}

// TestStopLeavesTheHandBackToARunningWatcher pins the hook's side: with the experiment daemon and a
// watcher running, the stop hook launches no hand-back process; without a watcher, it does.
func TestStopLeavesTheHandBackToARunningWatcher(t *testing.T) {
	for name, tc := range map[string]struct {
		watcher bool
		want    int
	}{
		"watcher running": {true, 0},
		"no watcher":      {false, 1},
	} {
		t.Run(name, func(t *testing.T) {
			stubLaunchdWithExperimentDaemon(t, "", "")
			stubJobDir(t)
			stubDeadlinePath(t)
			stubWatchers(t, tc.watcher)
			handBackCalls := stubInstallerHandBack(t, nil)
			require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

			require.NoError(t, preStopConfigExperimentDatadogAgent(testHookContext(t)))
			assert.Equal(t, tc.want, *handBackCalls)
		})
	}
}

// TestSupervisorFinishesAStopWhoseCommandDied covers the mid-stop crash: the stop's hook marked the
// deadline as reverting, then its command -- or the experiment daemon running it -- died with the
// jobs half handed back. Once the lock is free and the marker still there, the supervisor finishes
// the restore, every job and the daemon with it, and keeps the experiment for Fleet to stop again.
func TestSupervisorFinishesAStopWhoseCommandDied(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	s := newTestSupervisor(t)
	require.NoError(t, experimentDeadline().MarkReverting())

	done, err := s.onFailingExit(context.Background(), "the experiment daemon exited")
	require.NoError(t, err)
	assert.True(t, done)
	assertReverted(t, l, experimentPath)
}

// TestSupervisorFinishesAPromoteWhoseCommandDied is the same for a promote that had already moved
// the configuration directories: it is finished as a promote.
func TestSupervisorFinishesAPromoteWhoseCommandDied(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	s := newTestSupervisor(t)
	require.NoError(t, configExperimentDirs.PromoteExperiment(context.Background()))
	require.NoError(t, experimentDeadline().MarkReverting())

	done, err := s.onFailingExit(context.Background(), "the experiment daemon exited")
	require.NoError(t, err)
	assert.True(t, done)
	assertNothingDeployed(t, l, experimentPath)
}

// TestSupervisorLeavesALiveStopAlone pins the other side: a stop whose hook is under way holds the
// lock and clears the marker before its command exits, so the jobs its hook makes exit are not
// reverted from here.
func TestSupervisorLeavesALiveStopAlone(t *testing.T) {
	l, _, _ := runningExperiment(t)
	original := installerLockPoll
	installerLockPoll = 20 * time.Millisecond
	t.Cleanup(func() { installerLockPoll = original })
	s := newTestSupervisor(t)
	require.NoError(t, experimentDeadline().MarkReverting())
	release := holdInstallerLock(t, configExperimentLockPath)
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = experimentDeadline().Clear()
		release()
	}()

	done, err := s.onFailingExit(context.Background(), "experiment job com.datadoghq.agent exited")
	require.NoError(t, err)
	assert.False(t, done, "the stop owns the outcome")
	for _, label := range agentJobs {
		assert.True(t, l.isLoaded(label+"-exp"), "the supervisor must not touch %s", label+"-exp")
	}
}

// TestSupervisorFinishesARestoreLeftRevertingOnItsTicks covers a stop that died with no job exit for
// the supervisor to act on: the reverting marker seen on two ticks in a row, with the lock free, is
// finished from here.
func TestSupervisorFinishesARestoreLeftRevertingOnItsTicks(t *testing.T) {
	original := deadlineTickInterval
	deadlineTickInterval = 5 * time.Millisecond
	t.Cleanup(func() { deadlineTickInterval = original })
	l, _, experimentPath := runningExperiment(t)
	s := newTestSupervisor(t)
	require.NoError(t, experimentDeadline().MarkReverting())

	require.NoError(t, waitFor(t, runSupervisor(context.Background(), s)))
	assertReverted(t, l, experimentPath)
}

// TestFirstWatcherHandsOverSupervisesAndHandsBack runs W1 through a whole experiment on its own, as
// when the experiment daemon launches no W2: it arms on the agent jobs, hands the installer daemon
// over once the start command's lock is free, supervises, and hands the daemon back once the stop's
// hook clears the deadline.
func TestFirstWatcherHandsOverSupervisesAndHandsBack(t *testing.T) {
	running := []string{installerJob}
	for _, label := range agentJobs {
		running = append(running, label+"-exp")
	}
	l := stubLiveLaunchd(t, running...)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	stubStopSurvivingWatchers(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	done := make(chan error, 1)
	go func() { done <- watchExperiment(context.Background()) }()
	require.Eventually(t, func() bool { return l.isLoaded(installerJob + "-exp") }, 5*time.Second, time.Millisecond,
		"W1 should hand the installer daemon over")

	// The stop's hook hands the Agent back and clears the deadline.
	require.NoError(t, experimentDeadline().Clear())
	require.NoError(t, waitFor(t, done))
	assert.True(t, l.isLoaded(installerJob), "the stable daemon should be back")
	assert.False(t, l.isLoaded(installerJob+"-exp"), "the experiment daemon should be gone")
}

// TestFirstWatcherCatchesACrashBeforeTheHandOver pins what W1 adds over a watcher launched after the
// hand-over: an agent job crashing while the start command still holds the lock is caught, and
// reverted once the lock is free, without the daemon ever being handed over.
func TestFirstWatcherCatchesACrashBeforeTheHandOver(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	// W1 runs while the stable daemon is still in place.
	l.mu.Lock()
	l.loaded["system/"+installerJob+"-exp"] = false
	l.loaded["system/"+installerJob] = true
	l.mu.Unlock()
	stubStopSurvivingWatchers(t)
	s := newTestSupervisor(t)
	t.Cleanup(s.release)
	release := holdInstallerLock(t, configExperimentLockPath)
	original := installerLockPoll
	installerLockPoll = 20 * time.Millisecond
	t.Cleanup(func() { installerLockPoll = original })
	require.NoError(t, s.lead(context.Background()))

	go func() {
		time.Sleep(100 * time.Millisecond)
		release()
	}()
	done, err := s.onFailingExit(context.Background(), "experiment job com.datadoghq.agent exited")
	require.NoError(t, err)
	assert.True(t, done)
	assertReverted(t, l, experimentPath)
	select {
	case result := <-s.handOver:
		disarmResult(t, result)
		assert.Nil(t, result.observer, "the daemon must not be handed over to a failed experiment")
	case <-time.After(5 * time.Second):
		t.Fatal("the hand-over should have given up")
	}
}

// TestStartSupervisorRelayLaunchesTheSecondWatcher pins the experiment daemon's job: it launches W2
// when an experiment is pending, and nothing otherwise.
func TestStartSupervisorRelayLaunchesTheSecondWatcher(t *testing.T) {
	for name, tc := range map[string]struct {
		setUp func(t *testing.T)
		want  int
	}{
		"pending":   {func(t *testing.T) { require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow)) }, 1},
		"reverting": {func(t *testing.T) { require.NoError(t, experimentDeadline().MarkReverting()) }, 0},
		"none":      {func(*testing.T) {}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			stubDeadlinePath(t)
			watcherCalls := stubConfigExperimentWatcher(t)
			tc.setUp(t)

			require.NoError(t, startSupervisorRelay(context.Background()))
			assert.Equal(t, tc.want, *watcherCalls)
		})
	}
}

// TestStartSupervisorRelayNeverFailsTheDaemonStart pins that a W2 that cannot be launched leaves the
// experiment daemon free to start: W1 goes on supervising alone.
func TestStartSupervisorRelayNeverFailsTheDaemonStart(t *testing.T) {
	stubDeadlinePath(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	original := launchConfigExperimentWatcher
	launchConfigExperimentWatcher = func(context.Context) error { return errors.New("injected failure") }
	t.Cleanup(func() { launchConfigExperimentWatcher = original })

	require.NoError(t, startSupervisorRelay(context.Background()))
}

// TestExperimentDaemonRunningIgnoresAnIdleJob pins how a watcher tells it is W2: the experiment
// daemon running, not merely loaded, as launchd leaves it after a reboot.
func TestExperimentDaemonRunningIgnoresAnIdleJob(t *testing.T) {
	l := stubLiveLaunchd(t, installerJob+"-exp")
	running, err := experimentDaemonRunning(context.Background())
	require.NoError(t, err)
	assert.True(t, running)

	l.mu.Lock()
	l.idle["system/"+installerJob+"-exp"] = true
	l.mu.Unlock()
	running, err = experimentDaemonRunning(context.Background())
	require.NoError(t, err)
	assert.False(t, running)
}
