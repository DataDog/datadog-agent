// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/stretchr/testify/assert"
	"testing"
)

// SystemPaths returns a list of paths that are known to frequently change and should be ignored when collecting the list of files
func SystemPaths() []string {
	// Ignoring paths while creating the snapshot reduces the snapshot size by >90%
	return []string{
		`C:\Windows\assembly\`,
		`C:\Windows\Microsoft.NET\assembly\`,
		`C:\windows\AppReadiness\`,
		`C:\Windows\Temp\`,
		`C:\Windows\Prefetch\`,
		`C:\Windows\Installer\`,
		`C:\Windows\WinSxS\`,
		`C:\Windows\Logs\`,
		`C:\Windows\servicing\`,
		`c:\Windows\System32\catroot2\`,
		`c:\windows\System32\config\`,
		`c:\windows\System32\sru\`,
		`C:\Windows\ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization\Logs\`,
		`C:\Windows\ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization\Cache\`,
		`C:\Windows\SoftwareDistribution\DataStore\Logs\`,
		`C:\Windows\System32\wbem\Performance\`,
		`c:\windows\System32\LogFiles\`,
		`c:\windows\SoftwareDistribution\`,
		`c:\windows\ServiceProfiles\NetworkService\AppData\`,
		`C:\Windows\System32\Tasks\`,
		`C:\Windows\System32\spp\`,
		`C:\Windows\SystemTemp\`,
	}
}

// AssertDoesNotRemoveSystemFiles checks that the paths in the snapshot still exist
func AssertDoesNotRemoveSystemFiles(t *testing.T, host *components.RemoteHost, beforeInstall *windowsCommon.FileSystemSnapshot) {
	t.Run("does not change system files", func(tt *testing.T) {
		afterUninstall, err := windowsCommon.NewFileSystemSnapshot(host, SystemPaths())
		assert.NoError(tt, err)
		result, err := beforeInstall.CompareSnapshots(afterUninstall)
		assert.NoError(tt, err)

		// Since the result of this test can depend on Windows behavior unrelated to the agent,
		// we mark it as flaky so it doesn't block PRs.
		// See WINA-624 for investigation into better ways to perform this test.
		// If new Windows paths must be ignored, add them to the ignorePaths list in SystemPaths.
		flake.Mark(tt)
		assert.Empty(tt, result, "should not remove system files")
	})
}

// SystemPathsForPermissionsValidation returns paths that we should ensure permissions are not
// changed on by our installer.
//
// Paths were chosen because they are in the directory tree of our installed files.
//
// This test is a result of a bug in Windows MSI.DLL (reported, fix in progress).
// See https://github.com/oleg-shilo/wixsharp/issues/1336
func SystemPathsForPermissionsValidation() []string {
	return []string{
		`C:\`,
		`C:\Program Files\`,
		`C:\ProgramData\`,
	}
}

// SnapshotPermissionsForPaths returns a map of paths to their SDDL permissions
func SnapshotPermissionsForPaths(host *components.RemoteHost, paths []string) (map[string]string, error) {
	permissions := make(map[string]string)
	for _, path := range paths {
		perms, err := windowsCommon.GetSecurityInfoForPath(host, path)
		if err != nil {
			return nil, err
		}
		permissions[path] = perms.SDDL
	}
	return permissions, nil
}

// AssertDoesNotChangePathPermissions checks that the permissions on the paths in the snapshot are not changed
func AssertDoesNotChangePathPermissions(t *testing.T, host *components.RemoteHost, beforeInstall map[string]string) {
	t.Helper()
	for path, sddl := range beforeInstall {
		perms, err := windowsCommon.GetSecurityInfoForPath(host, path)
		assert.NoError(t, err)
		assert.Equal(t, sddl, perms.SDDL, "%s permissions should not have changed", path)
	}
}
