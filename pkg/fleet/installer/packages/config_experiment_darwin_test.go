// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// stubJobDir points the job definitions at a temporary directory.
func stubJobDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	original := launchdJobDir
	launchdJobDir = dir
	t.Cleanup(func() { launchdJobDir = original })
	return dir
}

// stubConfigExperimentWatcher replaces the detached watcher launcher with a recorder, so tests
// that exercise postStartConfigExperimentDatadogAgent don't spawn a real process. Returns the
// number of times it was called.
func stubConfigExperimentWatcher(t *testing.T) *int {
	t.Helper()

	calls := new(int)
	original := launchConfigExperimentWatcher
	launchConfigExperimentWatcher = func(context.Context) error {
		*calls++
		return nil
	}
	t.Cleanup(func() { launchConfigExperimentWatcher = original })
	return calls
}

// stubDeadlinePath points the deadline file at a temporary path.
func stubDeadlinePath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "experiment-deadline")
	original := configExperimentDeadlinePath
	configExperimentDeadlinePath = path
	t.Cleanup(func() { configExperimentDeadlinePath = original })
	return path
}

// stubConfigExperimentDirs points the configuration directories at a temporary state root laid
// out the way the installer leaves it: etc holding a stable deployment, and etc-exp resting as a
// symlink to it.
func stubConfigExperimentDirs(t *testing.T) config.Directories {
	t.Helper()

	root := t.TempDir()
	dirs := config.Directories{
		StablePath:     filepath.Join(root, "etc"),
		ExperimentPath: filepath.Join(root, "etc-exp"),
	}
	require.NoError(t, os.MkdirAll(dirs.StablePath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, ".deployment-id"), []byte("stable-1"), 0640))
	require.NoError(t, os.Symlink(dirs.StablePath, dirs.ExperimentPath))

	original := configExperimentDirs
	configExperimentDirs = dirs
	t.Cleanup(func() { configExperimentDirs = original })
	return dirs
}

// deployExperimentConfig replaces the resting link with a real experiment directory, as the
// installer does before it calls the post-start hook.
func deployExperimentConfig(t *testing.T, dirs config.Directories) {
	t.Helper()

	require.NoError(t, os.Remove(dirs.ExperimentPath))
	require.NoError(t, os.MkdirAll(dirs.ExperimentPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.ExperimentPath, ".deployment-id"), []byte("experiment-1"), 0640))
}

// experimentDeploymentID is the experiment the daemon's state refresh would report as running.
func experimentDeploymentID(t *testing.T, dirs config.Directories) string {
	t.Helper()

	state, err := dirs.GetState()
	require.NoError(t, err)
	return state.ExperimentDeploymentID
}

// launchctlCalls flattens the recorded invocations to "verb target" pairs, in order.
func launchctlCalls(calls [][]string, verb string) []string {
	var targets []string
	for _, args := range calls {
		if len(args) >= 2 && args[0] == verb {
			targets = append(targets, args[len(args)-1])
		}
	}
	return targets
}

// TestStartConfigExperimentHandsOverToTheExperimentSet pins the order of the swap: the stable jobs
// must be unloaded before the experiment ones are loaded, or two Agents would run at once against
// the same PID files and the same intake.
func TestStartConfigExperimentHandsOverToTheExperimentSet(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	watcherCalls := stubConfigExperimentWatcher(t)

	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, 1, *watcherCalls, "the watcher was not launched after a successful start")

	for _, label := range agentJobs {
		content, err := os.ReadFile(filepath.Join(dir, label+"-exp.plist"))
		require.NoError(t, err, "the experiment definition for %s was not written", label)
		assert.Contains(t, string(content), "<string>"+label+"-exp</string>")
		assert.NoFileExists(t, filepath.Join(dir, label+".plist"), "the stable definition was rewritten")
	}

	bootedOut := launchctlCalls(*calls, "bootout")
	bootstrapped := launchctlCalls(*calls, "bootstrap")
	require.NotEmpty(t, bootedOut)
	require.NotEmpty(t, bootstrapped)
	for _, label := range agentJobs {
		assert.Contains(t, bootedOut, "system/"+label)
		assert.Contains(t, bootstrapped, filepath.Join(dir, label+"-exp.plist"))
	}

	// The installer daemon is the process running this hook. Stopping it would abandon the
	// experiment it has just started, with nothing left to revert it.
	for _, args := range *calls {
		for _, argument := range args {
			assert.NotContains(t, argument, installerJob, "the swap touched the installer daemon: %v", args)
		}
	}

	// Every stable job is unloaded before any experiment job is loaded.
	lastStableBootout, firstExperimentStart := -1, len(*calls)
	for i, args := range *calls {
		joined := strings.Join(args, " ")
		if args[0] == "bootout" && !strings.Contains(joined, "-exp") {
			lastStableBootout = i
		}
		if (args[0] == "bootstrap" || args[0] == "kickstart") && strings.Contains(joined, "-exp") && i < firstExperimentStart {
			firstExperimentStart = i
		}
	}
	assert.Less(t, lastStableBootout, firstExperimentStart, "an experiment job was loaded while a stable one was still running")
}

// TestStopConfigExperimentRestoresTheStableSet is the revert path. It must leave the host with no
// trace of the experiment, including no experiment definitions on disk.
func TestStopConfigExperimentRestoresTheStableSet(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	deadlinePath := stubDeadlinePath(t)
	stubConfigExperimentWatcher(t)
	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	*calls = nil

	require.NoError(t, preStopConfigExperimentDatadogAgent(testHookContext(t)))
	assert.NoFileExists(t, deadlinePath, "the deadline file survived a deliberate stop")

	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(dir, label+"-exp.plist"), "an experiment definition survived the revert")
		content, err := os.ReadFile(filepath.Join(dir, label+".plist"))
		require.NoError(t, err, "the stable definition for %s was not restored", label)
		assert.Contains(t, string(content), "<string>"+label+"</string>")
	}

	bootedOut := launchctlCalls(*calls, "bootout")
	for _, label := range agentJobs {
		assert.Contains(t, bootedOut, "system/"+label+"-exp")
	}
	for _, label := range agentJobs {
		assert.Contains(t, launchctlCalls(*calls, "enable"), "system/"+label)
		assert.Contains(t, launchctlCalls(*calls, "kickstart"), "system/"+label)
	}
}

// TestPromoteConfigExperimentRestartsTheStableSet covers the other outcome. The stable jobs are
// started fresh so they read the configuration the installer has just promoted; a job left loaded
// would keep serving the pre-promotion configuration it was started with.
func TestPromoteConfigExperimentRestartsTheStableSet(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	deadlinePath := stubDeadlinePath(t)
	stubConfigExperimentWatcher(t)
	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	*calls = nil

	require.NoError(t, postPromoteConfigExperimentDatadogAgent(testHookContext(t)))
	assert.NoFileExists(t, deadlinePath, "the deadline file survived a promote")

	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(dir, label+"-exp.plist"))
		assert.Contains(t, launchctlCalls(*calls, "bootout"), "system/"+label+"-exp")
		assert.Contains(t, launchctlCalls(*calls, "kickstart"), "system/"+label)
	}
}

// TestExperimentJobsMatchTheEmbeddedDefinitions guards the two label lists against drifting apart
// from the definitions that ship: a job missing from agentJobs would never be swapped, and would
// keep running the stable configuration for the whole life of an experiment.
func TestExperimentJobsMatchTheEmbeddedDefinitions(t *testing.T) {
	assert.NotContains(t, agentJobs, installerJob)
	assert.Contains(t, stableJobs, installerJob)
	assert.Len(t, stableJobs, len(agentJobs)+1)

	for i, label := range agentJobs {
		assert.Equal(t, label+"-exp", experimentJobs[i])
	}
	assert.Len(t, experimentJobs, len(agentJobs))
}

// stubLaunchdFailing behaves like stubLaunchd, but fails any call whose verb and joined
// arguments contain match. Used to inject a failure partway through a swap.
func stubLaunchdFailing(t *testing.T, verb, match string) *[][]string {
	t.Helper()

	var calls [][]string
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.BootoutSettlePollInterval = time.Millisecond
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			calls = append(calls, args)
			if len(args) > 0 && args[0] == "print" {
				return []byte(notLoadedOutput), exitError(113)
			}
			if len(args) > 0 && args[0] == verb && strings.Contains(strings.Join(args, " "), match) {
				return nil, errors.New("injected failure")
			}
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	return &calls
}

// TestStartConfigExperimentRollsBackWhenTheExperimentFailsToBootstrap covers the gap PR #55770's
// review flagged in Start: if writing or starting the experiment set fails partway, the stable
// set — already stopped by then — must be put back rather than left down with nothing running.
func TestStartConfigExperimentRollsBackWhenTheExperimentFailsToBootstrap(t *testing.T) {
	stubLaunchdFailing(t, "bootstrap", "-exp")
	dir := stubJobDir(t)
	deadlinePath := stubDeadlinePath(t)

	err := configExperiment{jobs: agentJobSet()}.Start(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected failure", "expected the original bootstrap failure to be reported, since the rollback itself succeeded")

	for _, label := range agentJobs {
		content, readErr := os.ReadFile(filepath.Join(dir, label+".plist"))
		require.NoError(t, readErr, "the stable definition for %s was not restored after the failed handover", label)
		assert.Contains(t, string(content), "<string>"+label+"</string>")
	}
	assert.NoFileExists(t, deadlinePath, "the deadline survived a start that was fully rolled back")
}

// TestRestoreStableRetriesBeforeGivingUp covers the second gap PR #55770's review flagged: a
// transient failure restoring the stable set must be retried, not surfaced immediately.
func TestRestoreStableRetriesBeforeGivingUp(t *testing.T) {
	originalBackoff := configExperimentRestoreBackoff
	configExperimentRestoreBackoff = time.Millisecond
	t.Cleanup(func() { configExperimentRestoreBackoff = originalBackoff })

	originalClient := launchdClient
	t.Cleanup(func() { launchdClient = originalClient })

	var attempts int
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.BootoutSettlePollInterval = time.Millisecond
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "print" {
				return []byte(notLoadedOutput), exitError(113)
			}
			if len(args) > 0 && args[0] == "bootstrap" && !strings.Contains(strings.Join(args, " "), "-exp") {
				attempts++
			}
			return nil, nil
		}
		return client
	}
	stubJobDir(t)
	stubDeadlinePath(t)

	require.NoError(t, configExperiment{jobs: agentJobSet()}.Stop(testHookContext(t)))
	assert.Equal(t, len(agentJobs), attempts, "expected exactly one bootstrap attempt per job when nothing fails")
}

// TestRestoreStableGivesUpAfterExhaustingRetries asserts the bounded-retry policy actually has a
// bound: a failure that never clears must eventually be reported, not retried forever.
func TestRestoreStableGivesUpAfterExhaustingRetries(t *testing.T) {
	originalBackoff := configExperimentRestoreBackoff
	configExperimentRestoreBackoff = time.Millisecond
	t.Cleanup(func() { configExperimentRestoreBackoff = originalBackoff })

	originalClient := launchdClient
	t.Cleanup(func() { launchdClient = originalClient })

	var bootstrapAttempts int
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.BootoutSettlePollInterval = time.Millisecond
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "print" {
				return []byte(notLoadedOutput), exitError(113)
			}
			if len(args) > 0 && args[0] == "bootstrap" && !strings.Contains(strings.Join(args, " "), "-exp") {
				bootstrapAttempts++
				return nil, errors.New("injected failure")
			}
			return nil, nil
		}
		return client
	}
	stubJobDir(t)
	stubDeadlinePath(t)

	err := configExperiment{jobs: agentJobSet()}.Stop(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "after 3 attempts")
	assert.Equal(t, len(agentJobs)*configExperimentRestoreRetries, bootstrapAttempts, "expected every label's retries to stop at the configured bound")
}

// stubLaunchdScript is a launchctl stand-in driven by a function that decides each call's result,
// and that records every call. print falls back to "not loaded" when the function returns nothing.
func stubLaunchdScript(t *testing.T, respond func(args []string) ([]byte, error, bool)) *[][]string {
	t.Helper()

	var calls [][]string
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.BootoutSettlePollInterval = time.Millisecond
		client.BootoutSettleTimeout = 20 * time.Millisecond
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			calls = append(calls, args)
			if out, err, ok := respond(args); ok {
				return out, err
			}
			if args[0] == "print" {
				return []byte(notLoadedOutput), exitError(113)
			}
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	return &calls
}

// stableBootstraps counts the bootstraps of each stable job.
func stableBootstraps(calls [][]string) map[string]int {
	counts := map[string]int{}
	for _, args := range calls {
		if args[0] == "bootstrap" && !strings.Contains(args[len(args)-1], "-exp") {
			counts[strings.TrimSuffix(filepath.Base(args[len(args)-1]), ".plist")]++
		}
	}
	return counts
}

func shortRestoreBackoff(t *testing.T) {
	t.Helper()
	original := configExperimentRestoreBackoff
	configExperimentRestoreBackoff = time.Millisecond
	t.Cleanup(func() { configExperimentRestoreBackoff = original })
}

// TestRestoreStableRestoresEvenWhenRemovingTheExperimentFails pins that a leftover experiment
// definition, which is never loaded at boot, does not stop the stable set from coming back.
func TestRestoreStableRestoresEvenWhenRemovingTheExperimentFails(t *testing.T) {
	calls := stubLaunchdScript(t, func([]string) ([]byte, error, bool) { return nil, nil, false })
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	// A non-empty directory where an experiment definition should be cannot be removed.
	stuck := filepath.Join(dir, agentJobs[0]+"-exp.plist")
	require.NoError(t, os.MkdirAll(filepath.Join(stuck, "child"), 0755))

	err := configExperiment{jobs: agentJobSet()}.Stop(testHookContext(t))

	require.Error(t, err)
	for _, label := range agentJobs {
		assert.Equal(t, 1, stableBootstraps(*calls)[label], "%s was not restored", label)
	}
}

// TestRestoreStableSwapsTheOtherLabelsWhenOneCannotBeUnloaded pins both halves of the label by
// label swap: a stable job is never started while its experiment twin is still loaded, and a label
// that cannot be handed back does not keep the others from coming back.
func TestRestoreStableSwapsTheOtherLabelsWhenOneCannotBeUnloaded(t *testing.T) {
	shortRestoreBackoff(t)
	stuck := "com.datadoghq.sysprobe-exp"
	calls := stubLaunchdScript(t, func(args []string) ([]byte, error, bool) {
		switch {
		case args[0] == "bootout" && args[1] == "system/"+stuck:
			return nil, errors.New("injected failure"), true
		case args[0] == "print" && args[1] == "system/"+stuck:
			return []byte("state = running\n"), nil, true
		}
		return nil, nil, false
	})
	stubJobDir(t)
	stubDeadlinePath(t)

	err := configExperiment{jobs: agentJobSet()}.Stop(testHookContext(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "com.datadoghq.sysprobe")
	counts := stableBootstraps(*calls)
	assert.Zero(t, counts["com.datadoghq.sysprobe"], "the stable job started while its experiment twin was still loaded")
	assert.Equal(t, 1, counts["com.datadoghq.agent"])
	assert.Equal(t, 1, counts["com.datadoghq.data-plane"])
}

// TestRestoreStableRetriesOnlyTheFailingLabel pins that a retry does not restart the labels that
// were already handed back.
func TestRestoreStableRetriesOnlyTheFailingLabel(t *testing.T) {
	shortRestoreBackoff(t)
	failures := 1
	calls := stubLaunchdScript(t, func(args []string) ([]byte, error, bool) {
		if args[0] == "bootstrap" && filepath.Base(args[len(args)-1]) == "com.datadoghq.data-plane.plist" && failures > 0 {
			failures--
			return nil, errors.New("injected failure"), true
		}
		return nil, nil, false
	})
	stubJobDir(t)
	stubDeadlinePath(t)

	require.NoError(t, configExperiment{jobs: agentJobSet()}.Stop(testHookContext(t)))

	assert.Equal(t, map[string]int{
		"com.datadoghq.agent":      1,
		"com.datadoghq.sysprobe":   1,
		"com.datadoghq.data-plane": 2,
	}, stableBootstraps(*calls))
}

// TestStartConfigExperimentRestoresTheStableSetWhenStoppingItFails covers a handover that fails
// before the experiment set is touched: some stable jobs may already be down, so they are put back.
func TestStartConfigExperimentRestoresTheStableSetWhenStoppingItFails(t *testing.T) {
	shortRestoreBackoff(t)
	// The stable system-probe job stays loaded until a bootout of it gets through, and the first
	// one fails.
	target := "system/com.datadoghq.sysprobe"
	failures, loaded := 1, true
	calls := stubLaunchdScript(t, func(args []string) ([]byte, error, bool) {
		switch {
		case args[0] == "bootout" && args[1] == target:
			if failures > 0 {
				failures--
				return nil, errors.New("injected failure"), true
			}
			loaded = false
		case args[0] == "print" && args[1] == target && loaded:
			return []byte("state = running\n"), nil, true
		}
		return nil, nil, false
	})
	stubJobDir(t)
	stubDeadlinePath(t)

	err := configExperiment{jobs: agentJobSet()}.Start(testHookContext(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected failure")
	for _, label := range agentJobs {
		assert.Equal(t, 1, stableBootstraps(*calls)[label], "%s was not restored", label)
	}
	for _, args := range *calls {
		assert.False(t, args[0] == "bootstrap" && strings.Contains(args[len(args)-1], "-exp"), "the experiment set was started")
	}
}

// TestStartConfigExperimentDiscardsTheConfigWhenTheHandoverFails covers a start that is rolled back
// to the stable job set: the experiment directory the installer published must go with it, or the
// daemon would keep reporting the failed experiment as the running configuration.
func TestStartConfigExperimentDiscardsTheConfigWhenTheHandoverFails(t *testing.T) {
	stubLaunchdFailing(t, "bootstrap", "-exp")
	stubJobDir(t)
	stubDeadlinePath(t)
	watcherCalls := stubConfigExperimentWatcher(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	err := postStartConfigExperimentDatadogAgent(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected failure")
	assert.Zero(t, *watcherCalls, "a watcher was launched for an experiment that never started")
	assert.Empty(t, experimentDeploymentID(t, dirs), "the experiment configuration survived a failed start")
}

// TestStartConfigExperimentDiscardsTheConfigWhenTheWatcherFailsToLaunch is the same guarantee for
// the other revert in the post-start hook.
func TestStartConfigExperimentDiscardsTheConfigWhenTheWatcherFailsToLaunch(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	original := launchConfigExperimentWatcher
	launchConfigExperimentWatcher = func(context.Context) error { return errors.New("injected failure") }
	t.Cleanup(func() { launchConfigExperimentWatcher = original })
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	err := postStartConfigExperimentDatadogAgent(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "watcher failed to launch, experiment reverted")
	assert.Empty(t, experimentDeploymentID(t, dirs), "the experiment configuration survived a reverted start")
}

// TestStartConfigExperimentKeepsTheConfigWhenItSucceeds guards the other side: a running
// experiment must still be reported as running.
func TestStartConfigExperimentKeepsTheConfigWhenItSucceeds(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentWatcher(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs))
}

// TestResumeRevertsAnExperimentDeployedWithNoDeadline covers a daemon that died between the
// installer publishing the experiment directory and Start writing the deadline (or partway through
// a deliberate stop, which clears the deadline first). Nothing will ever resume that experiment,
// so it is reverted instead of being reported as running for good.
func TestResumeRevertsAnExperimentDeployedWithNoDeadline(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	watcherCalls := stubConfigExperimentWatcher(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Empty(t, experimentDeploymentID(t, dirs), "an orphaned experiment configuration was left deployed")
	assert.Zero(t, *watcherCalls, "an orphaned experiment was resumed instead of reverted")
	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(dir, label+"-exp.plist"))
		assert.Contains(t, launchctlCalls(*calls, "kickstart"), "system/"+label, "the stable job %s was not restored", label)
	}
}

// TestParseWatcherPIDs pins which processes resume stops as a surviving watcher: only root-owned
// processes running exactly the installer's watcher command, and never the caller itself. A shell
// or pgrep whose command line merely contains the watcher's arguments must not match.
func TestParseWatcherPIDs(t *testing.T) {
	ps := strings.Join([]string{
		"    1     0 /sbin/launchd",
		"  101     0 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent watchConfigExperiment",
		"  102   450 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent watchConfigExperiment",
		"  103     0 /bin/sh -c pgrep -f package-command datadog-agent watchConfigExperiment",
		"  104     0 /usr/bin/pgrep -f package-command datadog-agent watchConfigExperiment",
		"  105     0 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent otherCommand",
		"  106     0 /opt/datadog-agent/embedded/bin/installer package-command other-package watchConfigExperiment",
		"  107     0 /opt/datadog-agent/embedded/bin/installer run -c /opt/datadog-agent/etc",
		"  108     0 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent watchConfigExperiment",
		"  notapid 0 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent watchConfigExperiment",
		"",
	}, "\n")

	assert.Equal(t, []int{101, 108}, parseWatcherPIDs(ps, 999))
	assert.Equal(t, []int{101}, parseWatcherPIDs(ps, 108), "the caller must never be matched")
	assert.Empty(t, parseWatcherPIDs("", 999))
}

// TestResumeStopsTheSurvivingWatcherBeforeTouchingTheJobs covers a daemon restart in the middle of
// an experiment. The previous watcher outlives the daemon and is armed on the experiment pids, so
// when resume bounces the experiment jobs it would see them terminate, take that for a crash and
// revert the experiment under the resume. It has to be gone before the first job is touched.
func TestResumeStopsTheSurvivingWatcherBeforeTouchingTheJobs(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	path := stubDeadlinePath(t)
	watcherCalls := stubConfigExperimentWatcher(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, launchd.Deadline{Path: path}.Write(configExperimentDeadlineWindow))

	var stopped int
	launchctlCallsAtStop := -1
	original := stopSurvivingWatchers
	stopSurvivingWatchers = func(context.Context) error {
		stopped++
		launchctlCallsAtStop = len(*calls)
		return nil
	}
	t.Cleanup(func() { stopSurvivingWatchers = original })

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, 1, stopped, "the previous watcher should be stopped exactly once")
	assert.Zero(t, launchctlCallsAtStop, "resume touched launchd before it stopped the previous watcher")
	assert.Equal(t, 1, *watcherCalls, "a new watcher should supervise the resumed experiment")
	assert.NotEmpty(t, launchctlCalls(*calls, "kickstart"), "the experiment jobs were not started")
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs), "the resumed experiment must stay deployed")
}

// TestResumeStillResumesWhenTheWatcherCannotBeStopped pins that failing to find or stop the
// previous watcher is not fatal: resuming with a stale watcher at worst reverts the experiment,
// which is safe, whereas giving up would leave the experiment unsupervised.
func TestResumeStillResumesWhenTheWatcherCannotBeStopped(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	path := stubDeadlinePath(t)
	watcherCalls := stubConfigExperimentWatcher(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, launchd.Deadline{Path: path}.Write(configExperimentDeadlineWindow))

	original := stopSurvivingWatchers
	stopSurvivingWatchers = func(context.Context) error { return errors.New("injected failure") }
	t.Cleanup(func() { stopSurvivingWatchers = original })

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, 1, *watcherCalls)
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs))
}

// TestResumeLeavesARestingHostAlone pins the common case: no deadline and no experiment means
// nothing to recover, so resume must not touch any job.
func TestResumeLeavesARestingHostAlone(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	dirs := stubConfigExperimentDirs(t)

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Empty(t, *calls, "resume touched launchd on a host with no experiment")
	assert.Empty(t, experimentDeploymentID(t, dirs))
}

// TestWatcherExitsWhenTheExperimentIsAlreadyOver covers a watcher that starts after a deliberate
// stop or promote already cleared the deadline: there is nothing to supervise, so it must exit
// without touching any job.
func TestWatcherExitsWhenTheExperimentIsAlreadyOver(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentDirs(t)

	require.NoError(t, watchExperiment(context.Background()))
	assert.Empty(t, *calls, "a watcher with no experiment to supervise touched launchd")
}

// TestRevertIsANoopForANewerExperiment covers a watcher left over from an earlier experiment: its
// late events must not revert the experiment that has since rewritten the deadline.
func TestRevertIsANoopForANewerExperiment(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	path := stubDeadlinePath(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	deadline := launchd.Deadline{Path: path}
	require.NoError(t, deadline.Write(time.Hour))
	stale, err := deadline.Read()
	require.NoError(t, err)
	require.NoError(t, deadline.Write(time.Hour))

	reverted, err := revertExperimentIfStillPending(context.Background(), deadline, stale, "stale exit")
	require.NoError(t, err)
	assert.False(t, reverted)
	assert.Empty(t, *calls, "a stale watcher touched launchd")
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs))
}
