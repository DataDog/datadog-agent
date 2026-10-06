// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestSetupInstallerMSI(t *testing.T) {
	if os.Getenv("CI") == "" && os.Getenv("CI_JOB_ID") == "" {
		if err := winio.RunWithPrivileges([]string{"SeTakeOwnershipPrivilege"}, func() error { return nil }); err != nil {
			t.Skip("test requires SeTakeOwnershipPrivilege")
		}
	}
	for _, tt := range []struct {
		name     string
		fipsMode bool
	}{
		{name: "datadog-agent-7.85.0-1-x86_64.msi"},
		{name: "datadog-fips-agent-7.85.0-1-x86_64.msi", fipsMode: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			paths.SetupTestPaths(t)
			oldVersion := version.AgentPackageVersion
			version.AgentPackageVersion = "7.85.0-1"
			t.Cleanup(func() { version.AgentPackageVersion = oldVersion })

			server := fixtures.NewServer(t)
			installer := newTestPackageManager(t, server, paths.PackagesPath)
			installer.env.FIPSMode = tt.fipsMode
			t.Cleanup(func() { assert.NoError(t, installer.Close()) })
			source := filepath.Join(t.TempDir(), "download.msi")
			require.NoError(t, os.WriteFile(source, []byte("MSI payload"), 0600))

			require.NoError(t, installer.SetupInstaller(t.Context(), source))
			cached := filepath.Join(paths.PackagesPath, "datadog-agent", "stable", tt.name)
			payload, err := os.ReadFile(cached)
			require.NoError(t, err)
			assert.Equal(t, "MSI payload", string(payload))
		})
	}
}
