// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test"
	"testing"
)

type testSystemIntegrityInstallerSuite struct {
	baseSuite
	name  string
	sutFn func(suite *testSystemIntegrityInstallerSuite)
}

// TestInstallerSystemIntegrity runs a suite of tests to ensure the integrity of the system is respected.
// It's a special kind of test because we want to keep the "arrange" phase the same for all tests
// but vary the "act" phase.
// To do so we change the function that is called in TestSystemIntegrity, which explains the weird
// suite.sutFn(suite) call.
func TestInstallerSystemIntegrity(t *testing.T) {
	suites := []testSystemIntegrityInstallerSuite{
		{
			name: "test install uninstall",
			sutFn: func(suite *testSystemIntegrityInstallerSuite) {
				suite.Require().NoError(suite.installer.Install())
				suite.Require().NoError(suite.installer.Uninstall())
			},
		},
		{
			name: "test install rollback",
			sutFn: func(suite *testSystemIntegrityInstallerSuite) {
				msiErr := suite.installer.Install(WithMSIArg("WIXFAILWHENDEFERRED=1"))
				suite.Require().Error(msiErr)
			},
		},
	}

	for _, suite := range suites {
		suite := suite
		suite.Run(suite.name, func() {
			t.Parallel()
			e2e.Run(t, &suite, e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()))
		})
	}
}

// TestSystemIntegrity tests that we don't damage the system with our installer.
func (suite *testSystemIntegrityInstallerSuite) TestSystemIntegrity() {
	// Arrange
	systemFiles, err := common.NewFileSystemSnapshot(suite.Env().RemoteHost, installtest.SystemPaths())
	suite.Require().NoError(err)
	systemPathsPermissions, err := installtest.SnapshotPermissionsForPaths(suite.Env().RemoteHost, installtest.SystemPathsForPermissionsValidation())
	suite.Require().NoError(err)

	// Act
	suite.sutFn(suite)

	// Assert
	installtest.AssertDoesNotChangePathPermissions(suite.T(), suite.Env().RemoteHost, systemPathsPermissions)
	installtest.AssertDoesNotRemoveSystemFiles(suite.T(), suite.Env().RemoteHost, systemFiles)
}
