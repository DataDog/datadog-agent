// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installertests

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"testing"
)

type testInstallerRollbackSuite struct {
	baseInstallerSuite
}

// TestInstallerRollback tests the MSI rollback of the Datadog Installer on a system.
func TestInstallerRollback(t *testing.T) {
	e2e.Run(t, &testInstallerRollbackSuite{}, e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testInstallerRollbackSuite) TestInstallerRollback() {
	s.Run("Fresh install rollback", s.installRollback)
	s.Run("Fresh install", s.freshInstall)
	s.Run("uninstall rollback", s.uninstallRollback)
	s.Run("Start service with a configuration file", s.startServiceWithConfigFile)
}

// installRollback
func (s *testInstallerRollbackSuite) installRollback() {
	// Arrange

	// Act
	msiErr := s.Installer().Install(installerwindows.WithMSIArg("WIXFAILWHENDEFERRED=1"))
	s.Require().Error(msiErr)

	// Assert
	s.requireUninstalled()
}

// uninstallRollback
func (s *testInstallerRollbackSuite) uninstallRollback() {
	// Arrange

	// Act
	msiErr := s.Installer().Uninstall(installerwindows.WithMSIArg("WIXFAILWHENDEFERRED=1"))
	s.Require().Error(msiErr)

	// Assert
	s.requireInstalled()
}
