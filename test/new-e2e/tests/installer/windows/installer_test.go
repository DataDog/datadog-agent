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
	"path"
	"testing"
)

type testInstallerSuite struct {
	baseSuite
}

// TestInstaller is the test's entry-point.
func TestInstaller(t *testing.T) {
	e2e.Run(t, &testInstallerSuite{}, e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
}

func (suite *testInstallerSuite) TestInstallingTheInstaller() {
	// Install the Datadog Installer
	suite.Require().NoError(suite.installer.Install())

	suite.Require().Host().
		HasBinary(path.Join(InstallerPath, InstallerBinaryName)).
		WithSignature(command.DatadogCodeSignatureThumbprint).
		HasAService("Datadog Installer").
		WithStatus("Running").
		WithUserSid("S-1-5-18")

	// For now simply print the version and assert it is not empty
	// We cannot make assertions about the specific version installed.
	installerVersion, err := suite.installer.Version()
	suite.Require().NoError(err)
	fmt.Printf("installer version %s\n", installerVersion)
	suite.Require().NotEmpty(installerVersion)
}

func (suite *testInstallerSuite) TestInstallingSpecificVersion() {
	const stableInstallerVersion = "7.56.0-installer-0.4.5-1"
	suite.Require().NoError(suite.installer.Install(WithInstallerUrlFromInstallersJson(pipeline.AgentS3BucketTesting, pipeline.StableChannel, stableInstallerVersion)))

	suite.Require().Host().
		HasBinary(path.Join(InstallerPath, InstallerBinaryName)).
		WithSignature(command.DatadogCodeSignatureThumbprint).
		WithVersion(stableInstallerVersion)

	// Install "latest" from the pipeline
	suite.installer.Install()
}
