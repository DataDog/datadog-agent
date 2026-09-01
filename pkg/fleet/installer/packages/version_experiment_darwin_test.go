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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/experiment"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// testPool builds a pool with a stable version and an experiment version, and the two links, in a
// temporary directory. It returns the pool root.
func testPool(t *testing.T, stableVersion string, experimentVersion string) string {
	t.Helper()

	poolRoot := filepath.Join(t.TempDir(), "datadog-agent")
	for _, version := range []string{stableVersion, experimentVersion} {
		if version == "" {
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Join(poolRoot, version, "bin", "agent"), 0755))
	}
	require.NoError(t, os.Symlink(filepath.Join(poolRoot, stableVersion), filepath.Join(poolRoot, "stable")))
	target := filepath.Join(poolRoot, "stable")
	if experimentVersion != "" {
		target = filepath.Join(poolRoot, experimentVersion)
	}
	require.NoError(t, os.Symlink(target, filepath.Join(poolRoot, "experiment")))
	return poolRoot
}

// testVersionExperiment builds a versionExperiment over temporary directories and a recording
// launchctl.
func testVersionExperiment(t *testing.T, poolRoot string) (versionExperiment, *[][]string, string) {
	t.Helper()

	calls := stubLaunchd(t)
	jobDir := stubJobDir(t)
	appsDir := t.TempDir()
	return versionExperiment{
		jobs:      agentJobSet(),
		poolRoot:  poolRoot,
		deadline:  experiment.NewDeadlineAt(filepath.Join(t.TempDir(), "experiment-deadline")),
		appBundle: &appBundleSwap{appsDir: appsDir, name: "Datadog Agent.app"},
	}, calls, jobDir
}

// linkTarget reads a link, failing the test if it is not one.
func linkTarget(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	require.NoError(t, err)
	return target
}

// TestVersionExperimentStartRecordsTheDeadlineBeforeStoppingStable is the ordering that decides
// whether a bad version can strand a host. Everything reversible happens first, so there is no
// window in which an experiment is running with nothing that knows to end it.
func TestVersionExperimentStartRecordsTheDeadlineBeforeStoppingStable(t *testing.T) {
	e, calls, jobDir := testVersionExperiment(t, testPool(t, "7.99.0", "7.99.1"))

	require.NoError(t, e.Start(context.Background(), "7.99.1"))

	version, expiresAt, set, err := e.deadline.Get()
	require.NoError(t, err)
	require.True(t, set, "the experiment was started without a deadline")
	assert.Equal(t, "7.99.1", version)
	assert.WithinDuration(t, time.Now().Add(experiment.DefaultDuration), expiresAt, time.Minute)

	for _, label := range agentJobs {
		assert.FileExists(t, filepath.Join(jobDir, label+"-exp.plist"))
	}

	bootedOut := launchctlCalls(*calls, "bootout")
	for _, label := range agentJobs {
		assert.Contains(t, bootedOut, "system/"+label)
	}

}

// TestVersionExperimentStartDoesEverythingReversibleFirst is the ordering that decides whether a
// bad version can strand a host: by the time the first stable job is stopped -- the first
// irreversible act -- the experiment definitions must already be on disk and the deadline must
// already be recorded. Otherwise there is a window in which the Agent is down and nothing on the
// host knows an experiment is meant to be running, let alone when to end it.
//
// The check is on the state at the moment of that first stop, not on a position in the call list:
// writing plists and recording the deadline are not launchctl calls, so the first stop is
// legitimately the first call.
func TestVersionExperimentStartDoesEverythingReversibleFirst(t *testing.T) {
	jobDir := stubJobDir(t)
	appsDir := t.TempDir()
	deadline := experiment.NewDeadlineAt(filepath.Join(t.TempDir(), "experiment-deadline"))

	type snapshot struct {
		deadlineSet   bool
		plistsWritten bool
	}
	var atFirstStableStop *snapshot

	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			joined := strings.Join(args, " ")
			if atFirstStableStop == nil && args[0] == "bootout" && !strings.Contains(joined, "-exp") {
				_, _, set, err := deadline.Get()
				require.NoError(t, err)
				written := true
				for _, label := range agentJobs {
					if _, err := os.Stat(filepath.Join(jobDir, label+"-exp.plist")); err != nil {
						written = false
					}
				}
				atFirstStableStop = &snapshot{deadlineSet: set, plistsWritten: written}
			}
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })

	e := versionExperiment{
		jobs:      agentJobSet(),
		poolRoot:  testPool(t, "7.99.0", "7.99.1"),
		deadline:  deadline,
		appBundle: &appBundleSwap{appsDir: appsDir, name: "Datadog Agent.app"},
	}
	require.NoError(t, e.Start(context.Background(), "7.99.1"))

	require.NotNil(t, atFirstStableStop, "no stable job was ever stopped, so the swap did not happen")
	assert.True(t, atFirstStableStop.plistsWritten, "a stable job was stopped before the experiment definitions were on disk")
	assert.True(t, atFirstStableStop.deadlineSet, "a stable job was stopped before the deadline was recorded")
}

// TestVersionExperimentStartLeavesTheInstallerAlone is the invariant that keeps an experiment from
// killing the process supervising it.
func TestVersionExperimentStartLeavesTheInstallerAlone(t *testing.T) {
	e, calls, _ := testVersionExperiment(t, testPool(t, "7.99.0", "7.99.1"))
	require.NoError(t, e.Start(context.Background(), "7.99.1"))

	for _, args := range *calls {
		for _, argument := range args {
			assert.NotContains(t, argument, installerJob, "the swap touched the installer daemon: %v", args)
		}
	}
}

// TestVersionExperimentStartReturnsToStableWhenTheExperimentWillNotStart covers the failure that
// matters most: the new version cannot even be launched. The host must not be left running neither
// job set.
func TestVersionExperimentStartReturnsToStableWhenTheExperimentWillNotStart(t *testing.T) {
	poolRoot := testPool(t, "7.99.0", "7.99.1")
	e, _, jobDir := testVersionExperiment(t, poolRoot)

	// Fail only the experiment kickstarts, so the stable set can still be brought back.
	var calls [][]string
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			calls = append(calls, args)
			joined := strings.Join(args, " ")
			if args[0] == "kickstart" && strings.Contains(joined, "-exp") {
				return []byte("nope"), errors.New("exit status 3")
			}
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	e.jobs = agentJobSet()

	err := e.Start(context.Background(), "7.99.1")
	require.Error(t, err)

	// Back on stable: the link is collapsed, the experiment definitions are gone, the stable
	// definitions are written.
	assert.Equal(t, filepath.Join(poolRoot, "stable"), linkTarget(t, filepath.Join(poolRoot, "experiment")))
	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(jobDir, label+"-exp.plist"))
		assert.FileExists(t, filepath.Join(jobDir, label+".plist"))
	}
}

// TestVersionExperimentRevertCollapsesTheLinkBeforeStoppingTheJobs is the load-bearing ordering of
// the revert. The -exp definitions reach the binaries through the experiment link; once it names
// stable, nothing that comes back up -- a job launchd is slow to kill, one an operator kickstarts,
// one loaded from a definition that failed to be removed -- can run the version being abandoned.
func TestVersionExperimentRevertCollapsesTheLinkBeforeStoppingTheJobs(t *testing.T) {
	poolRoot := testPool(t, "7.99.0", "7.99.1")
	jobDir := stubJobDir(t)

	var linkAtFirstCall string
	var calls [][]string
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(calls) == 0 {
				linkAtFirstCall = linkTarget(t, filepath.Join(poolRoot, "experiment"))
			}
			calls = append(calls, args)
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })

	e := versionExperiment{
		jobs:     agentJobSet(),
		poolRoot: poolRoot,
		deadline: experiment.NewDeadlineAt(filepath.Join(t.TempDir(), "experiment-deadline")),
	}
	require.NoError(t, e.Revert(context.Background()))

	assert.Equal(t, filepath.Join(poolRoot, "stable"), linkAtFirstCall,
		"launchctl was called before the experiment link was collapsed onto stable")
	for _, label := range agentJobs {
		assert.NoFileExists(t, filepath.Join(jobDir, label+"-exp.plist"))
		assert.FileExists(t, filepath.Join(jobDir, label+".plist"))
	}
}

// TestVersionExperimentRevertLeavesTheDeadlineForTheHookToClear pins the "clear the deadline last"
// rule at the level it is implemented. A revert that fails partway must stay on the supervisor's
// books.
func TestVersionExperimentRevertLeavesTheDeadlineForTheHookToClear(t *testing.T) {
	e, _, _ := testVersionExperiment(t, testPool(t, "7.99.0", "7.99.1"))
	require.NoError(t, e.deadline.Set("7.99.1", time.Now().Add(time.Hour)))

	require.NoError(t, e.Revert(context.Background()))

	_, _, set, err := e.deadline.Get()
	require.NoError(t, err)
	assert.True(t, set, "Revert cleared the deadline itself, so a partial revert would be forgotten")
}

// TestVersionExperimentRevertOnAStableHostIsANoOpSequence is what lets the supervisor revert
// without first working out whether it needs to -- and what makes a revert safe to retry every
// tick.
func TestVersionExperimentRevertOnAStableHostIsANoOpSequence(t *testing.T) {
	poolRoot := testPool(t, "7.99.0", "")
	e, _, jobDir := testVersionExperiment(t, poolRoot)

	require.NoError(t, e.Revert(context.Background()))
	require.NoError(t, e.Revert(context.Background()))

	assert.Equal(t, filepath.Join(poolRoot, "stable"), linkTarget(t, filepath.Join(poolRoot, "experiment")))
	for _, label := range agentJobs {
		assert.FileExists(t, filepath.Join(jobDir, label+".plist"))
		assert.NoFileExists(t, filepath.Join(jobDir, label+"-exp.plist"))
	}
}

// TestVersionExperimentRevertRefusesAHostWithNoStableLink is the one case a revert cannot fix, and
// it must say so rather than quietly leaving a dangling link behind.
func TestVersionExperimentRevertRefusesAHostWithNoStableLink(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	poolRoot := filepath.Join(t.TempDir(), "datadog-agent")
	require.NoError(t, os.MkdirAll(poolRoot, 0755))

	e := versionExperiment{
		jobs:     agentJobSet(),
		poolRoot: poolRoot,
		deadline: experiment.NewDeadlineAt(filepath.Join(t.TempDir(), "experiment-deadline")),
	}
	assert.Error(t, e.Revert(context.Background()))
}

// TestPromoteStopsTheExperimentJobsBeforeTheLinkMoves covers the split across the two promote
// hooks: the installer moves the stable link between them, so the experiment jobs -- the processes
// running out of the directory that link is about to stop naming -- have to be down first.
func TestPromoteStopsTheExperimentJobsBeforeTheLinkMoves(t *testing.T) {
	e, calls, jobDir := testVersionExperiment(t, testPool(t, "7.99.0", "7.99.1"))
	require.NoError(t, e.jobs.Write(launchd.Experiment))

	require.NoError(t, e.StopExperimentJobs(context.Background()))

	bootedOut := launchctlCalls(*calls, "bootout")
	for _, label := range agentJobs {
		assert.Contains(t, bootedOut, "system/"+label+"-exp")
		assert.NoFileExists(t, filepath.Join(jobDir, label+"-exp.plist"))
	}
	// The stable set is not started here: the link has not moved yet, so starting it would run
	// the version being replaced.
	assert.Empty(t, launchctlCalls(*calls, "kickstart"))
}

// TestStartStableJobsPutsThePromotedVersionIntoService pins that nothing version-specific is
// written: the stable definitions name the façade, which names the stable link, so the promoted
// version is reached without rewriting anything for it.
func TestStartStableJobsPutsThePromotedVersionIntoService(t *testing.T) {
	e, calls, jobDir := testVersionExperiment(t, testPool(t, "7.99.1", ""))

	require.NoError(t, e.StartStableJobs(context.Background()))

	for _, label := range agentJobs {
		content, err := os.ReadFile(filepath.Join(jobDir, label+".plist"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "7.99.1", "a version was baked into a stable job definition")
	}
	assert.NotEmpty(t, launchctlCalls(*calls, "kickstart"))
}

// TestAppBundleIsSwappedFromTheVersionDirectory covers the promote-only bundle swap.
func TestAppBundleIsSwappedFromTheVersionDirectory(t *testing.T) {
	appsDir := t.TempDir()
	versionPath := t.TempDir()
	swap := &appBundleSwap{appsDir: appsDir, name: "Datadog Agent.app"}

	require.NoError(t, os.MkdirAll(filepath.Join(versionPath, swap.name, "Contents"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(versionPath, swap.name, "Contents", "marker"), []byte("new"), 0644))
	// An older bundle is already installed, with a file the new one does not have.
	require.NoError(t, os.MkdirAll(filepath.Join(appsDir, swap.name, "Contents"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appsDir, swap.name, "Contents", "stale"), []byte("old"), 0644))

	require.NoError(t, swap.Swap(context.Background(), versionPath))

	content, err := os.ReadFile(filepath.Join(appsDir, swap.name, "Contents", "marker"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
	// A bundle is a unit: a tree with files from two versions in it is a bundle from neither.
	assert.NoFileExists(t, filepath.Join(appsDir, swap.name, "Contents", "stale"))
}

// TestAppBundleSwapLeavesTheInstalledBundleWhenTheVersionShipsNone keeps a build without the GUI
// from taking the GUI away from a host.
func TestAppBundleSwapLeavesTheInstalledBundleWhenTheVersionShipsNone(t *testing.T) {
	appsDir := t.TempDir()
	swap := &appBundleSwap{appsDir: appsDir, name: "Datadog Agent.app"}
	require.NoError(t, os.MkdirAll(filepath.Join(appsDir, swap.name), 0755))

	require.NoError(t, swap.Swap(context.Background(), t.TempDir()))
	assert.DirExists(t, filepath.Join(appsDir, swap.name))
}

// TestNilAppBundleSwapIsANoOp covers the layout that ships no bundle at all.
func TestNilAppBundleSwapIsANoOp(t *testing.T) {
	var swap *appBundleSwap
	assert.NoError(t, swap.Swap(context.Background(), t.TempDir()))
}

// TestEveryVersionExperimentHookIsWired guards the wiring itself. A hook left nil is a silent gap:
// the installer would carry out the update and never swap the jobs, leaving the host running the
// old version while reporting the new one.
func TestEveryVersionExperimentHookIsWired(t *testing.T) {
	for name, hook := range map[string]packageHook{
		"preStartExperiment":    datadogAgentPackage.preStartExperiment,
		"postStartExperiment":   datadogAgentPackage.postStartExperiment,
		"preStopExperiment":     datadogAgentPackage.preStopExperiment,
		"postStopExperiment":    datadogAgentPackage.postStopExperiment,
		"prePromoteExperiment":  datadogAgentPackage.prePromoteExperiment,
		"postPromoteExperiment": datadogAgentPackage.postPromoteExperiment,
	} {
		assert.NotNil(t, hook, "%s is not wired", name)
	}
}

// TestAgentVersionExperimentUsesThePoolAndNotTheStateRoot pins the two roots apart. The state root
// holds configuration and never moves; the pool holds code and is versioned. A version experiment
// that reached into the state root would version configuration by accident.
func TestAgentVersionExperimentUsesThePoolAndNotTheStateRoot(t *testing.T) {
	e := agentVersionExperiment()
	assert.Equal(t, "/opt/datadog-packages/datadog-agent", e.poolRoot)
	assert.Equal(t, "/opt/datadog-packages/datadog-agent/stable", e.stableLink())
	assert.Equal(t, "/opt/datadog-packages/datadog-agent/experiment", e.experimentLink())
	assert.Equal(t, "/opt/datadog-agent/run/experiment-deadline", e.deadline.Path())
}
