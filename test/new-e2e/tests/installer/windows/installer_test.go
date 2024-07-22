// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/command"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	"os"
	"path"
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
			HasBinary(DatadogInstallerBinaryPath).
			WithSignature(command.DatadogCodeSignatureThumbprint).
			WithVersionMatchPredicate(func(version string) {
				suite.Require().NotEmpty(version)
			}).
			HasAService(DatadogInstallerServiceName).
			WithStatus("Running").
			WithUserSid("S-1-5-18")
	})

	suite.Run("uninstall the Datadog Installer", func() {
		suite.Require().NoError(suite.installer.Uninstall())

		suite.Require().Host(suite.Env().RemoteHost).
			NoFileExists(DatadogInstallerBinaryPath).
			HasNoService(DatadogInstallerServiceName)
	})
}

// TestUpgradeInstaller tests upgrading the stable version of the Datadog Installer to the latest from the pipeline.
func (suite *testInstallerSuite) TestUpgradeInstaller() {
	stableInstallerVersionPackageFormat := fmt.Sprintf("%s-1", DatadogInstallerVersion)

	suite.Require().NoError(suite.installer.Install(WithInstallerUrlFromInstallersJson(pipeline.AgentS3BucketTesting, pipeline.StableChannel, stableInstallerVersionPackageFormat)))

	suite.Require().Host(suite.Env().RemoteHost).
		HasBinary(path.Join(InstallerPath, InstallerBinaryName)).
		WithSignature(command.DatadogCodeSignatureThumbprint).
		WithVersionEqual(DatadogInstallerVersion)

	// Install "latest" from the pipeline
	suite.Require().NoError(suite.installer.Install())

	suite.Require().Host(suite.Env().RemoteHost).
		HasBinary(path.Join(InstallerPath, InstallerBinaryName)).
		WithSignature(command.DatadogCodeSignatureThumbprint).
		WithVersionMatchPredicate(func(version string) {
			pipelineVersion := os.Getenv("WINDOWS_AGENT_VERSION")
			if version == "" {
				suite.Require().NotEqual(DatadogInstallerVersion, version, "upgraded version should be different than stable version")
			} else {
				suite.Require().Equal(pipelineVersion, version, "upgraded version should be equal to pipeline version")
			}
		})
}
