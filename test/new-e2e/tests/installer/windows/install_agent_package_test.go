// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"testing"
)

type testAgentInstallSuite struct {
	baseSuite
}

// TestAgentInstalls tests the usage of the Datadog Installer to install the Datadog Agent package.
func TestAgentInstalls(t *testing.T) {
	e2e.Run(t, &testAgentInstallSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithInstaller(),
			)),
		e2e.WithStackName("datadog-windows-installer-test"))
}

// TestInstallAgentPackage tests installing and uninstalling the Datadog Agent using the Datadog Installer.
func (suite *testAgentInstallSuite) TestInstallAgentPackage() {
	suite.Run("install the Agent package", func() {
		output, err := suite.installer.InstallPackage(AgentPackage)
		suite.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
		suite.Require().Host(suite.Env().RemoteHost).HasARunningDatadogAgentService()
	})

	suite.Run("uninstall the Agent package", func() {
		output, err := suite.installer.RemovePackage(AgentPackage)
		suite.Require().NoErrorf(err, "failed to purge the Datadog Agent package: %s", output)
		suite.Require().Host(suite.Env().RemoteHost).HasNoDatadogAgentService()
	})
}
