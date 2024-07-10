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

func TestAgentInstalls(t *testing.T) {
	e2e.Run(t, &testAgentInstallSuite{}, e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake(winawshost.WithInstaller())), e2e.WithDevMode())
}

func (suite *testInstallSuite) TestInstallAgentPackage() {
	suite.T().Run("install the Agent package", func(t *testing.T) {
		_, err := suite.installer.InstallPackage(AgentPackage)
		suite.Require().NoError(err)
		suite.Require().Host().HasADatadogAgent()
	})

	suite.T().Run("uninstall the Agent package", func(t *testing.T) {
		_, err := suite.installer.Purge()
		suite.Require().NoError(err)
		suite.Require().Host().HasNoDatadogAgent()
	})
}
