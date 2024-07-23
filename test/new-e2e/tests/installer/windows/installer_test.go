// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installerwindows implements E2E tests for the Datadog Installer on Windows
package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	"os"
	"testing"
)

type testInstallerSuite struct {
	baseSuite
}

// TestInstaller tests the installation of the Datadog Installer on a system.
func TestInstaller(t *testing.T) {
	e2e.Run(t, &testInstallerSuite{}, e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
}

// TestInstallingTheInstaller tests installing and uninstalling the latest version of the Datadog Installer from the pipeline.
func (suite *testInstallerSuite) TestInstallingTheInstaller() {
	suite.Run("install the Datadog Installer", func() {
		suite.Require().NoError(suite.installer.Install())

		suite.Require().Host(suite.Env().RemoteHost).
			HasBinary(InstallerBinaryPath).
			WithSignature(agent.GetCodeSignatureThumbprints()).
			WithVersionMatchPredicate(func(version string) {
				suite.Require().NotEmpty(version)
			}).
			HasAService(InstallerServiceName).
			WithStatus("Running").
			WithUserSid("S-1-5-18")
	})

	suite.Run("uninstall the Datadog Installer", func() {
		suite.Require().NoError(suite.installer.Uninstall())

		suite.Require().Host(suite.Env().RemoteHost).
			NoFileExists(InstallerBinaryPath).
			HasNoService(InstallerServiceName)
	})
}

// TestUpgradeInstaller tests upgrading the stable version of the Datadog Installer to the latest from the pipeline.
func (suite *testInstallerSuite) TestUpgradeInstaller() {
	suite.Require().NoError(suite.installer.Install(WithInstallerURLFromInstallersJSON(pipeline.AgentS3BucketTesting, pipeline.StableChannel, installer.StableVersionPackage)))

	suite.Require().Host(suite.Env().RemoteHost).
		HasBinary(InstallerBinaryPath).
		WithSignature(agent.GetCodeSignatureThumbprints()).
		WithVersionEqual(installer.StableVersion)

	// Install "latest" from the pipeline
	suite.Require().NoError(suite.installer.Install())

	suite.Require().Host(suite.Env().RemoteHost).
		HasBinary(InstallerBinaryPath).
		WithSignature(agent.GetCodeSignatureThumbprints()).
		WithVersionMatchPredicate(func(version string) {
			pipelineVersion := os.Getenv("WINDOWS_AGENT_VERSION")
			if version == "" {
				suite.Require().NotEqual(installer.StableVersion, version, "upgraded version should be different than stable version")
			} else {
				suite.Require().Equal(pipelineVersion, version, "upgraded version should be equal to pipeline version")
			}
		})
}
