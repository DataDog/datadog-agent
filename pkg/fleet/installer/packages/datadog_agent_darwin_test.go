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
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
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
	return &calls
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

func TestLoadStableJobEnablesDisabledJobBeforeBootstrap(t *testing.T) {
	stubJobDir(t)

	client := launchd.NewClient(launchd.System)
	enabled := false
	started := false
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
		case "kickstart":
			started = true
		}
		return nil, nil
	}

	require.NoError(t, loadStableJob(context.Background(), client, "com.datadoghq.agent"))
	assert.True(t, started)
}

func testHookContext(t *testing.T) HookContext {
	t.Helper()
	return HookContext{Context: context.Background(), Package: agentPackage, PackageType: PackageTypeOCI}
}

// TestStableJobsAreTheAgentsOwn pins the job set the install loads: the Agent's own daemons, the
// process manager first, and nothing else, with the core Agent as the main job.
func TestStableJobsAreTheAgentsOwn(t *testing.T) {
	assert.Equal(t, []string{
		"com.datadoghq.procmgr",
		"com.datadoghq.agent",
		"com.datadoghq.sysprobe",
		"com.datadoghq.data-plane",
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
		assert.Contains(t, *calls, []string{"kickstart", "system/" + label})
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

	err := installStableJobs(testHookContext(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), failing)
	for _, label := range agentLaunchdJobs.LaunchdJobsStable[1:] {
		assert.Contains(t, *calls, []string{"kickstart", "system/" + label}, "%s was not started", label)
	}
}

// TestPostInstallDMGLoadsTheStableSet covers what `installer postinst datadog-agent dmg` does for
// the .dmg's postinstall script: it runs through RunHook, as the command does, and loads the stable
// jobs.
func TestPostInstallDMGLoadsTheStableSet(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
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

// TestPreRemoveKeepsTheDefinitionWhenBootoutFails pins that a job that can't be unloaded keeps its
// definition, as in loadStableJob, and doesn't stop the rest of the set from being torn down.
func TestPreRemoveKeepsTheDefinitionWhenBootoutFails(t *testing.T) {
	stubLaunchd(t)
	dir := stubJobDir(t)
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		require.NoError(t, os.WriteFile(filepath.Join(dir, label+".plist"), []byte(label), 0o644))
	}
	failing := agentLaunchdJobs.LaunchdJobsStable[1]
	stub := launchdClient
	launchdClient = func() *launchd.Client {
		client := stub()
		record := client.Runner
		client.Runner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if len(args) == 2 && args[1] == "system/"+failing {
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

	require.NoError(t, preRemoveDatadogAgent(testHookContext(t)))

	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		path := filepath.Join(dir, label+".plist")
		if label == failing {
			assert.FileExists(t, path)
		} else {
			assert.NoFileExists(t, path)
		}
	}
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

// TestLoadStableJobKeepsTheOldDefinitionWhenBootoutFails guards the postinstall fallback: when a
// loaded job can't be unloaded, its file must stay as it was and the job must fail, so the
// fallback finds a definition that differs from the shipped one and reloads the job, instead of
// skipping a job that launchd still runs from its old, cached definition.
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
