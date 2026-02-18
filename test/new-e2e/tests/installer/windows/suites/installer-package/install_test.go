// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installertests implements E2E tests for the Datadog installer package on Windows
package installertests

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

type testInstallerSuite struct {
	baseInstallerPackageSuite
}

// TestInstaller tests the installation of the Datadog installer on a system.
func TestInstaller(t *testing.T) {
	e2e.Run(t, &testInstallerSuite{}, e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
}

// TestInstalls tests installing and uninstalling the latest version of the Datadog installer from the pipeline.
func (s *testInstallerSuite) TestInstalls() {
	s.Run("Fresh install", func() {
		s.freshInstall()
		s.Run("Start service with a configuration file with updates disabled", s.startServiceWithConfigFileUpdatesDisabled)
		s.Run("Start service with a configuration file with updates enabled", s.startServiceWithConfigFileUpdatesEnabled)
		s.Run("Uninstall", func() {
			s.uninstall()
			s.Run("Install with existing configuration file", func() {
				s.installWithExistingConfigFile("with-config-install.log")
				s.Run("Repair", s.repair)
				s.Run("Purge", s.purge)
				s.Run("Install after purge", func() {
					s.installWithExistingConfigFile("after-purge-install.log")
				})
			})
		})
	})
}

func (s *testInstallerSuite) startServiceWithConfigFileUpdatesDisabled() {
	// Arrange
	s.Require().NoError(common.StopService(s.Env().RemoteHost, consts.ServiceName)) // Stop the service if it's running
	s.Env().RemoteHost.CopyFileFromFS(fixturesFS, "fixtures/sample_config_disabled", consts.ConfigPath)

	// Act
	// We still expect StartService to succeed, because the service should "start" then stop itself
	s.Require().NoError(common.StartService(s.Env().RemoteHost, consts.ServiceName))

	// Assert
	// Wait a bit for the service to stop itself
	s.Require().Eventually(func() bool {
		status, err := common.GetServiceStatus(s.Env().RemoteHost, consts.ServiceName)
		if err != nil {
			return false
		}
		return status == "Stopped"
	}, 60*time.Second, 5*time.Second, "Service should stop itself after starting")
	// Have to do this after the eventually b/c it uses require calls that will hard fail the test
	s.requireNotRunning()
}

func (s *testInstallerSuite) startServiceWithConfigFileUpdatesEnabled() {
	// Arrange
	s.Require().NoError(common.StopService(s.Env().RemoteHost, consts.ServiceName)) // Stop the service if it's running
	s.Env().RemoteHost.CopyFileFromFS(fixturesFS, "fixtures/sample_config_enabled", consts.ConfigPath)

	// Act
	s.Require().NoError(common.StartService(s.Env().RemoteHost, consts.ServiceName))

	// Assert
	s.requireRunning()
	status, err := s.Installer().Status()
	s.Require().NoError(err)
	// with no packages installed just prints version
	// e.g. Datadog Agent installer v7.60.0-devel+git.56.86b2ae2
	s.Require().Contains(status, "Datadog Agent installer")
}

func (s *testInstallerSuite) uninstall() {
	// Arrange

	// Act
	s.Require().NoError(s.Installer().Uninstall())

	// Assert
	s.requireUninstalled()
	s.Require().Host(s.Env().RemoteHost).
		FileExists(consts.ConfigPath)
}

func (s *testInstallerSuite) installWithExistingConfigFile(logFilename string) {
	// Arrange

	// Act
	s.InstallWithDiagnostics(
		installerwindows.WithMSILogFile(logFilename),
	)

	// Assert
	s.requireInstalled()
	s.requireRunning()
}

func (s *testInstallerSuite) repair() {
	// Arrange
	s.Require().NoError(common.StopService(s.Env().RemoteHost, consts.ServiceName))
	s.Require().NoError(s.Env().RemoteHost.Remove(consts.BinaryPath))

	// Act
	s.InstallWithDiagnostics(
		installerwindows.WithMSILogFile("repair.log"),
	)

	// Assert
	s.requireInstalled()
	s.requireRunning()
}

func (s *testInstallerSuite) purge() {
	// Arrange

	// Act
	_, err := s.Installer().Purge()

	// Assert
	s.Assert().NoError(err)
	s.requireUninstalled()
	s.Require().Host(s.Env().RemoteHost).
		NoFileExists(`C:\ProgramData\Datadog Installer\packages\packages.db`)
}
