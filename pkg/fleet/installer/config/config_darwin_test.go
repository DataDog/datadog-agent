// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDirectories lays out a state root the way the installer does: etc holding a
// configuration, and etc-exp resting as a sibling symlink to it.
func newTestDirectories(t *testing.T) *Directories {
	t.Helper()

	root := t.TempDir()
	dirs := &Directories{
		StablePath:     filepath.Join(root, "etc"),
		ExperimentPath: filepath.Join(root, "etc-exp"),
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dirs.StablePath, "conf.d"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, "datadog.yaml"), []byte("log_level: warn\n"), 0640))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, deploymentIDFile), []byte("stable-1"), 0640))
	require.NoError(t, dirs.experimentLink().Rest())
	return dirs
}

func mergePatch(deploymentID string, patch string) Operations {
	return Operations{
		DeploymentID: deploymentID,
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(patch)},
		},
	}
}

// assertResting checks the experiment path is a symlink to the stable path: the one and only
// representation of "no configuration experiment is deployed".
func assertResting(t *testing.T, dirs *Directories) {
	t.Helper()

	info, err := os.Lstat(dirs.ExperimentPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "%s is not a symlink", dirs.ExperimentPath)
	destination, err := os.Readlink(dirs.ExperimentPath)
	require.NoError(t, err)
	assert.Equal(t, dirs.StablePath, destination)
}

// assertNoScratchLeftBehind checks nothing of a failed experiment survives beside the stable
// directory. A host whose experiment failed must be indistinguishable from one that never started
// one, and a leftover scratch directory would be the difference.
func assertNoScratchLeftBehind(t *testing.T, dirs *Directories) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Dir(dirs.StablePath))
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), "."), "scratch directory %s was left behind", entry.Name())
	}
}

func TestRestingLinkTransitions(t *testing.T) {
	dirs := newTestDirectories(t)
	link := dirs.experimentLink()

	resting, err := link.IsResting()
	require.NoError(t, err)
	assert.True(t, resting)

	incoming, err := os.MkdirTemp(filepath.Dir(dirs.StablePath), incomingPrefix)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(incoming, "marker"), []byte("experiment"), 0640))
	require.NoError(t, link.Materialize(incoming))

	resting, err = link.IsResting()
	require.NoError(t, err)
	assert.False(t, resting, "a materialised experiment must not read as resting")
	content, err := os.ReadFile(filepath.Join(dirs.ExperimentPath, "marker"))
	require.NoError(t, err)
	assert.Equal(t, "experiment", string(content))
	assert.NoDirExists(t, incoming, "the scratch directory should have been renamed, not copied")

	require.NoError(t, link.Rest())
	assertResting(t, dirs)
	assert.NoFileExists(t, filepath.Join(dirs.StablePath, "marker"), "resting must not leak the experiment into stable")
}

// TestRestOnAnAbsentPathCreatesTheLink covers the upgrade path from a host that predates the
// layout: there is no etc-exp at all, and it must come back as a resting link rather than an error.
func TestRestOnAnAbsentPathCreatesTheLink(t *testing.T) {
	dirs := newTestDirectories(t)
	require.NoError(t, os.Remove(dirs.ExperimentPath))

	resting, err := dirs.experimentLink().IsResting()
	require.NoError(t, err)
	assert.True(t, resting, "an absent experiment path means nothing is deployed")

	require.NoError(t, dirs.RemoveExperiment(context.Background()))
	assertResting(t, dirs)
}

func TestMaterializeRefusesANonSibling(t *testing.T) {
	dirs := newTestDirectories(t)
	elsewhere := t.TempDir()

	err := dirs.experimentLink().Materialize(elsewhere)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sibling")
	assertResting(t, dirs)
}

func TestWriteExperimentPublishesAPatchedCopy(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()

	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	content, err := os.ReadFile(filepath.Join(dirs.ExperimentPath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "debug")

	stable, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(stable), "warn", "the stable configuration must be untouched")

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "stable-1", state.StableDeploymentID)
	assert.Equal(t, "experiment-1", state.ExperimentDeploymentID)

	assertNoScratchLeftBehind(t, dirs)
}

// TestWriteExperimentLeavesNoTraceWhenItFails is the property the copy-then-publish order exists
// for: everything that can fail happens in the scratch directory, so a failure is invisible.
func TestWriteExperimentLeavesNoTraceWhenItFails(t *testing.T) {
	dirs := newTestDirectories(t)

	err := dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{ this is not json`))
	require.Error(t, err)

	assertResting(t, dirs)
	assertNoScratchLeftBehind(t, dirs)

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "stable-1", state.StableDeploymentID)
	assert.Empty(t, state.ExperimentDeploymentID)
}

func TestWriteExperimentRefusesToOverwriteALiveExperiment(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	err := dirs.WriteExperiment(ctx, mergePatch("experiment-2", `{"log_level":"trace"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already deployed")

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "experiment-1", state.ExperimentDeploymentID, "the live experiment was replaced")
}

// TestGetStateReadsTheLinkBeforeTheDeploymentID guards the ordering in GetState. A resting link
// resolves every path under it to the stable directory, so reading the experiment's deployment ID
// first would report the stable one as an experiment's and make the daemon claim an experiment is
// running when none is.
func TestGetStateReadsTheLinkBeforeTheDeploymentID(t *testing.T) {
	dirs := newTestDirectories(t)

	// Through the resting link, this file is readable at the experiment path.
	content, err := os.ReadFile(filepath.Join(dirs.ExperimentPath, deploymentIDFile))
	require.NoError(t, err)
	require.Equal(t, "stable-1", string(content))

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "stable-1", state.StableDeploymentID)
	assert.Empty(t, state.ExperimentDeploymentID)
}

// TestCopyDoesNotTraverseTheExperimentPath pins invariant 6 at the copy. The configuration layer
// owns etc-exp alone, and a walk that followed a link into it would copy the stable tree into
// itself, or recurse.
func TestCopyDoesNotTraverseTheExperimentPath(t *testing.T) {
	dirs := newTestDirectories(t)
	// A link inside stable that points at the experiment path, which itself rests on stable.
	loop := filepath.Join(dirs.StablePath, "loop")
	require.NoError(t, os.Symlink(dirs.ExperimentPath, loop))

	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	require.NoError(t, configTree{sourcePath: dirs.StablePath, targetPath: incoming}.Copy(context.Background()))

	info, err := os.Lstat(filepath.Join(incoming, "loop"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link was followed instead of reproduced")
	destination, err := os.Readlink(filepath.Join(incoming, "loop"))
	require.NoError(t, err)
	assert.Equal(t, dirs.ExperimentPath, destination)

	// The walk produced exactly one entry per source entry: following the link would have copied
	// the stable tree in underneath it.
	source, err := os.ReadDir(dirs.StablePath)
	require.NoError(t, err)
	copied, err := os.ReadDir(incoming)
	require.NoError(t, err)
	assert.Len(t, copied, len(source))
	loopEntries, err := os.ReadDir(filepath.Join(incoming, "loop"))
	require.NoError(t, err)
	assert.Len(t, loopEntries, len(source), "the link resolves to stable; nothing was copied into it")
}

func TestCopyPreservesModes(t *testing.T) {
	dirs := newTestDirectories(t)
	secret := filepath.Join(dirs.StablePath, "conf.d", "secret.yaml")
	require.NoError(t, os.WriteFile(secret, []byte("token: x\n"), 0600))

	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	require.NoError(t, configTree{sourcePath: dirs.StablePath, targetPath: incoming}.Copy(context.Background()))

	info, err := os.Stat(filepath.Join(incoming, "conf.d", "secret.yaml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestPromoteExperimentReplacesStableAndRestsTheLink(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	require.NoError(t, dirs.PromoteExperiment(ctx))

	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "debug")

	assertResting(t, dirs)
	assertNoScratchLeftBehind(t, dirs)

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "experiment-1", state.StableDeploymentID)
	assert.Empty(t, state.ExperimentDeploymentID)
}

func TestPromoteExperimentRefusesWhenNothingIsDeployed(t *testing.T) {
	dirs := newTestDirectories(t)

	err := dirs.PromoteExperiment(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no configuration experiment")
	assertResting(t, dirs)
}

// TestPromoteRollsBackWhenTheSecondRenameFails is invariant 7's failure half: the live directory is
// moved aside rather than deleted precisely so that a failed second rename can be undone. Without
// the rollback the host would be left with no configuration directory at all.
func TestPromoteRollsBackWhenTheSecondRenameFails(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	boom := errors.New("boom")
	calls := 0
	original := rename
	rename = func(oldPath, newPath string) error {
		calls++
		if calls == 2 {
			return boom
		}
		return original(oldPath, newPath)
	}
	t.Cleanup(func() { rename = original })

	err := dirs.PromoteExperiment(ctx)
	require.ErrorIs(t, err, boom)

	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err, "the stable configuration was not restored")
	assert.Contains(t, string(content), "warn")

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "stable-1", state.StableDeploymentID)
	assert.Equal(t, "experiment-1", state.ExperimentDeploymentID, "the experiment must still be deployed")
	assertNoScratchLeftBehind(t, dirs)
}

func TestRemoveExperimentDiscardsTheExperiment(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	require.NoError(t, dirs.RemoveExperiment(ctx))

	assertResting(t, dirs)
	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "warn")
}

// TestRemoveExperimentOnAStableHostIsANoop covers the revert the daemon issues on start-up when it
// cannot tell whether an experiment is running. It must be safe on a host that has none.
func TestRemoveExperimentOnAStableHostIsANoop(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()

	require.NoError(t, dirs.RemoveExperiment(ctx))
	require.NoError(t, dirs.RemoveExperiment(ctx))

	assertResting(t, dirs)
	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "warn")

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "stable-1", state.StableDeploymentID)
	assert.Empty(t, state.ExperimentDeploymentID)
}

func TestDirSwapRefusesDifferentParents(t *testing.T) {
	live := filepath.Join(t.TempDir(), "live")
	incoming := filepath.Join(t.TempDir(), "incoming")
	require.NoError(t, os.MkdirAll(live, 0755))
	require.NoError(t, os.MkdirAll(incoming, 0755))

	err := (dirSwap{live: live, incoming: incoming}).Commit(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same directory")
}

// TestAgentAccountIsTheMacOSOne pins the ownership the patched files are given. macOS reserves the
// unprefixed namespace for the system, so a spec naming dd-agent would silently fail to apply --
// setFileOwnershipAndPermissions only warns on a failed chown -- and leave fleet configuration
// unreadable by the Agent.
func TestAgentAccountIsTheMacOSOne(t *testing.T) {
	spec := getConfigFileSpec("/datadog.yaml")
	require.NotNil(t, spec)
	assert.Equal(t, "_dd-agent", spec.owner)
	assert.Equal(t, "daemon", spec.group)
}
