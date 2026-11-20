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
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// stubConfigExperimentWatcher replaces the detached watcher launcher, and the hand-over launcher,
// with recorders, so tests that exercise the hooks don't spawn a real process. Returns the number
// of times the watcher was launched; stubInstallerHandOver returns the hand-over's.
func stubConfigExperimentWatcher(t *testing.T) *int {
	t.Helper()

	calls := new(int)
	original := launchConfigExperimentWatcher
	launchConfigExperimentWatcher = func(context.Context) error {
		*calls++
		return nil
	}
	t.Cleanup(func() { launchConfigExperimentWatcher = original })
	stubInstallerHandOver(t)
	return calls
}

// stubInstallerHandOver replaces the detached hand-over launcher with a recorder. Returns the
// number of times it was called.
func stubInstallerHandOver(t *testing.T) *int {
	t.Helper()

	calls := new(int)
	original := launchInstallerHandOver
	launchInstallerHandOver = func(context.Context) error {
		*calls++
		return nil
	}
	t.Cleanup(func() { launchInstallerHandOver = original })
	return calls
}

// stubSurvivingHelpers replaces the lookup and termination of the detached experiment processes a
// previous daemon left behind, so the settle never lists or signals the host's processes. Returns
// the number of times it was called, and the number of launchctl calls recorded in calls at the
// first one (-1 until then).
func stubSurvivingHelpers(t *testing.T, calls *[][]string, err error) (*int, *int) {
	t.Helper()

	stopped, callsAtStop := new(int), new(int)
	*callsAtStop = -1
	original := stopSurvivingHelpers
	stopSurvivingHelpers = func(context.Context) error {
		*stopped++
		if *callsAtStop < 0 && calls != nil {
			*callsAtStop = len(*calls)
		}
		return err
	}
	t.Cleanup(func() { stopSurvivingHelpers = original })
	return stopped, callsAtStop
}

// stubWatcherRunning makes the hand-over see a watcher as running or not.
func stubWatcherRunning(t *testing.T, running bool) {
	t.Helper()

	original := watcherRunning
	watcherRunning = func(context.Context) (bool, error) { return running, nil }
	t.Cleanup(func() { watcherRunning = original })
}

// stubInstallerHandBack replaces the detached hand-back launcher with a recorder, so tests that
// exercise the stop and promote hooks don't spawn a real process. Returns the number of times it
// was called.
func stubInstallerHandBack(t *testing.T, err error) *int {
	t.Helper()

	calls := new(int)
	original := launchInstallerHandBack
	launchInstallerHandBack = func(context.Context) error {
		*calls++
		return err
	}
	t.Cleanup(func() { launchInstallerHandBack = original })
	return calls
}

// stubLaunchdWithExperimentDaemon behaves like stubLaunchdFailing, but reports the experiment
// daemon as loaded.
func stubLaunchdWithExperimentDaemon(t *testing.T, verb, match string) *[][]string {
	t.Helper()

	var calls [][]string
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.BootoutSettlePollInterval = time.Millisecond
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			calls = append(calls, args)
			if len(args) > 0 && args[0] == "print" {
				if args[len(args)-1] == "system/"+installerJob+"-exp" {
					return nil, nil
				}
				return []byte(notLoadedOutput), exitError(113)
			}
			if verb != "" && len(args) > 0 && args[0] == verb && strings.Contains(strings.Join(args, " "), match) {
				return nil, errors.New("injected failure")
			}
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	return &calls
}

// touchesInstaller reports whether any recorded launchctl call other than print names the
// installer daemon, in either variant.
func touchesInstaller(calls [][]string) bool {
	for _, args := range calls {
		if len(args) > 0 && args[0] == "print" {
			continue
		}
		for _, argument := range args {
			if strings.Contains(argument, installerJob) {
				return true
			}
		}
	}
	return false
}

// stubDeadlinePath points the deadline file at a temporary path.
func stubDeadlinePath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "experiment-deadline")
	original := configExperimentDeadlinePath
	originalOwner := configExperimentStateOwner
	configExperimentDeadlinePath = path
	configExperimentStateOwner = os.Getuid()
	t.Cleanup(func() {
		configExperimentDeadlinePath = original
		configExperimentStateOwner = originalOwner
	})
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
		StatePath:      filepath.Join(root, "state", config.StateFileName),
	}
	require.NoError(t, os.MkdirAll(dirs.StablePath, 0755))
	require.NoError(t, os.Mkdir(filepath.Dir(dirs.StatePath), 0755))
	require.NoError(t, os.WriteFile(dirs.StatePath, []byte(`{"stable_deployment_id":"stable-1"}`), 0644))
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

	require.NoError(t, dirs.WriteExperiment(context.Background(), config.Operations{DeploymentID: "experiment-1"}))
}

// assertFailedOnHost checks the end state of an experiment that failed on the host after its start
// was reported: its configuration is still deployed, for Fleet to stop, and the deadline holds the
// failed marker (configExperiment.Fail).
func assertFailedOnHost(t *testing.T, dirs config.Directories) {
	t.Helper()

	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs), "the experiment configuration should be kept for Fleet to stop")
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	assert.Equal(t, launchd.DeadlineFailed, token, "the deadline should hold the failed marker")
}

// experimentDeploymentID is the experiment the daemon's state refresh would report as running.
func experimentDeploymentID(t *testing.T, dirs config.Directories) string {
	t.Helper()

	state, err := dirs.GetState()
	require.NoError(t, err)
	return state.ExperimentDeploymentID
}

// loadedJobs returns the "system/<label>" target of every job a recorded bootstrap loaded, in
// order. A stable job starts at load (RunAtLoad), so it is never kickstarted: this is how the tests
// see it come back.
func loadedJobs(calls [][]string) []string {
	var targets []string
	for _, path := range launchctlCalls(calls, "bootstrap") {
		targets = append(targets, "system/"+strings.TrimSuffix(filepath.Base(path), ".plist"))
	}
	return targets
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
	handOverCalls := stubInstallerHandOver(t)

	require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, 1, *handOverCalls, "the installer daemon hand-over was not launched after a successful start")

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
			assert.NotContains(t, argument, "com.datadoghq.installer", "the swap touched the installer daemon: %v", args)
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
		assert.Contains(t, launchctlCalls(*calls, "bootstrap"), filepath.Join(dir, label+".plist"))
		// The stable job starts at load; kickstarting one launchd can't spawn would block.
		assert.NotContains(t, launchctlCalls(*calls, "kickstart"), "system/"+label)
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
		assert.Contains(t, launchctlCalls(*calls, "bootstrap"), filepath.Join(dir, label+".plist"))
		// The stable job starts at load; kickstarting one launchd can't spawn would block.
		assert.NotContains(t, launchctlCalls(*calls, "kickstart"), "system/"+label)
	}
}

// TestExperimentJobsAreDerivedFromTheEmbeddedDefinitions pins that the jobs an experiment swaps
// and supervises are the ones the embedded definitions describe, in loading order: the installer
// that runs an experiment knows every job its own definitions have an experiment variant for, so a
// service a newer version adds is covered without a list to update.
func TestExperimentJobsAreDerivedFromTheEmbeddedDefinitions(t *testing.T) {
	assert.Equal(t, []string{
		"com.datadoghq.agent", "com.datadoghq.sysprobe", "com.datadoghq.data-plane", "com.datadoghq.installer",
	}, configExperimentJobs, "the installer daemon must come last")
	assert.Equal(t, []string{"com.datadoghq.agent", "com.datadoghq.sysprobe", "com.datadoghq.data-plane"}, agentJobs)
	assert.Equal(t, []string{procmgrJob}, stableOnlyJobs)
}

// TestExperimentJobsMatchTheEmbeddedDefinitions guards the label lists against drifting apart
// from the definitions that ship: a job missing from configExperimentJobs would never be swapped,
// and would keep running the stable configuration for the whole life of an experiment.
func TestExperimentJobsMatchTheEmbeddedDefinitions(t *testing.T) {
	// The hooks never swap the installer daemon they run in, but an experiment does.
	assert.NotContains(t, agentJobs, installerJob)
	assert.Equal(t, append(append([]string{}, agentJobs...), installerJob), configExperimentJobs)
	// The procmgr daemon runs on its stable job throughout an experiment.
	assert.NotContains(t, configExperimentJobs, procmgrJob)
	assert.Equal(t, append([]string{procmgrJob}, configExperimentJobs...), stableJobs)

	for i, label := range configExperimentJobs {
		assert.Equal(t, label+"-exp", experimentJobs[i])
	}
	assert.Len(t, experimentJobs, len(configExperimentJobs))

	embeddedStable, err := embedded.LaunchdJobs(embedded.LaunchdStable)
	require.NoError(t, err)
	assert.ElementsMatch(t, embeddedStable, stableJobs)
	embeddedExperiment, err := embedded.LaunchdJobs(embedded.LaunchdExperiment)
	require.NoError(t, err)
	assert.ElementsMatch(t, embeddedExperiment, configExperimentJobs)
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
			if len(args) > 0 && args[0] == "bootstrap" && !strings.Contains(strings.Join(args, " "), "-exp") && !namesProcmgr(args) {
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
			if len(args) > 0 && args[0] == "bootstrap" && !strings.Contains(strings.Join(args, " "), "-exp") && !namesProcmgr(args) {
				bootstrapAttempts++
				return nil, errors.New("injected failure")
			}
			return nil, nil
		}
		return client
	}
	stubJobDir(t)
	stubDeadlinePath(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	err := configExperiment{jobs: agentJobSet()}.Stop(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("after %d attempts", configExperimentRestoreRetries))
	assert.Equal(t, len(agentJobs)*configExperimentRestoreRetries, bootstrapAttempts, "expected every label's retries to stop at the configured bound")

	// The failed restore is left for the next daemon start to retry, not forgotten.
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	assert.Equal(t, launchd.DeadlineReverting, token)
}

// TestRestoreStableClearsTheDeadlineOnceTheStableSetIsBack pins the other half: a restore that
// completes leaves no marker behind.
func TestRestoreStableClearsTheDeadlineOnceTheStableSetIsBack(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	require.NoError(t, configExperiment{jobs: agentJobSet()}.Stop(testHookContext(t)))

	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	assert.Empty(t, token)
}

// TestSettleRetriesAFailedRestore covers the backstop: a restore that gave up leaves the marker,
// and the next stable daemon start hands back every job that is not on its stable job yet.
func TestSettleRetriesAFailedRestore(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentWatcher(t)
	stubSurvivingHelpers(t, nil, nil)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().MarkReverting())

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))

	var started []string
	for _, label := range agentJobs {
		started = append(started, "system/"+label)
	}
	assert.ElementsMatch(t, started, loadedJobs(stableOnly(*calls)), "every stable agent job should be started again")
	assertFailedOnHost(t, dirs)
}

// namesProcmgr reports whether a recorded launchctl call is about the process manager, which the
// end of every experiment makes sure is loaded (ensureProcmgr): the stubs report it unloaded.
func namesProcmgr(args []string) bool {
	return strings.Contains(strings.Join(args, " "), procmgrJob)
}

// stableOnly keeps the recorded launchctl calls that name a stable job the experiment swaps.
func stableOnly(calls [][]string) [][]string {
	var kept [][]string
	for _, args := range calls {
		if !strings.Contains(strings.Join(args, " "), "-exp") && !namesProcmgr(args) {
			kept = append(kept, args)
		}
	}
	return kept
}

// TestRestoreStableDoesNotRewriteAnUpToDateStableDefinition pins that handing a label back to its
// stable job leaves an unchanged stable definition on disk alone, so a full disk cannot break a
// rollback.
func TestRestoreStableDoesNotRewriteAnUpToDateStableDefinition(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	for _, label := range agentJobs {
		definition, err := embedded.GetLaunchdJob(label, launchd.Stable)
		require.NoError(t, err)
		require.NoError(t, launchd.Job{Label: label, Dir: dir}.Write(definition))
	}
	before := map[string]os.FileInfo{}
	for _, label := range agentJobs {
		info, err := os.Stat(filepath.Join(dir, label+".plist"))
		require.NoError(t, err)
		before[label] = info
	}

	require.NoError(t, configExperiment{jobs: agentJobSet()}.Stop(testHookContext(t)))

	for _, label := range agentJobs {
		info, err := os.Stat(filepath.Join(dir, label+".plist"))
		require.NoError(t, err)
		assert.True(t, os.SameFile(before[label], info), "%s should not have been rewritten", label)
	}
}

// TestHandBackProceedsAfterAFailedRestore pins that the reverting marker a failed restore leaves
// is not mistaken for a newer experiment: the daemon is still handed back to its stable job, whose
// next start finishes the revert.
func TestHandBackProceedsAfterAFailedRestore(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	require.NoError(t, experimentDeadline().MarkReverting())

	require.NoError(t, handBackInstaller(context.Background()))

	assert.Contains(t, loadedJobs(*calls), "system/"+installerJob)
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
		if args[0] == "bootstrap" && !strings.Contains(args[len(args)-1], "-exp") && !namesProcmgr(args) {
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
	handOverCalls := stubInstallerHandOver(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	err := postStartConfigExperimentDatadogAgent(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected failure")
	assert.Zero(t, *handOverCalls, "a hand-over was launched for an experiment that never started")
	assert.Empty(t, experimentDeploymentID(t, dirs), "the experiment configuration survived a failed start")
}

// TestStartConfigExperimentDiscardsTheConfigWhenTheHandOverFailsToLaunch is the same guarantee for
// the other revert in the post-start hook: with no hand-over, no experiment daemon would ever
// launch the watcher.
func TestStartConfigExperimentDiscardsTheConfigWhenTheHandOverFailsToLaunch(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	original := launchInstallerHandOver
	launchInstallerHandOver = func(context.Context) error { return errors.New("injected failure") }
	t.Cleanup(func() { launchInstallerHandOver = original })
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	err := postStartConfigExperimentDatadogAgent(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hand-over failed to launch, experiment reverted")
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

// TestSettleDiscardsAnExperimentDeployedWithNoDeadline covers a daemon that died between the
// installer publishing the experiment directory and Start writing the deadline (or partway through
// a deliberate stop, which clears the deadline first). Nothing will ever end that experiment, so it
// is reverted and discarded instead of being reported as running for good.
func TestSettleDiscardsAnExperimentDeployedWithNoDeadline(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	stubSurvivingHelpers(t, nil, nil)
	handOverCalls := stubInstallerHandOver(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Empty(t, experimentDeploymentID(t, dirs), "an orphaned experiment configuration was left deployed")
	assert.Zero(t, *handOverCalls, "an orphaned experiment was handed over instead of reverted")
	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(dir, label+"-exp.plist"))
		assert.Contains(t, launchctlCalls(*calls, "bootstrap"), filepath.Join(dir, label+".plist"), "the stable job %s was not restored", label)
	}
}

// TestParseHelperPIDs pins which processes a daemon start stops as a surviving watcher: only root-owned
// processes running exactly the installer's watcher command, and never the caller itself. A shell
// or pgrep whose command line merely contains the watcher's arguments must not match.
func TestParseHelperPIDs(t *testing.T) {
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

	assert.Equal(t, []int{101, 108}, parseHelperPIDs(ps, 999, watchConfigExperimentCommand))
	assert.Equal(t, []int{101}, parseHelperPIDs(ps, 108, watchConfigExperimentCommand), "the caller must never be matched")
	assert.Empty(t, parseHelperPIDs("", 999, watchConfigExperimentCommand))

	helpers := ps + strings.Join([]string{
		"  201     0 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent handOverInstaller",
		"  202     0 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent handBackInstaller",
		"  203   450 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent handOverInstaller",
		"",
	}, "\n")
	assert.Equal(t, []int{101, 108}, parseHelperPIDs(helpers, 999, watchConfigExperimentCommand), "a watcher lookup must not match the hand-over or hand-back")
	assert.Equal(t, []int{101, 108, 201, 202}, parseHelperPIDs(helpers, 999, experimentHelperCommands...))
}

// TestSettleStopsTheSurvivingHelpersBeforeTouchingTheJobs covers a stable daemon start in the
// middle of an experiment. A previous watcher outlives the daemon and is armed on the experiment
// pids, so when the settle hands the jobs back it would see them terminate, take that for a crash
// and revert on top of the settle. It has to be gone before the first job is touched, and the
// experiment is failed rather than brought back.
func TestSettleStopsTheSurvivingHelpersBeforeTouchingTheJobs(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	handOverCalls := stubInstallerHandOver(t)
	watcherCalls := stubConfigExperimentWatcher(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	stopped, callsAtStop := stubSurvivingHelpers(t, calls, nil)

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, 1, *stopped, "the previous helpers should be stopped exactly once")
	assert.Zero(t, *callsAtStop, "the settle touched launchd before it stopped the previous helpers")
	assert.Zero(t, *handOverCalls, "the stable daemon must not hand the installer daemon over again")
	assert.Zero(t, *watcherCalls, "the stable daemon must not launch a watcher")
	for _, target := range launchctlCalls(*calls, "kickstart") {
		assert.NotContains(t, target, "-exp", "the experiment must not be brought back")
	}
	assertFailedOnHost(t, dirs)
}

// TestSettleStillSettlesWhenTheHelpersCannotBeStopped pins that failing to find or stop the
// previous helpers is not fatal: a stale watcher at worst reverts the experiment again, which is
// safe, whereas giving up would leave the host off its stable set.
func TestSettleStillSettlesWhenTheHelpersCannotBeStopped(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	handOverCalls := stubInstallerHandOver(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	stubSurvivingHelpers(t, nil, errors.New("injected failure"))

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Zero(t, *handOverCalls)
	assertFailedOnHost(t, dirs)
}

// TestSettleLeavesARestingHostAlone pins the common case: no deadline and no experiment means
// nothing to settle, so the stable daemon start must not touch any job or look for helpers.
func TestSettleLeavesARestingHostAlone(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stopped, _ := stubSurvivingHelpers(t, nil, nil)
	dirs := stubConfigExperimentDirs(t)

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Empty(t, *calls, "the settle touched launchd on a host with no experiment")
	assert.Zero(t, *stopped, "the settle looked for helpers on a host with no experiment")
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
	stubDeadlinePath(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	stale, err := deadline.Read()
	require.NoError(t, err)
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))

	reverted, err := revertExperimentIfStillPending(context.Background(), configExperimentJobSet(), deadline, stale, "stale exit")
	require.NoError(t, err)
	assert.False(t, reverted)
	assert.Empty(t, *calls, "a stale watcher touched launchd")
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs))
}

// stubConfigExperimentLock points the installer's lock at a temporary database.
func stubConfigExperimentLock(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "packages.db")
	original := configExperimentLockPath
	configExperimentLockPath = path
	t.Cleanup(func() { configExperimentLockPath = original })
	return path
}

// holdInstallerLock opens the lock database the way an installer command does, and returns the
// function that releases it.
func holdInstallerLock(t *testing.T, path string) func() {
	t.Helper()

	lock, err := db.New(context.Background(), path)
	require.NoError(t, err)
	var once sync.Once
	release := func() { once.Do(func() { require.NoError(t, lock.Close()) }) }
	t.Cleanup(release)
	return release
}

// TestWatcherRevertWaitsForARunningInstallerCommand covers a revert that fires while an installer
// command, here a promote, is running: it must wait for the command, then find the experiment
// already handled and leave it alone.
func TestWatcherRevertWaitsForARunningInstallerCommand(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	release := holdInstallerLock(t, stubConfigExperimentLock(t))

	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	token, err := deadline.Read()
	require.NoError(t, err)

	type result struct {
		reverted bool
		err      error
	}
	done := make(chan result, 1)
	go func() {
		reverted, err := revertFromWatcher(context.Background(), deadline, token, "experiment pid 1 exited")
		done <- result{reverted, err}
	}()

	select {
	case <-done:
		t.Fatal("the revert ran while an installer command held the lock")
	case <-time.After(100 * time.Millisecond):
	}

	// The promote's hook clears the deadline before the command exits.
	require.NoError(t, deadline.Clear())
	release()

	r := <-done
	require.NoError(t, r.err)
	assert.False(t, r.reverted)
	assert.Empty(t, *calls, "a watcher that waited for the installer touched launchd")
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs))
}

// TestWatcherRevertTakesTheInstallerLock covers a revert that wins the race: it reverts under the
// lock and releases it, and a promote that comes in afterwards finds nothing to promote.
func TestWatcherRevertTakesTheInstallerLock(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	lockPath := stubConfigExperimentLock(t)

	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	token, err := deadline.Read()
	require.NoError(t, err)

	reverted, err := revertFromWatcher(context.Background(), deadline, token, "experiment deadline expired")
	require.NoError(t, err)
	assert.True(t, reverted)
	assertFailedOnHost(t, dirs)

	lock, err := db.New(context.Background(), lockPath, db.WithTimeout(time.Second))
	require.NoError(t, err, "the watcher kept the installer lock after reverting")
	require.NoError(t, lock.Close())
}

// TestWatcherStopsWaitingWhenTheExperimentIsReplaced covers a watcher stuck behind an installer
// command that replaces its experiment: it must give up without the lock rather than revert the
// newer experiment once the command exits.
func TestWatcherStopsWaitingWhenTheExperimentIsReplaced(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	holdInstallerLock(t, stubConfigExperimentLock(t))
	original := installerLockPoll
	installerLockPoll = 20 * time.Millisecond
	t.Cleanup(func() { installerLockPoll = original })

	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	stale, err := deadline.Read()
	require.NoError(t, err)
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))

	reverted, err := revertFromWatcher(context.Background(), deadline, stale, "stale exit")
	require.NoError(t, err)
	assert.False(t, reverted)
	assert.Empty(t, *calls, "a superseded watcher touched launchd")
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs))
}

// TestStopAndPromoteHandTheInstallerBackOnlyWhenTheExperimentDaemonIsLoaded covers who hands the
// installer daemon back. The hooks may run inside the daemon's job, so they must never swap it
// themselves; with the experiment daemon loaded they launch the detached hand-back, and with only
// the stable daemon there is nothing to hand back.
func TestStopAndPromoteHandTheInstallerBackOnlyWhenTheExperimentDaemonIsLoaded(t *testing.T) {
	hooks := map[string]func(HookContext) error{
		"stop":    preStopConfigExperimentDatadogAgent,
		"promote": postPromoteConfigExperimentDatadogAgent,
	}
	for name, hook := range hooks {
		for _, experimentDaemon := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/experimentDaemon=%t", name, experimentDaemon), func(t *testing.T) {
				var calls *[][]string
				if experimentDaemon {
					calls = stubLaunchdWithExperimentDaemon(t, "", "")
				} else {
					calls = stubLaunchd(t)
				}
				stubJobDir(t)
				stubDeadlinePath(t)
				stubConfigExperimentWatcher(t)
				handBacks := stubInstallerHandBack(t, nil)
				require.NoError(t, postStartConfigExperimentDatadogAgent(testHookContext(t)))
				*calls = nil

				require.NoError(t, hook(testHookContext(t)))
				assert.False(t, touchesInstaller(*calls), "the hook swapped the installer daemon it runs in: %v", *calls)
				if experimentDaemon {
					assert.Equal(t, 1, *handBacks)
				} else {
					assert.Zero(t, *handBacks)
				}
			})
		}
	}
}

// TestStopHandsTheInstallerBackEvenWhenTheAgentCannotBeRestored pins that a failed restore still
// brings the stable daemon back: its settle is what reverts an experiment left half torn down.
func TestStopHandsTheInstallerBackEvenWhenTheAgentCannotBeRestored(t *testing.T) {
	stubLaunchdWithExperimentDaemon(t, "bootstrap", "")
	stubJobDir(t)
	stubDeadlinePath(t)
	original := configExperimentRestoreBackoff
	configExperimentRestoreBackoff = 0
	t.Cleanup(func() { configExperimentRestoreBackoff = original })
	handBacks := stubInstallerHandBack(t, nil)

	assert.Error(t, preStopConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, 1, *handBacks)
}

// TestStopAndPromoteSucceedWhenTheHandBackFailsToLaunch pins that the installer daemon's hand-back
// does not decide a stop or a promote: by then the Agent is back on the stable set, and failing the
// hook would keep the installer from discarding or keeping the configuration directory.
func TestStopAndPromoteSucceedWhenTheHandBackFailsToLaunch(t *testing.T) {
	for name, hook := range map[string]func(HookContext) error{
		"stop":    preStopConfigExperimentDatadogAgent,
		"promote": postPromoteConfigExperimentDatadogAgent,
	} {
		t.Run(name, func(t *testing.T) {
			stubLaunchdWithExperimentDaemon(t, "", "")
			stubJobDir(t)
			stubDeadlinePath(t)
			handBacks := stubInstallerHandBack(t, errors.New("injected failure"))

			assert.NoError(t, hook(testHookContext(t)))
			assert.Equal(t, 1, *handBacks, "the hand-back should still have been tried")
		})
	}
}

// TestHandBackWaitsForTheInstallerCommandThenRestoresTheStableDaemon covers the hand-back: it must
// not stop the experiment daemon while the command that ran the stop or promote hook is still
// running, then swap it back to its stable job and leave no experiment definition behind.
func TestHandBackWaitsForTheInstallerCommandThenRestoresTheStableDaemon(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	release := holdInstallerLock(t, stubConfigExperimentLock(t))
	original := installerLockPoll
	installerLockPoll = 20 * time.Millisecond
	t.Cleanup(func() { installerLockPoll = original })
	experimentPlist := filepath.Join(dir, installerJob+"-exp.plist")
	require.NoError(t, os.WriteFile(experimentPlist, []byte("leftover"), 0644))

	done := make(chan error, 1)
	go func() { done <- handBackInstaller(context.Background()) }()

	select {
	case <-done:
		t.Fatal("the hand-back ran while an installer command held the lock")
	case <-time.After(100 * time.Millisecond):
	}
	assert.Empty(t, *calls, "the hand-back touched launchd while an installer command held the lock")

	release()
	require.NoError(t, <-done)
	bootedOut := launchctlCalls(*calls, "bootout")
	require.NotEmpty(t, bootedOut)
	assert.Equal(t, "system/"+installerJob+"-exp", bootedOut[0])
	assert.Contains(t, loadedJobs(*calls), "system/"+installerJob)
	assert.NoFileExists(t, experimentPlist)
	assert.FileExists(t, filepath.Join(dir, installerJob+".plist"))
}

// TestHandBackLeavesTheDaemonToANewerExperiment covers an experiment daemon that started a new
// experiment before the hand-back got the lock: that experiment's watcher owns the daemon now.
func TestHandBackLeavesTheDaemonToANewerExperiment(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	require.NoError(t, handBackInstaller(context.Background()))
	assert.Empty(t, *calls, "the hand-back touched launchd during a newer experiment")
}

// TestHandOverWaitsForTheInstallerCommandThenStartsTheExperimentDaemon covers the watcher's first
// step: it must not stop the stable daemon while the command that ran the start hook is still
// running, then swap it to its experiment job, stable first so the two never run at once.
func TestHandOverWaitsForTheInstallerCommandThenStartsTheExperimentDaemon(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	release := holdInstallerLock(t, stubConfigExperimentLock(t))
	original := installerLockPoll
	installerLockPoll = 20 * time.Millisecond
	t.Cleanup(func() { installerLockPoll = original })

	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	token, err := deadline.Read()
	require.NoError(t, err)

	stubWatcherRunning(t, true)
	done := make(chan error, 1)
	go func() {
		done <- handOverInstaller(context.Background(), deadline, token)
	}()

	select {
	case <-done:
		t.Fatal("the hand-over ran while an installer command held the lock")
	case <-time.After(100 * time.Millisecond):
	}
	assert.Empty(t, *calls, "the hand-over touched launchd while an installer command held the lock")

	release()
	require.NoError(t, <-done)
	bootedOut := launchctlCalls(*calls, "bootout")
	require.NotEmpty(t, bootedOut)
	assert.Equal(t, "system/"+installerJob, bootedOut[0], "the stable daemon must leave before its experiment twin starts")
	assert.Equal(t, []string{"system/" + installerJob + "-exp"}, launchctlCalls(*calls, "kickstart"))
	assert.FileExists(t, filepath.Join(dir, installerJob+"-exp.plist"))
	assert.False(t, touchesAgent(*calls), "the hand-over touched an agent job: %v", *calls)
}

// touchesAgent reports whether any recorded launchctl call names an agent job.
func touchesAgent(calls [][]string) bool {
	for _, args := range calls {
		for _, argument := range args {
			for _, label := range agentJobs {
				if strings.Contains(argument, label) {
					return true
				}
			}
		}
	}
	return false
}

// TestHandOverIsANoopWhenTheExperimentIsOver covers a watcher whose experiment was stopped while
// it waited for the start command: there is no daemon to hand over.
func TestHandOverIsANoopWhenTheExperimentIsOver(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)

	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	token, err := deadline.Read()
	require.NoError(t, err)
	require.NoError(t, deadline.Clear())

	require.NoError(t, handOverInstaller(context.Background(), deadline, token))
	assert.Empty(t, *calls)
}

// TestHandOverRevertsEverythingWhenTheSwapFails covers a hand-over that leaves the host with no
// daemon: the watcher reverts every job, the daemon included, and discards the experiment.
func TestHandOverRevertsEverythingWhenTheSwapFails(t *testing.T) {
	calls := stubLaunchdFailing(t, "bootstrap", installerJob+"-exp")
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)

	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	token, err := deadline.Read()
	require.NoError(t, err)

	require.NoError(t, handOverInstaller(context.Background(), deadline, token))
	assertFailedOnHost(t, dirs)
	loaded := loadedJobs(*calls)
	for _, label := range configExperimentJobs {
		assert.Contains(t, loaded, "system/"+label, "the stable job %s was not restored", label)
	}
}

// TestResumeLaunchesTheWatcherFromTheExperimentDaemon covers the experiment daemon starting, which
// runs the resume hook like any daemon start: it launches the watcher that supervises the
// experiment, after stopping a previous one, and leaves the experiment itself alone -- resuming
// would bounce the jobs the watcher is about to supervise.
func TestResumeLaunchesTheWatcherFromTheExperimentDaemon(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	watcherCalls := stubConfigExperimentWatcher(t)
	handOverCalls := stubInstallerHandOver(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	stopped := 0
	original := stopSurvivingWatchers
	stopSurvivingWatchers = func(context.Context) error { stopped++; return nil }
	t.Cleanup(func() { stopSurvivingWatchers = original })
	t.Setenv(experimentDaemonEnv, "true")

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, 1, *watcherCalls, "the experiment daemon should launch the watcher")
	assert.Equal(t, 1, stopped, "a previous watcher must be stopped before the new one is launched")
	assert.Zero(t, *handOverCalls, "the experiment daemon must not hand anything over")
	assert.Empty(t, *calls, "the experiment daemon's resume touched launchd")
	assert.Equal(t, "experiment-1", experimentDeploymentID(t, dirs))
}

// TestResumeFromTheExperimentDaemonLaunchesTheWatcherWhenTheDeadlineCannotBeRead pins that an
// unreadable deadline does not leave the experiment without a watcher: the watcher is the one that
// reverts an experiment nobody can tell the state of.
func TestResumeFromTheExperimentDaemonLaunchesTheWatcherWhenTheDeadlineCannotBeRead(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	path := stubDeadlinePath(t)
	watcherCalls := stubConfigExperimentWatcher(t)
	stubInstallerHandBack(t, nil)
	require.NoError(t, os.Mkdir(path, 0o755))
	t.Setenv(experimentDaemonEnv, "true")

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Equal(t, 1, *watcherCalls)
}

// TestResumeFromTheExperimentDaemonHandsItBackWhenNoExperimentIsPending covers an experiment that
// ended while the daemon was being handed over to it: it has nothing to run under, so it launches
// its own hand-back, and no watcher.
func TestResumeFromTheExperimentDaemonHandsItBackWhenNoExperimentIsPending(t *testing.T) {
	for name, setUp := range map[string]func(t *testing.T){
		"no deadline": func(*testing.T) {},
		"reverting": func(t *testing.T) {
			require.NoError(t, experimentDeadline().MarkReverting())
		},
	} {
		t.Run(name, func(t *testing.T) {
			stubLiveLaunchd(t, installerJob+"-exp")
			stubJobDir(t)
			stubDeadlinePath(t)
			watcherCalls := stubConfigExperimentWatcher(t)
			handBackCalls := stubInstallerHandBack(t, nil)
			setUp(t)
			t.Setenv(experimentDaemonEnv, "true")

			require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
			assert.Zero(t, *watcherCalls, "a watcher was launched for an experiment that is over")
			assert.Equal(t, 1, *handBackCalls, "the experiment daemon should have itself handed back")
		})
	}
}

// TestResumeFromTheExperimentDaemonRevertsWhenTheWatcherCannotBeLaunched pins that an experiment
// nobody can supervise is not left running: the agent jobs are reverted from the hook, the
// experiment configuration discarded, and the daemon handed back, since a hook cannot swap the
// daemon it runs in.
func TestResumeFromTheExperimentDaemonRevertsWhenTheWatcherCannotBeLaunched(t *testing.T) {
	running := []string{installerJob + "-exp"}
	for _, label := range agentJobs {
		running = append(running, label+"-exp")
	}
	l := stubLiveLaunchd(t, running...)
	stubJobDir(t)
	stubDeadlinePath(t)
	shortRestoreBackoff(t)
	handBackCalls := stubInstallerHandBack(t, nil)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	original := launchConfigExperimentWatcher
	launchConfigExperimentWatcher = func(context.Context) error { return errors.New("injected failure") }
	t.Cleanup(func() { launchConfigExperimentWatcher = original })
	stopOriginal := stopSurvivingWatchers
	stopSurvivingWatchers = func(context.Context) error { return nil }
	t.Cleanup(func() { stopSurvivingWatchers = stopOriginal })
	t.Setenv(experimentDaemonEnv, "true")

	err := resumeConfigExperimentDatadogAgent(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "watcher failed to launch, experiment reverted")
	for _, label := range agentJobs {
		assert.True(t, l.isLoaded(label), "%s should be back on its stable job", label)
		assert.False(t, l.isLoaded(label+"-exp"), "%s should be unloaded", label+"-exp")
	}
	assertFailedOnHost(t, dirs)
	assert.Equal(t, 1, *handBackCalls, "the daemon should be handed back")
}

// TestSettleNeverSwapsTheStableDaemon covers the stable daemon settling an experiment after a
// reboot: it runs in the installer's stable job, so it hands the agent jobs back only, and launches
// no hand-over.
func TestSettleNeverSwapsTheStableDaemon(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubSurvivingHelpers(t, nil, nil)
	handOverCalls := stubInstallerHandOver(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assert.Zero(t, *handOverCalls)
	assert.False(t, touchesInstaller(*calls), "the stable daemon's settle swapped itself: %v", *calls)
	assertFailedOnHost(t, dirs)
}

// TestSettleRemovesTheExperimentDaemonDefinition covers a reboot mid-experiment: the settle also
// removes the experiment daemon's definition the reboot left on disk, without swapping the stable
// daemon it runs in.
func TestSettleRemovesTheExperimentDaemonDefinition(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	stubDeadlinePath(t)
	stubSurvivingHelpers(t, nil, nil)
	stubConfigExperimentWatcher(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	experimentPlist := filepath.Join(dir, installerJob+"-exp.plist")
	require.NoError(t, os.WriteFile(experimentPlist, []byte("leftover"), 0644))

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))
	assertFailedOnHost(t, dirs)
	assert.NoFileExists(t, experimentPlist)
	assert.False(t, touchesInstaller(*calls), "the stable daemon's settle swapped itself: %v", *calls)
}

// rebootedHost lays out the jobs a reboot leaves running: launchd started every stable job at boot
// (RunAtLoad), and none of the experiment jobs, whose definitions have no RunAtLoad.
func rebootedHost(t *testing.T) *liveLaunchd {
	t.Helper()

	running := []string{procmgrJob, installerJob}
	running = append(running, agentJobs...)
	l := stubLiveLaunchd(t, running...)
	stubJobDir(t)
	stubDeadlinePath(t)
	shortRestoreBackoff(t)
	stubSurvivingHelpers(t, nil, nil)
	return l
}

// assertNoStableJobRestarted checks that the settle left every stable agent job launchd started at
// boot alone, and no experiment job loaded.
func assertNoStableJobRestarted(t *testing.T, l *liveLaunchd) {
	t.Helper()

	l.mu.Lock()
	defer l.mu.Unlock()
	for _, label := range agentJobs {
		assert.True(t, l.loaded["system/"+label], "%s should still be on its stable job", label)
		assert.False(t, l.loaded["system/"+label+"-exp"], "%s should not be loaded", label+"-exp")
		for _, args := range l.calls {
			if args[0] == "bootout" || args[0] == "bootstrap" || args[0] == "kickstart" {
				assert.NotEqual(t, "system/"+label, args[len(args)-1], "the settle restarted %s: %v", label, args)
				assert.NotContains(t, args[len(args)-1], "/"+label+".plist", "the settle restarted %s: %v", label, args)
			}
		}
	}
}

// TestSettleOutcomes pins what the stable daemon leaves after a restart, for every deadline the
// previous daemon can have left beside a deployed experiment configuration or none: an unfinished
// experiment fails for Fleet to stop, or is finished as a promote once its configuration is
// promoted; a failed one waits for Fleet; one with no deadline is discarded. Whatever the outcome,
// no stable job launchd has just started is restarted, and the experiment is never brought back.
func TestSettleOutcomes(t *testing.T) {
	const (
		failedForFleet = "failed, kept for Fleet to stop"
		promoted       = "promoted"
		discarded      = "discarded"
		resting        = "resting"
	)
	deadlines := map[string]func(t *testing.T, path string){
		"deadline": func(t *testing.T, _ string) {
			require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
		},
		"expired deadline": func(t *testing.T, _ string) {
			require.NoError(t, experimentDeadline().Write(-time.Minute))
		},
		"reverting": func(t *testing.T, _ string) {
			require.NoError(t, experimentDeadline().MarkReverting())
		},
		"failed": func(t *testing.T, _ string) {
			require.NoError(t, experimentDeadline().MarkFailed())
		},
		"none": func(*testing.T, string) {},
		"unreadable": func(t *testing.T, path string) {
			require.NoError(t, os.Mkdir(path, 0o755))
		},
	}
	for _, tc := range []struct {
		deadline string
		deployed bool
		want     string
	}{
		{"deadline", true, failedForFleet},
		{"expired deadline", true, failedForFleet},
		{"reverting", true, failedForFleet},
		{"failed", true, failedForFleet},
		{"none", true, discarded},
		// Fail falls back to discarding an experiment it cannot mark as failed.
		{"unreadable", true, discarded},
		{"deadline", false, promoted},
		{"expired deadline", false, promoted},
		{"reverting", false, promoted},
		{"failed", false, resting},
		{"none", false, resting},
		{"unreadable", false, resting},
	} {
		t.Run(fmt.Sprintf("%s/deployed=%t", tc.deadline, tc.deployed), func(t *testing.T) {
			l := rebootedHost(t)
			handOverCalls := stubInstallerHandOver(t)
			watcherCalls := stubConfigExperimentWatcher(t)
			dirs := stubConfigExperimentDirs(t)
			if tc.deployed {
				deployExperimentConfig(t, dirs)
			}
			deadlines[tc.deadline](t, configExperimentDeadlinePath)

			require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))

			assert.Zero(t, *handOverCalls, "the experiment must not be handed over again")
			assert.Zero(t, *watcherCalls, "the stable daemon must not launch a watcher")
			assertNoStableJobRestarted(t, l)
			token, err := experimentDeadline().Read()
			require.NoError(t, err)
			state, err := dirs.GetState()
			require.NoError(t, err)
			switch tc.want {
			case failedForFleet:
				assertFailedOnHost(t, dirs)
			case promoted:
				assert.Empty(t, token)
				assert.Empty(t, state.ExperimentDeploymentID)
			case discarded:
				assert.Empty(t, token)
				assert.Equal(t, config.State{StableDeploymentID: "stable-1"}, state)
			case resting:
				assert.Empty(t, token)
				assert.Equal(t, config.State{StableDeploymentID: "stable-1"}, state)
			}
		})
	}
}

// TestSettleUnloadsTheIdleExperimentDaemon covers the experiment daemon's job after a reboot: launchd
// loaded its definition at boot without starting it. The settle unloads and removes it, and has
// nothing to hand back.
func TestSettleUnloadsTheIdleExperimentDaemon(t *testing.T) {
	l := rebootedHost(t)
	l.loaded["system/"+installerJob+"-exp"] = true
	l.idle["system/"+installerJob+"-exp"] = true
	handBackCalls := stubInstallerHandBack(t, nil)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().MarkFailed())
	experimentPlist := filepath.Join(launchdJobDir, installerJob+"-exp.plist")
	require.NoError(t, os.WriteFile(experimentPlist, []byte("leftover"), 0644))

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))

	assert.False(t, l.isLoaded(installerJob+"-exp"), "the idle experiment daemon should be unloaded")
	assert.True(t, l.isLoaded(installerJob), "the stable daemon must stay loaded")
	assert.NoFileExists(t, experimentPlist)
	assert.Zero(t, *handBackCalls, "an idle experiment daemon has nothing to hand back")
	assertFailedOnHost(t, dirs)
}

// TestSettleHandsBackARunningExperimentDaemon covers a stable daemon started by hand while the
// experiment daemon still runs: the settle cannot swap the daemon it runs in, so it launches the
// hand-back, which unloads the experiment daemon and removes its definition.
func TestSettleHandsBackARunningExperimentDaemon(t *testing.T) {
	l := rebootedHost(t)
	l.loaded["system/"+installerJob+"-exp"] = true
	handBackCalls := stubInstallerHandBack(t, nil)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	experimentPlist := filepath.Join(launchdJobDir, installerJob+"-exp.plist")
	require.NoError(t, os.WriteFile(experimentPlist, []byte("running"), 0644))

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))

	assert.Equal(t, 1, *handBackCalls, "the running experiment daemon should be handed back")
	assert.True(t, l.isLoaded(installerJob+"-exp"), "the settle must leave the swap to the hand-back")
	assert.FileExists(t, experimentPlist, "the hand-back removes the definition once it has swapped")
	assertFailedOnHost(t, dirs)
}

// TestRestoreStableRestartsLoadedStableJobsOutsideTheSettle pins that only the settle leaves a
// running stable job alone: a stop or promote hands back every label, so a promote restarts each
// stable job onto the promoted configuration.
func TestRestoreStableRestartsLoadedStableJobsOutsideTheSettle(t *testing.T) {
	l := rebootedHost(t)

	require.NoError(t, configExperiment{jobs: agentJobSet()}.Promote(testHookContext(t)))

	for _, label := range agentJobs {
		assert.Contains(t, launchctlCalls(l.calls, "bootout"), "system/"+label, "the promote should restart %s", label)
	}
}

// TestInstallerSwapsOutwaitTheDaemonsExitTimeOut pins that a bootout of the installer daemon waits
// longer than the ExitTimeOut launchd gives it: the daemon finishes the request it is handling
// before it exits, so it can leave the domain only once launchd kills it, and a wait of the same
// length gives up just before.
func TestInstallerSwapsOutwaitTheDaemonsExitTimeOut(t *testing.T) {
	stubLaunchd(t)
	definition, err := embedded.GetLaunchdJob(installerJob, embedded.LaunchdStable)
	require.NoError(t, err)
	require.Contains(t, string(definition), "<key>ExitTimeOut</key>\n        <integer>10</integer>")

	for _, jobs := range []launchd.JobSet{installerJobSet(), configExperimentJobSet()} {
		assert.Greater(t, jobs.Client.BootoutSettleTimeout, 10*time.Second)
	}
}

// stubLaunchdOnBootout behaves like stubLaunchd, and calls onBootout whenever target is booted out.
func stubLaunchdOnBootout(t *testing.T, target string, onBootout func()) *[][]string {
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
			if len(args) > 0 && args[0] == "bootout" && args[len(args)-1] == target {
				onBootout()
			}
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	return &calls
}

// lockIsFree reports whether the installer's lock can be taken right now, and releases it again.
func lockIsFree(t *testing.T, path string) bool {
	t.Helper()

	lock, err := db.New(context.Background(), path, db.WithTimeout(100*time.Millisecond))
	if err != nil {
		return false
	}
	require.NoError(t, lock.Close())
	return true
}

// TestHandOverStopsTheDaemonWithTheLockReleased pins what keeps the start task from being reported
// as running forever: once the start command exits, the daemon still finishes the request through
// an installer command that takes the same lock, so the hand-over must not hold it while stopping
// the daemon.
func TestHandOverStopsTheDaemonWithTheLockReleased(t *testing.T) {
	lockPath := stubConfigExperimentLock(t)
	var checked, free bool
	stubLaunchdOnBootout(t, "system/"+installerJob, func() {
		checked = true
		free = lockIsFree(t, lockPath)
	})
	stubJobDir(t)
	stubDeadlinePath(t)
	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	token, err := deadline.Read()
	require.NoError(t, err)

	stubWatcherRunning(t, true)
	require.NoError(t, handOverInstaller(context.Background(), deadline, token))
	require.True(t, checked, "the stable daemon was never booted out")
	assert.True(t, free, "the stable daemon was stopped while the hand-over held the installer lock")
}

// TestHandBackStopsTheDaemonWithTheLockReleased is TestHandOverStopsTheDaemonWithTheLockReleased
// for the hand-back: the stop or promote task must be able to finish before the experiment daemon
// is stopped.
func TestHandBackStopsTheDaemonWithTheLockReleased(t *testing.T) {
	lockPath := stubConfigExperimentLock(t)
	var checked, free bool
	stubLaunchdOnBootout(t, "system/"+installerJob+"-exp", func() {
		checked = true
		free = lockIsFree(t, lockPath)
	})
	stubJobDir(t)
	stubDeadlinePath(t)

	require.NoError(t, handBackInstaller(context.Background()))
	require.True(t, checked, "the experiment daemon was never booted out")
	assert.True(t, free, "the experiment daemon was stopped while the hand-back held the installer lock")
}

// TestHandOverHandsBackAnExperimentStoppedDuringTheSwap covers the window the released lock opens:
// a stop that clears the deadline while the daemon is being handed over leaves nothing for the
// experiment daemon to run, so it is handed straight back.
func TestHandOverHandsBackAnExperimentStoppedDuringTheSwap(t *testing.T) {
	stubConfigExperimentLock(t)
	stubDeadlinePath(t)
	deadline := experimentDeadline()
	calls := stubLaunchdOnBootout(t, "system/"+installerJob, func() {
		require.NoError(t, deadline.Clear())
	})
	stubJobDir(t)
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	token, err := deadline.Read()
	require.NoError(t, err)

	stubWatcherRunning(t, false)
	require.NoError(t, handOverInstaller(context.Background(), deadline, token))
	loaded := loadedJobs(*calls)
	require.NotEmpty(t, loaded)
	assert.Equal(t, "system/"+installerJob, loaded[len(loaded)-1], "the stable daemon should be running again")
}

// TestHandOverWaitsForTheExperimentDaemonToLaunchTheWatcher pins that the hand-over only ends once
// a watcher supervises the experiment, so an experiment never runs without one.
func TestHandOverWaitsForTheExperimentDaemonToLaunchTheWatcher(t *testing.T) {
	stubLiveLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	originalPoll, originalRunning := handOverWatcherPoll, watcherRunning
	handOverWatcherPoll = time.Millisecond
	polls := 0
	watcherRunning = func(context.Context) (bool, error) {
		polls++
		return polls > 3, nil
	}
	t.Cleanup(func() { handOverWatcherPoll, watcherRunning = originalPoll, originalRunning })
	deadline := experimentDeadline()
	require.NoError(t, deadline.Write(configExperimentDeadlineWindow))
	token, err := deadline.Read()
	require.NoError(t, err)

	require.NoError(t, handOverInstaller(context.Background(), deadline, token))
	assert.Equal(t, 4, polls, "the hand-over should wait until the watcher is running")
}

// TestInstallerHandOverRevertsWhenNoWatcherAppears covers an experiment daemon that never launches
// the watcher -- it crashed, or could not run its resume command: the hand-over reverts every job,
// the daemon included, and discards the experiment, rather than leave it without a supervisor.
func TestInstallerHandOverRevertsWhenNoWatcherAppears(t *testing.T) {
	l := stubLiveLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	shortFailSafeBackoff(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	stubWatcherRunning(t, false)
	originalTimeout, originalPoll := handOverWatcherTimeout, handOverWatcherPoll
	handOverWatcherTimeout, handOverWatcherPoll = 30*time.Millisecond, time.Millisecond
	t.Cleanup(func() { handOverWatcherTimeout, handOverWatcherPoll = originalTimeout, originalPoll })
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	err := runInstallerHandOver(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not launch a watcher")
	assertFailedOnHost(t, dirs)
	for _, label := range configExperimentJobs {
		assert.True(t, l.isLoaded(label), "the stable job %s was not restored", label)
		assert.False(t, l.isLoaded(label+"-exp"), "%s should be unloaded", label+"-exp")
	}
}

// TestInstallerHandOverRevertsAtOnceWhenTheExperimentDaemonExits covers an experiment daemon that
// cannot start under the experiment configuration -- here an invalid value, which the Agent and
// the daemon both read: no watcher will ever be launched, so the hand-over must not wait out its
// timeout before reverting. The timeout is an hour, so a pass proves the revert came from the
// daemon's exit.
func TestInstallerHandOverRevertsAtOnceWhenTheExperimentDaemonExits(t *testing.T) {
	daemonGone := false
	calls := stubLaunchdScript(t, func(args []string) ([]byte, error, bool) {
		target := args[len(args)-1]
		if target != "system/"+installerJob+"-exp" {
			return nil, nil, false
		}
		switch args[0] {
		case "bootout":
			daemonGone = true
			return nil, nil, true
		case "print":
			if daemonGone {
				return []byte(notLoadedOutput), exitError(113), true
			}
			// Loaded, and exited with a status launchd recorded: the daemon crashed at startup.
			return []byte("system/com.datadoghq.installer-exp = {\n\tstate = not running\n\tlast exit code = 1\n}\n"), nil, true
		}
		return nil, nil, false
	})
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	shortFailSafeBackoff(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	stubWatcherRunning(t, false)
	originalTimeout := handOverWatcherTimeout
	handOverWatcherTimeout = time.Hour
	t.Cleanup(func() { handOverWatcherTimeout = originalTimeout })
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	start := time.Now()
	err := runInstallerHandOver(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exited before it launched a watcher")
	assert.Less(t, time.Since(start), 30*time.Second, "the hand-over waited instead of reverting")
	assertFailedOnHost(t, dirs)
	loaded := loadedJobs(*calls)
	for _, label := range configExperimentJobs {
		assert.Contains(t, loaded, "system/"+label, "the stable job %s was not restored", label)
	}
}

// TestInstallerHandOverIsANoopWhenTheExperimentIsOver covers a hand-over process that starts after
// its experiment was stopped or promoted.
func TestInstallerHandOverIsANoopWhenTheExperimentIsOver(t *testing.T) {
	for name, setUp := range map[string]func(t *testing.T){
		"no deadline": func(*testing.T) {},
		"reverting": func(t *testing.T) {
			require.NoError(t, experimentDeadline().MarkReverting())
		},
	} {
		t.Run(name, func(t *testing.T) {
			calls := stubLaunchd(t)
			stubJobDir(t)
			stubDeadlinePath(t)
			stubConfigExperimentLock(t)
			setUp(t)

			require.NoError(t, runInstallerHandOver(context.Background()))
			assert.Empty(t, *calls)
		})
	}
}

// TestExitFailsExperiment pins which exits revert an experiment: any exit of the Agent or the
// installer daemon, a clean one included, and only an abnormal exit of the other jobs, which exit
// cleanly by design when their feature is off.
func TestExitFailsExperiment(t *testing.T) {
	for _, tc := range []struct {
		label string
		clean bool
		fails bool
	}{
		{"com.datadoghq.agent", true, true},
		{"com.datadoghq.agent", false, true},
		{"com.datadoghq.installer", true, true},
		{"com.datadoghq.installer", false, true},
		{"com.datadoghq.sysprobe", true, false},
		{"com.datadoghq.sysprobe", false, true},
		{"com.datadoghq.data-plane", true, false},
		{"com.datadoghq.data-plane", false, true},
	} {
		t.Run(fmt.Sprintf("%s/clean=%t", tc.label, tc.clean), func(t *testing.T) {
			assert.Equal(t, tc.fails, exitFailsExperiment(tc.label, tc.clean))
		})
	}
	assert.ElementsMatch(t, configExperimentJobs, []string{"com.datadoghq.agent", "com.datadoghq.sysprobe", "com.datadoghq.data-plane", "com.datadoghq.installer"},
		"a new experiment job needs a decision in TestExitFailsExperiment")
}

// stubLaunchdExited behaves like stubLaunchd, but reports every experiment job as running except
// exitedLabel, which launchd reports as having exited with exitCode.
func stubLaunchdExited(t *testing.T, exitedLabel string, exitCode int) {
	t.Helper()

	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.BootoutSettlePollInterval = time.Millisecond
		pid := 100
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) == 0 || args[0] != "print" {
				return nil, nil
			}
			target := args[len(args)-1]
			if !strings.HasSuffix(target, "-exp") {
				return []byte(notLoadedOutput), exitError(113)
			}
			if target == "system/"+exitedLabel+"-exp" {
				return fmt.Appendf(nil, "\tstate = not running\n\tlast exit code = %d\n", exitCode), nil
			}
			pid++
			return fmt.Appendf(nil, "\tstate = running\n\tpid = %d\n", pid), nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
}

// TestResolveExperimentPidsFailsOnACleanExitOfAnEssentialJob covers the pre-arm check: a clean exit
// launchd already recorded for the Agent or the installer daemon is returned as a failure, while
// one for system-probe or data-plane is tolerated and the rest of the set is still watched.
func TestResolveExperimentPidsFailsOnACleanExitOfAnEssentialJob(t *testing.T) {
	for _, tc := range []struct {
		label string
		fails bool
	}{
		{"com.datadoghq.agent", true},
		{"com.datadoghq.installer", true},
		{"com.datadoghq.sysprobe", false},
		{"com.datadoghq.data-plane", false},
	} {
		t.Run(tc.label, func(t *testing.T) {
			stubLaunchdExited(t, tc.label, 0)
			stubJobDir(t)

			jobPIDs, exited, err := resolveExperimentPids(context.Background(), configExperimentJobSet())
			require.NoError(t, err)
			if tc.fails {
				require.NotNil(t, exited, "a clean exit of %s must fail the experiment", tc.label)
				assert.Equal(t, tc.label+"-exp", exited.Label)
				return
			}
			assert.Nil(t, exited, "a clean exit of %s must be tolerated", tc.label)
			assert.Len(t, jobPIDs, len(configExperimentJobs)-1, "the rest of the set should still be watched")
			assert.NotContains(t, slices.Collect(maps.Values(jobPIDs)), tc.label)
		})
	}
}

// TestSettleFinishesAnInterruptedPromoteAsAPromote pins that a promote whose hook never ran is
// finished as a promote: the process manager has no experiment variant, so it only picks up the
// promoted configuration when it is restarted.
func TestSettleFinishesAnInterruptedPromoteAsAPromote(t *testing.T) {
	l := stubLiveLaunchd(t, procmgrJob)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubSurvivingHelpers(t, nil, nil)
	stubConfigExperimentWatcher(t)
	stubConfigExperimentDirs(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))

	assert.Equal(t, [][]string{{"kickstart", "-k", "system/" + procmgrJob}}, l.kickstarts("system/"+procmgrJob))
}

// TestSettleFinishesAPromoteInterruptedAfterItsSwap covers a daemon that died in the middle of the
// configuration swap itself, with the previous stable configuration still at the experiment path:
// the next start must finish the promote, not run that previous configuration as the experiment.
func TestSettleFinishesAPromoteInterruptedAfterItsSwap(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubSurvivingHelpers(t, nil, nil)
	watchers := stubConfigExperimentWatcher(t)
	handOvers := stubInstallerHandOver(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	// What PromoteExperiment leaves when the daemon dies right after its swap.
	interruptPromoteAfterSwap(t, dirs)

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))

	for _, target := range launchctlCalls(*calls, "kickstart") {
		assert.NotContains(t, target, "-exp", "the previous stable configuration must not be run as the experiment")
	}
	assert.Zero(t, *handOvers)
	assert.Zero(t, *watchers)
	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, config.State{StableDeploymentID: "experiment-1"}, state)
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	assert.Empty(t, token)
}

// interruptPromoteAfterSwap lays out what PromoteExperiment leaves when the process dies right after
// its swap: the marked experiment directory now stable, and the previous stable configuration,
// with its own deployment ID, still at the experiment path.
func interruptPromoteAfterSwap(t *testing.T, dirs config.Directories) {
	t.Helper()

	// The marker config.PromoteExperiment writes before the swap.
	require.NoError(t, os.WriteFile(filepath.Join(dirs.ExperimentPath, ".promoting"), nil, 0640))
	previous := dirs.StablePath + ".previous"
	require.NoError(t, os.Rename(dirs.StablePath, previous))
	require.NoError(t, os.Rename(dirs.ExperimentPath, dirs.StablePath))
	require.NoError(t, os.Rename(previous, dirs.ExperimentPath))
}

// TestASecondHandBackLeavesTheStableDaemonAlone pins that the hand-back is idempotent: two can run
// for one experiment (the watcher failing to launch hands back from the hook while the hand-over
// does too), and the second must not swap again, which would stop the stable daemon the first one
// started and whatever command it runs.
func TestASecondHandBackLeavesTheStableDaemonAlone(t *testing.T) {
	l := stubLiveLaunchd(t, installerJob+"-exp")
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	require.NoError(t, handBackInstaller(context.Background()))
	require.True(t, l.isLoaded(installerJob))
	l.mu.Lock()
	first := len(l.calls)
	l.mu.Unlock()

	require.NoError(t, handBackInstaller(context.Background()))

	l.mu.Lock()
	defer l.mu.Unlock()
	for _, args := range l.calls[first:] {
		assert.Equal(t, "print", args[0], "the second hand-back touched launchd: %v", args)
	}
}

// TestHandBackStartsTheStableDaemonWhenNeitherJobIsLoaded pins that the idempotence check does not
// mistake a daemon on neither job -- a swap that failed halfway -- for one already handed back.
func TestHandBackStartsTheStableDaemonWhenNeitherJobIsLoaded(t *testing.T) {
	l := stubLiveLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)

	require.NoError(t, handBackInstaller(context.Background()))

	assert.True(t, l.isLoaded(installerJob))
}

// TestStopLeavesTheJobsOfAFailedExperimentAlone covers Fleet's stop for an experiment that failed on
// the host: its jobs are already back on the stable set, so the stop hook only clears the failed
// marker. Handing the jobs back again would restart every stable job.
func TestStopLeavesTheJobsOfAFailedExperimentAlone(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	handBackCalls := stubInstallerHandBack(t, nil)
	require.NoError(t, experimentDeadline().MarkFailed())

	require.NoError(t, preStopConfigExperimentDatadogAgent(testHookContext(t)))

	// The process manager's backstop may load it (ensureProcmgr); no experiment job is touched.
	for _, label := range agentJobs {
		assert.NotContains(t, launchctlCalls(*calls, "bootout"), "system/"+label, "the stop restarted %s", label)
	}
	assert.Zero(t, *handBackCalls, "no experiment daemon is left to hand back")
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	assert.Empty(t, token, "the stop should clear the failed marker")
}

// TestSettleLeavesAFailedExperimentForFleet covers a daemon start after an experiment failed on the
// host: its configuration is kept for Fleet's stop, so the settle neither brings it back, reverts
// it again, nor discards it.
func TestSettleLeavesAFailedExperimentForFleet(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubDeadlinePath(t)
	handOverCalls := stubInstallerHandOver(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().MarkFailed())

	require.NoError(t, resumeConfigExperimentDatadogAgent(testHookContext(t)))

	assert.Empty(t, launchctlCalls(*calls, "bootout"), "the settle touched the jobs of a failed experiment")
	assert.Zero(t, *handOverCalls, "a failed experiment must not be handed over again")
	assertFailedOnHost(t, dirs)
}
