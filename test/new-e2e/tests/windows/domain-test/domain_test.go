// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package domain contains e2e tests for the Windows agent installer on Domain Controllers
package domain

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/agent"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testSuite struct {
	e2e.BaseSuite[environments.Host]

	agentPackage *windowsAgent.Package
	majorVersion string
}

func TestDomainInstallations(t *testing.T) {
	opts := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithoutAgent(),
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		)),
	}

	agentPackage, err := windowsAgent.GetPackageFromEnv()
	if err != nil {
		t.Fatalf("failed to get MSI URL from env: %v", err)
	}
	t.Logf("Using Agent: %#v", agentPackage)
	majorVersion := strings.Split(agentPackage.Version, ".")[0]

	s := &testSuite{
		agentPackage: agentPackage,
		majorVersion: majorVersion,
	}

	t.Run(fmt.Sprintf("Agent v%s", majorVersion), func(t *testing.T) {
		e2e.Run(t, s, opts...)
	})
}

func installAgentPackage(host *components.RemoteHost, agentPackage *windowsAgent.Package, args string, logfile string) (string, error) {
	remoteMSIPath, err := windows.GetTemporaryFile(host)
	if err != nil {
		return "", err
	}
	err = windows.PutOrDownloadFile(host, agentPackage.URL, remoteMSIPath)
	if err != nil {
		return "", err
	}
	return remoteMSIPath, windows.InstallMSI(host, remoteMSIPath, args, logfile)
}

func waitForHostToReboot(host *components.RemoteHost, bootTime string) error {
	for {
		newBootTime, err := PsHost().GetLastBootTime().Execute(host)
		if err != nil {
			continue
		}
		if newBootTime != bootTime {
			return nil
		}
	}
}

func setupForest(host *components.RemoteHost, domainName, domainPassword string) error {
	_, err := PsHost().
		AddActiveDirectoryDomainServicesWindowsFeature().
		ImportActiveDirectoryDomainServicesModule().
		InstallADDSForest(domainName, domainPassword).
		Execute(host)
	if err != nil {
		return err
	}
	// Still send a reboot, just in case
	_, _ = PsHost().Reboot().Execute(host)
	return nil
}

// prepareHostAndExec provisions the host with the necessary pre-requisites, then calls the test function
func (suite *testSuite) prepareHostAndExec(test func(host *components.RemoteHost, outputDir string)) {
	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), suite.T())
	suite.Require().NoError(err, "should get output dir")
	suite.T().Logf("Output dir: %s", outputDir)

	vm := suite.Env().RemoteHost
	bootTime, err := PsHost().GetLastBootTime().Execute(vm)
	suite.Require().NoError(err)

	err = setupForest(vm, "datadogqalab.com", "Test1234#")
	suite.Require().NoError(err, "should create forest")

	suite.Require().NoError(waitForHostToReboot(vm, bootTime))

	_, err = PsHost().AddActiveDirectoryUser("DatadogTestUser", "TestPassword1234#").Execute(vm)
	suite.Require().NoError(err)

	test(vm, outputDir)
}

func (suite *testSuite) TestInvalidInstall() {
	suite.prepareHostAndExec(func(host *components.RemoteHost, outputDir string) {
		suite.Require().True(suite.Run("TC-INS-DC-001", func() {
			_, err := installAgentPackage(host, suite.agentPackage, "", filepath.Join(outputDir, "tc_ins_dc_001_install.log"))
			suite.Require().Error(err, "should not succeed to install Agent on a Domain Controller with default ddagentuser")
		}))

		suite.Require().True(suite.Run("TC-INS-DC-002", func() {
			_, err := installAgentPackage(host, suite.agentPackage, "DDAGENTUSER_NAME=datadogqalab.com\\test", filepath.Join(outputDir, "tc_ins_dc_002_install.log"))
			suite.Require().Error(err, "should not succeed to install Agent on a Domain Controller with a non existing domain account")
		}))

		// TODO: TC-INS-DC-003 requires a parent or sibling domain

		suite.Require().True(suite.Run("TC-INS-DC-004", func() {
			_, err := installAgentPackage(host, suite.agentPackage, "DDAGENTUSER_NAME=datadogqalab.com\\DatadogTestUser", filepath.Join(outputDir, "tc_ins_dc_004_install.log"))
			suite.Require().Error(err, "should not succeed to install Agent on a Domain Controller without the domain account password")
		}))

		suite.Require().True(suite.Run("TC-INS-DC-005", func() {
			_, err := installAgentPackage(host, suite.agentPackage, "DDAGENTUSER_NAME=datadogqalab.com\\DatadogTestUser DDAGENTUSER_PASSWORD='Incorrect'", filepath.Join(outputDir, "tc_ins_dc_005_install.log"))
			suite.Require().Error(err, "should not succeed to install Agent on a Domain Controller with an incorrect domain account password")
		}))
	})
}

type MyAssertions struct {
	a require.Assertions
}

func (ass *MyAssertions) UserSDDL() {

}
func (suite *testSuite) require() *MyAssertions {
	return &MyAssertions{
		a: *suite.Require(),
	}
}

func (suite *testSuite) TestValidInstall() {
	suite.prepareHostAndExec(func(host *components.RemoteHost, outputDir string) {
		// TC-INS-DC-006
		_, err := installAgentPackage(host, suite.agentPackage, "DDAGENTUSER_NAME=datadogqalab.com\\DatadogTestUser DDAGENTUSER_PASSWORD='TestPassword1234#'", filepath.Join(outputDir, "tc_ins_dc_006_install.log"))
		suite.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")
		suite.EventuallyWithT(func(c *assert.CollectT) {
			metricNames, err := suite.Env().FakeIntake.Client().GetMetricNames()
			assert.NoError(c, err)
			assert.Greater(c, len(metricNames), 0)
		}, 5*time.Minute, 10*time.Second)
	})
}
