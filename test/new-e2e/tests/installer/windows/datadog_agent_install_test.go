// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer_windows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	"strings"
	"testing"
)

type testInstallSuite struct {
	baseSuite
}

// TestInstalls is the test's entry-point.
func TestInstalls(t *testing.T) {
	e2e.Run(t, &testInstallSuite{}, e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()), e2e.WithDevMode())
}

func (suite *testInstallSuite) TestInstall() {
	host := suite.Env().RemoteHost

	var datadogInstallerArtifact string
	var err error
	datadogInstallerArtifact, err = pipeline.GetArtifact(suite.pipelineID, pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
		return strings.Contains(artifact, "datadog-installer-1-x86_64.msi")
	})
	suite.Require().NoError(err)
	suite.Require().NoError(windowsCommon.InstallMSI(host, datadogInstallerArtifact, "", ""))

	installer := NewInstaller(host)
	installerVersion := installer.Version()
	fmt.Printf("installer version %s\n", installerVersion)
	suite.Require().NotEmpty(installerVersion)

	suite.It().HasAService("Datadog Installer").
		WithStatus("Running").
		WithUserSid("S-1-5-18")

	fmt.Printf(installer.Install(fmt.Sprintf("oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/v2/datadog-agent:pipeline-%s", suite.pipelineID)))
	suite.Require().True(host.FileExists("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"))
	suite.It().HasAService("datadogagent").
		WithStatus("Running")
}

/*
func (suite *testInstallSuite) TestInstallAgent() {
	host := suite.Env().RemoteHost

	installer := NewInstaller(host)
	installerVersion := installer.Version()
	fmt.Printf("installer version %s\n", installerVersion)

	installer.Install(fmt.Sprintf("oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/v2/datadog-agent:pipeline-%s", suite.pipelineID))
	suite.Require().True(host.FileExists("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"))
	suite.It().HasAService("datadogagent").
		WithStatus("Running")
}
*/
