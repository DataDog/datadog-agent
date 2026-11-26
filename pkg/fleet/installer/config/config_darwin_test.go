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
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// newTestDirectories lays out the directories the way the installer does: etc holding a stable
// configuration and its deployment ID, and etc-exp resting as a sibling symlink to it.
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
	require.NoError(t, os.Symlink(dirs.StablePath, dirs.ExperimentPath))
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

// assertResting checks the experiment path is a symlink to the stable path: "no configuration
// experiment is deployed".
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
// directory: a host whose experiment failed must be indistinguishable from one that never started
// one.
func assertNoScratchLeftBehind(t *testing.T, dirs *Directories) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Dir(dirs.StablePath))
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), incomingPrefix), "scratch directory %s was left behind", entry.Name())
	}
}

func readState(t *testing.T, dirs *Directories) State {
	t.Helper()

	state, err := dirs.GetState()
	require.NoError(t, err)
	return state
}

// clearFlagsOnCleanup makes sure t.TempDir can remove path once the test is done.
func clearFlagsOnCleanup(t *testing.T, path string) {
	t.Helper()
	t.Cleanup(func() { _ = unix.Chflags(path, 0) })
}

func setACL(t *testing.T, path string, entry string) {
	t.Helper()
	require.NoError(t, exec.Command("/bin/chmod", "+a", entry, path).Run())
}

func readACL(t *testing.T, path string) string {
	t.Helper()
	listing, err := exec.Command("/bin/ls", "-led", path).Output()
	require.NoError(t, err)
	lines := strings.SplitN(string(listing), "\n", 2)
	if len(lines) < 2 {
		return ""
	}
	return lines[1]
}

// TestRemoveExperimentRestsTheExperimentPath pins that whatever is at the experiment path, nothing,
// the resting link or a directory, a remove leaves the resting link.
func TestRemoveExperimentRestsTheExperimentPath(t *testing.T) {
	for name, plant := range map[string]func(t *testing.T, path string){
		"absent":  func(t *testing.T, path string) { require.NoError(t, os.Remove(path)) },
		"resting": func(*testing.T, string) {},
		"directory": func(t *testing.T, path string) {
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0755))
		},
	} {
		t.Run(name, func(t *testing.T) {
			dirs := newTestDirectories(t)
			plant(t, dirs.ExperimentPath)

			require.NoError(t, dirs.RemoveExperiment(context.Background()))
			assertResting(t, dirs)
			assert.Equal(t, State{StableDeploymentID: "stable-1"}, readState(t, dirs))
		})
	}
}

func TestWriteExperimentPublishesAPatchedCopy(t *testing.T) {
	dirs := newTestDirectories(t)

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	content, err := os.ReadFile(filepath.Join(dirs.ExperimentPath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "debug")
	stable, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(stable), "warn", "the stable configuration must be untouched")
	assert.Equal(t, State{StableDeploymentID: "stable-1", ExperimentDeploymentID: "experiment-1"}, readState(t, dirs))
	assertNoScratchLeftBehind(t, dirs)
}

// TestGetStateThroughTheRestingLinkIsNoExperiment pins how "nothing deployed" reads: through the
// resting link both paths name the stable deployment ID.
func TestGetStateThroughTheRestingLinkIsNoExperiment(t *testing.T) {
	dirs := newTestDirectories(t)

	assert.Equal(t, State{StableDeploymentID: "stable-1"}, readState(t, dirs))
}

// TestReadDeploymentIDIgnoresWhatIsNotAnID pins that a deployment ID planted by the Agent's account
// can neither hang nor fail the state report: anything but a small file of printable characters
// reads as no ID.
func TestReadDeploymentIDIgnoresWhatIsNotAnID(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(outside, []byte("forged"), 0600))
	for name, plant := range map[string]func(t *testing.T, path string){
		"a symlink":   func(t *testing.T, path string) { require.NoError(t, os.Symlink(outside, path)) },
		"a fifo":      func(t *testing.T, path string) { require.NoError(t, syscall.Mkfifo(path, 0640)) },
		"a directory": func(t *testing.T, path string) { require.NoError(t, os.Mkdir(path, 0755)) },
		"too large": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, make([]byte, maxDeploymentIDSize+1), 0640))
		},
		"unprintable": func(t *testing.T, path string) { require.NoError(t, os.WriteFile(path, []byte("id\n\x00"), 0640)) },
	} {
		t.Run(name, func(t *testing.T) {
			dirs := newTestDirectories(t)
			path := filepath.Join(dirs.StablePath, deploymentIDFile)
			require.NoError(t, os.Remove(path))
			plant(t, path)

			assert.Equal(t, State{}, readState(t, dirs))
		})
	}
}

// TestWriteExperimentDoesNotCarryTheStableIDIntoTheExperiment pins that the copy of the stable
// tree never reports the stable deployment ID as the experiment's, even before the experiment's is
// written.
func TestWriteExperimentDoesNotCarryTheStableIDIntoTheExperiment(t *testing.T) {
	dirs := newTestDirectories(t)
	err := dirs.stageExperiment(context.Background(), func(dir string) error {
		assert.NoFileExists(t, filepath.Join(dir, deploymentIDFile))
		return errors.New("stop here")
	})
	require.Error(t, err)
	assertResting(t, dirs)
}

// TestWriteExperimentBuildsTheCopyInAPrivateDirectory pins that the experiment is patched where
// only root can reach it, and published only once complete.
func TestWriteExperimentBuildsTheCopyInAPrivateDirectory(t *testing.T) {
	dirs := newTestDirectories(t)
	require.NoError(t, dirs.stageExperiment(context.Background(), func(dir string) error {
		info, err := os.Stat(filepath.Dir(dir))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
		assertResting(t, dirs)
		return os.WriteFile(filepath.Join(dir, deploymentIDFile), []byte("experiment-1"), 0640)
	}))
	assert.Equal(t, "experiment-1", readState(t, dirs).ExperimentDeploymentID)
	assertNoScratchLeftBehind(t, dirs)
}

// TestWriteExperimentLeavesNoTraceWhenItFails is the property the scratch directory exists for:
// everything that can fail happens before the publish, so a failure is invisible.
func TestWriteExperimentLeavesNoTraceWhenItFails(t *testing.T) {
	dirs := newTestDirectories(t)

	require.Error(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{ this is not json`)))

	assertResting(t, dirs)
	assertNoScratchLeftBehind(t, dirs)
	assert.Equal(t, State{StableDeploymentID: "stable-1"}, readState(t, dirs))
}

// TestWriteExperimentReplacesADeployedExperiment pins that a start over a deployed experiment
// replaces it, as on Linux.
func TestWriteExperimentReplacesADeployedExperiment(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-2", `{"log_level":"trace"}`)))

	assert.Equal(t, State{StableDeploymentID: "stable-1", ExperimentDeploymentID: "experiment-2"}, readState(t, dirs))
	content, err := os.ReadFile(filepath.Join(dirs.ExperimentPath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "trace")
}

// TestWriteExperimentClearsAStrayExperimentDirectory pins that a real directory at the experiment
// path that is not a deployed experiment does not block a new one.
func TestWriteExperimentClearsAStrayExperimentDirectory(t *testing.T) {
	dirs := newTestDirectories(t)
	require.NoError(t, os.Remove(dirs.ExperimentPath))
	require.NoError(t, os.Mkdir(dirs.ExperimentPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.ExperimentPath, "stray"), nil, 0640))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	assert.NoFileExists(t, filepath.Join(dirs.ExperimentPath, "stray"))
	assert.Equal(t, "experiment-1", readState(t, dirs).ExperimentDeploymentID)
}

// TestCopyReproducesLinksWithoutFollowingThem pins that a link in the stable tree, into the
// experiment path or out of the tree, is copied as a link: following it would copy the tree into
// itself, or copy what lies outside it.
func TestCopyReproducesLinksWithoutFollowingThem(t *testing.T) {
	dirs := newTestDirectories(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600))
	require.NoError(t, os.Symlink(dirs.ExperimentPath, filepath.Join(dirs.StablePath, "loop")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dirs.StablePath, "escape")))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	for name, destination := range map[string]string{"loop": dirs.ExperimentPath, "escape": outside} {
		got, err := os.Readlink(filepath.Join(dirs.ExperimentPath, name))
		require.NoError(t, err, "%s was not copied as a link", name)
		assert.Equal(t, destination, got)
	}
}

func TestWriteExperimentPreservesModes(t *testing.T) {
	dirs := newTestDirectories(t)
	require.NoError(t, os.Chmod(dirs.StablePath, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, "conf.d", "secret.yaml"), []byte("token: x\n"), 0600))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	for path, mode := range map[string]os.FileMode{"": 0750, filepath.Join("conf.d", "secret.yaml"): 0600} {
		info, err := os.Stat(filepath.Join(dirs.ExperimentPath, path))
		require.NoError(t, err)
		assert.Equal(t, mode, info.Mode().Perm(), "mode of %q", path)
	}
}

// TestCopyKeepsExplicitACLsAndExtendedAttributes pins what /bin/cp carries over: an explicit
// access control entry and an extended attribute.
func TestCopyKeepsExplicitACLsAndExtendedAttributes(t *testing.T) {
	dirs := newTestDirectories(t)
	path := filepath.Join(dirs.StablePath, "datadog.yaml")
	setACL(t, path, "group:everyone allow read")
	require.NoError(t, unix.Setxattr(path, "com.datadoghq.test", []byte("value"), 0))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	copied := filepath.Join(dirs.ExperimentPath, "datadog.yaml")
	assert.Contains(t, readACL(t, copied), "group:everyone allow read")
	value := make([]byte, 16)
	n, err := unix.Getxattr(copied, "com.datadoghq.test", value)
	require.NoError(t, err)
	assert.Equal(t, "value", string(value[:n]))
}

// TestWriteExperimentRefusesAFileWithSeveralLinks covers a file of the stable tree that is also
// linked from elsewhere: cp would turn it into an independent copy of a file the Agent's account
// may not be able to read, so the start is refused.
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
}

// TestWriteExperimentClearsFileFlags pins that a flag set in the stable tree does not reach the
// experiment: a flagged copy could be neither patched nor removed by a stop.
func TestWriteExperimentClearsFileFlags(t *testing.T) {
	dirs := newTestDirectories(t)
	flagged := filepath.Join(dirs.StablePath, "conf.d", "locked.yaml")
	require.NoError(t, os.WriteFile(flagged, []byte("a: b\n"), 0640))
	require.NoError(t, unix.Chflags(flagged, unix.UF_IMMUTABLE))
	clearFlagsOnCleanup(t, flagged)

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	var st unix.Stat_t
	require.NoError(t, unix.Lstat(filepath.Join(dirs.ExperimentPath, "conf.d", "locked.yaml"), &st))
	assert.Zero(t, st.Flags, "the copy kept the stable file's flags")
	require.NoError(t, dirs.RemoveExperiment(context.Background()))
	assertResting(t, dirs)
}

// TestWriteExperimentCopiesANameWithAControlCharacter pins that such a name is copied like any
// other: nothing parses names any more.
func TestWriteExperimentCopiesANameWithAControlCharacter(t *testing.T) {
	dirs := newTestDirectories(t)
	name := "tab\there.yaml"
	require.NoError(t, os.WriteFile(filepath.Join(dirs.StablePath, "conf.d", name), nil, 0640))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level": "debug"}`)))
	assert.FileExists(t, filepath.Join(dirs.ExperimentPath, "conf.d", name))
}

// TestWriteExperimentRecopiesAFileTheTreeOwnerDoesNotOwn needs root: only root can own a file
// inside a tree another account owns. Such a file is read again from the stable tree through a
// handle, so its copy holds what that file holds, and the tree's owner keeps its access to it.
func TestWriteExperimentRecopiesAFileTheTreeOwnerDoesNotOwn(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to create entries with another owner")
	}
	nobody, err := user.Lookup("nobody")
	require.NoError(t, err)
	uid, err := strconv.Atoi(nobody.Uid)
	require.NoError(t, err)
	gid, err := strconv.Atoi(nobody.Gid)
	require.NoError(t, err)
	dirs := newTestDirectories(t)
	require.NoError(t, filepath.Walk(dirs.StablePath, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, uid, gid)
	}))
	foreign := filepath.Join(dirs.StablePath, "conf.d", "foreign.yaml")
	require.NoError(t, os.WriteFile(foreign, []byte("root: only\n"), 0600))
	require.NoError(t, os.Chown(foreign, 0, 0))

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))

	content, err := os.ReadFile(filepath.Join(dirs.ExperimentPath, "conf.d", "foreign.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "root: only\n", string(content))
	// The tree's owner keeps access to it, as it has in the stable tree through an inherited entry.
	assert.Contains(t, readACL(t, filepath.Join(dirs.ExperimentPath, "conf.d", "foreign.yaml")), "user:"+nobody.Username+" allow read,write")
}

// TestAgentConfigACEsAreTheDmgOnes pins the entries an entry the tree's owner does not own is
// given: the ones the .dmg set-up gives the whole tree.
func TestAgentConfigACEsAreTheDmgOnes(t *testing.T) {
	assert.Equal(t, "user:_dd-agent allow read,write,readattr,writeattr,readextattr,writeextattr,readsecurity,list,search,add_file,add_subdirectory,file_inherit,directory_inherit", AgentConfigDirACE)
	assert.Equal(t, "user:_dd-agent allow read,write,readattr,writeattr,readextattr,writeextattr,readsecurity", AgentConfigFileACE)
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

// TestWriteExperimentDoesNotMigrateALegacyLayout pins that macOS, which never had the legacy
// managed/ layout, does not run its migration: that walk reads the Agent-writable stable tree as
// root and follows a managed/datadog-agent/stable link wherever it points.
func TestWriteExperimentDoesNotMigrateALegacyLayout(t *testing.T) {
	assert.False(t, migrateLegacyLayout)
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
	assert.Equal(t, State{StableDeploymentID: "experiment-1"}, readState(t, dirs))
}

// TestPromoteExperimentRefusesWhenNothingIsDeployed pins that only a deployed experiment is
// promoted: the exchange would otherwise swap the stable directory with the resting link, or with
// a stray directory.
func TestPromoteExperimentRefusesWhenNothingIsDeployed(t *testing.T) {
	for name, plant := range map[string]func(t *testing.T, path string){
		"resting": func(*testing.T, string) {},
		"a directory with no ID": func(t *testing.T, path string) {
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0755))
		},
	} {
		t.Run(name, func(t *testing.T) {
			dirs := newTestDirectories(t)
			plant(t, dirs.ExperimentPath)

			err := dirs.PromoteExperiment(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no configuration experiment")
			content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
			require.NoError(t, err)
			assert.Contains(t, string(content), "warn")
			assert.Equal(t, "stable-1", readState(t, dirs).StableDeploymentID)
		})
	}
}

// TestPromoteLeavesBothDirectoriesWhenTheSwapFails pins that a failed exchange moves nothing: the
// experiment is still deployed beside an intact stable configuration.
func TestPromoteLeavesBothDirectoriesWhenTheSwapFails(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))
	boom := errors.New("boom")
	original := renameSwap
	renameSwap = func(string, string) error { return boom }
	t.Cleanup(func() { renameSwap = original })

	require.ErrorIs(t, dirs.PromoteExperiment(ctx), boom)

	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err, "the stable configuration was disturbed")
	assert.Contains(t, string(content), "warn")
	assert.Equal(t, State{StableDeploymentID: "stable-1", ExperimentDeploymentID: "experiment-1"}, readState(t, dirs))
}

// TestPromoteFallsBackWhenTheSwapIsUnsupported pins the fallback on a volume that cannot exchange
// two paths: the promote renames the directories instead, as on Linux.
func TestPromoteFallsBackWhenTheSwapIsUnsupported(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))
	original := renameSwap
	renameSwap = func(string, string) error { return unix.ENOTSUP }
	t.Cleanup(func() { renameSwap = original })

	require.NoError(t, dirs.PromoteExperiment(ctx))

	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "debug")
	assert.Equal(t, "experiment-1", readState(t, dirs).StableDeploymentID)
}

// TestAPromoteInterruptedAfterTheSwapIsReadAsPromoted covers a crash between the exchange and the
// removal of what it left at the experiment path: the previous stable directory must not read as
// a deployed experiment, and the next remove finishes the promote.
func TestAPromoteInterruptedAfterTheSwapIsReadAsPromoted(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	require.NoError(t, exchangeDirectories(dirs.StablePath, dirs.ExperimentPath))

	assert.Equal(t, State{StableDeploymentID: "experiment-1"}, readState(t, dirs))
	require.NoError(t, dirs.RemoveExperiment(ctx))
	assertResting(t, dirs)
	assert.Equal(t, State{StableDeploymentID: "experiment-1"}, readState(t, dirs))
}

func TestRemoveExperimentDiscardsTheExperiment(t *testing.T) {
	dirs := newTestDirectories(t)
	ctx := context.Background()
	require.NoError(t, dirs.WriteExperiment(ctx, mergePatch("experiment-1", `{"log_level":"debug"}`)))

	require.NoError(t, dirs.RemoveExperiment(ctx))

	assertResting(t, dirs)
	assert.Equal(t, State{StableDeploymentID: "stable-1"}, readState(t, dirs))
	content, err := os.ReadFile(filepath.Join(dirs.StablePath, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "warn")
}

// TestStaleScratchDirectoriesAreDiscarded pins that a scratch directory a crashed start left
// behind, flags included, goes with the next start.
func TestStaleScratchDirectoriesAreDiscarded(t *testing.T) {
	dirs := newTestDirectories(t)
	stale := filepath.Join(filepath.Dir(dirs.StablePath), incomingPrefix+"crashed")
	require.NoError(t, os.MkdirAll(filepath.Join(stale, "etc"), 0700))
	flagged := filepath.Join(stale, "etc", "locked")
	require.NoError(t, os.WriteFile(flagged, nil, 0640))
	require.NoError(t, unix.Chflags(flagged, unix.UF_IMMUTABLE))
	clearFlagsOnCleanup(t, flagged)

	require.NoError(t, dirs.WriteExperiment(context.Background(), mergePatch("experiment-1", `{"log_level":"debug"}`)))
	assert.NoDirExists(t, stale)
}

// TestAgentAccountIsTheMacOSOne pins the ownership the patched files are given. macOS reserves the
// unprefixed namespace for the system, so an account named dd-agent would silently fail to apply --
// setFileOwnershipAndPermissions only warns on a failed chown -- and leave fleet configuration
// unreadable by the Agent. The group is the one the installer's .dmg set-up gives the tree.
func TestAgentAccountIsTheMacOSOne(t *testing.T) {
	assert.Equal(t, "_dd-agent", AgentUser)
	assert.Equal(t, "admin", AgentGroup)
	assert.Equal(t, "/bin/cp", cpPath)
}

func TestIsPrintable(t *testing.T) {
	for id, want := range map[string]bool{"abc-123": true, "": true, "a\nb": false, "a\x00": false, "\x7f": false} {
		assert.Equal(t, want, isPrintable([]byte(id)), fmt.Sprintf("%q", id))
	}
}
