// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// The watcher reverts through the installer's remove-config-experiment command. Tests run it in
// process instead of spawning the installer binary.
func init() {
	runRemoveConfigExperiment = removeConfigExperimentInProcess
}

// removeConfigExperimentInProcess does in process what remove-config-experiment does on macOS: the
// stop hook (token check, Agent jobs back to stable), the installer discarding the experiment
// configuration, then the hand-back the stop hook launches, which runs once the command has exited.
func removeConfigExperimentInProcess(ctx context.Context, token string) error {
	if err := checkRevertToken(token); err != nil {
		return err
	}
	stopErr := configExperiment{jobs: agentJobSet()}.Stop(ctx)
	if stopErr == nil {
		stopErr = discardExperimentConfig(ctx)
	}
	loaded, err := launchdClient().Loaded(ctx, installerJob+string(launchd.Experiment))
	if err == nil && loaded {
		err = handBackInstaller(ctx)
	}
	return errors.Join(stopErr, err)
}

// startedProcess starts command and returns its pid, reaping it once it exits.
func startedProcess(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(name, args...)
	require.NoError(t, cmd.Start())
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd
}

// TestProbeExperimentPassesWhileTheJobsKeepRunning is the healthy start: nothing exits during the
// window, so the start goes on.
func TestProbeExperimentPassesWhileTheJobsKeepRunning(t *testing.T) {
	runningExperiment(t)

	require.NoError(t, probeExperiment(context.Background(), agentJobSet(), 50*time.Millisecond))
}

// TestProbeExperimentFailsWhenTheAgentCrashes is T15: the Agent is killed seconds after the start.
// The probe must fail the start, so the daemon reports the task as an error.
func TestProbeExperimentFailsWhenTheAgentCrashes(t *testing.T) {
	l, _, _ := runningExperiment(t)
	agent := startedProcess(t, "sleep", "60")
	l.pids["system/"+agentJob+"-exp"] = agent.Process.Pid
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = agent.Process.Signal(syscall.SIGKILL)
	}()

	err := probeExperiment(context.Background(), agentJobSet(), 10*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), agentJob)
}

// TestProbeExperimentIgnoresACleanExitOfAnOptionalJob keeps the rule the watcher applies: a job
// whose feature is off exits 0 right after it starts, and must not fail the experiment.
func TestProbeExperimentIgnoresACleanExitOfAnOptionalJob(t *testing.T) {
	l, _, _ := runningExperiment(t)
	optional := startedProcess(t, "sh", "-c", "sleep 0.1; exit 0")
	l.pids["system/com.datadoghq.sysprobe-exp"] = optional.Process.Pid

	require.NoError(t, probeExperiment(context.Background(), agentJobSet(), time.Second))
}

// TestStartConfigExperimentFailsAndRevertsWhenTheProbeFails pins what the probe is for: a failed
// probe fails the start, puts the stable jobs back, discards the experiment configuration and never
// launches the installer hand-over.
func TestStartConfigExperimentFailsAndRevertsWhenTheProbeFails(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	handOverCalls := stubInstallerHandOver(t)
	probeConfigExperiment = func(context.Context, launchd.JobSet, time.Duration) error {
		return errors.New("injected crash")
	}
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	err := postStartConfigExperimentDatadogAgent(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected crash")
	assert.Zero(t, *handOverCalls, "the installer daemon was handed over for an experiment that failed")
	assert.Empty(t, experimentDeploymentID(t, dirs), "the experiment configuration survived a failed start")
	for _, label := range agentJobs {
		assert.Contains(t, launchctlCalls(*calls, "bootout"), "system/"+label+"-exp", "%s-exp was left running", label)
		assert.NoFileExists(t, filepath.Join(dir, label+"-exp.plist"), "the experiment definition of %s was left behind", label)
	}
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	assert.Empty(t, token, "the deadline survived a failed start")
}

// TestStopHookRefusesARevertForAnExperimentThatEnded closes the gap the watcher leaves when it
// releases the installer lock: a stop or promote, or a new experiment, can take it before the
// command does. The stop hook then fails, and the installer keeps the configuration.
func TestStopHookRefusesARevertForAnExperimentThatEnded(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubInstallerHandBack(t, nil)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	t.Setenv(configExperimentRevertTokenEnv, "a deadline from an earlier experiment")

	err := preStopConfigExperimentDatadogAgent(testHookContext(t))
	require.ErrorIs(t, err, errRevertedExperimentEnded)
	assert.Empty(t, launchctlCalls(*calls, "bootout"), "the stop hook touched the jobs of an experiment it was not for")
}

// TestStopHookRevertsTheExperimentTheWatcherSaw is the matching case: the deadline still holds the
// watcher's token, so the stop goes ahead.
func TestStopHookRevertsTheExperimentTheWatcherSaw(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubInstallerHandBack(t, nil)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	t.Setenv(configExperimentRevertTokenEnv, token)

	require.NoError(t, preStopConfigExperimentDatadogAgent(testHookContext(t)))
	assert.NotEmpty(t, launchctlCalls(*calls, "bootout"))
}

// TestWatcherRevertsThroughTheInstaller checks the watcher hands the revert to the installer
// command, with the deadline it read, and that the host ends on the stable set.
func TestWatcherRevertsThroughTheInstaller(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	deadline := experimentDeadline()
	token, err := deadline.Read()
	require.NoError(t, err)
	var passed []string
	runRemoveConfigExperiment = func(ctx context.Context, token string) error {
		passed = append(passed, token)
		return removeConfigExperimentInProcess(ctx, token)
	}
	t.Cleanup(func() { runRemoveConfigExperiment = removeConfigExperimentInProcess })

	reverted, err := revertFromWatcher(context.Background(), deadline, token, "test crash")
	require.NoError(t, err)
	assert.True(t, reverted)
	assert.Equal(t, []string{token}, passed)
	assertReverted(t, l, experimentPath)
}

// TestWatcherRevertsDirectlyWhenTheInstallerCommandFails keeps the experiment from running
// unsupervised when the installer command cannot run.
func TestWatcherRevertsDirectlyWhenTheInstallerCommandFails(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	deadline := experimentDeadline()
	token, err := deadline.Read()
	require.NoError(t, err)
	runRemoveConfigExperiment = func(context.Context, string) error { return errors.New("injected failure") }
	t.Cleanup(func() { runRemoveConfigExperiment = removeConfigExperimentInProcess })

	reverted, err := revertFromWatcher(context.Background(), deadline, token, "test crash")
	require.NoError(t, err)
	assert.True(t, reverted)
	assertReverted(t, l, experimentPath)
}

// TestWatcherLeavesANewExperimentAlone covers a new experiment started between the watcher
// releasing the lock and its command running: the stop hook refuses, and the fallback finds a
// deadline that is neither the watcher's nor the reverting marker, so nothing is reverted.
func TestWatcherLeavesANewExperimentAlone(t *testing.T) {
	l, _, _ := runningExperiment(t)
	deadline := experimentDeadline()
	token, err := deadline.Read()
	require.NoError(t, err)
	runRemoveConfigExperiment = func(ctx context.Context, token string) error {
		time.Sleep(time.Millisecond) // the deadline is a timestamp: make the new one differ
		require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
		return removeConfigExperimentInProcess(ctx, token)
	}
	t.Cleanup(func() { runRemoveConfigExperiment = removeConfigExperimentInProcess })

	reverted, err := revertFromWatcher(context.Background(), deadline, token, "test crash")
	require.NoError(t, err)
	assert.False(t, reverted)
	for _, label := range agentJobs {
		assert.True(t, l.isLoaded(label+"-exp"), "%s-exp of the new experiment was reverted", label)
	}
}
