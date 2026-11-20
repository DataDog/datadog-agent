// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listeningSupervisor is W1's state for the current deadline, listening on the supervisor socket.
func listeningSupervisor(t *testing.T) *experimentSupervisor {
	t.Helper()

	s := newTestSupervisor(t)
	listener, err := listenSupervisor()
	require.NoError(t, err)
	s.listener = listener
	t.Cleanup(s.release)
	return s
}

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

// TestSupervisorHandsOverToTheSecondWatcher pins the relay: once W2 reports it is armed for the
// same experiment, W1 acks, stops listening so W2 can take the socket, and exits.
func TestSupervisorHandsOverToTheSecondWatcher(t *testing.T) {
	stubDeadlinePath(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := listeningSupervisor(t)
	done := runSupervisor(context.Background(), s)

	answer, err := askSupervisor(supervisorMsgArmed+" "+s.token, time.Second)
	require.NoError(t, err)
	assert.Equal(t, supervisorReplyAck, answer)
	require.NoError(t, waitFor(t, done))
	assert.True(t, s.finished)
	_, err = os.Lstat(configExperimentSupervisorSocketPath)
	assert.True(t, os.IsNotExist(err), "W1 must stop listening before W2 takes the socket")
}

// TestSupervisorRefusesAHandshakeForAnotherExperiment pins that W1 keeps supervising when the
// handshake is for an experiment other than its own.
func TestSupervisorRefusesAHandshakeForAnotherExperiment(t *testing.T) {
	stubDeadlinePath(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := listeningSupervisor(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := runSupervisor(ctx, s)

	answer, err := askSupervisor(supervisorMsgArmed+" another-experiment", time.Second)
	require.NoError(t, err)
	assert.Equal(t, supervisorReplyNak, answer)
	select {
	case <-done:
		t.Fatal("W1 must keep supervising after refusing a handshake")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	require.NoError(t, waitFor(t, done))
}

// TestSupervisorKeepsSupervisingWhenTheAckIsLost covers a W2 that sends the handshake and goes away
// before reading the answer: W1 must not leave the experiment to it, and listens again.
func TestSupervisorKeepsSupervisingWhenTheAckIsLost(t *testing.T) {
	stubDeadlinePath(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := listeningSupervisor(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := runSupervisor(ctx, s)

	conn, err := net.Dial("unix", configExperimentSupervisorSocketPath)
	require.NoError(t, err)
	_, err = conn.Write([]byte(supervisorMsgArmed + " " + s.token + "\n"))
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	select {
	case <-done:
		t.Fatal("W1 must not exit on a handshake whose ack was lost")
	case <-time.After(300 * time.Millisecond):
	}
	assert.True(t, supervisorListening(), "W1 should listen again")
	cancel()
	require.NoError(t, waitFor(t, done))
}

// TestSupervisorRefusesAnotherUser pins that only the owner of the experiment state may talk to the
// supervisor.
func TestSupervisorRefusesAnotherUser(t *testing.T) {
	stubDeadlinePath(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := listeningSupervisor(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := runSupervisor(ctx, s)
	configExperimentStateOwner = os.Getuid() + 1

	_, err := askSupervisor(supervisorMsgEnd, time.Second)
	require.Error(t, err)
	assert.False(t, errors.Is(err, errNoSupervisor), "the supervisor listens, it refuses")
	cancel()
	require.NoError(t, waitFor(t, done))
	assert.False(t, s.finished, "a refused request must not end supervision")
}

// TestSecondWatcherTakesOverWhenNoneSupervises covers W1 gone before the handshake: W2, armed on
// every experiment job, finds no supervisor and becomes it.
func TestSecondWatcherTakesOverWhenNoneSupervises(t *testing.T) {
	runningExperiment(t)
	s := newTestSupervisor(t)
	t.Cleanup(s.release)

	require.NoError(t, s.takeOver(context.Background()))
	assert.False(t, s.finished)
	require.NotNil(t, s.listener, "W2 should listen once it supervises")
	assert.NotNil(t, s.agentObserver, "W2 should be armed on every experiment job")
	assert.NotEmpty(t, s.pids)
}

// TestSecondWatcherLeavesTheExperimentToARefusingSupervisor covers a supervisor that refuses the
// handshake: W2 exits and leaves the experiment to it.
func TestSecondWatcherLeavesTheExperimentToARefusingSupervisor(t *testing.T) {
	runningExperiment(t)
	other, err := listenSupervisor()
	require.NoError(t, err)
	t.Cleanup(other.close)
	go func() {
		for request := range other.requests {
			_ = request.reply(supervisorReplyNak)
		}
	}()
	s := newTestSupervisor(t)
	t.Cleanup(s.release)

	require.NoError(t, s.takeOver(context.Background()))
	assert.True(t, s.finished, "W2 should leave the experiment to the supervisor that refused it")
	assert.Nil(t, s.listener)
}

// TestSupervisorHandsTheDaemonBackOnEnd pins the end of an experiment: the stop or promote hook
// sends "end" once it has handed the Agent back, and the supervisor hands the installer daemon back
// and exits.
func TestSupervisorHandsTheDaemonBackOnEnd(t *testing.T) {
	l := stubLiveLaunchd(t, installerJob+"-exp")
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := listeningSupervisor(t)
	done := runSupervisor(context.Background(), s)
	// What the hook does before it sends "end".
	require.NoError(t, experimentDeadline().Clear())

	require.NoError(t, notifySupervisorEnd())
	require.NoError(t, waitFor(t, done))
	assert.True(t, l.isLoaded(installerJob), "the stable daemon should be back")
	assert.False(t, l.isLoaded(installerJob+"-exp"), "the experiment daemon should be gone")
}

// TestSupervisorHandsTheDaemonBackWhenEndNeverCame covers a stop whose "end" did not reach the
// supervisor: its next tick sees the deadline cleared and hands the daemon back.
func TestSupervisorHandsTheDaemonBackWhenEndNeverCame(t *testing.T) {
	original := deadlineTickInterval
	deadlineTickInterval = 5 * time.Millisecond
	t.Cleanup(func() { deadlineTickInterval = original })
	l := stubLiveLaunchd(t, installerJob+"-exp")
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	s := newTestSupervisor(t)
	done := runSupervisor(context.Background(), s)
	require.NoError(t, experimentDeadline().Clear())

	require.NoError(t, waitFor(t, done))
	assert.True(t, l.isLoaded(installerJob), "the stable daemon should be back")
}

// TestStopSendsEndToTheSupervisor pins the hook's side: with the experiment daemon running and a
// supervisor listening, the stop hook sends "end" and launches no hand-back process.
func TestStopSendsEndToTheSupervisor(t *testing.T) {
	stubLaunchdWithExperimentDaemon(t, "", "")
	stubJobDir(t)
	stubDeadlinePath(t)
	handBackCalls := stubInstallerHandBack(t, nil)
	supervisor, err := listenSupervisor()
	require.NoError(t, err)
	t.Cleanup(supervisor.close)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	require.NoError(t, preStopConfigExperimentDatadogAgent(testHookContext(t)))

	select {
	case request := <-supervisor.requests:
		assert.Equal(t, supervisorMsgEnd, request.kind)
	case <-time.After(5 * time.Second):
		t.Fatal("the supervisor never got the end of the experiment")
	}
	assert.Zero(t, *handBackCalls, "no hand-back process is needed when a supervisor takes the end")
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

// TestFirstWatcherHandsOverSupervisesAndHandsBack runs W1 through a whole experiment: it arms on the
// agent jobs, hands the installer daemon over once the start command's lock is free, supervises, and
// hands the daemon back on the stop's "end".
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
	require.Eventually(t, func() bool {
		_, err := os.Lstat(configExperimentSupervisorSocketPath)
		return err == nil
	}, 5*time.Second, time.Millisecond, "W1 should listen on the supervisor socket")

	// The stop's hook hands the Agent back, clears the deadline and sends "end".
	require.NoError(t, experimentDeadline().Clear())
	require.NoError(t, notifySupervisorEnd())
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
		assert.False(t, result.observer != nil, "the daemon must not be handed over to a failed experiment")
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
