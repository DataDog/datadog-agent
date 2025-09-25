// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Built with linux_bpf since the tests need to run as root for testing the privileged access.
//go:build linux && linux_bpf

package file

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	privilegedlogstest "github.com/DataDog/datadog-agent/pkg/privileged-logs/test"
)

type PrivilegedLogsTestSetupStrategy struct {
	tempDirs [2]string
}

func (s *PrivilegedLogsTestSetupStrategy) Setup(t *testing.T) TestSetupResult {
	handler := privilegedlogstest.Setup(t, func() {
		s.tempDirs = [2]string{}
		for i := 0; i < 2; i++ {
			testDir := t.TempDir()
			s.tempDirs[i] = testDir
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

	return TestSetupResult{TestDirs: s.tempDirs[:]}
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
	result := strategy.Setup(t)

	return &privilegedLogsTestSetup{
		tempDirs: [2]string{result.TestDirs[0], result.TestDirs[1]},
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
