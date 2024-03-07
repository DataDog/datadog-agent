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

	"testing"

	"github.com/stretchr/testify/assert"
)

// SystemFileIntegrityTester is a test helper for testing that system files are not removed.
type SystemFileIntegrityTester struct {
	host *components.RemoteHost

	firstSnapshotPath  string
	secondSnapshotPath string
}

// NewSystemFileIntegrityTester creates a new SystemFileIntegrityTester
func NewSystemFileIntegrityTester(host *components.RemoteHost) *SystemFileIntegrityTester {
	t := &SystemFileIntegrityTester{}

	t.host = host

	t.firstSnapshotPath = `C:\Windows\SystemDirListBeforeInstall.txt`
	t.secondSnapshotPath = `C:\Windows\SystemDirListAfterUninstall.txt`

	return t
}

// FirstSnapshotTaken returns true if the first snapshot exists
func (t *SystemFileIntegrityTester) FirstSnapshotTaken() (bool, error) {
	return t.host.FileExists(t.firstSnapshotPath)
}

// TakeSnapshot takes a snapshot of the system files that can be used to compare against later.
// The snapshot is overridden if it already exists.
func (t *SystemFileIntegrityTester) TakeSnapshot() error {
	return snapshotSystemfiles(t.host, t.firstSnapshotPath)
}

// RemoveSnapshots removes any snapshots if they exist
func (t *SystemFileIntegrityTester) RemoveSnapshots() error {
	exists, err := t.host.FileExists(t.firstSnapshotPath)
	if err != nil {
		return fmt.Errorf("failed to check if first snapshot exists: %w", err)
	}
	if exists {
		err := t.host.Remove(t.firstSnapshotPath)
		if err != nil {
			return fmt.Errorf("failed to remove first snapshot: %w", err)
		}
	}
	exists, err = t.host.FileExists(t.secondSnapshotPath)
	if err != nil {
		return fmt.Errorf("failed to check if second snapshot exists: %w", err)
	}
	if exists {
		err := t.host.Remove(t.secondSnapshotPath)
		if err != nil {
			return fmt.Errorf("failed to remove second snapshot: %w", err)
		}
	}
	return nil
}

// AssertDoesRemoveSystemFiles takes a new snapshot and compares it to the original snapshot taken
// by TakeSnapshot(). If any files have been removes the test will fail.
func (t *SystemFileIntegrityTester) AssertDoesRemoveSystemFiles(tt *testing.T) bool {
	tt.Helper()

	// ensure initial snapshot exists
	err := validateSystemFIleSnapshot(t.host, t.firstSnapshotPath)
	if !assert.NoError(tt, err) {
		return false
	}

	// take a new snapshot
	err = snapshotSystemfiles(t.host, t.secondSnapshotPath)
	if !assert.NoError(tt, err) {
		return false
	}
	err = validateSystemFIleSnapshot(t.host, t.secondSnapshotPath)
	if !assert.NoError(tt, err) {
		return false
	}

	// compare the two snapshots
	return assertDoesRemoveSystemFiles(tt, t.host, t.firstSnapshotPath, t.secondSnapshotPath)
}

// snapshotSystemfiles saves a list of system files to a file on the remote host.
// Some system paths are ignored because they are known to frequently change.
func snapshotSystemfiles(host *components.RemoteHost, remotePath string) error {
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
	if len(cmd) > 8192 {
		return fmt.Errorf("command length %d exceeds max command length: '%s'", len(cmd), cmd)
	}
	_, err := host.Execute(cmd)
	if err != nil {
		return fmt.Errorf("snapshot system files command failed: %s", err)
	}
	return nil
}

// validateSystemFIleSnapshot ensures the snapshot file exists and is a reasonable size
func validateSystemFIleSnapshot(host *components.RemoteHost, remotePath string) error {
	// ensure file exists
	_, err := host.Lstat(remotePath)
	if err != nil {
		return fmt.Errorf("system file snapshot %s does not exist: %w", remotePath, err)
	}
	// sanity check to ensure file contains a reasonable amount of output
	stat, err := host.Lstat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", remotePath, err)
	}
	if stat.Size() < int64(1024*1024) {
		return fmt.Errorf("system file snapshot %s is too small: %d bytes", remotePath, stat.Size())
	}
	return nil
}

// getLinesMissingFromSecondSnapshot compares two system file snapshots and returns a list of files that are missing in the second snapshot
func getLinesMissingFromSecondSnapshot(host *components.RemoteHost, beforeSnapshotPath string, afterSnapshotPath string) (string, error) {
	// Diff the two files on the remote host, selecting missing items
	// diffing remotely saves bandwidth and is faster than downloading the (relatively large) files
	cmd := fmt.Sprintf(`Compare-Object -ReferenceObject (Get-Content "%s") -DifferenceObject (Get-Content "%s") | Where-Object -Property SideIndicator -EQ '<=' | Select -ExpandProperty InputObject`, beforeSnapshotPath, afterSnapshotPath)
	output, err := host.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("compare system files command failed: %s", err)
	}
	output = strings.TrimSpace(output)
	return output, nil
}

// assertDoesRemoveSystemFiles compares two system file snapshots and fails the test if any files are missing in the second snapshot
func assertDoesRemoveSystemFiles(tt *testing.T, host *components.RemoteHost, beforeSnapshotPath string, afterSnapshotPath string) bool {
	output, err := getLinesMissingFromSecondSnapshot(host, beforeSnapshotPath, afterSnapshotPath)
	if !assert.NoError(tt, err) {
		return false
	}
	if output != "" {
		// Log result since flake.Mark may skip the test before the assertion is run
		tt.Logf("should not remove system files: %s", output)
		// Since the result of this test can depend on Windows behavior unrelated to the agent,
		// we mark it as flaky so it doesn't block PRs.
		// See WINA-624 for investigation into better ways to perform this test.
		// If new Windows paths must be ignored, add them to the ignorePaths list in snapshotSystemfiles.
		flake.Mark(tt)
		// Skipping does not remove the failed test status, so we must run the assertion after flake.Mark.
		if !assert.Empty(tt, output, "should not remove system files") {
			return false
		}
	}
	return true
}
