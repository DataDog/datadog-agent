// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package domain contains e2e tests for the Windows agent installer on Domain Controllers
package domain

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/agent"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"testing"
)

type testSuite struct {
	e2e.BaseSuite[environments.Host]

	agentPackage *windowsAgent.Package
	majorVersion string
}

func TestDomainInstallations(t *testing.T) {
	opts := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		)),
		//e2e.WithDevMode(),
	}

	s := &testSuite{}

	t.Run(fmt.Sprintf("Agent v%s", "7"), func(t *testing.T) {
		e2e.Run(t, s, opts...)
	})
}

func (suite *testSuite) Test_INS_DC_001() {
	vm := suite.Env().RemoteHost

	infoCmd := PsHost().
		GetLastBootTime().
		SelectProperties("csname", "lastbootuptime").
		AddColumn("Public IP Address", PsHost().GetPublicIPAddress()).
		Compile()

	suite.T().Logf("Command: %s", infoCmd)
	res, err := vm.Execute(infoCmd)
	suite.T().Logf(res)
	suite.Require().NoError(err, "should get info")

	res, err = PsHost().
		AddActiveDirectoryDomainServicesWindowsFeature().
		ImportActiveDirectoryDomainServicesModule().
		InstallADDSForest("datadogqalab.com", "test1234#").
		Execute(vm)
	suite.T().Logf("forest output: %s", res)
	suite.Require().NoError(err, "should create forest")

	res, err = vm.Execute(infoCmd)
	suite.T().Logf(res)
	suite.Require().NoError(err, "should get info")

	res, err = PsHost().GetActiveDirectoryDomainController().Execute(vm)
	suite.T().Logf("Get-ADDomainController output: %s", res)
	suite.Require().NoError(err, "should get domain controller")
}
