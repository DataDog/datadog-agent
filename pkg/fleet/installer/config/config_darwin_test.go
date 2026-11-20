// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

import (
	"context"
	"errors"
	"fmt"
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
		StatePath:      filepath.Join(root, "state", StateFileName),
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dirs.StablePath, "conf.d"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, "datadog.yaml"), []byte("log_level: warn\n"), 0640))
	require.NoError(t, os.Mkdir(filepath.Dir(dirs.StatePath), 0755))
	require.NoError(t, dirs.writeState(configState{StableDeploymentID: "stable-1"}))
	require.NoError(t, dirs.experimentLink().Rest())
	return dirs
}

// useTestStateDir points the default state file at a new directory for the rest of the test, as
// the installer's own state directory is root-owned and may not exist on a development host.
func useTestStateDir(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, os.Mkdir(dir, 0755))
	original := defaultStatePath
	defaultStatePath = filepath.Join(dir, StateFileName)
	t.Cleanup(func() { defaultStatePath = original })
}

// readTestState returns the state file as recorded.
func readTestState(t *testing.T, dirs *Directories) configState {
	t.Helper()

	state, err := dirs.readState()
	require.NoError(t, err)
	return state
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

// TestMarkersInTheTreeAreIgnored pins that the configuration tree carries no state: deployment
// IDs and promote markers planted in it change nothing that is reported.
func TestMarkersInTheTreeAreIgnored(t *testing.T) {
	dirs := newTestDirectories(t)
	for _, name := range []string{".deployment-id", ".promoting"} {
		require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, name), []byte("planted"), 0640))
	}

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "stable-1"}, state)

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))
	for _, name := range []string{".deployment-id", ".promoting"} {
		require.NoError(t, os.WriteFile(filepath.Join(dirs.ExperimentPath, name), []byte("planted"), 0640))
	}
	state, err = dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "stable-1", ExperimentDeploymentID: "experiment-1"}, state)
}

// TestALegacyStableDeploymentIDIsCarriedOver covers a host whose configuration experiment a build
// before the state file promoted: that build left the stable ID in etc/.deployment-id. With no ID
// recorded yet, it is reported as the stable ID, and the next experiment records it.
func TestALegacyStableDeploymentIDIsCarriedOver(t *testing.T) {
	dirs := newTestDirectories(t)
	require.NoError(t, os.Remove(dirs.StatePath))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, ".deployment-id"), []byte("legacy-1"), 0640))

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "legacy-1"}, state)

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))
	assert.Equal(t, "legacy-1", readTestState(t, dirs).StableDeploymentID)
}

// TestALinkedLegacyDeploymentIDIsIgnored pins that the legacy ID is only taken from a file the
// Agent's account cannot have planted: a link, or a file linked from elsewhere, reads as no ID.
func TestALinkedLegacyDeploymentIDIsIgnored(t *testing.T) {
	for name, plant := range map[string]func(t *testing.T, outside, path string){
		"a symlink":  func(t *testing.T, outside, path string) { require.NoError(t, os.Symlink(outside, path)) },
		"a hardlink": func(t *testing.T, outside, path string) { require.NoError(t, os.Link(outside, path)) },
	} {
		t.Run(name, func(t *testing.T) {
			dirs := newTestDirectories(t)
			require.NoError(t, os.Remove(dirs.StatePath))
			outside := filepath.Join(t.TempDir(), "id")
			require.NoError(t, os.WriteFile(outside, []byte("forged"), 0640))
			plant(t, outside, filepath.Join(dirs.StablePath, ".deployment-id"))

			state, err := dirs.GetState()
			require.NoError(t, err)
			assert.Empty(t, state.StableDeploymentID)
		})
	}
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

// TestARecordWithNoPublishedDirectoryIsNotAnExperiment covers a crash between recording an
// experiment and publishing it: the record names a directory found at neither path.
func TestARecordWithNoPublishedDirectoryIsNotAnExperiment(t *testing.T) {
	dirs := newTestDirectories(t)
	scratch := filepath.Join(filepath.Dir(dirs.StablePath), incomingPrefix+"crashed")
	require.NoError(t, os.Mkdir(scratch, 0700))
	inode, ok, err := inodeOf(scratch)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, dirs.writeState(configState{
		StableDeploymentID: "stable-1",
		Experiment:         &experimentRecord{DeploymentID: "experiment-1", Inode: inode},
	}))

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "stable-1"}, state)

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-2", `{"log_level":"debug"}`)))
	state, err = dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "stable-1", ExperimentDeploymentID: "experiment-2"}, state)
	assertNoScratchLeftBehind(t, dirs)
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

	_, err = copyRegularFile(source, target, "datadog.yaml")
	require.Error(t, err)

	assert.NoFileExists(t, filepath.Join(targetPath, "datadog.yaml"))
}

// TestWriteExperimentRefusesAFileWithSeveralLinks covers a file of the stable tree that is also
// linked from elsewhere: its content and owner belong to that other file, so the copy refuses it
// and no experiment is published.
func TestWriteExperimentRefusesAFileWithSeveralLinks(t *testing.T) {
	dirs := newTestDirectories(t)
	outside := filepath.Join(t.TempDir(), "elsewhere.yaml")
	require.NoError(t, os.WriteFile(outside, []byte("token: x\n"), 0600))
	require.NoError(t, os.Link(outside, filepath.Join(dirs.StablePath, "conf.d", "linked.yaml")))

	err := dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level": "debug"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "linked.yaml")

	assertResting(t, dirs)
	assertNoScratchLeftBehind(t, dirs)
	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Empty(t, state.ExperimentDeploymentID)
}

func TestWriteExperimentRefusesANameWithAControlCharacter(t *testing.T) {
	for _, name := range []string{"k\n 0: user:nobody allow read", "tab\there.yaml", "del\x7f.yaml"} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			dirs := newTestDirectories(t)
			require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, "conf.d", name), nil, 0640))

			err := dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level": "debug"}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "control character")

			assertResting(t, dirs)
			assertNoScratchLeftBehind(t, dirs)
		})
	}
}

// TestCopyKeepsAccessControlListsOnlyForTheTreeOwnersEntries needs root: only root can own a file
// inside a tree that another account owns.
func TestCopyKeepsAccessControlListsOnlyForTheTreeOwnersEntries(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to create entries with another owner")
	}
	dirs := newTestDirectories(t)
	nobody, err := user.Lookup("nobody")
	require.NoError(t, err)
	var uid, gid int
	_, err = fmt.Sscan(nobody.Uid, &uid)
	require.NoError(t, err)
	_, err = fmt.Sscan(nobody.Gid, &gid)
	require.NoError(t, err)
	require.NoError(t, filepath.Walk(dirs.StablePath, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, uid, gid)
	}))
	owned := filepath.Join(dirs.StablePath, "conf.d", "owned.yaml")
	foreign := filepath.Join(dirs.StablePath, "conf.d", "foreign.yaml")
	for _, path := range []string{owned, foreign} {
		require.NoError(t, os.WriteFile(path, nil, 0600))
		setACL(t, path, "group:everyone allow read")
	}
	require.NoError(t, os.Chown(owned, uid, gid))
	require.NoError(t, os.Chown(foreign, 0, 0))

	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	tree := configTree{sourcePath: dirs.StablePath, targetPath: incoming}
	require.NoError(t, tree.Copy(context.Background()))

	assert.Equal(t, readACL(t, owned), readACL(t, filepath.Join(incoming, "conf.d", "owned.yaml")))
	assert.Empty(t, readACL(t, filepath.Join(incoming, "conf.d", "foreign.yaml")))
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
// configuration file has the mode the installer's .dmg set-up gives the tree.
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
// unreadable by the Agent. The group is the one the installer's .dmg set-up gives the tree.
func TestAgentAccountIsTheMacOSOne(t *testing.T) {
	assert.Equal(t, "_dd-agent", agentConfigUser)
	assert.Equal(t, "admin", agentConfigGroup)
}

// TestReadStateRefusesWhatItShouldNotRead pins the checks made before the state file is trusted.
func TestReadStateRefusesWhatItShouldNotRead(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, []byte(`{"stable_deployment_id":"forged"}`), 0600))

	for name, plant := range map[string]func(t *testing.T, path string){
		"a symlink": func(t *testing.T, path string) {
			require.NoError(t, os.Symlink(secret, path))
		},
		"a directory": func(t *testing.T, path string) {
			require.NoError(t, os.Mkdir(path, 0755))
		},
		"a file writable by others": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(`{}`), 0644))
			require.NoError(t, os.Chmod(path, 0666))
		},
		"a directory writable by others": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(`{}`), 0644))
			require.NoError(t, os.Chmod(filepath.Dir(path), 0777))
		},
		"an oversized file": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(strings.Repeat(" ", maxStateSize+1)), 0644))
		},
		"malformed content": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte("not json"), 0644))
		},
	} {
		t.Run(name, func(t *testing.T) {
			dirs := &Directories{StatePath: filepath.Join(t.TempDir(), "state", StateFileName)}
			require.NoError(t, os.Mkdir(filepath.Dir(dirs.StatePath), 0755))
			plant(t, dirs.StatePath)
			_, err := dirs.readState()
			assert.Error(t, err)
		})
	}
}

// TestReadStateRefusesAnotherOwner needs root: only root can create a file owned by another
// account.
func TestReadStateRefusesAnotherOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to create a file with another owner")
	}
	dirs := &Directories{StatePath: filepath.Join(t.TempDir(), "state", StateFileName)}
	require.NoError(t, os.Mkdir(filepath.Dir(dirs.StatePath), 0755))
	require.NoError(t, dirs.writeState(configState{StableDeploymentID: "stable-1"}))
	require.NoError(t, os.Chown(dirs.StatePath, 1, 1))

	_, err := dirs.readState()
	assert.Error(t, err)
}

func TestReadStateRoundTrips(t *testing.T) {
	dirs := &Directories{StatePath: filepath.Join(t.TempDir(), "state", StateFileName)}
	state, err := dirs.readState()
	require.NoError(t, err)
	assert.Equal(t, configState{}, state, "a missing state directory reports no state")

	require.NoError(t, os.Mkdir(filepath.Dir(dirs.StatePath), 0755))
	state, err = dirs.readState()
	require.NoError(t, err)
	assert.Equal(t, configState{}, state, "a missing state file reports no state")

	want := configState{
		StableDeploymentID: "jq-replace-tag:123-0",
		Experiment:         &experimentRecord{DeploymentID: "experiment-1", Inode: 42},
	}
	require.NoError(t, dirs.writeState(want))
	state, err = dirs.readState()
	require.NoError(t, err)
	assert.Equal(t, want, state)
	info, err := os.Lstat(dirs.StatePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

// TestWriteExperimentDoesNotMigrateALegacyLayout pins that macOS, which never had the legacy
// managed/ layout, does not run its migration: that walk reads the Agent-writable stable tree as
// root and follows a managed/datadog-agent/stable link wherever it points.
func TestWriteExperimentDoesNotMigrateALegacyLayout(t *testing.T) {
	dirs := newTestDirectories(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "datadog.yaml"), []byte("api_key: stolen\n"), 0600))
	managed := filepath.Join(dirs.StablePath, "managed", "datadog-agent")
	require.NoError(t, os.MkdirAll(managed, 0755))
	require.NoError(t, os.Symlink(outside, filepath.Join(managed, "stable")))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	content, err := os.ReadFile(filepath.Join(dirs.ExperimentPath, "datadog.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "stolen", "a file reached through the legacy link must not be merged in")
	assert.Contains(t, string(content), "debug")
}

// TestAPromoteInterruptedAfterTheSwapIsReadAsPromoted covers a crash between the swap and recording
// it: the recorded experiment directory is then at the stable path, and the experiment path holds
// the previous stable configuration, which must not be reported as a deployed experiment.
func TestAPromoteInterruptedAfterTheSwapIsReadAsPromoted(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	// PromoteExperiment up to and including the swap, then the crash.
	require.NoError(t, renameSwap(dirs.ExperimentPath, dirs.StablePath))

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "experiment-1"}, state)

	// Recovery discards what is left, which finishes the promote.
	require.NoError(t, dirs.RemoveExperiment(ctx))
	assertResting(t, dirs)
	assert.Equal(t, configState{StableDeploymentID: "experiment-1"}, readTestState(t, dirs))
	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "debug", "the promoted configuration should be the stable one")
}

// TestAPromoteInterruptedBeforeRestingIsFinished covers a crash after the promote is recorded but
// before the link is put back: the previous stable configuration still stands at the experiment
// path, and is cleared by the next remove or write.
func TestAPromoteInterruptedBeforeRestingIsFinished(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))
	// PromoteExperiment up to and including recording the promote, then the crash.
	require.NoError(t, renameSwap(dirs.ExperimentPath, dirs.StablePath))
	require.NoError(t, dirs.writeState(configState{StableDeploymentID: "experiment-1"}))

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "experiment-1"}, state)

	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-2", `{"log_level":"info"}`)))
	state, err = dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "experiment-1", ExperimentDeploymentID: "experiment-2"}, state)
	assertNoScratchLeftBehind(t, dirs)
}

// TestARemoveInterruptedAfterRestingIsNotAnExperiment covers a crash between resting the link and
// clearing the record: the record then names a directory that is gone.
func TestARemoveInterruptedAfterRestingIsNotAnExperiment(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	require.NoError(t, dirs.experimentLink().Rest())

	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, State{StableDeploymentID: "stable-1"}, state)
	require.NoError(t, dirs.RemoveExperiment(ctx))
	assert.Equal(t, configState{StableDeploymentID: "stable-1"}, readTestState(t, dirs))
}

// TestWriteExperimentClearsAStrayExperimentDirectory pins that a real directory at the
// experiment path that is not the recorded experiment does not block a new one.
func TestWriteExperimentClearsAStrayExperimentDirectory(t *testing.T) {
	dirs := newTestDirectories(t)
	require.NoError(t, os.Remove(dirs.ExperimentPath))
	require.NoError(t, os.Mkdir(dirs.ExperimentPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.ExperimentPath, "stray"), nil, 0640))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	assert.NoFileExists(t, filepath.Join(dirs.ExperimentPath, "stray"))
	state, err := dirs.GetState()
	require.NoError(t, err)
	assert.Equal(t, "experiment-1", state.ExperimentDeploymentID)
}

func TestRemoveExperimentWithNoStateDirectoryWritesNothing(t *testing.T) {
	root := t.TempDir()
	dirs := &Directories{
		StablePath:     filepath.Join(root, "etc"),
		ExperimentPath: filepath.Join(root, "etc-exp"),
		StatePath:      filepath.Join(root, "state", StateFileName),
	}
	require.NoError(t, os.Mkdir(dirs.StablePath, 0755))

	require.NoError(t, dirs.RemoveExperiment(context.Background()))

	assertResting(t, dirs)
	assert.NoDirExists(t, filepath.Dir(dirs.StatePath))
}

func TestSplitListing(t *testing.T) {
	listing := `drwxrwx---+ 3 _dd-agent  admin  96 Oct  1 12:00 /etc/conf.d
 0: user:_dd-agent allow list,add_file,search,file_inherit,directory_inherit
-rw-rw----  1 _dd-agent  admin  0 Oct  1 12:00 /etc/a file.yaml
-rw-rw----+ 1 _dd-agent  admin  0 Oct  1 12:00 /etc/datadog.yaml
 0: group:everyone allow read
 1: user:_dd-agent inherited allow read,write
`
	blocks := splitListing([]byte(listing))
	require.Len(t, blocks, 3)
	assert.Equal(t, []string{"user:_dd-agent allow list,add_file,search,file_inherit,directory_inherit"}, aclEntries(blocks[0]))
	assert.Empty(t, aclEntries(blocks[1]))
	assert.Equal(t, []string{"group:everyone allow read", "user:_dd-agent allow read,write"}, aclEntries(blocks[2]))
}

// TestCopyReproducesAccessControlListsAcrossBatches pins that batching the access control list
// copy never hands an entry another entry's list: more entries than one batch holds, two distinct
// lists, entries with none, and a name ls cannot list on one line.
func TestCopyReproducesAccessControlListsAcrossBatches(t *testing.T) {
	dirs := newTestDirectories(t)
	var paths []string
	for i := 0; i < aclBatchSize+40; i++ {
		path := filepath.Join("conf.d", fmt.Sprintf("check-%03d.yaml", i))
		require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, path), nil, 0640))
		switch i % 3 {
		case 0:
			setACL(t, filepath.Join(dirs.StablePath, path), "group:everyone allow read")
		case 1:
			setACL(t, filepath.Join(dirs.StablePath, path), "group:everyone deny write")
		}
		paths = append(paths, path)
	}
	incoming := filepath.Join(filepath.Dir(dirs.StablePath), ".incoming")
	tree := configTree{sourcePath: dirs.StablePath, targetPath: incoming}
	require.NoError(t, tree.Copy(context.Background()))

	for _, path := range paths {
		assert.Equal(t, readACL(t, filepath.Join(dirs.StablePath, path)), readACL(t, filepath.Join(incoming, path)),
			"access control list of %q", path)
	}
}
