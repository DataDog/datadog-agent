// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// liveLaunchd is a launchctl stand-in that tracks which labels are loaded: bootout unloads a label,
// bootstrap loads one (a stable job starts at load), kickstart starts one. print reports a running
// label under pids[target], this test's own pid by default, which never exits during the test.
type liveLaunchd struct {
	mu     sync.Mutex
	loaded map[string]bool
	// idle makes print report a loaded target as not running, as launchd leaves a job with no
	// RunAtLoad whose definition it loaded at boot.
	idle map[string]bool
	// exited makes print report a target as not running, with this exit code recorded.
	exited map[string]int
	// exitOnStart makes a kickstart of a target leave it exited with this code, as a job that exits
	// at once.
	exitOnStart map[string]int
	// stuck makes a kickstart of a target leave it not running, as a job that never starts.
	stuck map[string]bool
	// failBootstrap makes the next bootstraps of a target fail, as many as its count.
	failBootstrap map[string]int
	// disabled makes bootstrap refuse a target until it is enabled, as launchd does a job with a
	// disabled override.
	disabled map[string]bool
	pids     map[string]int
	calls    [][]string
}

func stubLiveLaunchd(t *testing.T, loaded ...string) *liveLaunchd {
	t.Helper()

	l := &liveLaunchd{loaded: map[string]bool{}, idle: map[string]bool{}, exited: map[string]int{}, exitOnStart: map[string]int{}, stuck: map[string]bool{}, failBootstrap: map[string]int{}, disabled: map[string]bool{}, pids: map[string]int{}}
	for _, label := range loaded {
		l.loaded["system/"+label] = true
	}
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.BootoutSettlePollInterval = time.Millisecond
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.calls = append(l.calls, args)
			target := args[len(args)-1]
			switch args[0] {
			case "print":
				if !l.loaded[target] {
					return []byte(notLoadedOutput), exitError(113)
				}
				if code, ok := l.exited[target]; ok {
					return fmt.Appendf(nil, "%s = {\n\tstate = not running\n\tlast exit code = %d\n}\n", target, code), nil
				}
				if l.idle[target] {
					return []byte(target + " = {\n\tstate = not running\n}\n"), nil
				}
				pid := os.Getpid()
				if p, ok := l.pids[target]; ok {
					pid = p
				}
				return fmt.Appendf(nil, "%s = {\n\tpid = %d\n}\n", target, pid), nil
			case "bootout":
				l.loaded[target] = false
				delete(l.idle, target)
				delete(l.exited, target)
			case "bootstrap":
				label := "system/" + strings.TrimSuffix(filepath.Base(target), ".plist")
				if l.disabled[label] {
					return []byte("Bootstrap failed: 119: Service is disabled"), exitError(119)
				}
				if l.failBootstrap[label] > 0 {
					l.failBootstrap[label]--
					return []byte("Bootstrap failed: 5: Input/output error"), exitError(5)
				}
				l.loaded[label] = true
				// A job with no RunAtLoad loads without running.
				if strings.HasSuffix(label, "-exp") || label == "system/"+unitSupervisorJob {
					l.idle[label] = true
				}
			case "enable":
				delete(l.disabled, target)
			case "kickstart":
				l.loaded[target] = true
				if !l.stuck[target] {
					delete(l.idle, target)
				}
				if code, ok := l.exitOnStart[target]; ok {
					l.exited[target] = code
				}
			}
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	return l
}

func (l *liveLaunchd) isRunning(label string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded["system/"+label] && !l.idle["system/"+label]
}

func (l *liveLaunchd) count(verb, target string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, args := range l.calls {
		if args[0] == verb && args[len(args)-1] == target {
			n++
		}
	}
	return n
}

// countBootstraps reports how many times label's job definition was bootstrapped.
func (l *liveLaunchd) countBootstraps(label string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, args := range l.calls {
		if args[0] == "bootstrap" && filepath.Base(args[len(args)-1]) == label+".plist" {
			n++
		}
	}
	return n
}

// stubUnitsState points the supervisor's state files at a temporary directory beside the deadline,
// and shortens its waits.
func stubUnitsState(t *testing.T) {
	t.Helper()

	dir := filepath.Dir(configExperimentDeadlinePath)
	originalTarget, originalStatus := unitsTargetPath, unitsStatusPath
	unitsTargetPath, unitsStatusPath = filepath.Join(dir, "units-target"), filepath.Join(dir, "units-status")
	originalPoll, originalTick, originalRetry := unitsPollInterval, unitsTickInterval, unitsRetryDelay
	unitsPollInterval, unitsTickInterval, unitsRetryDelay = 5*time.Millisecond, 20*time.Millisecond, 10*time.Millisecond
	originalLockPoll := installerLockPoll
	installerLockPoll = 20 * time.Millisecond
	t.Cleanup(func() {
		unitsTargetPath, unitsStatusPath = originalTarget, originalStatus
		unitsPollInterval, unitsTickInterval, unitsRetryDelay = originalPoll, originalTick, originalRetry
		installerLockPoll = originalLockPoll
	})
}

// supervisedHost lays out a host for the supervisor: the stable jobs running, the deadline,
// configuration and lock stubbed, and an experiment configuration deployed.
func supervisedHost(t *testing.T) *liveLaunchd {
	t.Helper()

	l := stubLiveLaunchd(t, append([]string{procmgrJob}, configExperimentJobs...)...)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubUnitsState(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	return l
}

func request(t *testing.T, target string) unitsRequest {
	t.Helper()

	r := unitsRequest{target: target, nonce: strconv.FormatInt(time.Now().UnixNano(), 10)}
	require.NoError(t, writeUnitsRequest(r))
	return r
}

func waitForStatus(t *testing.T, r unitsRequest, phase string) {
	t.Helper()

	require.Eventually(t, func() bool {
		status, err := readUnitsStatus()
		return err == nil && status.nonce == r.nonce && status.phase == phase
	}, 10*time.Second, 5*time.Millisecond, "the supervisor should report %s for %s", phase, r.target)
}

// TestSupervisorStartsAndStopsAnExperiment runs the supervisor through an experiment: the start
// moves the Agent jobs to their experiment variants first and the installer daemon last, and the
// stop brings every job back, the installer daemon last, before the supervisor exits.
func TestSupervisorStartsAndStopsAnExperiment(t *testing.T) {
	l := supervisedHost(t)
	start := request(t, unitsTargetExperiment)
	done := make(chan error, 1)
	go func() { done <- superviseUnits(context.Background()) }()

	waitForStatus(t, start, unitsPhaseDone)
	for _, label := range agentJobs {
		assert.True(t, l.isRunning(label+"-exp"), "%s should run", label+"-exp")
		assert.False(t, l.isRunning(label), "%s should be stopped", label)
	}
	assert.True(t, l.isRunning(installerJob+"-exp"), "the experiment daemon should run")
	assert.False(t, l.isRunning(installerJob), "the stable daemon should be stopped")
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	assert.NotEmpty(t, token, "the experiment should be bounded")

	stop := request(t, unitsTargetStable)
	waitForStatus(t, stop, unitsPhaseDone)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the supervisor should exit once every job is back on stable")
	}
	for _, label := range configExperimentJobs {
		assert.True(t, l.isRunning(label), "%s should be back", label)
		assert.False(t, l.isRunning(label+"-exp"), "%s should be stopped", label+"-exp")
	}
	token, err = experimentDeadline().Read()
	require.NoError(t, err)
	assert.Empty(t, token, "the stop should clear the deadline")
}

// TestSupervisorFailsTheExperimentAsOnFailure pins OnFailure: every job goes back to stable, the
// installer daemon last, the experiment configuration stays deployed for Fleet's stop, and the
// target is left so that a relaunched supervisor does not start the experiment again.
func TestSupervisorFailsTheExperimentAsOnFailure(t *testing.T) {
	l := supervisedHost(t)
	r := request(t, unitsTargetExperiment)
	require.NoError(t, configExperiment{jobs: configExperimentJobSet()}.Start(context.Background()))
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	require.NoError(t, failExperiment(context.Background(), r, "the experiment Agent exited"))
	for _, label := range configExperimentJobs {
		assert.True(t, l.isRunning(label), "%s should be back", label)
		assert.False(t, l.isRunning(label+"-exp"), "%s should be stopped", label+"-exp")
	}
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, configExperimentDirs), "the configuration is kept for Fleet's stop")
	latest, err := readUnitsRequest()
	require.NoError(t, err)
	assert.Equal(t, unitsTargetFailed, latest.target)
	require.NoError(t, superviseUnits(context.Background()), "a relaunched supervisor has nothing to do")
	assert.False(t, l.isRunning(agentJob+"-exp"), "the failed experiment must not start again")
}

// TestSupervisorRetriesAFailedRestore pins that a failed experiment whose stable jobs could not all
// be restored is restored again on the next turn, rather than looked up as a running experiment.
func TestSupervisorRetriesAFailedRestore(t *testing.T) {
	l := supervisedHost(t)
	r := request(t, unitsTargetExperiment)
	l.mu.Lock()
	l.pids["system/"+agentJob+"-exp"] = exitingProcess(t, 1)
	// One whole restore fails on the stable installer daemon; the next one gets it back.
	l.failBootstrap["system/"+installerJob] = configExperimentRestoreRetries
	l.mu.Unlock()
	runSupervisor(t)

	waitForStatus(t, r, unitsPhaseFailed)
	for _, label := range configExperimentJobs {
		assert.True(t, l.isRunning(label), "%s should be back", label)
		assert.False(t, l.isRunning(label+"-exp"), "%s should be stopped", label+"-exp")
	}
	latest, err := readUnitsRequest()
	require.NoError(t, err)
	assert.Equal(t, unitsTargetFailed, latest.target)
}

// TestSupervisorResumesARestoreAfterARelaunch pins that a supervisor relaunched while it was
// restoring the stable jobs of a failed experiment finishes that restore.
func TestSupervisorResumesARestoreAfterARelaunch(t *testing.T) {
	l := supervisedHost(t)
	r := request(t, unitsTargetExperiment)
	require.NoError(t, configExperiment{jobs: configExperimentJobSet()}.Start(context.Background()))
	writeUnitsStatus(r.nonce, unitsPhaseFailing, "the experiment Agent exited (status 1)")

	require.NoError(t, superviseUnits(context.Background()))
	for _, label := range configExperimentJobs {
		assert.True(t, l.isRunning(label), "%s should be back", label)
		assert.False(t, l.isRunning(label+"-exp"), "%s should be stopped", label+"-exp")
	}
	status, err := readUnitsStatus()
	require.NoError(t, err)
	assert.Equal(t, unitsStatus{nonce: r.nonce, phase: unitsPhaseFailed, message: "the experiment Agent exited (status 1)"}, status)
}

// exitingProcess starts a process that exits with code after a short while, for a job whose exit
// the supervisor watches.
func exitingProcess(t *testing.T, code int) int {
	t.Helper()

	p := exec.Command("/bin/sh", "-c", fmt.Sprintf("sleep 1; exit %d", code))
	require.NoError(t, p.Start())
	go func() { _ = p.Wait() }()
	return p.Process.Pid
}

// runSupervisor runs the supervisor until the test ends.
func runSupervisor(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- superviseUnits(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the supervisor should stop with its context")
		}
	})
}

// assertExperimentFailed checks that every job is back on stable and the experiment is recorded as
// failed, its configuration kept for Fleet's stop.
func assertExperimentFailed(t *testing.T, l *liveLaunchd, r unitsRequest) {
	t.Helper()

	waitForStatus(t, r, unitsPhaseFailed)
	for _, label := range configExperimentJobs {
		assert.True(t, l.isRunning(label), "%s should be back", label)
		assert.False(t, l.isRunning(label+"-exp"), "%s should be stopped", label+"-exp")
	}
	latest, err := readUnitsRequest()
	require.NoError(t, err)
	assert.Equal(t, unitsTargetFailed, latest.target)
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, configExperimentDirs), "the configuration is kept for Fleet's stop")
}

// TestSupervisorFailsTheExperimentOnAnyServiceCrash pins the failure rule (exitFailsExperiment): a
// failed exit of any job, and any exit of the Agent or the installer daemon, fails the experiment.
// Nothing is restarted.
func TestSupervisorFailsTheExperimentOnAnyServiceCrash(t *testing.T) {
	for name, tc := range map[string]struct {
		job  string
		code int
	}{
		"child crash":                 {"com.datadoghq.sysprobe-exp", 3},
		"agent clean exit":            {agentJob + "-exp", 0},
		"installer daemon crash":      {installerJob + "-exp", 1},
		"installer daemon clean exit": {installerJob + "-exp", 0},
	} {
		t.Run(name, func(t *testing.T) {
			l := supervisedHost(t)
			r := request(t, unitsTargetExperiment)
			l.mu.Lock()
			l.pids["system/"+tc.job] = exitingProcess(t, tc.code)
			l.mu.Unlock()
			runSupervisor(t)

			assertExperimentFailed(t, l, r)
			assert.Equal(t, 1, l.count("kickstart", "system/"+tc.job), "%s must not be restarted", tc.job)
		})
	}
}

// TestWriteUnitsStatusFitsItsFile pins that a failure message, however long, leaves a status the
// supervisor and the hooks can still read: the status file is read with the deadline's size limit.
func TestWriteUnitsStatusFitsItsFile(t *testing.T) {
	stubDeadlinePath(t)
	stubUnitsState(t)
	const nonce = "1234567890123456789"

	writeUnitsStatus(nonce, unitsPhaseFailed, "could not restore com.datadoghq.agent")
	status, err := readUnitsStatus()
	require.NoError(t, err)
	assert.Equal(t, unitsStatus{nonce: nonce, phase: unitsPhaseFailed, message: "could not restore com.datadoghq.agent"}, status, "a short message is kept whole")

	// The cut falls inside the two-byte "é": it moves back to the start of the character.
	prefix := nonce + " " + unitsPhaseFailed + " "
	cutInMessage := launchd.MaxDeadlineSize - len(unitsStatusEllipsis) - len(prefix)
	long := strings.Repeat("a", cutInMessage-1) + "é" + strings.Repeat("b", 800)
	writeUnitsStatus(nonce, unitsPhaseFailed, long)

	raw, err := os.ReadFile(unitsStatusPath)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(raw), launchd.MaxDeadlineSize)
	status, err = readUnitsStatus()
	require.NoError(t, err)
	assert.Equal(t, nonce, status.nonce)
	assert.Equal(t, unitsPhaseFailed, status.phase)
	assert.Equal(t, strings.Repeat("a", cutInMessage-1)+unitsStatusEllipsis, status.message)
}

// TestSupervisorLeavesACleanChildExitDown pins that a job other than the Agent and the installer
// daemon may exit cleanly, as it does when its feature is off, without failing the experiment.
func TestSupervisorLeavesACleanChildExitDown(t *testing.T) {
	for name, exitBeforeArming := range map[string]bool{"while watched": false, "before it is watched": true} {
		t.Run(name, func(t *testing.T) {
			l := supervisedHost(t)
			r := request(t, unitsTargetExperiment)
			l.mu.Lock()
			if exitBeforeArming {
				l.exitOnStart["system/com.datadoghq.data-plane-exp"] = 0
			} else {
				l.pids["system/com.datadoghq.data-plane-exp"] = exitingProcess(t, 0)
			}
			l.mu.Unlock()
			handled := make(chan childExit, 1)
			if !exitBeforeArming {
				childExitHandled = func(c childExit) { handled <- c }
				t.Cleanup(func() { childExitHandled = func(childExit) {} })
			}
			runSupervisor(t)

			waitForStatus(t, r, unitsPhaseDone)
			if exitBeforeArming {
				// Nothing to wait on: the job was never watched. Nothing may fail it meanwhile.
				assert.Never(t, func() bool {
					s, err := readUnitsStatus()
					return err != nil || s.phase != unitsPhaseDone
				}, 500*time.Millisecond, 50*time.Millisecond, "a clean child exit must not fail the experiment")
			} else {
				select {
				case c := <-handled:
					assert.Equal(t, "com.datadoghq.data-plane", c.label)
				case <-time.After(10 * time.Second):
					t.Fatal("the supervisor never saw the child exit")
				}
			}
			status, err := readUnitsStatus()
			require.NoError(t, err)
			assert.Equal(t, unitsPhaseDone, status.phase, "a clean child exit must not fail the experiment")
			assert.True(t, l.isRunning(agentJob+"-exp"))
			assert.Equal(t, 1, l.count("kickstart", "system/com.datadoghq.data-plane-exp"), "the child must stay down")
		})
	}
}

// TestSupervisorFailsTheExperimentOnAnExitBeforeArming pins that an exit launchd recorded before the
// supervisor could watch the job fails the experiment like one it watched, as does a job the
// supervisor cannot find running.
func TestSupervisorFailsTheExperimentOnAnExitBeforeArming(t *testing.T) {
	for name, tc := range map[string]struct {
		job   string
		code  int
		stuck bool
	}{
		"child exited non-zero": {job: "com.datadoghq.sysprobe-exp", code: 1},
		"daemon exited cleanly": {job: installerJob + "-exp", code: 0},
		"child never started":   {job: "com.datadoghq.data-plane-exp", stuck: true},
	} {
		t.Run(name, func(t *testing.T) {
			l := supervisedHost(t)
			original := startupPollTimeout
			startupPollTimeout = 100 * time.Millisecond
			t.Cleanup(func() { startupPollTimeout = original })
			r := request(t, unitsTargetExperiment)
			l.mu.Lock()
			if tc.stuck {
				l.stuck["system/"+tc.job] = true
			} else {
				l.exitOnStart["system/"+tc.job] = tc.code
			}
			l.mu.Unlock()
			runSupervisor(t)

			assertExperimentFailed(t, l, r)
		})
	}
}

// TestSupervisorRelaunchSeesAChildThatDiedMeanwhile pins that a supervisor relaunched after a crash
// fails the experiment when a child exited while nothing watched it.
func TestSupervisorRelaunchSeesAChildThatDiedMeanwhile(t *testing.T) {
	l := supervisedHost(t)
	r := request(t, unitsTargetExperiment)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- superviseUnits(ctx) }()
	waitForStatus(t, r, unitsPhaseDone)
	cancel()
	require.NoError(t, <-done)

	l.mu.Lock()
	l.exited["system/com.datadoghq.data-plane-exp"] = 2
	l.mu.Unlock()
	runSupervisor(t)

	assertExperimentFailed(t, l, r)
}

// TestSuperviseUnitsNeverExitsNonZero pins that the supervisor keeps retrying rather than exiting
// with an error, which launchd would not relaunch: a target it cannot read and a panic in a step
// are both retried until it is stopped.
func TestSuperviseUnitsNeverExitsNonZero(t *testing.T) {
	t.Run("unreadable target", func(t *testing.T) {
		stubDeadlinePath(t)
		stubUnitsState(t)
		require.NoError(t, os.Mkdir(unitsTargetPath, 0o700))
		assertSupervisorKeepsRunning(t)
	})
	t.Run("panic", func(t *testing.T) {
		l := supervisedHost(t)
		request(t, unitsTargetStable)
		live := launchdClient
		var panicked bool
		launchdClient = func() *launchd.Client {
			if !panicked {
				panicked = true
				panic("injected panic")
			}
			return live()
		}
		t.Cleanup(func() { launchdClient = live })

		require.NoError(t, superviseUnits(context.Background()))
		assert.True(t, panicked)
		assert.True(t, l.isRunning(installerJob), "the retried step should complete")
	})
}

func assertSupervisorKeepsRunning(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- superviseUnits(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("the supervisor exited (%v) instead of retrying", err)
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the supervisor should stop with its context")
	}
}

// TestStopAfterARebootLeavesTheStableJobsRunning pins the stop for an experiment a reboot
// interrupted: launchd already started the stable jobs, so none is restarted, and the experiment
// daemon launchd loaded at boot without starting is unloaded.
func TestStopAfterARebootLeavesTheStableJobsRunning(t *testing.T) {
	l := supervisedHost(t)
	l.mu.Lock()
	l.loaded["system/"+installerJob+"-exp"] = true
	l.idle["system/"+installerJob+"-exp"] = true
	l.calls = nil
	l.mu.Unlock()
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	r := request(t, unitsTargetStable)

	require.NoError(t, superviseUnits(context.Background()))
	waitForStatus(t, r, unitsPhaseDone)
	for _, label := range agentJobs {
		assert.Zero(t, l.count("bootout", "system/"+label), "%s must not be restarted", label)
	}
	assert.False(t, l.isRunning(installerJob+"-exp"))
	assert.True(t, l.isRunning(installerJob), "the stable daemon must stay")
}

// TestStopStartsTheStableDaemonWhenTheExperimentDaemonDied pins that a stop never leaves the host
// without an installer daemon: an experiment daemon that died during the stop is unloaded, and the
// stable daemon started.
func TestStopStartsTheStableDaemonWhenTheExperimentDaemonDied(t *testing.T) {
	l := supervisedHost(t)
	l.mu.Lock()
	l.loaded["system/"+installerJob] = false
	l.loaded["system/"+installerJob+"-exp"] = true
	l.idle["system/"+installerJob+"-exp"] = true
	l.mu.Unlock()
	r := request(t, unitsTargetStable)

	require.NoError(t, superviseUnits(context.Background()))
	waitForStatus(t, r, unitsPhaseDone)
	assert.True(t, l.isRunning(installerJob), "the stable daemon should be started")
	assert.False(t, l.isRunning(installerJob+"-exp"))
}

// TestPromoteRestartsTheStableJobs pins that a promote, unlike a stop, restarts every stable job so
// it reads the promoted configuration.
func TestPromoteRestartsTheStableJobs(t *testing.T) {
	l := supervisedHost(t)
	l.mu.Lock()
	l.calls = nil
	l.mu.Unlock()
	request(t, unitsTargetRestartStable)

	require.NoError(t, superviseUnits(context.Background()))
	for _, label := range agentJobs {
		assert.Positive(t, l.count("bootout", "system/"+label), "%s should be restarted", label)
	}
}

// TestPromoteRetriesTheDaemonStepWithoutRestartingTheAgentJobs pins that a promote whose installer
// daemon step fails is retried from that step: the stable Agent jobs and the process manager are
// restarted once, not again on every retry.
func TestPromoteRetriesTheDaemonStepWithoutRestartingTheAgentJobs(t *testing.T) {
	l := supervisedHost(t)
	require.NoError(t, configExperiment{jobs: configExperimentJobSet()}.Start(context.Background()))
	l.mu.Lock()
	l.calls = nil
	// Two passes fail to start the stable installer daemon; the third gets it.
	l.failBootstrap["system/"+installerJob] = 2
	l.mu.Unlock()
	r := request(t, unitsTargetRestartStable)

	require.NoError(t, superviseUnits(context.Background()))
	status, err := readUnitsStatus()
	require.NoError(t, err)
	assert.Equal(t, unitsStatus{nonce: r.nonce, phase: unitsPhaseDone}, status)
	assert.True(t, l.isRunning(installerJob), "the stable daemon should be back")
	for _, label := range agentJobs {
		assert.Equal(t, 1, l.countBootstraps(label), "%s should be restarted once", label)
	}
	assert.Equal(t, 1, l.count("kickstart", "system/"+procmgrJob), "the process manager should be restarted once")
	assert.Zero(t, l.count("kickstart", "system/"+installerJob), "the swap already started the stable daemon on the promoted configuration")
}

// TestPromoteRestartsARunningStableDaemon pins that a promote with no experiment daemon to swap,
// after the experiment failed on the host or a reboot, restarts the stable daemon that runs, so it
// reads the promoted configuration.
func TestPromoteRestartsARunningStableDaemon(t *testing.T) {
	l := supervisedHost(t)
	l.mu.Lock()
	l.calls = nil
	l.mu.Unlock()
	r := request(t, unitsTargetRestartStable)

	require.NoError(t, superviseUnits(context.Background()))
	status, err := readUnitsStatus()
	require.NoError(t, err)
	assert.Equal(t, unitsStatus{nonce: r.nonce, phase: unitsPhaseDone}, status)
	assert.Equal(t, 1, l.count("kickstart", "system/"+installerJob), "the stable daemon should be restarted once")
	assert.True(t, l.isRunning(installerJob))
}

// TestStopKeepsRestoringAStableJobLaunchdRefuses pins that a stop whose stable Agent job launchd
// refuses through every attempt keeps the supervisor restoring it, reported as restoring so the
// hook fails, until it is back, rather than leaving the job on neither variant.
func TestStopKeepsRestoringAStableJobLaunchdRefuses(t *testing.T) {
	l := supervisedHost(t)
	require.NoError(t, configExperiment{jobs: configExperimentJobSet()}.Start(context.Background()))
	l.mu.Lock()
	l.failBootstrap["system/"+agentJob] = configExperimentRestoreRetries + 1
	l.mu.Unlock()
	r := request(t, unitsTargetStable)
	runSupervisor(t)

	waitForStatus(t, r, unitsPhaseRestoring)
	waitForStatus(t, r, unitsPhaseDone)
	for _, label := range configExperimentJobs {
		assert.True(t, l.isRunning(label), "%s should be back", label)
		assert.False(t, l.isRunning(label+"-exp"), "%s should be stopped", label+"-exp")
	}
}

// TestFailedStartKeepsRestoringItsRollback pins that a start whose rollback cannot restore a stable
// Agent job through every attempt keeps the supervisor restoring it until it is back, and only then
// reports the start as failed.
func TestFailedStartKeepsRestoringItsRollback(t *testing.T) {
	l := supervisedHost(t)
	l.mu.Lock()
	l.failBootstrap["system/"+agentJob+"-exp"] = 1
	l.failBootstrap["system/"+agentJob] = configExperimentRestoreRetries
	l.mu.Unlock()
	r := request(t, unitsTargetExperiment)
	runSupervisor(t)

	waitForStatus(t, r, unitsPhaseRestoring)
	waitForStatus(t, r, unitsPhaseFailed)
	for _, label := range agentJobs {
		assert.True(t, l.isRunning(label), "%s should be back", label)
		assert.False(t, l.isRunning(label+"-exp"), "%s should be stopped", label+"-exp")
	}
	assert.True(t, l.isRunning(installerJob), "the stable daemon was never replaced")
}

// TestRequestUnitsWaitsForTheAgentJobs pins the hooks' side: a request returns once the supervisor
// reports the Agent jobs on the target, and fails with the supervisor's reason.
func TestRequestUnitsWaitsForTheAgentJobs(t *testing.T) {
	for name, tc := range map[string]struct {
		phase, message string
		wantErr        bool
	}{
		"applied":   {unitsPhaseAgents, "", false},
		"failed":    {unitsPhaseFailed, "injected failure", true},
		"restoring": {unitsPhaseRestoring, "injected failure", true},
	} {
		t.Run(name, func(t *testing.T) {
			stubDeadlinePath(t)
			stubUnitsState(t)
			stubLiveLaunchd(t, unitSupervisorJob)
			original := ensureUnitSupervisor
			ensureUnitSupervisor = func(context.Context) error {
				go func() {
					r, err := readUnitsRequest()
					if err == nil {
						writeUnitsStatus(r.nonce, tc.phase, tc.message)
					}
				}()
				return nil
			}
			t.Cleanup(func() { ensureUnitSupervisor = original })

			err := requestUnits(context.Background(), unitsTargetExperiment)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "injected failure")
				return
			}
			require.NoError(t, err)
		})
	}
}

// stubRequestUnits records the targets the hooks ask the supervisor for.
func stubRequestUnits(t *testing.T, err error) *[]string {
	t.Helper()

	var targets []string
	original := requestUnits
	requestUnits = func(_ context.Context, target string) error {
		targets = append(targets, target)
		return err
	}
	t.Cleanup(func() { requestUnits = original })
	return &targets
}

// TestConfigExperimentHooksMirrorLinux pins the darwin hooks against their Linux counterparts
// (datadog_agent_linux.go): start = write then start the experiment, stop = stop then remove it,
// promote = restart the stable jobs.
func TestConfigExperimentHooksMirrorLinux(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	targets := stubRequestUnits(t, nil)

	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	for _, label := range configExperimentJobs {
		assert.FileExists(t, filepath.Join(dir, label+"-exp.plist"), "the start should write %s", label+"-exp")
	}
	require.NoError(t, preStopConfigExperimentDatadogAgent(testHookContext(t)))
	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(dir, label+"-exp.plist"), "the stop should remove %s", label+"-exp")
	}
	require.NoError(t, postPromoteConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, []string{unitsTargetExperiment, unitsTargetStable, unitsTargetRestartStable}, *targets)
}

// TestFailedStartDiscardsTheConfig pins that a start the supervisor could not apply leaves no
// experiment reported: the installer does not discard what it published when a hook fails.
func TestFailedStartDiscardsTheConfig(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubUnitsState(t)
	stubEnsureUnitSupervisor(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	stubRequestUnits(t, errors.New("injected failure"))

	require.Error(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Empty(t, experimentDeploymentID(t, dirs))
}

// TestEnsureUnitSupervisorWritesTheEmbeddedDefinition pins that the supervisor job runs the
// definition rendered from its template, and is started on every request.
func TestEnsureUnitSupervisorWritesTheEmbeddedDefinition(t *testing.T) {
	l := stubLiveLaunchd(t)
	dir := stubJobDir(t)

	require.NoError(t, ensureUnitSupervisor(context.Background()))
	written, err := os.ReadFile(filepath.Join(dir, unitSupervisorJob+".plist"))
	require.NoError(t, err)
	assert.Equal(t, embedded.GetLaunchdUnitSupervisorJob(), written)
	assert.Contains(t, string(written), "<string>"+superviseUnitsCommand+"</string>")
	assert.Equal(t, 1, l.count("bootstrap", filepath.Join(dir, unitSupervisorJob+".plist")))
	assert.Equal(t, 1, l.count("kickstart", "system/"+unitSupervisorJob))

	require.NoError(t, ensureUnitSupervisor(context.Background()))
	assert.Equal(t, 1, l.count("bootstrap", filepath.Join(dir, unitSupervisorJob+".plist")), "an unchanged loaded definition is not reloaded")
	assert.Equal(t, 2, l.count("kickstart", "system/"+unitSupervisorJob))
	assert.Equal(t, 1, l.count("enable", "system/"+unitSupervisorJob), "a loaded supervisor is not enabled again")
}

// TestEnsureUnitSupervisorClearsADisabledOverride pins that a supervisor someone disabled is still
// loaded: launchd keeps the override across a bootout and refuses to bootstrap a disabled job.
func TestEnsureUnitSupervisorClearsADisabledOverride(t *testing.T) {
	l := stubLiveLaunchd(t)
	stubJobDir(t)
	l.disabled["system/"+unitSupervisorJob] = true

	require.NoError(t, ensureUnitSupervisor(context.Background()))
	assert.Equal(t, 1, l.count("kickstart", "system/"+unitSupervisorJob))
}

// stubEnsureUnitSupervisor counts the times the supervisor is started.
func stubEnsureUnitSupervisor(t *testing.T) *int {
	t.Helper()

	var started int
	original := ensureUnitSupervisor
	ensureUnitSupervisor = func(context.Context) error {
		started++
		return nil
	}
	t.Cleanup(func() { ensureUnitSupervisor = original })
	return &started
}

// TestFailedStartWithdrawsTheExperimentTarget pins that a start the hook gives up on asks the
// supervisor for the stable jobs, so a supervisor still applying the experiment does not go on with
// it once the configuration is discarded.
func TestFailedStartWithdrawsTheExperimentTarget(t *testing.T) {
	stubDeadlinePath(t)
	stubUnitsState(t)
	started := stubEnsureUnitSupervisor(t)
	stubRequestUnits(t, errors.New("the unit supervisor did not apply experiment within 5m0s"))

	require.Error(t, agentService.StartExperiment(testHookContext(t)))
	latest, err := readUnitsRequest()
	require.NoError(t, err)
	assert.Equal(t, unitsTargetStable, latest.target)
	assert.Equal(t, 1, *started)
}

// TestWatchExperimentStateSeesEveryChange pins how the supervisor learns of a new target at once:
// every write and removal in the deadline's directory is seen.
func TestWatchExperimentStateSeesEveryChange(t *testing.T) {
	stubDeadlinePath(t)
	stubUnitsState(t)
	changes, stop, err := watchExperimentState()
	require.NoError(t, err)
	t.Cleanup(stop)

	expect := func(what string) {
		t.Helper()
		select {
		case <-changes:
		case <-time.After(5 * time.Second):
			t.Fatalf("no change was seen when the %s", what)
		}
	}
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	expect("deadline was written")
	request(t, unitsTargetStable)
	expect("target was written")
	require.NoError(t, experimentDeadline().Clear())
	expect("deadline was cleared")
	stop()
	stop()
}
