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
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// notLoadedOutput is real launchctl output for a label that is not in the domain, used so the
// stub below can answer Bootout's post-bootout print poll without spinning for the full
// BootoutSettleTimeout. Kept in sync with launchd_test.go's fixture of the same name.
const notLoadedOutput = `Bad request.
Could not find service "com.datadoghq.agent" in domain for system
`

// exitError stands in for the *exec.ExitError a failed launchctl run returns: the client reads
// the status it exited with, not its output.
type exitError int

func (e exitError) Error() string { return "exit status " + strconv.Itoa(int(e)) }
func (e exitError) ExitCode() int { return int(e) }

// stubLaunchd replaces the launchd client with one that records its invocations instead of
// running launchctl. print always reports the label as not loaded, so Bootout's settle loop
// (which polls print after every bootout) returns immediately instead of spinning for the full
// BootoutSettleTimeout.
func stubLaunchd(t *testing.T) *[][]string {
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
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	stubUninstall(t)
	stubProcmgrCatalog(t, &calls)
	return &calls
}

// stubUninstall points the uninstall at nothing on the host: an empty layout and a command runner
// that runs nothing. Every pre-remove test goes through stubLaunchd, so none can reach the real
// install.
func stubUninstall(t *testing.T) *[][]string {
	t.Helper()

	var commands [][]string
	originalLayout, originalCommand := defaultUninstallLayout, hostCommand
	defaultUninstallLayout = uninstallLayout{}
	hostCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string{name}, args...))
		return nil, nil
	}
	t.Cleanup(func() { defaultUninstallLayout, hostCommand = originalLayout, originalCommand })
	return &commands
}

type catalogStub struct {
	writeErr  error
	removeErr error
}

const (
	catalogWriteCall  = "write-processes.d"
	catalogRemoveCall = "remove-processes.d"
)

// stubProcmgrCatalog records catalog writes and removals in calls and returns the errors it is given.
func stubProcmgrCatalog(t *testing.T, calls *[][]string) *catalogStub {
	t.Helper()

	stub := &catalogStub{}
	originalWrite, originalRemove := writeProcmgrCatalog, removeProcmgrCatalog
	writeProcmgrCatalog = func(installRoot string) error {
		*calls = append(*calls, []string{catalogWriteCall, installRoot})
		return stub.writeErr
	}
	removeProcmgrCatalog = func(installRoot string) error {
		*calls = append(*calls, []string{catalogRemoveCall, installRoot})
		return stub.removeErr
	}
	t.Cleanup(func() { writeProcmgrCatalog, removeProcmgrCatalog = originalWrite, originalRemove })
	return stub
}

func launchctlCalls(calls [][]string) [][]string {
	return slices.DeleteFunc(slices.Clone(calls), func(c []string) bool {
		return len(c) > 0 && (c[0] == catalogWriteCall || c[0] == catalogRemoveCall)
	})
}

func callIndex(calls [][]string, call ...string) int {
	return slices.IndexFunc(calls, func(c []string) bool { return slices.Equal(c, call) })
}

// stubJobDir points the hooks at a temporary directory instead of /Library/LaunchDaemons.
func stubJobDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	original := launchdJobDir
	launchdJobDir = dir
	t.Cleanup(func() { launchdJobDir = original })
	return dir
}

// stubInstallerState points the post-install at a temporary packages root, skips creating the
// installer's root-owned directories, which only root can do, and reports the Agent's account as
// present.
func stubInstallerState(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "datadog-packages")
	originalRoot, originalEnsure, originalUserExists := packagesRoot, ensureInstallerDataDir, agentUserExists
	packagesRoot = root
	ensureInstallerDataDir = func() error { return nil }
	agentUserExists = func() bool { return true }
	t.Cleanup(func() {
		packagesRoot, ensureInstallerDataDir, agentUserExists = originalRoot, originalEnsure, originalUserExists
	})
	stubDmgLayout(t)
	return root
}

// stubDmgLayout points the .dmg install steps at a temporary install, with the example
// configuration the payload ships, and makes them chown to the current user, run no commands and
// never sleep.
func stubDmgLayout(t *testing.T) dmgLayout {
	t.Helper()

	root := t.TempDir()
	layout := dmgLayout{
		installDir:      filepath.Join(root, "opt", "datadog-agent"),
		applicationsDir: filepath.Join(root, "Applications"),
		logLink:         filepath.Join(root, "var", "log", "datadog"),
		stagingDir:      filepath.Join(root, "staging"),
		linkDir:         filepath.Join(root, "usr", "local", "bin"),
		launchAgentsDir: filepath.Join(root, "LaunchAgents"),
		usersDir:        filepath.Join(root, "Users"),
		consoleDevice:   filepath.Join(root, "console"),
		packagesDir:     filepath.Join(root, "datadog-packages"),
		packagesRunDir:  filepath.Join(root, "datadog-packages", "run"),
	}
	for _, dir := range []string{layout.confDir(), layout.applicationsDir, layout.launchAgentsDir, layout.usersDir} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(layout.confDir(), "datadog.yaml.example"), []byte("api_key:\n# site: datadoghq.com\n"), 0o644))
	require.NoError(t, os.WriteFile(layout.consoleDevice, nil, 0o600))

	originalLayout, originalOwner, originalSleep := defaultDmgLayout, lookupOwner, sleep
	defaultDmgLayout = layout
	lookupOwner = func(string, string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	sleep = func(time.Duration) {}
	t.Cleanup(func() { defaultDmgLayout, lookupOwner, sleep = originalLayout, originalOwner, originalSleep })
	stubUninstall(t)
	return layout
}

// withVersions stamps the two version symbols a real build sets through ldflags, so a test can
// tell them apart: nothing stamps them under `go test`, where AgentPackageVersion is empty and
// AgentVersion carries pkg/version's default. Without this, an assertion against either one passes
// whichever of the two the code under test actually reads.
func withVersions(t *testing.T, packageVersion string, agentVersion string) {
	t.Helper()

	originalPackageVersion, originalAgentVersion := version.AgentPackageVersion, version.AgentVersion
	version.AgentPackageVersion, version.AgentVersion = packageVersion, agentVersion
	t.Cleanup(func() {
		version.AgentPackageVersion, version.AgentVersion = originalPackageVersion, originalAgentVersion
	})
}

func TestLoadStableJobEnablesDisabledJobBeforeBootstrap(t *testing.T) {
	stubJobDir(t)

	client := launchd.NewClient(launchd.System)
	enabled := false
	loaded := false
	client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "print":
			return []byte(notLoadedOutput), exitError(113)
		case "enable":
			enabled = true
		case "bootstrap":
			if !enabled {
				return nil, errors.New("service is disabled")
			}
			loaded = true
		case "kickstart":
			return nil, errors.New("kickstart must not be called: bootstrap starts the job")
		}
		return nil, nil
	}

	require.NoError(t, loadStableJob(context.Background(), client, "com.datadoghq.agent"))
	assert.True(t, loaded)
}

func testHookContext(t *testing.T) HookContext {
	t.Helper()
	return HookContext{Context: context.Background(), Package: agentPackage, PackageType: PackageTypeOCI}
}

// TestStableJobsAreTheAgentsOwn pins the job set the install loads: the Agent's own daemons, the
// process manager first and the installer daemon last, with the core Agent as the main job.
func TestStableJobsAreTheAgentsOwn(t *testing.T) {
	assert.Equal(t, []string{
		"com.datadoghq.procmgr",
		"com.datadoghq.agent",
		"com.datadoghq.sysprobe",
		"com.datadoghq.data-plane",
		"com.datadoghq.installer",
	}, agentLaunchdJobs.LaunchdJobsStable)
	assert.Equal(t, "com.datadoghq.agent", agentLaunchdJobs.LaunchdMainJobStable)
	assert.Contains(t, agentLaunchdJobs.LaunchdJobsStable, agentLaunchdJobs.LaunchdMainJobStable)
}

func TestInstallStableJobsLoadsTheStableSet(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)

	require.NoError(t, installStableJobs(testHookContext(t)))

	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		path := filepath.Join(dir, label+".plist")
		content, err := os.ReadFile(path)
		require.NoError(t, err, "job definition %s was not written", label)
		assert.Contains(t, string(content), "<string>"+label+"</string>")
	}

	// Every job is enabled as well as bootstrapped: a disabled override survives bootout and a
	// rewritten definition, so a job disabled by a previous uninstall would load and never run.
	var enabled []string
	for _, args := range *calls {
		if len(args) == 2 && args[0] == "enable" {
			enabled = append(enabled, args[1])
		}
	}
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		assert.Contains(t, enabled, "system/"+label)
		assert.Contains(t, *calls, []string{"bootstrap", "system", filepath.Join(dir, label+".plist")})
	}
	// Bootstrapping starts the jobs, which all set RunAtLoad; kickstart could block on one launchd
	// cannot spawn.
	for _, args := range *calls {
		assert.NotEqual(t, "kickstart", args[0], "a job was kickstarted")
	}

	// Only the stable set is touched.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, len(agentLaunchdJobs.LaunchdJobsStable))
}

func TestInstallStableJobsLoadsTheRestWhenOneFails(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	failing := agentLaunchdJobs.LaunchdJobsStable[0]
	stub := launchdClient
	launchdClient = func() *launchd.Client {
		client := stub()
		record := client.Runner
		client.Runner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if args[0] == "bootstrap" && filepath.Base(args[len(args)-1]) == failing+".plist" {
				return nil, errors.New("input/output error")
			}
			return record(ctx, name, args...)
		}
		return client
	}

	dir := launchdJobDir
	err := installStableJobs(testHookContext(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), failing)
	for _, label := range agentLaunchdJobs.LaunchdJobsStable[1:] {
		assert.Contains(t, *calls, []string{"bootstrap", "system", filepath.Join(dir, label+".plist")}, "%s was not loaded", label)
	}
}

// TestPostInstallDMGLoadsTheStableSet covers what `installer postinst datadog-agent dmg` does for
// the .dmg's postinstall script: it runs through RunHook, as the command does, and loads the stable
// jobs.
func TestPostInstallDMGLoadsTheStableSet(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
	stubInstallerState(t)
	ctx := testHookContext(t)
	ctx.Hook = "postInstall"
	ctx.PackageType = PackageTypeDMG

	require.NoError(t, RunHook(ctx))

	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		assert.FileExists(t, filepath.Join(dir, label+".plist"))
	}
}

func TestPostInstallDatadogAgentRejectsOtherPackageTypes(t *testing.T) {
	for _, packageType := range []PackageType{PackageTypeOCI, PackageTypeDEB, PackageTypeRPM, PackageTypeMSI} {
		calls := stubLaunchd(t)
		ctx := testHookContext(t)
		ctx.PackageType = packageType

		assert.Error(t, postInstallDatadogAgent(ctx), "package type %s", packageType)
		assert.Empty(t, *calls, "package type %s reached launchd", packageType)
	}
}

// TestPostInstallDMGIsIdempotent pins that the post-install can run twice: the second run leaves
// the same job definitions.
func TestPostInstallDMGIsIdempotent(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
	stubInstallerState(t)
	ctx := testHookContext(t)
	ctx.PackageType = PackageTypeDMG

	require.NoError(t, postInstallDMG(ctx))
	first := map[string][]byte{}
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		content, err := os.ReadFile(filepath.Join(dir, label+".plist"))
		require.NoError(t, err)
		first[label] = content
	}
	require.NoError(t, postInstallDMG(ctx))
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		content, err := os.ReadFile(filepath.Join(dir, label+".plist"))
		require.NoError(t, err)
		assert.Equal(t, first[label], content)
	}
}

// TestDatadogAgentPackageHasPreRemove guards the .dmg preinstall handoff: `is-prerm-supported`
// answers yes for every installer, and RunHook silently succeeds for a hook the package doesn't
// define, so a release without this hook would let the next .dmg skip the teardown unnoticed.
func TestDatadogAgentPackageHasPreRemove(t *testing.T) {
	assert.NotNil(t, getHook(agentPackage, "preRemove"))
}

// TestPreRemoveUnloadsTheStableSet covers what `installer prerm --upgrade datadog-agent dmg` does
// for the .dmg's preinstall script: it runs through RunHook, as the command does, unloads every
// stable job in the reverse of the order they are loaded in, and removes their definitions.
func TestPreRemoveUnloadsTheStableSet(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		require.NoError(t, os.WriteFile(filepath.Join(dir, label+".plist"), []byte(label), 0o644))
	}
	ctx := testHookContext(t)
	ctx.Hook = "preRemove"
	ctx.PackageType = PackageTypeDMG
	ctx.Upgrade = true

	require.NoError(t, RunHook(ctx))

	var bootedOut []string
	for _, args := range *calls {
		if len(args) == 2 && args[0] == "bootout" {
			bootedOut = append(bootedOut, args[1])
		}
	}
	var expected []string
	for _, label := range slices.Backward(agentLaunchdJobs.LaunchdJobsStable) {
		expected = append(expected, "system/"+label)
	}
	assert.Equal(t, expected, bootedOut)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// stubFailingBootout makes the bootout of label fail, with the job still loaded.
func stubFailingBootout(t *testing.T, label string) {
	t.Helper()

	stub := launchdClient
	launchdClient = func() *launchd.Client {
		client := stub()
		record := client.Runner
		client.Runner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if len(args) == 2 && args[1] == "system/"+label {
				switch args[0] {
				case "bootout":
					return nil, errors.New("operation now in progress")
				case "print":
					return []byte("state = running"), nil
				}
			}
			return record(ctx, name, args...)
		}
		return client
	}
	t.Cleanup(func() { launchdClient = stub })
}

// TestPreRemoveKeepsTheDefinitionWhenBootoutFails pins that a job that can't be unloaded keeps its
// definition, as in loadStableJob, and doesn't stop the rest of the set from being torn down. On an
// upgrade nothing is reported: the next version's postinstall reloads the jobs.
func TestPreRemoveKeepsTheDefinitionWhenBootoutFails(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		require.NoError(t, os.WriteFile(filepath.Join(dir, label+".plist"), []byte(label), 0o644))
	}
	failing := agentLaunchdJobs.LaunchdJobsStable[1]
	stubFailingBootout(t, failing)
	ctx := testHookContext(t)
	ctx.Upgrade = true

	require.NoError(t, preRemoveDatadogAgent(ctx))

	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		path := filepath.Join(dir, label+".plist")
		if label == failing {
			assert.FileExists(t, path)
		} else {
			assert.NoFileExists(t, path)
		}
	}
}

// TestPreRemoveReportsAFailedBootoutOnUninstall pins that an uninstall still removes the install
// when a job can't be unloaded, and reports that job, so cmd/agent/macos/uninstall_mac_os.sh runs
// its own removal for what is left rather than reporting a complete uninstall.
func TestPreRemoveReportsAFailedBootoutOnUninstall(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		require.NoError(t, os.WriteFile(filepath.Join(dir, label+".plist"), []byte(label), 0o644))
	}
	failing := agentLaunchdJobs.LaunchdJobsStable[1]
	stubFailingBootout(t, failing)
	installRoot := filepath.Join(t.TempDir(), "datadog-agent")
	require.NoError(t, os.MkdirAll(installRoot, 0o755))
	defaultUninstallLayout = uninstallLayout{paths: []string{installRoot}}

	err := preRemoveDatadogAgent(testHookContext(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), failing)
	assert.NoDirExists(t, installRoot, "the rest of the install must still be removed")
	assert.FileExists(t, filepath.Join(dir, failing+".plist"), "the job that could not be unloaded keeps its definition")
}

// TestPreRemoveIsIdempotent pins that the pre-remove succeeds on a host with nothing installed, and
// behaves the same whatever the package type and whether it is an upgrade.
func TestPreRemoveIsIdempotent(t *testing.T) {
	for _, packageType := range []PackageType{PackageTypeDMG, PackageTypeOCI} {
		for _, upgrade := range []bool{true, false} {
			calls := stubLaunchd(t)
			dir := stubJobDir(t)
			ctx := testHookContext(t)
			ctx.PackageType = packageType
			ctx.Upgrade = upgrade

			require.NoError(t, preRemoveDatadogAgent(ctx), "package type %s, upgrade %t", packageType, upgrade)

			for _, label := range agentLaunchdJobs.LaunchdJobsStable {
				assert.Contains(t, *calls, []string{"bootout", "system/" + label}, "package type %s, upgrade %t", packageType, upgrade)
			}
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Empty(t, entries)
		}
	}
}

// TestLoadStableJobKeepsTheOldDefinitionWhenBootoutFails pins that when a loaded job can't be
// unloaded, its file stays as it was and the job fails: launchd still runs it from its old, cached
// definition, so the file must not claim otherwise, and the install must report the failure.
func TestLoadStableJobKeepsTheOldDefinitionWhenBootoutFails(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	label := agentLaunchdJobs.LaunchdJobsStable[0]
	path := filepath.Join(dir, label+".plist")
	require.NoError(t, os.WriteFile(path, []byte("old definition"), 0o644))
	stub := launchdClient
	launchdClient = func() *launchd.Client {
		client := stub()
		record := client.Runner
		client.Runner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if args[0] == "bootout" {
				return nil, errors.New("operation now in progress")
			}
			if args[0] == "print" {
				return []byte("state = running"), nil
			}
			return record(ctx, name, args...)
		}
		return client
	}

	err := loadStableJob(context.Background(), launchdClient(), label)

	require.Error(t, err)
	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "old definition", string(content))
	for _, call := range *calls {
		assert.NotContains(t, []string{"enable", "bootstrap", "kickstart"}, call[0], "%s ran after a failed bootout", call[0])
	}
}

// TestPostInstallDMGCreatesTheInstallerStateFirst pins that the installer's root-owned directories
// are created before any job is loaded, so the installer daemon never starts without them.
func TestPostInstallDMGCreatesTheInstallerStateFirst(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubInstallerState(t)
	var callsWhenEnsured int
	ensured := false
	ensureInstallerDataDir = func() error {
		ensured = true
		callsWhenEnsured = len(launchctlCalls(*calls))
		return nil
	}
	ctx := testHookContext(t)
	ctx.PackageType = PackageTypeDMG

	require.NoError(t, postInstallDMG(ctx))

	assert.True(t, ensured)
	assert.Zero(t, callsWhenEnsured, "a job was loaded before the installer's directories existed")
}

// TestPostInstallDMGLoadsTheJobsWhenSetupFails pins that neither the installer's directories nor
// the package registration can leave the Agent stopped: their failures are reported, and every job
// is loaded all the same.
func TestPostInstallDMGLoadsTheJobsWhenSetupFails(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
	root := stubInstallerState(t)
	ensureInstallerDataDir = func() error { return errors.New("refusing to use /opt/datadog-packages/run") }
	// A file where the packages root should be makes the registration fail.
	require.NoError(t, os.WriteFile(root, nil, 0o644))
	ctx := testHookContext(t)
	ctx.PackageType = PackageTypeDMG

	err := postInstallDMG(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to use")
	assert.Contains(t, err.Error(), "failed to create")
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		assert.FileExists(t, filepath.Join(dir, label+".plist"))
	}
}

// TestPostInstallDMGLeavesTheJobsUnloadedWithoutTheAgentsAccount pins that the post-install does not
// load jobs launchd cannot start: without the Agent's account it reports the failure, which fails
// the install, and still registers the package.
func TestPostInstallDMGLeavesTheJobsUnloadedWithoutTheAgentsAccount(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	root := stubInstallerState(t)
	agentUserExists = func() bool { return false }
	ctx := testHookContext(t)
	ctx.PackageType = PackageTypeDMG

	err := postInstallDMG(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "_dd-agent does not exist")
	assert.Empty(t, launchctlCalls(*calls), "launchd was called without the Agent's account")
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		assert.NoFileExists(t, filepath.Join(dir, label+".plist"))
	}
	assert.DirExists(t, root)
}

// entryNames lists the names in dir, sorted.
func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// TestLinkPackageEntryLinksTheInstallRoot pins the layout the deb and rpm packages ship on Linux:
// a version link to the install root outside the repository, stable to it and experiment to
// stable. The state the installer reports must name the Agent actually installed, under the
// version the daemon reports as running: Fleet Automation compares the two, hence the two forms of
// the same version here, differing only where AgentVersion uses '+'.
func TestLinkPackageEntryLinksTheInstallRoot(t *testing.T) {
	withVersions(t, "7.99.0-devel.git.1.deadbee", "7.99.0-devel+git.1.deadbee")
	root := stubInstallerState(t)
	installDir := defaultDmgLayout.installDir

	require.NoError(t, linkPackageEntry(testHookContext(t), installDir))

	versionLink := filepath.Join(root, "run", agentPackage, version.AgentPackageVersion)
	for link, target := range map[string]string{
		versionLink: installDir,
		filepath.Join(root, agentPackage, "stable"):     versionLink,
		filepath.Join(root, agentPackage, "experiment"): filepath.Join(root, agentPackage, "stable"),
	} {
		got, err := os.Readlink(link)
		require.NoError(t, err, link)
		assert.Equal(t, target, got, link)
	}
	assert.DirExists(t, filepath.Join(root, agentPackage, "stable", "etc"), "stable must resolve to the install root")

	state, err := repository.NewRepositories(root, AsyncPreRemoveHooks).GetState(agentPackage)
	require.NoError(t, err)
	assert.Equal(t, version.AgentPackageVersion, state.Stable)
	assert.NotEqual(t, version.AgentVersion, state.Stable,
		"the entry must name the version the daemon reports, not the human-facing one")
	assert.Empty(t, state.Experiment, "a fresh install must not look like it has a version experiment")
}

// TestLinkPackageEntryFallsBackToAgentVersion covers a build that did not go through omnibus, which
// is the only thing that stamps AgentPackageVersion: the entry still needs a version to be named
// after.
func TestLinkPackageEntryFallsBackToAgentVersion(t *testing.T) {
	withVersions(t, "", "7.99.0-devel+git.1.deadbee")
	root := stubInstallerState(t)

	require.NoError(t, linkPackageEntry(testHookContext(t), defaultDmgLayout.installDir))

	state, err := repository.NewRepositories(root, AsyncPreRemoveHooks).GetState(agentPackage)
	require.NoError(t, err)
	assert.Equal(t, version.AgentVersion, state.Stable)
}

// TestLinkPackageEntryIsIdempotent is the property every hook here needs: the .dmg runs postInstall
// on a first install, on every upgrade, and again on a host already in this state.
func TestLinkPackageEntryIsIdempotent(t *testing.T) {
	withVersions(t, "7.99.0-devel.git.1.deadbee", "7.99.0-devel+git.1.deadbee")
	root := stubInstallerState(t)
	ctx := testHookContext(t)

	require.NoError(t, linkPackageEntry(ctx, defaultDmgLayout.installDir))
	require.NoError(t, linkPackageEntry(ctx, defaultDmgLayout.installDir))

	assert.Equal(t, []string{agentPackage, "run"}, entryNames(t, root))
	assert.Equal(t, []string{"experiment", "stable"}, entryNames(t, filepath.Join(root, agentPackage)))
	assert.Equal(t, []string{version.AgentPackageVersion}, entryNames(t, filepath.Join(root, "run", agentPackage)))
}

// TestLinkPackageEntryMovesForwardOnUpgrade covers the .dmg upgrade: the Agent is replaced in the
// install root, and the entry has to name the new version, without the previous version's link.
func TestLinkPackageEntryMovesForwardOnUpgrade(t *testing.T) {
	root := stubInstallerState(t)
	ctx := testHookContext(t)

	withVersions(t, "7.98.0-devel.git.1.0ldc0de", "7.98.0-devel+git.1.0ldc0de")
	require.NoError(t, linkPackageEntry(ctx, defaultDmgLayout.installDir))
	withVersions(t, "7.99.0-devel.git.1.deadbee", "7.99.0-devel+git.1.deadbee")
	require.NoError(t, linkPackageEntry(ctx, defaultDmgLayout.installDir))

	state, err := repository.NewRepositories(root, AsyncPreRemoveHooks).GetState(agentPackage)
	require.NoError(t, err)
	assert.Equal(t, "7.99.0-devel.git.1.deadbee", state.Stable)
	assert.Equal(t, []string{"7.99.0-devel.git.1.deadbee"}, entryNames(t, filepath.Join(root, "run", agentPackage)))
}

// TestLinkPackageEntryReplacesAnEarlierPlaceholder covers an upgrade from a build that registered
// the Agent as an empty placeholder directory: stable moves to the version link, and the placeholder
// goes. An unexpected directory with content is left in place.
func TestLinkPackageEntryReplacesAnEarlierPlaceholder(t *testing.T) {
	withVersions(t, "7.99.0-devel.git.1.deadbee", "7.99.0-devel+git.1.deadbee")
	root := stubInstallerState(t)
	repoDir := filepath.Join(root, agentPackage)
	placeholder := filepath.Join(repoDir, "7.98.0-devel.git.1.0ldc0de")
	require.NoError(t, os.MkdirAll(placeholder, 0755))
	require.NoError(t, os.Symlink(placeholder, filepath.Join(repoDir, "stable")))
	require.NoError(t, os.Symlink(filepath.Join(repoDir, "stable"), filepath.Join(repoDir, "experiment")))
	unexpected := filepath.Join(repoDir, "unexpected")
	require.NoError(t, os.MkdirAll(unexpected, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unexpected, "f"), nil, 0644))

	require.NoError(t, linkPackageEntry(testHookContext(t), defaultDmgLayout.installDir))

	assert.NoDirExists(t, placeholder)
	assert.DirExists(t, unexpected)
	state, err := repository.NewRepositories(root, AsyncPreRemoveHooks).GetState(agentPackage)
	require.NoError(t, err)
	assert.Equal(t, version.AgentPackageVersion, state.Stable)
}

// TestLinkPackageEntryLeavesARealPackageAlone pins the guard that keeps this safe to call from
// postinst unconditionally: a real package registered at stable, as an OCI install registers one,
// is not replaced.
func TestLinkPackageEntryLeavesARealPackageAlone(t *testing.T) {
	withVersions(t, "7.99.0-devel.git.1.deadbee", "7.99.0-devel+git.1.deadbee")
	root := stubInstallerState(t)
	ctx := testHookContext(t)

	repositories := repository.NewRepositories(root, AsyncPreRemoveHooks)
	require.NoError(t, os.MkdirAll(root, 0755))
	source, err := repositories.MkdirTemp()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "payload"), []byte("real package"), 0644))
	require.NoError(t, repositories.Create(ctx, agentPackage, "1.2.3-real", source))

	require.NoError(t, linkPackageEntry(ctx, defaultDmgLayout.installDir))

	state, err := repositories.GetState(agentPackage)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3-real", state.Stable, "an already registered package must not be replaced")
	payload, err := os.ReadFile(filepath.Join(root, agentPackage, "stable", "payload"))
	require.NoError(t, err)
	assert.Equal(t, "real package", string(payload))
}

// TestGarbageCollectionKeepsTheInstallRoot pins that the repository's garbage collection, which the
// daemon runs on a timer, never reaches the install root through the entry.
func TestGarbageCollectionKeepsTheInstallRoot(t *testing.T) {
	root := stubInstallerState(t)
	ctx := testHookContext(t)
	installDir := defaultDmgLayout.installDir

	withVersions(t, "7.98.0-devel.git.1.0ldc0de", "7.98.0-devel+git.1.0ldc0de")
	require.NoError(t, linkPackageEntry(ctx, installDir))
	withVersions(t, "7.99.0-devel.git.1.deadbee", "7.99.0-devel+git.1.deadbee")
	require.NoError(t, linkPackageEntry(ctx, installDir))

	require.NoError(t, repository.NewRepositories(root, AsyncPreRemoveHooks).Cleanup(ctx))

	assert.DirExists(t, filepath.Join(installDir, "etc"))
	state, err := repository.NewRepositories(root, AsyncPreRemoveHooks).GetState(agentPackage)
	require.NoError(t, err)
	assert.Equal(t, "7.99.0-devel.git.1.deadbee", state.Stable)
}

// TestUninstallRemovesTheInstall pins what a real uninstall removes besides the LaunchDaemons: the
// GUI for every logged-in user, its processes, and every path in the layout, links included, as
// cmd/agent/macos/uninstall_mac_os.sh does for versions without the installer.
func TestUninstallRemovesTheInstall(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	commands := stubUninstall(t)
	current, err := user.Current()
	require.NoError(t, err)
	hostCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		*commands = append(*commands, append([]string{name}, args...))
		if name == "who" {
			return []byte(current.Username + " console  Nov  4 09:00\n" + current.Username + " ttys000  Nov  4 09:01\n"), nil
		}
		return nil, nil
	}

	root := t.TempDir()
	installRoot := filepath.Join(root, "opt", "datadog-agent")
	require.NoError(t, os.MkdirAll(filepath.Join(installRoot, "embedded", "bin"), 0o755))
	app := filepath.Join(root, "Applications", "Datadog Agent.app")
	require.NoError(t, os.MkdirAll(app, 0o755))
	agentPlist := filepath.Join(root, "LaunchAgents", "com.datadoghq.gui.plist")
	require.NoError(t, os.MkdirAll(filepath.Dir(agentPlist), 0o755))
	require.NoError(t, os.WriteFile(agentPlist, nil, 0o644))
	link := filepath.Join(root, "bin", "datadog-agent")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(installRoot, link))
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.WriteFile(outside, []byte("kept"), 0o644))
	defaultUninstallLayout = uninstallLayout{
		guiLabels:       []string{"com.datadoghq.gui"},
		processPatterns: []string{"Datadog Agent.app"},
		paths:           []string{agentPlist, app, link, filepath.Join(root, "missing"), installRoot},
	}

	ctx := testHookContext(t)
	ctx.PackageType = PackageTypeDMG
	require.NoError(t, preRemoveDatadogAgent(ctx))

	for _, path := range []string{agentPlist, app, link, installRoot} {
		_, err := os.Lstat(path)
		assert.True(t, os.IsNotExist(err), "%s was not removed", path)
	}
	assert.FileExists(t, outside, "a path outside the layout was removed")
	assert.Contains(t, *commands, []string{"launchctl", "bootout", "gui/" + current.Uid + "/com.datadoghq.gui"})
	assert.Contains(t, *commands, []string{"pkill", "-f", "Datadog Agent.app"})
	bootouts := 0
	for _, command := range *commands {
		if command[0] == "launchctl" {
			bootouts++
		}
	}
	assert.Equal(t, 1, bootouts, "a user logged in twice must be handled once")
}

// TestUpgradeLeavesTheInstall pins that the pre-remove an upgrade runs only takes down the
// LaunchDaemons: the next version's payload replaces the rest in place.
func TestUpgradeLeavesTheInstall(t *testing.T) {
	stubLaunchd(t)
	stubJobDir(t)
	commands := stubUninstall(t)
	installRoot := filepath.Join(t.TempDir(), "datadog-agent")
	require.NoError(t, os.MkdirAll(installRoot, 0o755))
	defaultUninstallLayout = uninstallLayout{
		guiLabels:       []string{"com.datadoghq.gui"},
		processPatterns: []string{"Datadog Agent.app"},
		paths:           []string{installRoot},
	}

	ctx := testHookContext(t)
	ctx.PackageType = PackageTypeDMG
	ctx.Upgrade = true
	require.NoError(t, preRemoveDatadogAgent(ctx))

	assert.DirExists(t, installRoot)
	assert.Empty(t, *commands)
}

// TestTheUninstallLayoutMatchesTheInstall pins the default layout against the install: the install
// root goes last, since the installer running the uninstall lives in it.
func TestTheUninstallLayoutMatchesTheInstall(t *testing.T) {
	require.NotEmpty(t, defaultUninstallLayout.paths)
	assert.Equal(t, "/opt/datadog-agent", defaultUninstallLayout.paths[len(defaultUninstallLayout.paths)-1])
	assert.Contains(t, defaultUninstallLayout.paths, "/opt/datadog-packages")
	assert.Contains(t, defaultUninstallLayout.paths, "/Applications/Datadog Agent.app")
	assert.Contains(t, defaultUninstallLayout.paths, "/usr/local/bin/datadog-installer")
}

// dmgHookContext is a .dmg hook context, with the installer state and install layout stubbed so
// the post-install runs against temporary directories.
func dmgHookContext(t *testing.T) HookContext {
	t.Helper()
	stubInstallerState(t)
	ctx := testHookContext(t)
	ctx.PackageType = PackageTypeDMG
	return ctx
}

func assertLoaded(t *testing.T, calls [][]string, dir string) {
	t.Helper()
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		assert.FileExists(t, filepath.Join(dir, label+".plist"))
		assert.Contains(t, calls, []string{"bootstrap", "system", filepath.Join(dir, label+".plist")}, "%s was not loaded", label)
	}
}

func TestPostInstallDMGWritesTheCatalogBeforeLoadingProcmgr(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)

	require.NoError(t, postInstallDMG(dmgHookContext(t)))

	write := callIndex(*calls, catalogWriteCall, defaultDmgLayout.installDir)
	require.NotEqual(t, -1, write, "the catalog was not written to %s", defaultDmgLayout.installDir)
	bootout := callIndex(*calls, "bootout", "system/"+procmgrLabel)
	require.NotEqual(t, -1, bootout)
	assert.Less(t, write, bootout)
	assertLoaded(t, *calls, dir)
}

func TestPostInstallDMGLoadsEveryJobWhenTheCatalogWriteFails(t *testing.T) {
	calls := stubLaunchd(t)
	dir := stubJobDir(t)
	stubProcmgrCatalog(t, calls).writeErr = errors.New("no space left on device")

	require.NoError(t, postInstallDMG(dmgHookContext(t)))
	assertLoaded(t, *calls, dir)
}

func TestPreRemoveRemovesTheCatalogAfterTheJobs(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)

	require.NoError(t, preRemoveDatadogAgent(dmgHookContext(t)))

	remove := callIndex(*calls, catalogRemoveCall, defaultDmgLayout.installDir)
	require.NotEqual(t, -1, remove, "the catalog was not removed from %s", defaultDmgLayout.installDir)
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		bootout := callIndex(*calls, "bootout", "system/"+label)
		require.NotEqual(t, -1, bootout)
		assert.Less(t, bootout, remove, "%s was unloaded after the catalog was removed", label)
	}
}

func TestPreRemoveSucceedsWhenTheCatalogCannotBeRemoved(t *testing.T) {
	calls := stubLaunchd(t)
	stubJobDir(t)
	stubProcmgrCatalog(t, calls).removeErr = errors.New("permission denied")

	require.NoError(t, preRemoveDatadogAgent(dmgHookContext(t)))
}
