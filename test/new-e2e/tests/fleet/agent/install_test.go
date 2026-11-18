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
	params, err := resolveInstallParams(
		WithStablePackageVersion("v7.86.3-2"),
		WithPipelineID("12345"),
	)
	require.NoError(t, err)
	require.True(t, params.stablePackages, "a pipeline ID must not override a historical stable source")
	require.Equal(t, map[string]string{
		"DD_AGENT_MAJOR_VERSION":                         "7",
		"DD_AGENT_MINOR_VERSION":                         "86.3-2",
		"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT": "7.86.3-2",
	}, params.pinnedStableEnvironment())
	require.Equal(t, "https://s3.amazonaws.com/dd-agent/datadog-installer-7.86.3-2-x86_64.exe", params.stableInstallerURL())
}

func TestStablePackageVersionRejectsInvalidBeforeHostAccess(t *testing.T) {
	// A nil host ensures invalid pins fail before executing any command.
	err := (&Agent{}).Install(WithStablePackageVersion("latest"))
	require.ErrorContains(t, err, "invalid stable package version")
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeNativePackageVersion(tc.version, tc.debian))
		})
	}
}
