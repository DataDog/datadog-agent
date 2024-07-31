// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installertests implements E2E tests for the Datadog installer package on Windows
package installertests

import (
	"embed"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"testing"
)

//go:embed fixtures/sample_config
var fixturesFS embed.FS

type testInstallerSuite struct {
	installerwindows.BaseInstallerSuite
}

// TestInstaller tests the installation of the Datadog installer on a system.
func TestInstaller(t *testing.T) {
	e2e.Run(t, &testInstallerSuite{}, e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
}

// TestInstalls tests installing and uninstalling the latest version of the Datadog installer from the pipeline.
func (s *testInstallerSuite) TestInstalls() {
	s.Run("Fresh install", func() {
		s.freshInstall()
		s.Run("Start service with a configuration file", s.startServiceWithConfigFile)
		s.Run("Uninstall", func() {
			s.uninstall()
			s.Run("Install with existing configuration file", func() {
				s.installWithExistingConfigFile()
				s.Run("Repair", s.repair)
			})
		})
	})
}

func (s *testInstallerSuite) freshInstall() {
	// Arrange

	// Act
	s.Require().NoError(s.Installer().Install())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(installerwindows.BinaryPath).
		WithSignature(agent.GetCodeSignatureThumbprints()).
		WithVersionMatchPredicate(func(version string) {
			s.Require().NotEmpty(version)
		}).
		HasAService(installerwindows.ServiceName).
		// the service cannot start because of the missing API key
		WithStatus("Stopped").
		WithIdentity(common.GetIdentityForSID(common.LocalSystemSID))
}

func (s *testInstallerSuite) startServiceWithConfigFile() {
	// Arrange
	s.Env().RemoteHost.CopyFileFromFS(fixturesFS, "fixtures/sample_config", installerwindows.ConfigPath)

	// Act
	s.Require().NoError(common.StartService(s.Env().RemoteHost, installerwindows.ServiceName))

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasAService(installerwindows.ServiceName).
		WithStatus("Running")
}

func (s *testInstallerSuite) uninstall() {
	// Arrange

	// Act
	s.Require().NoError(s.Installer().Uninstall())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		NoFileExists(installerwindows.BinaryPath).
		HasNoService(installerwindows.ServiceName).
		FileExists(installerwindows.ConfigPath)
}

func (s *testInstallerSuite) installWithExistingConfigFile() {
	// Arrange

	// Act
	s.Require().NoError(s.Installer().Install())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasAService(installerwindows.ServiceName).
		WithStatus("Running")
}

func (s *testInstallerSuite) repair() {
	// Arrange
	s.Require().NoError(common.StopService(s.Env().RemoteHost, installerwindows.ServiceName))
	s.Require().NoError(s.Env().RemoteHost.Remove(installerwindows.BinaryPath))

	// Act
	s.Require().NoError(s.Installer().Install())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasAService(installerwindows.ServiceName).
		WithStatus("Running")
}
