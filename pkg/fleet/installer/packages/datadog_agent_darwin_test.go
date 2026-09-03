// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// testLayout returns a layout rooted in temporary directories and owned by the user running the
// test, so the ownership pass the hook performs is permitted without root.
func testLayout(t *testing.T) agentLayout {
	t.Helper()

	current, err := user.Current()
	require.NoError(t, err)
	group, err := user.LookupGroupId(current.Gid)
	require.NoError(t, err)

	root := t.TempDir()
	return agentLayout{
		installRoot:  filepath.Join(root, "opt", "datadog-agent"),
		linkDir:      filepath.Join(root, "usr", "local", "bin"),
		packagesRoot: filepath.Join(root, "opt", "datadog-packages"),
		owner:        current.Username,
		group:        group.Name,
	}
}

// stubLaunchd replaces the launchd client with one that records its invocations instead of
// running launchctl.
func stubLaunchd(t *testing.T) *[][]string {
	t.Helper()

	var calls [][]string
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			calls = append(calls, args)
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	return &calls
}

// stubAgentUser stops the hook reaching the machine's directory service.
func stubAgentUser(t *testing.T) {
	t.Helper()

	original := ensureAgentUser
	ensureAgentUser = func(context.Context, string) error { return nil }
	t.Cleanup(func() { ensureAgentUser = original })
}

func testHookContext(t *testing.T) HookContext {
	t.Helper()
	return HookContext{Context: context.Background(), Package: agentPackage, PackageType: PackageTypeOCI}
}

func TestInstallFilesystemCreatesTheStateDirectoriesAndTheLinks(t *testing.T) {
	stubAgentUser(t)
	layout := testLayout(t)

	require.NoError(t, installFilesystem(testHookContext(t), layout))

	for _, dir := range []string{
		layout.installRoot,
		layout.etcDir(),
		filepath.Join(layout.etcDir(), "managed"),
		layout.runDir(),
		filepath.Join(layout.runDir(), "ipc"),
		layout.logDir(),
	} {
		info, err := os.Stat(dir)
		require.NoError(t, err, "missing directory %s", dir)
		assert.True(t, info.IsDir(), "%s is not a directory", dir)
	}

	// The convenience links name the install root, which is the same address for the life of
	// the machine, so they never need updating.
	for link, want := range layout.convenienceLinks() {
		target, err := os.Readlink(link)
		require.NoError(t, err, "missing convenience link %s", link)
		assert.Equal(t, want, target)
		assert.True(t, strings.HasPrefix(target, layout.installRoot),
			"convenience link %s points outside the install root", link)
	}
}

// TestInstallFilesystemIsIdempotent is the property both install paths rely on: the hook runs on a
// first install, on every upgrade, and again on a host already in the desired state.
func TestInstallFilesystemIsIdempotent(t *testing.T) {
	stubAgentUser(t)
	layout := testLayout(t)
	ctx := testHookContext(t)

	require.NoError(t, installFilesystem(ctx, layout))

	// Something the operator left behind in the configuration directory must survive.
	configFile := filepath.Join(layout.etcDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("api_key: unchanged\n"), 0640))

	require.NoError(t, installFilesystem(ctx, layout))

	content, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.Equal(t, "api_key: unchanged\n", string(content))
}

// TestInstallFilesystemLeavesRestingEtcExpAlone is the constraint the configuration layer imposes
// on this hook. etc-exp rests as a symlink to etc; a recursive ownership pass that traversed it
// would write through to the stable configuration, and the hook must not create, remove or
// dereference it at all.
func TestInstallFilesystemLeavesRestingEtcExpAlone(t *testing.T) {
	stubAgentUser(t)
	layout := testLayout(t)
	ctx := testHookContext(t)

	require.NoError(t, installFilesystem(ctx, layout))

	// Put the host in the idle state the configuration layer maintains.
	require.NoError(t, os.Symlink(layout.etcDir(), layout.etcExpDir()))
	sentinel := filepath.Join(layout.etcDir(), "sentinel.yaml")
	require.NoError(t, os.WriteFile(sentinel, []byte("sentinel"), 0600))
	before, err := os.Stat(sentinel)
	require.NoError(t, err)

	require.NoError(t, installFilesystem(ctx, layout))

	// Still a symlink, still pointing at etc: nothing created it as a real directory and
	// nothing removed it.
	info, err := os.Lstat(layout.etcExpDir())
	require.NoError(t, err)
	assert.Equal(t, os.ModeSymlink, info.Mode()&os.ModeSymlink,
		"etc-exp is no longer a symlink")
	target, err := os.Readlink(layout.etcExpDir())
	require.NoError(t, err)
	assert.Equal(t, layout.etcDir(), target)

	// The mode of a file in etc is unchanged, so no pass wrote through the resting symlink.
	after, err := os.Stat(sentinel)
	require.NoError(t, err)
	assert.Equal(t, before.Mode(), after.Mode())
}

// TestConfigPermissionsAreRootedAtEtc is the structural reason the assertion above holds: every
// recursive ownership pass is rooted inside etc, never at the install root, whose children
// include the resting etc-exp symlink.
func TestConfigPermissionsAreRootedAtEtc(t *testing.T) {
	layout := testLayout(t)
	for _, permission := range layout.configPermissions() {
		assert.False(t, filepath.IsAbs(permission.Path),
			"config permission %q escapes etc", permission.Path)
		assert.NotContains(t, permission.Path, "..",
			"config permission %q escapes etc", permission.Path)
	}
	for _, dir := range layout.directories() {
		assert.NotEqual(t, layout.etcExpDir(), dir.Path,
			"the hook must not create etc-exp; the configuration layer owns it alone")
	}
}

// TestRegisterPackageRepositoryLetsAConfigExperimentStart is the reason the hook registers a
// package at all. InstallConfigExperiment cleans the package repository before it writes the
// experiment, so on a host without one that read fails with ENOENT and no configuration
// experiment can start -- which is every macOS host, since the Agent arrives in a .dmg.
func TestRegisterPackageRepositoryLetsAConfigExperimentStart(t *testing.T) {
	layout := testLayout(t)
	ctx := testHookContext(t)
	repositories := repository.NewRepositories(layout.packagesRoot, AsyncPreRemoveHooks)

	require.Error(t, repositories.Get(agentPackage).DeleteExperiment(ctx),
		"the repository is meant to be missing before the hook runs")

	require.NoError(t, registerPackageRepository(ctx, layout))

	assert.NoError(t, repositories.Get(agentPackage).DeleteExperiment(ctx))
}

// TestRegisterPackageRepositoryReportsTheInstalledVersion covers what the entry tells the backend:
// the state the installer reports for the package must name the Agent that is actually installed.
func TestRegisterPackageRepositoryReportsTheInstalledVersion(t *testing.T) {
	layout := testLayout(t)
	repositories := repository.NewRepositories(layout.packagesRoot, AsyncPreRemoveHooks)

	require.NoError(t, registerPackageRepository(testHookContext(t), layout))

	state, err := repositories.GetState(agentPackage)
	require.NoError(t, err)
	assert.Equal(t, version.AgentVersion, state.Stable)
	assert.Empty(t, state.Experiment, "a fresh install must not look like it has a version experiment")
}

// TestRegisterPackageRepositoryIsIdempotent is the property every hook here needs: the .dmg runs
// postInstall on a first install, on every upgrade, and again on a host already in this state.
func TestRegisterPackageRepositoryIsIdempotent(t *testing.T) {
	layout := testLayout(t)
	ctx := testHookContext(t)

	require.NoError(t, registerPackageRepository(ctx, layout))
	require.NoError(t, registerPackageRepository(ctx, layout))

	state, err := repository.NewRepositories(layout.packagesRoot, AsyncPreRemoveHooks).GetState(agentPackage)
	require.NoError(t, err)
	assert.Equal(t, version.AgentVersion, state.Stable)

	// The placeholder directory Create moves in must not accumulate one temporary directory per
	// run: the packages root holds the package and nothing else.
	entries, err := os.ReadDir(layout.packagesRoot)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{agentPackage}, names)
}

func TestInstallStableJobsLoadsTheStableSetIncludingTheInstaller(t *testing.T) {
	calls := stubLaunchd(t)
	dir := t.TempDir()
	original := launchdJobDir
	launchdJobDir = dir
	t.Cleanup(func() { launchdJobDir = original })

	require.NoError(t, installStableJobs(testHookContext(t)))

	for _, label := range stableJobs {
		path := filepath.Join(dir, label+".plist")
		content, err := os.ReadFile(path)
		require.NoError(t, err, "job definition %s was not written", label)
		assert.Contains(t, string(content), "<string>"+label+"</string>")
	}

	// The experiment set is not loaded on install: an experiment is started by the backend.
	for _, args := range *calls {
		for _, arg := range args {
			assert.NotContains(t, arg, "-exp", "install loaded an experiment job: %v", args)
		}
	}

	// Every job is enabled as well as bootstrapped: a disabled override survives bootout and a
	// rewritten definition, so a job disabled by a previous uninstall would load and never run.
	var enabled []string
	for _, args := range *calls {
		if len(args) == 2 && args[0] == "enable" {
			enabled = append(enabled, args[1])
		}
	}
	for _, label := range stableJobs {
		assert.Contains(t, enabled, "system/"+label)
	}
}

func TestPreRemoveStopsBothJobSets(t *testing.T) {
	calls := stubLaunchd(t)
	dir := t.TempDir()
	original := launchdJobDir
	launchdJobDir = dir
	t.Cleanup(func() { launchdJobDir = original })

	ctx := testHookContext(t)
	ctx.Upgrade = true
	require.NoError(t, preRemoveDatadogAgent(ctx))

	var bootedOut []string
	for _, args := range *calls {
		if len(args) == 2 && args[0] == "bootout" {
			bootedOut = append(bootedOut, args[1])
		}
	}
	for _, label := range append(append([]string{}, experimentJobs...), stableJobs...) {
		assert.Contains(t, bootedOut, "system/"+label)
	}

	// The experiment set goes first: a leftover -exp job would otherwise keep running against a
	// configuration that is about to be removed.
	assert.Equal(t, "system/"+experimentJobs[0], bootedOut[0])
}
