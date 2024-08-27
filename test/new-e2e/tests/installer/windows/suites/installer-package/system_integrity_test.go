// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installertests

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test"
	"testing"
)

type testSystemIntegrityInstallerSuite struct {
	installerwindows.BaseInstallerSuite
	name  string
	sutFn func(suite *testSystemIntegrityInstallerSuite)
}

// TestInstallerSystemIntegrity runs a suite of tests to ensure the integrity of the system is respected.
// It's a special kind of test because we want to keep the "arrange" phase the same for all tests
// but vary the "act" phase.
// To do so we change the function that is called in TestSystemIntegrity, which explains the weird
// suite.sutFn(suite) call.
func TestInstallerSystemIntegrity(t *testing.T) {
	suites := []*testSystemIntegrityInstallerSuite{
		{
			name: "test-install-uninstall",
			sutFn: func(suite *testSystemIntegrityInstallerSuite) {
				suite.Require().NoError(suite.Installer().Install())
				suite.Require().NoError(suite.Installer().Uninstall())
			},
		},
		{
			name: "test-install-rollback",
			sutFn: func(suite *testSystemIntegrityInstallerSuite) {
				msiErr := suite.Installer().Install(installerwindows.WithMSIArg("WIXFAILWHENDEFERRED=1"))
				suite.Require().Error(msiErr)
			},
		},
	}

	for _, suite := range suites {
		suite := suite
		t.Run(suite.name, func(t *testing.T) {
			t.Parallel()
			e2e.Run(t, suite, e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()), e2e.WithStackName(suite.name))
		})
	}
}

// TestSystemIntegrity tests that we don't damage the system with our installer.
func (s *testSystemIntegrityInstallerSuite) TestSystemIntegrity() {
	// Arrange
	systemFiles, err := common.NewFileSystemSnapshot(s.Env().RemoteHost, installtest.SystemPaths())
	s.Require().NoError(err)
	systemPathsPermissions, err := installtest.SnapshotPermissionsForPaths(s.Env().RemoteHost, installtest.SystemPathsForPermissionsValidation())
	s.Require().NoError(err)

	// Act
	s.sutFn(s)

	// Assert
	installtest.AssertDoesNotChangePathPermissions(s.T(), s.Env().RemoteHost, systemPathsPermissions)
	installtest.AssertDoesNotRemoveSystemFiles(s.T(), s.Env().RemoteHost, systemFiles)
}
