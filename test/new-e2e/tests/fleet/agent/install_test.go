// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStablePackageVersionPin(t *testing.T) {
	for _, version := range []string{"7.86.3-2", "v7.86.3-2"} {
		t.Run(version, func(t *testing.T) {
			params, err := resolveInstallParams(
				WithStablePackageVersion(version),
				WithPipelineID("12345"),
				WithRemoteUpdates(),
			)
			require.NoError(t, err)
			require.True(t, params.stablePackages, "a pipeline ID must not override a historical stable source")
			require.True(t, params.remoteUpdates)
			require.Equal(t, map[string]string{
				"DD_AGENT_MAJOR_VERSION":                         "7",
				"DD_AGENT_MINOR_VERSION":                         "86.3-2",
				"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT": "7.86.3-2",
			}, params.pinnedStableEnvironment())
			require.Equal(t, "https://s3.amazonaws.com/dd-agent/datadog-installer-7.86.3-2-x86_64.exe", params.stableInstallerURL())
		})
	}
}

func TestStablePackageVersionRejectsInvalidBeforeHostAccess(t *testing.T) {
	for _, version := range []string{
		"", "latest", "7.86", "7.086.0-1", "7.86.0-rc.1-1", "7.86.0-devel.git.123-1",
		"7.86.0-0", "7.86.0-1; echo unexpected", "7.86.0-1\n", "7.86.0+build",
	} {
		t.Run(version, func(t *testing.T) {
			// A nil host ensures invalid pins fail before executing any command.
			err := (&Agent{}).Install(WithStablePackageVersion(version))
			require.ErrorContains(t, err, "invalid stable package version")
		})
	}
}

func TestStablePackageVersionRejectsStagingConflict(t *testing.T) {
	for _, options := range [][]InstallOption{
		{WithStablePackageVersion("7.86.3-1"), WithStagingPackages("7.87.0~rc.1")},
		{WithStagingPackages("7.87.0~rc.1"), WithStablePackageVersion("7.86.3-1")},
	} {
		_, err := resolveInstallParams(options...)
		require.ErrorContains(t, err, "cannot be combined with staging packages")
	}
}

func TestStablePackageVersionDoesNotChangeDefaults(t *testing.T) {
	_, err := resolveInstallParams(WithStablePackageVersion("7.86.3-1"))
	require.NoError(t, err)

	params, err := resolveInstallParams(WithStablePackages())
	require.NoError(t, err)
	require.Nil(t, params.pinnedStableEnvironment())
	require.Equal(t, windowsInstallerLatestURL, params.stableInstallerURL())

	params, err = resolveInstallParams(WithPipelineID("12345"))
	require.NoError(t, err)
	require.False(t, params.stablePackages)
	require.Nil(t, params.pinnedStableEnvironment())
}

func TestNormalizeNativePackageVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version string
		debian  bool
		want    string
	}{
		{name: "debian epoch", version: "1:7.86.3-2\n", debian: true, want: "7.86.3-2"},
		{name: "debian no epoch", version: "7.86.3-2\n", debian: true, want: "7.86.3-2"},
		{name: "rpm revision", version: "7.86.3-2\n", want: "7.86.3-2"},
		{name: "only debian epoch is stripped", version: "1:7.86.3-2", want: "1:7.86.3-2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeNativePackageVersion(tc.version, tc.debian))
		})
	}
}
