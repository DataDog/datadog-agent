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
	idle  map[string]bool
	pids  map[string]int
	calls [][]string
}

func stubLiveLaunchd(t *testing.T, loaded ...string) *liveLaunchd {
	t.Helper()

	l := &liveLaunchd{loaded: map[string]bool{}, idle: map[string]bool{}, pids: map[string]int{}}
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
				if l.idle[target] {
					return []byte(target + " = {\n\tstate = not running\n}\n"), nil
				}
				pid := os.Getpid()
				if p, ok := l.pids[target]; ok {
					pid = p
				}
				return []byte(fmt.Sprintf("%s = {\n\tpid = %d\n}\n", target, pid)), nil
			case "bootout":
				l.loaded[target] = false
				delete(l.idle, target)
			case "bootstrap":
				label := "system/" + strings.TrimSuffix(filepath.Base(target), ".plist")
				l.loaded[label] = true
				// A job with no RunAtLoad loads without running.
				if strings.HasSuffix(label, "-exp") || label == "system/"+unitSupervisorJob {
					l.idle[label] = true
				}
			case "kickstart":
				l.loaded[target] = true
				delete(l.idle, target)
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

// stubUnitsState points the supervisor's state files at a temporary directory beside the deadline,
// and shortens its waits.
func stubUnitsState(t *testing.T) {
	t.Helper()

	dir := filepath.Dir(configExperimentDeadlinePath)
	originalTarget, originalStatus := unitsTargetPath, unitsStatusPath
	unitsTargetPath, unitsStatusPath = filepath.Join(dir, "units-target"), filepath.Join(dir, "units-status")
	originalPoll, originalTick, originalRestart := unitsPollInterval, unitsTickInterval, unitsRestartDelay
	unitsPollInterval, unitsTickInterval, unitsRestartDelay = 5*time.Millisecond, 20*time.Millisecond, 10*time.Millisecond
	originalLockPoll := installerLockPoll
	installerLockPoll = 20 * time.Millisecond
	t.Cleanup(func() {
		unitsTargetPath, unitsStatusPath = originalTarget, originalStatus
		unitsPollInterval, unitsTickInterval, unitsRestartDelay = originalPoll, originalTick, originalRestart
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

// TestSupervisorRestartsAChildThatFails pins Restart=on-failure: a child job exiting non-zero is
// restarted, and does not fail the experiment.
func TestSupervisorRestartsAChildThatFails(t *testing.T) {
	l := supervisedHost(t)
	r := request(t, unitsTargetExperiment)
	crashing := exec.Command("/bin/sh", "-c", "sleep 0.3; exit 3")
	require.NoError(t, crashing.Start())
	go func() { _ = crashing.Wait() }()
	l.mu.Lock()
	l.pids["system/com.datadoghq.sysprobe-exp"] = crashing.Process.Pid
	l.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- superviseUnits(context.Background()) }()

	waitForStatus(t, r, unitsPhaseDone)
	require.Eventually(t, func() bool { return l.count("kickstart", "system/com.datadoghq.sysprobe-exp") >= 2 }, 10*time.Second, 5*time.Millisecond,
		"the failed child should be started again")
	assert.True(t, l.isRunning(agentJob+"-exp"), "a child failure must not fail the experiment")

	request(t, unitsTargetStable)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the supervisor should exit")
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

// TestRequestUnitsWaitsForTheAgentJobs pins the hooks' side: a request returns once the supervisor
// reports the Agent jobs on the target, and fails with the supervisor's reason.
func TestRequestUnitsWaitsForTheAgentJobs(t *testing.T) {
	for name, tc := range map[string]struct {
		phase, message string
		wantErr        bool
	}{
		"applied": {unitsPhaseAgents, "", false},
		"failed":  {unitsPhaseFailed, "injected failure", true},
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
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	stubRequestUnits(t, errors.New("injected failure"))

	require.Error(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Empty(t, experimentDeploymentID(t, dirs))
}

// TestUnitSupervisorDefinitionDoesNotStartAtBoot pins what keeps a reboot on the stable jobs: the
// supervisor never starts at load, and its relaunch policy does not imply RunAtLoad.
func TestUnitSupervisorDefinitionDoesNotStartAtBoot(t *testing.T) {
	definition := string(unitSupervisorDefinition("/opt/datadog-agent/embedded/bin/installer"))
	assert.Regexp(t, `<key>RunAtLoad</key>\s*<false/>`, definition)
	assert.Contains(t, definition, "<key>Crashed</key>")
	assert.NotContains(t, definition, "SuccessfulExit", "SuccessfulExit implies RunAtLoad")
	assert.Contains(t, definition, "<string>"+superviseUnitsCommand+"</string>")
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
