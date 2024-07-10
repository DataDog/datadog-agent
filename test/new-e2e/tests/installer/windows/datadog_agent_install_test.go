// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
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
	// Install the Datadog Installer
	suite.Require().NoError(suite.installer.Install())

	installerVersion := suite.installer.Version()
	fmt.Printf("installer version %s\n", installerVersion)
	suite.Require().NotEmpty(installerVersion)

	suite.Require().Host().HasAService("Datadog Installer").
		WithStatus("Running").
		WithUserSid("S-1-5-18")
}
