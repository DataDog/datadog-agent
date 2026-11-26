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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

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
	originalPoll := stateWatchPoll
	stateWatchPoll = 10 * time.Millisecond
	t.Cleanup(func() { stateWatchPoll = originalPoll })
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

	require.NoError(t, dirs.WriteExperiment(context.Background(), config.Operations{DeploymentID: "experiment-1"}))
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

	require.NoError(t, configExperiment{jobs: agentJobSet()}.Start(testHookContext(t)))

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

	require.NoError(t, configExperiment{jobs: agentJobSet()}.restoreStable(testHookContext(t)))
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

	err := configExperiment{jobs: agentJobSet()}.restoreStable(testHookContext(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("after %d attempts", configExperimentRestoreRetries))
	assert.Equal(t, len(agentJobs)*configExperimentRestoreRetries, bootstrapAttempts, "expected every label's retries to stop at the configured bound")
}

// namesProcmgr reports whether a recorded launchctl call is about the process manager, which the
// end of every experiment makes sure is loaded (ensureProcmgr): the stubs report it unloaded.
func namesProcmgr(args []string) bool {
	return strings.Contains(strings.Join(args, " "), procmgrJob)
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

	require.NoError(t, configExperiment{jobs: agentJobSet()}.restoreStable(testHookContext(t)))

	for _, label := range agentJobs {
		info, err := os.Stat(filepath.Join(dir, label+".plist"))
		require.NoError(t, err)
		assert.True(t, os.SameFile(before[label], info), "%s should not have been rewritten", label)
	}
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

	err := configExperiment{jobs: agentJobSet()}.restoreStable(testHookContext(t))

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

	err := configExperiment{jobs: agentJobSet()}.restoreStable(testHookContext(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "com.datadoghq.sysprobe")
	counts := stableBootstraps(*calls)
	assert.Zero(t, counts["com.datadoghq.sysprobe"], "the stable job started while its experiment twin was still loaded")
	assert.Equal(t, 1, counts["com.datadoghq.agent"])
	assert.Equal(t, 1, counts["com.datadoghq.data-plane"])
}

// TestRestoreStableStopsRetryingOnceCancelled pins that a restore whose context is cancelled, a
// watcher being stopped, does not sleep through the rest of its backoff: every swap would fail, so
// it reports at once and leaves the reverting marker for whatever ends the experiment next.
func TestRestoreStableStopsRetryingOnceCancelled(t *testing.T) {
	originalBackoff := configExperimentRestoreBackoff
	configExperimentRestoreBackoff = time.Hour
	t.Cleanup(func() { configExperimentRestoreBackoff = originalBackoff })

	originalClient := launchdClient
	t.Cleanup(func() { launchdClient = originalClient })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
				cancel()
				return nil, errors.New("injected failure")
			}
			return nil, nil
		}
		return client
	}
	stubJobDir(t)
	stubDeadlinePath(t)
	require.NoError(t, experimentDeadline().Write(configExperimentDeadlineWindow))
	hookCtx := testHookContext(t)
	hookCtx.Context = ctx

	done := make(chan error, 1)
	go func() { done <- configExperiment{jobs: agentJobSet()}.restoreStable(hookCtx) }()
	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(30 * time.Second):
		t.Fatal("the restore kept waiting for its backoff after its context was cancelled")
	}
	assert.Equal(t, len(agentJobs), bootstrapAttempts, "no label may be retried after the cancellation")
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

	require.NoError(t, configExperiment{jobs: agentJobSet()}.restoreStable(testHookContext(t)))

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

	assert.Equal(t, []int{101, 108}, parseHelperPIDs(ps, 999, "watchConfigExperiment"))
	assert.Equal(t, []int{101}, parseHelperPIDs(ps, 108, "watchConfigExperiment"), "the caller must never be matched")
	assert.Empty(t, parseHelperPIDs("", 999, "watchConfigExperiment"))

	helpers := ps + strings.Join([]string{
		"  201     0 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent handOverInstaller",
		"  202     0 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent handBackInstaller",
		"  203   450 /opt/datadog-agent/embedded/bin/installer package-command datadog-agent handOverInstaller",
		"",
	}, "\n")
	assert.Equal(t, []int{101, 108}, parseHelperPIDs(helpers, 999, "watchConfigExperiment"), "a watcher lookup must not match the hand-back")
	// The helpers an install stops are the ones earlier versions launched; the hand-over was never
	// one of them.
	assert.Equal(t, []int{101, 108, 202}, parseHelperPIDs(helpers, 999, legacyHelperCommands...))
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

// TestWaitForInstallerLockSurvivesACorruptDatabase pins that the lock does not depend on the
// database it guards: a failed experiment's stable jobs are restored even when packages.db cannot
// be read.
func TestWaitForInstallerLockSurvivesACorruptDatabase(t *testing.T) {
	path := stubConfigExperimentLock(t)
	require.NoError(t, os.WriteFile(path, []byte("not a bbolt database"), 0o644))

	lock, err := waitForInstallerLock(context.Background(), func() (bool, error) { return true, nil })
	require.NoError(t, err)
	require.NotNil(t, lock)
	require.NoError(t, lock.Close())
}

// TestWaitForInstallerLockWaitsForAnInstallerCommand pins that the lock is the one an installer
// command holds through bbolt: it is taken only once the command's database is closed, and the
// waiter checks meanwhile that it still has a reason to wait.
func TestWaitForInstallerLockWaitsForAnInstallerCommand(t *testing.T) {
	path := stubConfigExperimentLock(t)
	original := installerLockPoll
	installerLockPoll = 20 * time.Millisecond
	t.Cleanup(func() { installerLockPoll = original })
	command, err := db.New(context.Background(), path)
	require.NoError(t, err)

	checked := make(chan struct{}, 1)
	got := make(chan error, 1)
	go func() {
		lock, err := waitForInstallerLock(context.Background(), func() (bool, error) {
			select {
			case checked <- struct{}{}:
			default:
			}
			return true, nil
		})
		if err == nil {
			err = lock.Close()
		}
		got <- err
	}()
	select {
	case <-checked:
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter should check it is still wanted while the command runs")
	}
	select {
	case err := <-got:
		t.Fatalf("the lock was taken while the command held it: %v", err)
	default:
	}
	require.NoError(t, command.Close())
	select {
	case err := <-got:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the lock should be taken once the command exits")
	}

	// And the other way round: a command waits while the lock is held.
	lock, err := waitForInstallerLock(context.Background(), func() (bool, error) { return true, nil })
	require.NoError(t, err)
	_, err = db.New(context.Background(), path, db.WithTimeout(100*time.Millisecond))
	assert.Error(t, err, "an installer command must not open the database while the lock is held")
	require.NoError(t, lock.Close())
}

// TestWaitForInstallerCommandRechecksOnceItHasTheLock pins that a request the command waited for
// withdrew before it exited is not acted on: the waiter checks it is still wanted once it has the
// lock, not only while it waits.
func TestWaitForInstallerCommandRechecksOnceItHasTheLock(t *testing.T) {
	path := stubConfigExperimentLock(t)
	original := installerLockPoll
	// No check while waiting: only the one after the lock is taken can see the withdrawal.
	installerLockPoll = time.Hour
	t.Cleanup(func() { installerLockPoll = original })
	command, err := db.New(context.Background(), path)
	require.NoError(t, err)

	var withdrawn atomic.Bool
	got := make(chan bool, 1)
	go func() {
		ok, err := waitForInstallerCommand(context.Background(), func() (bool, error) { return !withdrawn.Load(), nil })
		assert.NoError(t, err)
		got <- ok
	}()
	// The command withdraws the request, then exits.
	withdrawn.Store(true)
	require.NoError(t, command.Close())
	select {
	case ok := <-got:
		assert.False(t, ok, "a withdrawn request must not be acted on")
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter should take the lock once the command exits")
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

// TestResolveExperimentPidsReturnsAJobThatIsNotLoaded pins that an experiment job missing from the
// domain is returned at once as exited, not waited on as one that is starting.
func TestResolveExperimentPidsReturnsAJobThatIsNotLoaded(t *testing.T) {
	stubJobDir(t)
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if args[len(args)-1] == "system/"+agentJob+"-exp" {
				return []byte(notLoadedOutput), exitError(113)
			}
			return []byte("\tstate = running\n\tpid = 4242\n"), nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })

	started := time.Now()
	_, exited, err := resolveExperimentPids(context.Background(), launchd.JobSet{Labels: []string{agentJob}, Dir: launchdJobDir, Client: launchdClient()})
	require.NoError(t, err)
	require.NotNil(t, exited)
	assert.Equal(t, agentJob+"-exp", exited.Label)
	assert.False(t, exited.Loaded)
	assert.Less(t, time.Since(started), startupPollTimeout, "a job that is not loaded is not waited on")
}

// TestResolveExperimentPidsFailsOnlyOnAFailedExit pins the pre-arm check: a clean exit launchd
// recorded, as a job whose feature is off leaves, is tolerated, and a non-zero one is returned.
func TestResolveExperimentPidsFailsOnlyOnAFailedExit(t *testing.T) {
	for _, tc := range []struct {
		exitCode int
		fails    bool
	}{{0, false}, {1, true}} {
		t.Run(fmt.Sprintf("exit %d", tc.exitCode), func(t *testing.T) {
			stubLaunchdExited(t, "com.datadoghq.sysprobe", tc.exitCode)
			stubJobDir(t)

			jobPIDs, exited, err := resolveExperimentPids(context.Background(), configExperimentJobSet())
			require.NoError(t, err)
			if tc.fails {
				require.NotNil(t, exited)
				assert.Equal(t, "com.datadoghq.sysprobe-exp", exited.Label)
				return
			}
			assert.Nil(t, exited)
			assert.Len(t, jobPIDs, len(configExperimentJobs)-1, "the rest of the set should still be resolved")
			assert.NotContains(t, slices.Collect(maps.Values(jobPIDs)), "com.datadoghq.sysprobe")
		})
	}
}
