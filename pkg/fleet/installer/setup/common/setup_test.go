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
)

type setupInstallerSpy struct {
	installer.Installer
	installed      bool
	isInstalledErr error
	installCalls   int
	forceCalls     int
}

func (i *setupInstallerSpy) IsInstalled(context.Context, string) (bool, error) {
	return i.installed, i.isInstalledErr
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

func TestReinstallAPMInjectorIfInstalled(t *testing.T) {
	t.Run("ignores setup without Agent installation", func(t *testing.T) {
		spy := &setupInstallerSpy{isInstalledErr: errors.New("must not be called")}
		setup := &Setup{installer: spy, Packages: Packages{install: map[string]packageWithVersion{}}}
		setup.Packages.Install(DatadogAPMInjectPackage, "latest")

		require.NoError(t, setup.reinstallAPMInjectorIfInstalled(context.Background()))
		assert.False(t, setup.Packages.install[DatadogAPMInjectPackage].forceInstall)
	})

	t.Run("leaves a new injector installation unchanged", func(t *testing.T) {
		spy := &setupInstallerSpy{}
		setup := &Setup{installer: spy, Packages: Packages{install: map[string]packageWithVersion{}}}
		setup.Packages.Install(DatadogAgentPackage, "7.80.4-1")
		setup.Packages.Install(DatadogAPMInjectPackage, "0.38.0-1")

		require.NoError(t, setup.reinstallAPMInjectorIfInstalled(context.Background()))
		assert.False(t, setup.Packages.install[DatadogAPMInjectPackage].forceInstall)
	})

	t.Run("reinstalls the requested injector version", func(t *testing.T) {
		spy := &setupInstallerSpy{installed: true}
		setup := &Setup{installer: spy, Packages: Packages{install: map[string]packageWithVersion{}}}
		setup.Packages.Install(DatadogAgentPackage, "7.80.4-1")
		setup.Packages.Install(DatadogAPMInjectPackage, "0.39.0-1")

		require.NoError(t, setup.reinstallAPMInjectorIfInstalled(context.Background()))
		assert.Equal(t, packageWithVersion{
			name:         DatadogAPMInjectPackage,
			version:      "0.39.0-1",
			forceInstall: true,
		}, setup.Packages.install[DatadogAPMInjectPackage])
	})

	t.Run("adds the latest injector to an Agent-only install", func(t *testing.T) {
		spy := &setupInstallerSpy{installed: true}
		setup := &Setup{installer: spy, Packages: Packages{install: map[string]packageWithVersion{}}}
		setup.Packages.Install(DatadogAgentPackage, "7.80.4-1")

		require.NoError(t, setup.reinstallAPMInjectorIfInstalled(context.Background()))
		assert.Equal(t, packageWithVersion{
			name:         DatadogAPMInjectPackage,
			version:      "latest",
			forceInstall: true,
		}, setup.Packages.install[DatadogAPMInjectPackage])
	})
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
		version:      "0.38.0-1",
		forceInstall: true,
	}, "oci://example/apm-inject-package:0.38.0-1"))
	assert.Zero(t, spy.installCalls)
	assert.Equal(t, 1, spy.forceCalls)
}
