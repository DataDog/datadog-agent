// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	installerFile "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/linuxunits"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivilegedRshellPolicyDirectoryPermissions(t *testing.T) {
	assert.Equal(t, "/etc/datadog-agent-rshell", privilegedRshellPolicyDir)
	assert.Contains(t, agentDirectories, installerFile.Directory{
		Path:  privilegedRshellPolicyDir,
		Mode:  0755,
		Owner: "root",
		Group: "root",
	})
	assert.NotEqual(t, "/etc/datadog-agent", filepath.Dir(privilegedRshellPolicyDir),
		"the policy directory must not be below the dd-agent-owned config directory")
}

func TestPrivilegedRshellPackagePermissionsProtectParentDirectories(t *testing.T) {
	assert.Equal(t, installerFile.Permissions{
		{Path: "embedded", Owner: "root", Group: "root", Mode: 0755},
		{Path: "embedded/bin", Owner: "root", Group: "root", Mode: 0755},
		{Path: privilegedRshellBinaryRelPath, Owner: "root", Group: "root", Mode: 0755},
	}, privilegedRshellPackagePermissions)
	for _, permission := range privilegedRshellPackagePermissions {
		assert.NotEqual(t, ".", permission.Path, "the package root must keep dd-agent ownership")
	}
}

func TestRootExecutedBinariesAreRootOwned(t *testing.T) {
	// system-probe runs as root and executes clang-bpf for runtime compilation,
	// so it must not be left writable by dd-agent like the rest of the package.
	assert.Contains(t, agentPackagePermissions, installerFile.Permission{
		Path:  "embedded/bin/clang-bpf",
		Owner: "root",
		Group: "root",
	})
}

func TestEnsurePrivilegedRshellPermissionsRejectsSymlink(t *testing.T) {
	packagePath := t.TempDir()
	binaryPath := filepath.Join(packagePath, privilegedRshellBinaryRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
	require.NoError(t, os.Symlink("/bin/true", binaryPath))

	err := ensurePrivilegedRshellPermissions(HookContext{Context: context.Background(), PackagePath: packagePath})
	require.ErrorContains(t, err, "privileged rshell helper is not a regular file")
}

func TestPrivilegedRshellSupported(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("privileged rshell is supported only on amd64 and arm64")
	}

	packagePath := t.TempDir()
	binaryPath := filepath.Join(packagePath, privilegedRshellBinaryRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
	require.NoError(t, os.WriteFile(binaryPath, []byte("rshell"), 0755))

	originalGetLandlockABIVersion := getLandlockABIVersion
	t.Cleanup(func() { getLandlockABIVersion = originalGetLandlockABIVersion })

	getLandlockABIVersion = func() (int, error) { return privilegedRshellMinLandlockABI, nil }
	assert.True(t, privilegedRshellSupported(packagePath))

	getLandlockABIVersion = func() (int, error) { return privilegedRshellMinLandlockABI - 1, nil }
	assert.False(t, privilegedRshellSupported(packagePath))

	getLandlockABIVersion = func() (int, error) { return 0, errors.New("landlock unavailable") }
	assert.False(t, privilegedRshellSupported(packagePath))

	require.NoError(t, os.Remove(binaryPath))
	getLandlockABIVersion = func() (int, error) { return privilegedRshellMinLandlockABI, nil }
	assert.False(t, privilegedRshellSupported(packagePath))
}

func TestPrivilegedRshellEnabled(t *testing.T) {
	configDir := t.TempDir()
	originalConfigDir := privilegedRshellConfigDir
	privilegedRshellConfigDir = func(HookContext) string { return configDir }
	t.Cleanup(func() { privilegedRshellConfigDir = originalConfigDir })

	originalEnv, envWasSet := os.LookupEnv(privilegedRshellEnabledEnv)
	require.NoError(t, os.Unsetenv(privilegedRshellEnabledEnv))
	t.Cleanup(func() {
		if envWasSet {
			require.NoError(t, os.Setenv(privilegedRshellEnabledEnv, originalEnv))
		} else {
			require.NoError(t, os.Unsetenv(privilegedRshellEnabledEnv))
		}
	})

	ctx := HookContext{Context: context.Background()}
	assert.False(t, privilegedRshellEnabled(ctx), "the helper must be disabled when config is absent")

	require.NoError(t, os.WriteFile(filepath.Join(configDir, "datadog.yaml"), []byte(`
private_action_runner:
  restricted_shell:
    privileged:
      enabled: true
`), 0640))
	assert.True(t, privilegedRshellEnabled(ctx))

	managedDir := filepath.Join(configDir, "managed", agentPackage, "stable")
	require.NoError(t, os.MkdirAll(managedDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(managedDir, "datadog.yaml"), []byte(`
private_action_runner:
  restricted_shell:
    privileged:
      enabled: false
`), 0640))
	assert.False(t, privilegedRshellEnabled(ctx), "managed config must override the base config")

	require.NoError(t, os.Setenv(privilegedRshellEnabledEnv, "true"))
	assert.True(t, privilegedRshellEnabled(ctx), "the environment must override file config")
}

func TestInstallableUnitsFiltersPrivilegedRshell(t *testing.T) {
	all := []units.Unit{
		linuxunits.Systemd.Agent,
		linuxunits.Systemd.RshellService,
		linuxunits.Systemd.RshellSocket,
		linuxunits.Systemd.Action,
	}

	ctx := HookContext{Context: context.Background(), PackagePath: t.TempDir()}
	assert.Equal(t, []units.Unit{linuxunits.Systemd.Agent, linuxunits.Systemd.Action}, installableUnits(ctx, all))
	assert.Len(t, all, 4, "filtering must not mutate the lifecycle cleanup list")
}

func TestStartedUnits(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("privileged rshell is supported only on amd64 and arm64")
	}

	packagePath := t.TempDir()
	binaryPath := filepath.Join(packagePath, privilegedRshellBinaryRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0755))
	require.NoError(t, os.WriteFile(binaryPath, []byte("rshell"), 0755))

	originalGetLandlockABIVersion := getLandlockABIVersion
	t.Cleanup(func() { getLandlockABIVersion = originalGetLandlockABIVersion })

	started := func() []string {
		ctx := HookContext{Context: context.Background(), PackagePath: packagePath}
		return units.Names(startedUnits(ctx, agentService.systemd), true)
	}

	getLandlockABIVersion = func() (int, error) { return privilegedRshellMinLandlockABI, nil }
	t.Setenv(privilegedRshellEnabledEnv, "true")
	assert.Equal(t, []string{"datadog-agent-exp.service", "datadog-agent-rshell-privileged-exp.socket"}, started())

	t.Setenv(privilegedRshellEnabledEnv, "false")
	assert.Equal(t, []string{"datadog-agent-exp.service"}, started())

	t.Setenv(privilegedRshellEnabledEnv, "true")
	getLandlockABIVersion = func() (int, error) { return 0, errors.New("landlock unavailable") }
	assert.Equal(t, []string{"datadog-agent-exp.service"}, started())
}

// TestAgentUnitSetsDifferOnlyByManagerSpecificUnits pins the agent unit sets: the procmgr service
// manager swaps the main unit content and runs procmgr instead of the DDOT unit.
func TestAgentUnitSetsDifferOnlyByManagerSpecificUnits(t *testing.T) {
	common := []string{"datadog-agent-installer.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service", "datadog-agent-data-plane.service", "datadog-agent-action.service", "datadog-agent-rshell-privileged.service", "datadog-agent-rshell-privileged.socket"}
	assert.Equal(t, append(append([]string{"datadog-agent.service"}, common...), "datadog-agent-ddot.service"), units.Names(agentService.systemd.units, false))
	assert.Equal(t, append(append([]string{"datadog-agent.service"}, common...), "datadog-agent-procmgr.service"), units.Names(agentService.procmgr.units, false))
	assert.Equal(t, linuxunits.Procmgr.Agent, agentService.procmgr.units[0])
	assert.Equal(t, "datadog-agent-ddot-exp.service", agentDDOTService.systemd.main.ExpName())
	assert.Equal(t, []string{"datadog-agent-ddot"}, units.Names(agentDDOTService.upstart.units, false))
}

func TestStopProcmgrOnOptOut(t *testing.T) {
	originalInitSystem := currentInitSystem
	originalStop := stopProcmgrService
	t.Cleanup(func() {
		currentInitSystem = originalInitSystem
		stopProcmgrService = originalStop
	})

	var stopped []service.Type
	stopProcmgrService = func(_ HookContext, manager service.Type) error {
		stopped = append(stopped, manager)
		return errors.New("not running")
	}
	ctx := HookContext{Context: context.Background(), PackagePath: t.TempDir()}

	currentInitSystem = func() service.Type { return service.SystemdType }
	t.Setenv("DD_PROCESS_MANAGER_ENABLED", "false")
	stopProcmgrOnOptOut(ctx)
	assert.Empty(t, stopped)

	currentInitSystem = func() service.Type { return service.UpstartType }
	stopProcmgrOnOptOut(ctx)
	assert.Equal(t, []service.Type{service.UpstartType}, stopped)

	require.NoError(t, os.Unsetenv("DD_PROCESS_MANAGER_ENABLED"))
	stopped = nil
	stopProcmgrOnOptOut(ctx)
	assert.Empty(t, stopped)

	t.Setenv("DD_PROCESS_MANAGER_ENABLED", "true")
	stopProcmgrOnOptOut(ctx)
	assert.Empty(t, stopped)

	currentInitSystem = func() service.Type { return service.SysvinitType }
	t.Setenv("DD_PROCESS_MANAGER_ENABLED", "FALSE")
	stopProcmgrOnOptOut(ctx)
	assert.Equal(t, []service.Type{service.SysvinitType}, stopped)
}

// TestOnlyOwnerOfProcmgrStopsItOnOptOut checks that DDOT, which shares EnableStable,
// does not stop the main Agent's procmgr.
func TestOnlyOwnerOfProcmgrStopsItOnOptOut(t *testing.T) {
	stubServiceManager(t, service.UpstartType, service.UpstartType)
	originalStop := stopProcmgrService
	t.Cleanup(func() {
		stopProcmgrService = originalStop
	})

	var stopped []service.Type
	stopProcmgrService = func(_ HookContext, manager service.Type) error {
		stopped = append(stopped, manager)
		return nil
	}
	t.Setenv("DD_PROCESS_MANAGER_ENABLED", "false")
	ctx := HookContext{Context: context.Background(), PackagePath: t.TempDir(), PackageType: PackageTypeDEB}

	require.NoError(t, agentDDOTService.EnableStable(ctx))
	assert.Empty(t, stopped)

	require.NoError(t, agentService.EnableStable(ctx))
	assert.Equal(t, []service.Type{service.UpstartType}, stopped)
}

func stubServiceManager(t *testing.T, manager service.Type, initSystem service.Type) {
	t.Helper()
	originalManager := currentServiceManager
	originalInitSystem := currentInitSystem
	currentServiceManager = func(string) service.Type { return manager }
	currentInitSystem = func() service.Type { return initSystem }
	t.Cleanup(func() {
		currentServiceManager = originalManager
		currentInitSystem = originalInitSystem
	})
}

// TestProcmgrOnSysVAndUpstartKeepsPlainResults covers the procmgr branches that do not shell out.
func TestProcmgrOnSysVAndUpstartKeepsPlainResults(t *testing.T) {
	t.Setenv("DD_PROCESS_MANAGER_ENABLED", "")
	for _, initSystem := range []service.Type{service.UpstartType, service.SysvinitType} {
		t.Run(string(initSystem), func(t *testing.T) {
			stubServiceManager(t, service.ProcmgrType, initSystem)
			ctx := HookContext{Context: context.Background(), PackagePath: t.TempDir(), PackageType: PackageTypeDEB}

			assert.NoError(t, agentService.WriteStable(ctx))
			assert.NoError(t, agentService.RemoveStable(ctx))
			assert.NoError(t, agentService.StopExperiment(ctx))
			assert.NoError(t, agentService.RemoveExperiment(ctx))
			assert.EqualError(t, agentService.StartExperiment(ctx), "experiments are not supported on "+string(initSystem))
			assert.EqualError(t, agentService.WriteExperiment(ctx), "experiments are not supported on "+string(initSystem))
			if initSystem == service.UpstartType {
				assert.NoError(t, agentService.EnableStable(ctx))
				assert.NoError(t, agentService.DisableStable(ctx))
			}
		})
	}
}

func TestWriteProcessesOnlyUnderProcmgr(t *testing.T) {
	processes := units.Names(agentService.processes, false)
	require.NotEmpty(t, processes)
	stubServiceManager(t, service.UpstartType, service.UpstartType)
	packagePath := t.TempDir()
	require.NoError(t, agentService.WriteProcesses(packagePath))
	_, err := os.Stat(service.ConfigDir(packagePath))
	assert.ErrorIs(t, err, os.ErrNotExist)

	dir := service.ConfigDir(packagePath)
	require.NoError(t, os.MkdirAll(dir, 0755))
	for _, process := range processes {
		require.NoError(t, os.WriteFile(filepath.Join(dir, process), []byte("managed"), 0644))
	}
	custom := filepath.Join(dir, "custom.yaml")
	require.NoError(t, os.WriteFile(custom, []byte("admin"), 0644))
	require.NoError(t, agentService.WriteProcesses(packagePath))
	for _, process := range processes {
		_, err := os.Stat(filepath.Join(dir, process))
		assert.ErrorIs(t, err, os.ErrNotExist)
	}
	assert.FileExists(t, custom)

	stubServiceManager(t, service.ProcmgrType, service.UpstartType)
	require.NoError(t, agentService.WriteProcesses(packagePath))
	for _, process := range processes {
		path := filepath.Join(dir, process)
		assert.FileExists(t, path)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
	}
	assert.FileExists(t, custom)
}

// TestSysVAndUpstartServicesStartProcmgr checks the SysV and Upstart lists include procmgr
// ahead of the action service. StopStable reverses the lists.
func TestSysVAndUpstartServicesStartProcmgr(t *testing.T) {
	upstart := []string{"datadog-agent", "datadog-agent-trace", "datadog-agent-process", "datadog-agent-sysprobe", "datadog-agent-security", "datadog-agent-data-plane", "datadog-agent-procmgr", "datadog-agent-action"}
	sysvinit := []string{"datadog-agent", "datadog-agent-trace", "datadog-agent-process", "datadog-agent-security", "datadog-agent-data-plane", "datadog-agent-procmgr", "datadog-agent-action"}
	assert.Equal(t, upstart, units.Names(agentService.upstart.units, false))
	assert.Equal(t, sysvinit, units.Names(agentService.sysvinit.units, false))
}
