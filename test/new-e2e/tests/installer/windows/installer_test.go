// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installerwindows implements E2E tests for the Datadog Installer on Windows
package installerwindows

import (
	"embed"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	"os"
	"testing"
)

//go:embed fixtures/sample_config
var fixturesFS embed.FS

type testInstallerSuite struct {
	baseSuite
}

// TestInstaller tests the installation of the Datadog Installer on a system.
func TestInstaller(t *testing.T) {
	e2e.Run(t, &testInstallerSuite{}, e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
}

// TestInstalls tests installing and uninstalling the latest version of the Datadog Installer from the pipeline.
func (s *testInstallerSuite) TestInstalls() {
	s.Run("Fresh install", func() {
		// Arrange

		// Act
		s.Require().NoError(s.installer.Install())

		// Assert
		s.Require().Host(s.Env().RemoteHost).
			HasBinary(InstallerBinaryPath).
			WithSignature(agent.GetCodeSignatureThumbprints()).
			WithVersionMatchPredicate(func(version string) {
				s.Require().NotEmpty(version)
			}).
			HasAService(InstallerServiceName).
			// the service cannot start because of the missing API key
			WithStatus("Stopped").
			WithIdentity(common.GetIdentityForSID(common.LocalSystemSID))
	})

	s.Run("Start service with a configuration file", func() {
		// Arrange
		s.Env().RemoteHost.CopyFileFromEmbedded(fixturesFS, "fixtures/sample_config", InstallerConfigPath)

		// Act
		s.Require().NoError(common.StartService(s.Env().RemoteHost, InstallerServiceName))

		// Assert
		s.Require().Host(s.Env().RemoteHost).
			HasAService(InstallerServiceName).
			WithStatus("Running")
	})

	s.Run("Uninstall", func() {
		// Arrange

		// Act
		s.Require().NoError(s.installer.Uninstall())

		// Assert
		s.Require().Host(s.Env().RemoteHost).
			NoFileExists(InstallerBinaryPath).
			HasNoService(InstallerServiceName).
			FileExists(InstallerConfigPath)
	})

	s.Run("Install with existing configuration file", func() {
		// Arrange

		// Act
		s.Require().NoError(s.installer.Install())

		// Assert
		s.Require().Host(s.Env().RemoteHost).
			HasAService(InstallerServiceName).
			WithStatus("Running")
	})

	s.Run("Repair", func() {
		// Arrange
		s.Require().NoError(common.StopService(s.Env().RemoteHost, InstallerServiceName))
		s.Require().NoError(s.Env().RemoteHost.Remove(InstallerBinaryPath))

		// Act
		s.Require().NoError(s.installer.Install())

		// Assert
		s.Require().Host(s.Env().RemoteHost).
			HasAService(InstallerServiceName).
			WithStatus("Running")
	})
}

// TestUpgrades tests upgrading the stable version of the Datadog Installer to the latest from the pipeline.
func (s *testInstallerSuite) TestUpgrades() {
	// Arrange
	s.Require().NoError(s.installer.Install(WithInstallerURLFromInstallersJSON(pipeline.AgentS3BucketTesting, pipeline.StableChannel, installer.StableVersionPackage)))
	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(InstallerBinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionEqual(installer.StableVersion)

	// Act
	// Install "latest" from the pipeline
	s.Require().NoError(s.installer.Install())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(InstallerBinaryPath).
		WithSignature(agent.GetCodeSignatureThumbprints()).
		WithVersionMatchPredicate(func(version string) {
			pipelineVersion := os.Getenv("CURRENT_AGENT_VERSION")
			if pipelineVersion == "" {
				s.Require().NotEqual(installer.StableVersion, version, "upgraded version should be different than stable version")
			} else {
				s.Require().Equal(pipelineVersion, version, "upgraded version should be equal to pipeline version")
			}
		})
}
