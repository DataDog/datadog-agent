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
	"os/exec"
	"os/user"
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

func TestWriteExperimentDoesNotFollowDeploymentIDSymlink(t *testing.T) {
	dirs := newTestDirectories(t)
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("untouched"), 0600))
	idPath := filepath.Join(dirs.StablePath, deploymentIDFile)
	require.NoError(t, os.Remove(idPath))
	require.NoError(t, os.Symlink(sentinel, idPath))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "untouched", string(content), "deployment metadata must not overwrite a copied link's target")
	info, err := os.Lstat(filepath.Join(dirs.ExperimentPath, deploymentIDFile))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	content, err = os.ReadFile(filepath.Join(dirs.ExperimentPath, deploymentIDFile))
	require.NoError(t, err)
	assert.Equal(t, "experiment-1", string(content))
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

func TestCopyKeepsTheTargetRootPrivateUntilFinish(t *testing.T) {
	dirs := newTestDirectories(t)
	require.NoError(t, os.Chmod(dirs.StablePath, 0750))

	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	tree := configTree{sourcePath: dirs.StablePath, targetPath: incoming}
	require.NoError(t, tree.Copy(context.Background()))

	info, err := os.Stat(incoming)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "the copy was opened up before Finish")

	require.NoError(t, tree.Finish(context.Background()))

	info, err = os.Stat(incoming)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0750), info.Mode().Perm())
}

func TestWriteExperimentPublishesWithTheStableRootMode(t *testing.T) {
	dirs := newTestDirectories(t)
	require.NoError(t, os.Chmod(dirs.StablePath, 0750))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("exp-1", `{"log_level":"debug"}`)))

	info, err := os.Stat(dirs.ExperimentPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0750), info.Mode().Perm())
}

func TestSetFileOwnershipAndPermissionsRefusesALink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "datadog.yaml")))
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()

	require.Error(t, setFileOwnershipAndPermissions(context.Background(), root, "datadog.yaml", &configFileSpec{}))

	info, err := os.Stat(outside)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "the mode was applied through the link")
}

func TestCopyDoesNotWriteThroughADirectoryLinkInTheTarget(t *testing.T) {
	dirs := newTestDirectories(t)
	outside := t.TempDir()
	require.NoError(t, os.Chmod(outside, 0700))
	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	require.NoError(t, os.Mkdir(incoming, 0700))
	require.NoError(t, os.Symlink(outside, filepath.Join(incoming, "conf.d")))

	require.Error(t, configTree{sourcePath: dirs.StablePath, targetPath: incoming}.Copy(context.Background()))

	info, err := os.Stat(outside)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "the mode was applied through the link")
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries, "files were written through the link")
}

func TestCopyDoesNotWriteThroughAFileLinkInTheTarget(t *testing.T) {
	dirs := newTestDirectories(t)
	outside := filepath.Join(t.TempDir(), "datadog.yaml")
	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	require.NoError(t, os.Mkdir(incoming, 0700))
	require.NoError(t, os.Symlink(outside, filepath.Join(incoming, "datadog.yaml")))

	require.Error(t, configTree{sourcePath: dirs.StablePath, targetPath: incoming}.Copy(context.Background()))

	assert.NoFileExists(t, outside, "the file was written through the link")
}

func TestCopyRegularFileRefusesALink(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0600))
	sourcePath, targetPath := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(sourcePath, "datadog.yaml")))
	source, err := os.OpenRoot(sourcePath)
	require.NoError(t, err)
	defer source.Close()
	target, err := os.OpenRoot(targetPath)
	require.NoError(t, err)
	defer target.Close()

	require.Error(t, copyRegularFile(source, target, "datadog.yaml"))

	assert.NoFileExists(t, filepath.Join(targetPath, "datadog.yaml"))
}

func TestCopyDoesNotWalkALinkOutOfTheSource(t *testing.T) {
	dirs := newTestDirectories(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dirs.StablePath, "escape")))

	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	require.NoError(t, configTree{sourcePath: dirs.StablePath, targetPath: incoming}.Copy(context.Background()))

	info, err := os.Lstat(filepath.Join(incoming, "escape"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link was followed instead of reproduced")
	destination, err := os.Readlink(filepath.Join(incoming, "escape"))
	require.NoError(t, err)
	assert.Equal(t, outside, destination)
}

func TestACLEntries(t *testing.T) {
	listing := `drwxrwx---+ 3 _dd-agent  admin  96 Oct  1 12:00 /opt/datadog-agent/etc/conf.d
 0: user:_dd-agent allow list,add_file,search,file_inherit,directory_inherit
 1: user:_dd-agent inherited allow read,write
`
	assert.Equal(t, []string{
		"user:_dd-agent allow list,add_file,search,file_inherit,directory_inherit",
		"user:_dd-agent allow read,write",
	}, aclEntries([]byte(listing)))
	assert.Empty(t, aclEntries([]byte("-rw-rw----  1 _dd-agent  admin  0 Oct  1 12:00 datadog.yaml\n")))
}

// readACL returns a path's access control entries.
func readACL(t *testing.T, path string) []string {
	t.Helper()
	listing, err := exec.Command("/bin/ls", "-led", path).Output()
	require.NoError(t, err)
	return aclEntries(listing)
}

func setACL(t *testing.T, path string, entry string) {
	t.Helper()
	require.NoError(t, exec.Command("/bin/chmod", "+a", entry, path).Run())
}

func TestCopyReproducesAccessControlLists(t *testing.T) {
	current, err := user.Current()
	require.NoError(t, err)
	dirs := newTestDirectories(t)
	setACL(t, dirs.StablePath, "user:"+current.Username+" allow list,search,file_inherit,directory_inherit")
	setACL(t, filepath.Join(dirs.StablePath, "conf.d"), "user:"+current.Username+" allow list,add_file,search,file_inherit")
	setACL(t, filepath.Join(dirs.StablePath, "datadog.yaml"), "group:everyone allow read")
	// Created after conf.d's inheritable entry, so it carries an inherited one.
	require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, "conf.d", "inherited.yaml"), nil, 0640))

	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	tree := configTree{sourcePath: dirs.StablePath, targetPath: incoming}
	require.NoError(t, tree.Copy(context.Background()))
	require.NoError(t, tree.Finish(context.Background()))

	for _, path := range []string{"", "conf.d", "datadog.yaml", filepath.Join("conf.d", "inherited.yaml")} {
		assert.Equal(t, readACL(t, filepath.Join(dirs.StablePath, path)), readACL(t, filepath.Join(incoming, path)),
			"access control list of %q", path)
	}
	// deploymentIDFile has no entries of its own, and must not pick any up from the root it is
	// copied into.
	assert.Empty(t, readACL(t, filepath.Join(incoming, deploymentIDFile)))
}

func TestDiscardSucceedsWhenTheCopyIsGone(t *testing.T) {
	parent := t.TempDir()
	tree := configTree{targetPath: filepath.Join(parent, ".incoming")}
	require.NoError(t, tree.Discard(context.Background()))

	tree = configTree{targetPath: filepath.Join(parent, "missing", ".incoming")}
	require.NoError(t, tree.Discard(context.Background()))
}

// TestWriteExperimentGivesWrittenFilesThePostinstallMode covers a file whose Linux spec differs
// from the .dmg layout: system-probe.yaml is root-owned 0640 on Linux, but on macOS every
// configuration file has the mode the .dmg's postinstall script gives the tree.
func TestWriteExperimentGivesWrittenFilesThePostinstallMode(t *testing.T) {
	dirs := newTestDirectories(t)
	operations := Operations{
		DeploymentID: "exp-1",
		FileOperations: []FileOperation{
			{FileOperationType: FileOperationMergePatch, FilePath: "/system-probe.yaml", Patch: []byte(`{"network_config":{"enabled":true}}`)},
		},
	}

	require.NoError(t, dirs.WriteExperiment(context.Background(), operations))

	info, err := os.Stat(filepath.Join(dirs.ExperimentPath, "system-probe.yaml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(agentConfigFileMode), info.Mode().Perm())
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

// TestPromoteLeavesBothDirectoriesWhenTheSwapFails is invariant 7's failure half: the swap is a
// single exchange, so when it fails neither directory has moved and the experiment is still
// deployed beside an intact stable configuration.
func TestPromoteLeavesBothDirectoriesWhenTheSwapFails(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	boom := errors.New("boom")
	original := renameSwap
	renameSwap = func(string, string) error { return boom }
	t.Cleanup(func() { renameSwap = original })

	err := dirs.PromoteExperiment(ctx)
	require.ErrorIs(t, err, boom)

	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err, "the stable configuration was disturbed")
	assert.Contains(t, string(content), "warn")

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "stable-1", state.StableDeploymentID)
	assert.Equal(t, "experiment-1", state.ExperimentDeploymentID, "the experiment must still be deployed")
	assertNoScratchLeftBehind(t, dirs)
}

// TestDirSwapExchangesTheDirectories pins the exchange itself: live ends up with the incoming
// content and the previous live directory is discarded rather than left at the incoming path.
func TestDirSwapExchangesTheDirectories(t *testing.T) {
	parent := t.TempDir()
	live := filepath.Join(parent, "live")
	incoming := filepath.Join(parent, "incoming")
	require.NoError(t, os.MkdirAll(live, 0755))
	require.NoError(t, os.MkdirAll(incoming, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(live, "marker"), []byte("old"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(incoming, "marker"), []byte("new"), 0644))

	require.NoError(t, (dirSwap{live: live, incoming: incoming}).Commit(context.Background()))

	content, err := os.ReadFile(filepath.Join(live, "marker"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
	_, err = os.Lstat(incoming)
	assert.ErrorIs(t, err, os.ErrNotExist)
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
// unprefixed namespace for the system, so an account named dd-agent would silently fail to apply --
// setFileOwnershipAndPermissions only warns on a failed chown -- and leave fleet configuration
// unreadable by the Agent. The group is the one the .dmg's postinstall script gives the tree.
func TestAgentAccountIsTheMacOSOne(t *testing.T) {
	assert.Equal(t, "_dd-agent", agentConfigUser)
	assert.Equal(t, "admin", agentConfigGroup)
}
