// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Built with linux_bpf since the tests need to run as root for testing the privileged access.
//go:build linux && linux_bpf

package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	privilegedlogstest "github.com/DataDog/datadog-agent/pkg/privileged-logs/test"
)

type PrivilegedLogsTestSetupStrategy struct {
	searchableTempDirs   [2]string
	unsearchableTempDirs [2]string
}

func (s *PrivilegedLogsTestSetupStrategy) Setup(t *testing.T) TestSetupResult {
	handler := privilegedlogstest.Setup(t, func() {
		s.searchableTempDirs = [2]string{}
		s.unsearchableTempDirs = [2]string{}
		for i := 0; i < 2; i++ {
			testDir := t.TempDir()

			s.searchableTempDirs[i] = testDir

			// Use a subdirectory without execute permissions so that the
			// unprivileged user can't even stat(2) the file.
			subdir := filepath.Join(testDir, "subdir")
			err := os.Mkdir(subdir, 0)
			require.NoError(t, err)

			s.unsearchableTempDirs[i] = subdir

			// Restore permissions after the test so that the cleanup of the
			// temporary directories doesn't fail.
			t.Cleanup(func() {
				err := os.Chmod(subdir, 0755)
				require.NoError(t, err)
			})
		}
	})

	t.Cleanup(func() {
		// Safety check since if something is mistakenly changed in the helper
		// setup, the test may still pass without the fd-transfer being used.
		require.True(t, handler.Called, "fd-transfer was not used")
	})

	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetWithoutSource("privileged_logs.enabled", true)
	systemProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", handler.SocketPath)

	return TestSetupResult{TestDirs: s.unsearchableTempDirs[:], TestOps: TestOps{
		create: func(name string) (*os.File, error) {
			var file *os.File
			err := privilegedlogstest.WithParentPermFixup(t, name, func() error {
				var err error
				file, err = os.Create(name)
				return err
			})
			return file, err
		}, rename: func(oldPath, newPath string) error {
			return privilegedlogstest.WithParentPermFixup(t, newPath, func() error {
				return os.Rename(oldPath, newPath)
			})
		}, remove: func(name string) error {
			return privilegedlogstest.WithParentPermFixup(t, name, func() error {
				return os.Remove(name)
			})
		}}}
}

type PrivilegedLogsLauncherTestSuite struct {
	BaseLauncherTestSuite
}

func (suite *PrivilegedLogsLauncherTestSuite) SetupSuite() {
	suite.setupStrategy = &PrivilegedLogsTestSetupStrategy{}
}

func TestPrivilegedLogsLauncherTestSuite(t *testing.T) {
	suite.Run(t, new(PrivilegedLogsLauncherTestSuite))
}

func TestPrivilegedLogsLauncherTestSuiteWithConfigID(t *testing.T) {
	s := new(PrivilegedLogsLauncherTestSuite)
	s.configID = "123456789"
	suite.Run(t, s)
}

func TestPrivilegedLogsLauncherScanStartNewTailer(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanStartNewTailerTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherWithConcurrentContainerTailer(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherWithConcurrentContainerTailerTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherTailFromTheBeginning(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherTailFromTheBeginningTest(t, setup.tempDirs[:], true)
}

func TestPrivilegedLogsLauncherSetTail(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherSetTailTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherConfigIdentifier(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherConfigIdentifierTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanWithTooManyFiles(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanWithTooManyFilesTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherUpdatesSourceForExistingTailer(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherUpdatesSourceForExistingTailerTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanRecentFilesWithRemoval(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanRecentFilesWithRemovalTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanRecentFilesWithNewFiles(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanRecentFilesWithNewFilesTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherFileRotation(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherFileRotationTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherFileDetectionSingleScan(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherFileDetectionSingleScanTest(t, setup.tempDirs[:])
}

// setupPrivilegedLogsTest is a helper type for privileged logs test setup
type privilegedLogsTestSetup struct {
	tempDirs [2]string
}

// setupPrivilegedLogsTest sets up the privileged logs test environment
func setupPrivilegedLogsTest(t *testing.T) *privilegedLogsTestSetup {
	strategy := &PrivilegedLogsTestSetupStrategy{}
	strategy.Setup(t)

	return &privilegedLogsTestSetup{
		// Use the searchable temp dirs since several of the non-suite tests use
		// wildcard patterns which do not work with non-searchable directories
		// since the logs agent is unable to scan the directory for files.
		tempDirs: [2]string{strategy.searchableTempDirs[0], strategy.searchableTempDirs[1]},
	}
}

func TestPrivilegedLogsLauncherScanStartNewTailerForEmptyFile(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanStartNewTailerForEmptyFileTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanStartNewTailerWithOneLine(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanStartNewTailerWithOneLineTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanStartNewTailerWithLongLine(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanStartNewTailerWithLongLineTest(t, setup.tempDirs[:])
}
