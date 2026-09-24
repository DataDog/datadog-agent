// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

type setupInstallerSpy struct {
	installer.Installer
	installed        bool
	isInstalledErr   error
	state            repository.State
	stateErr         error
	stateCalls       int
	isInstalledCalls int
	installCalls     int
	forceCalls       int
}

func (i *setupInstallerSpy) IsInstalled(context.Context, string) (bool, error) {
	i.isInstalledCalls++
	return i.installed, i.isInstalledErr
}

func (i *setupInstallerSpy) State(context.Context, string) (repository.State, error) {
	i.stateCalls++
	return i.state, i.stateErr
}

func (i *setupInstallerSpy) Install(context.Context, string, []string) error {
	i.installCalls++
	return nil
}

func (i *setupInstallerSpy) ForceInstall(context.Context, string, []string) error {
	i.forceCalls++
	return nil
}

func TestParActionsAllowlist_ExplicitEnv(t *testing.T) {
	explicit := "com.datadoghq.http.request,com.datadoghq.http.response"
	got := parActionsAllowlist(explicit, "linux", true)
	assert.Equal(t, []string{"com.datadoghq.http.request", "com.datadoghq.http.response"}, got)
}

func TestParActionsAllowlist_ExplicitEnvOnReinstall(t *testing.T) {
	// Explicit env var always wins, even on reinstall.
	got := parActionsAllowlist("com.datadoghq.http.request", "windows", false)
	assert.Equal(t, []string{"com.datadoghq.http.request"}, got)
}

func TestParActionsAllowlist_DefaultNixFreshInstall(t *testing.T) {
	got := parActionsAllowlist("", "linux", true)
	assert.Equal(t, []string{parDefaultAllowlistNix}, got)
}

func TestParActionsAllowlist_DefaultWindowsFreshInstall(t *testing.T) {
	got := parActionsAllowlist("", "windows", true)
	assert.Equal(t, []string{parDefaultAllowlistWindows}, got)
}

func TestParActionsAllowlist_NoOverwriteOnReinstall(t *testing.T) {
	// No env var + reinstall → nil so WriteConfigs does not clobber existing allowlist.
	got := parActionsAllowlist("", "linux", false)
	assert.Nil(t, got)

	got = parActionsAllowlist("", "windows", false)
	assert.Nil(t, got)
}

func TestParActionsAllowlist_DefaultCurrentOSFreshInstall(t *testing.T) {
	// Current OS gets one of the two known defaults on fresh install.
	got := parActionsAllowlist("", runtime.GOOS, true)
	assert.Len(t, got, 1)
	assert.Contains(t,
		[]string{parDefaultAllowlistNix, parDefaultAllowlistWindows},
		got[0],
	)
}

func TestPrepareAPMInjectorReinstall(t *testing.T) {
	prepare := func(setup *Setup) (bool, error) {
		return setup.prepareAPMInjectorReinstall(context.Background(), resolvePackages(setup.Env, setup.Packages))
	}
	newSetup := func(spy *setupInstallerSpy) *Setup {
		return &Setup{
			installer: spy,
			Env: &env.Env{
				DefaultPackagesInstallOverride: make(map[string]bool),
				DefaultPackagesVersionOverride: make(map[string]string),
			},
			Packages: Packages{install: map[string]packageWithVersion{}},
		}
	}

	t.Run("leaves a new standalone injector installation unchanged", func(t *testing.T) {
		spy := &setupInstallerSpy{}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAPMInjectPackage, "0.69.0-1")

		reinstall, err := prepare(setup)
		require.NoError(t, err)
		assert.False(t, reinstall)
		assert.Equal(t, 1, spy.isInstalledCalls)
	})

	t.Run("reinstalls a stale standalone injector", func(t *testing.T) {
		previousDetector := detectAPMInjectorReinstall
		detectAPMInjectorReinstall = func() bool { return true }
		t.Cleanup(func() { detectAPMInjectorReinstall = previousDetector })

		spy := &setupInstallerSpy{installed: true}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAPMInjectPackage, "0.69.0-1")

		reinstall, err := prepare(setup)
		require.NoError(t, err)
		assert.True(t, reinstall)
		assert.Zero(t, spy.stateCalls)
	})

	t.Run("ignores Agent-only installation", func(t *testing.T) {
		spy := &setupInstallerSpy{isInstalledErr: errors.New("must not be called")}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAgentPackage, "7.80.4-1")

		reinstall, err := prepare(setup)
		require.NoError(t, err)
		assert.False(t, reinstall)
		assert.Zero(t, spy.isInstalledCalls)
	})

	t.Run("leaves a new injector installation unchanged", func(t *testing.T) {
		spy := &setupInstallerSpy{}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAgentPackage, "7.80.4-1")
		setup.Packages.Install(DatadogAPMInjectPackage, "0.69.0-1")

		reinstall, err := prepare(setup)
		require.NoError(t, err)
		assert.False(t, reinstall)
		assert.Equal(t, 1, spy.isInstalledCalls)
		assert.Zero(t, spy.stateCalls)
	})

	t.Run("does not reinstall for the same Agent version", func(t *testing.T) {
		spy := &setupInstallerSpy{installed: true, state: repository.State{Stable: "7.84.0"}}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAgentPackage, "7.84.0-1")
		setup.Packages.Install(DatadogAPMInjectPackage, "0.69.0-1")

		reinstall, err := prepare(setup)
		require.NoError(t, err)
		assert.False(t, reinstall, "an idempotent run must preserve injector experiments and avoid hook downtime")
	})

	t.Run("reinstalls when the requested Agent differs", func(t *testing.T) {
		spy := &setupInstallerSpy{installed: true, state: repository.State{Stable: "7.80.4"}}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAgentPackage, "7.84.0-devel.git.1-1")
		setup.Packages.Install(DatadogAPMInjectPackage, "0.69.0-1")

		reinstall, err := prepare(setup)
		require.NoError(t, err)
		assert.True(t, reinstall)
	})

	t.Run("uses the resolved Agent version override", func(t *testing.T) {
		spy := &setupInstallerSpy{installed: true, state: repository.State{Stable: "7.84.0"}}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAgentPackage, "7.84.0-1")
		setup.Packages.Install(DatadogAPMInjectPackage, "0.69.0-1")
		setup.Env.DefaultPackagesVersionOverride[DatadogAgentPackage] = "7.80.4-1"

		reinstall, err := prepare(setup)
		require.NoError(t, err)
		assert.True(t, reinstall)
	})

	t.Run("returns Agent state errors", func(t *testing.T) {
		stateErr := errors.New("state failed")
		spy := &setupInstallerSpy{installed: true, stateErr: stateErr}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAgentPackage, "7.80.4-1")
		setup.Packages.Install(DatadogAPMInjectPackage, "0.69.0-1")

		_, err := prepare(setup)
		require.ErrorIs(t, err, stateErr)
	})

	t.Run("returns package lookup errors", func(t *testing.T) {
		lookupErr := errors.New("lookup failed")
		spy := &setupInstallerSpy{isInstalledErr: lookupErr}
		setup := newSetup(spy)
		setup.Packages.Install(DatadogAgentPackage, "7.80.4-1")
		setup.Packages.Install(DatadogAPMInjectPackage, "0.69.0-1")

		_, err := prepare(setup)
		require.ErrorIs(t, err, lookupErr)
	})
}

func TestSamePackageVersion(t *testing.T) {
	assert.True(t, samePackageVersion("7.80.4-1", "7.80.4"))
	assert.True(t, samePackageVersion("7.80.4", "7.80.4"))
	assert.False(t, samePackageVersion("7.80.4-rc.1-1", "7.80.4-1"))
	assert.False(t, samePackageVersion("pipeline-123", "7.80.4"))
}

func TestInstallPackageForceInstall(t *testing.T) {
	spy := &setupInstallerSpy{}
	setup := &Setup{
		installer: spy,
		Out:       &Output{},
		Env:       &env.Env{},
		Ctx:       context.Background(),
	}

	require.NoError(t, setup.installPackage(packageWithVersion{
		name:         DatadogAPMInjectPackage,
		version:      "0.69.0-1",
		forceInstall: true,
	}, "oci://example/apm-inject-package:0.69.0-1"))
	assert.Zero(t, spy.installCalls)
	assert.Equal(t, 1, spy.forceCalls)
}
