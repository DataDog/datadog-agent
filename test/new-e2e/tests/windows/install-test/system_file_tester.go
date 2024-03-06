// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test/service-test"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tester is a test helper for testing agent installations
type SystemFileIntegrityTester struct {
	host *components.RemoteHost

	beforeInstallSystemDirListPath  string
	afterUninstallSystemDirListPath string
}

func NewSystemFileIntegrityTester(host *components.RemoteHost) *SystemFileIntegrityTester {
	t := &SystemFileIntegrityTester{}

	t.host = host

	t.beforeInstallSystemDirListPath = `C:\Windows\SystemDirListBeforeInstall.txt`
	t.afterUninstallSystemDirListPath = `C:\Windows\SystemDirListAfterUninstall.txt`

	return t
}

func (t *SystemFileIntegrityTester) TakeSnapshot() error {
	return t.snapshotSystemfiles(t.beforeInstallSystemDirListPath)
}

// TestDoesNotChangeSystemFiles
func (t *SystemFileIntegrityTester) AssertDoesNotChangeSystemFiles(tt *testing.T) bool {
	tt.Helper()

	err := t.snapshotSystemfiles(t.afterUninstallSystemDirListPath)
	require.NoError(tt, err)
	return t.assertDoesNotChangeSystemFiles(tt)
}

// assertSnapshotExists
func (t *SystemFileIntegrityTester) assertSnapshotExists(tt *testing.T, remotePath string) {
	tt.Helper()
	// ensure file exists
	_, err := t.host.Lstat(remotePath)
	require.NoErrorf(tt, err, "system file snapshot %s should exist", remotePath)
	// sanity check to ensure file contains a reasonable amount of output
	stat, err := t.host.Lstat(remotePath)
	require.Greater(tt, stat.Size(), int64(1024*1024), "system file snapshot should be at least 1MB")
}

func (t *SystemFileIntegrityTester) snapshotSystemfiles(remotePath string) error {
	// Ignore these paths when collecting the list of files, they are known to frequently change
	// Ignoring paths while creating the snapshot reduces the snapshot size by >90%
	ignorePaths := []string{
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
	}
	// quote each path and join with commas
	pattern := ""
	for _, ignorePath := range ignorePaths {
		pattern += fmt.Sprintf(`'%s',`, ignorePath)
	}
	// PowerShell list syntax
	pattern = fmt.Sprintf(`@(%s)`, strings.Trim(pattern, ","))
	// Recursively list Windows directory and ignore the paths above
	// Compare-Object is case insensitive by default
	cmd := fmt.Sprintf(`cmd /c dir C:\Windows /b /s | Out-String -Stream | Select-String -NotMatch -SimpleMatch -Pattern %s | Select -ExpandProperty Line > "%s"`, pattern, remotePath)
	require.Less(tt, len(cmd), 8192, "should not exceed max command length")
	_, err := t.host.Execute(cmd)
	require.NoError(tt, err, "should snapshot system files")

	return err
}

func (t *SystemFileIntegrityTester) assertDoesNotChangeSystemFiles(tt *testing.T) bool {
	return tt.Run("does not remove system files", func(tt *testing.T) {
		tt.Cleanup(func() {
			// Remove the snapshot files after the test
			err := t.host.Remove(t.beforeInstallSystemDirListPath)
			if err != nil {
				tt.Logf("failed to remove %s: %s", t.beforeInstallSystemDirListPath, err)
			}
			err = t.host.Remove(t.afterUninstallSystemDirListPath)
			if err != nil {
				tt.Logf("failed to remove %s: %s", t.afterUninstallSystemDirListPath, err)
			}
		})
		// Diff the two files on the remote host, selecting missing items
		cmd := fmt.Sprintf(`Compare-Object -ReferenceObject (Get-Content "%s") -DifferenceObject (Get-Content "%s") | Where-Object -Property SideIndicator -EQ '<=' | Select -ExpandProperty InputObject`, t.beforeInstallSystemDirListPath, t.afterUninstallSystemDirListPath)
		output, err := t.host.Execute(cmd)
		require.NoError(tt, err, "should compare system files")
		output = strings.TrimSpace(output)
		if output != "" {
			// Log result since flake.Mark may skip the test before the assertion is run
			tt.Logf("should not remove system files: %s", output)
			// Since the result of this test can depend on Windows behavior unrelated to the agent,
			// we mark it as flaky so it doesn't block PRs.
			// See WINA-624 for investigation into better ways to perform this test.
			// If new Windows paths must be ignored, add them to the ignorePaths list in snapshotSystemfiles.
			flake.Mark(tt)
			// Skipping does not remove the failed test status, so we must run the assertion after flake.Mark.
			require.Empty(tt, output, "should not remove system files")
		}
	})
}
