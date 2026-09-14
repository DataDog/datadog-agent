// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	filedeletionaudit "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/file-deletion-audit"
	"github.com/stretchr/testify/assert"
)

// SystemPathAuditExclusions returns deliberately volatile C:\Windows roots
// whose deletions remain diagnostic rather than blocking MSI scenarios.
func SystemPathAuditExclusions() []string {
	return []string{
		`C:\Windows\assembly\`,
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
		`C:\Windows\Microsoft.NET\`,
	}
}

// DeletionEventClassification explains whether an observed event blocks the
// test or remains diagnostic.
type DeletionEventClassification = filedeletionaudit.DeletionEventClassification

const (
	deletionBlocksControlledInstaller = filedeletionaudit.DeletionBlocksControlledInstaller
	deletionOutsideOperation          = filedeletionaudit.DeletionOutsideOperation
)

// ClassifiedDeletionEvent retains the original evidence and its disposition.
type ClassifiedDeletionEvent = filedeletionaudit.ClassifiedDeletionEvent

// ClassifyDeletionEvent classifies an event relative to one controlled MSI
// operation. Only msiexec.exe and dllhost.exe deletion access to non-excluded
// paths below C:\Windows blocks the test.
func ClassifyDeletionEvent(event windowsCommon.FileDeletionEvent, start, end windowsCommon.SecurityLogCheckpoint, exclusions []string) (ClassifiedDeletionEvent, error) {
	return filedeletionaudit.ClassifyDeletionEvent(event, start, end, windowsCommon.WindowsDirectory, exclusions)
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
// by comparing their SDDL strings
func AssertDoesNotChangePathPermissions(t *testing.T, host *components.RemoteHost, beforeInstall map[string]string) {
	t.Helper()
	for path, sddl := range beforeInstall {
		perms, err := windowsCommon.GetSecurityInfoForPath(host, path)
		if assert.NoError(t, err) {
			assert.Equal(t, sddl, perms.SDDL, "%s permissions should not have changed", path)
		}
	}
}
