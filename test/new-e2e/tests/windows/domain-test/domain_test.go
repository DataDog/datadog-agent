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

// waitForHostToReboot
func waitForHostToReboot(host *components.RemoteHost, bootTime string, logf func(format string, args ...any)) error {
	waitCount := 0
	for {
		newBootTime, err := PsHost().GetLastBootTime().Execute(host)
		if err == nil && newBootTime == bootTime {
			logf("host has rebooted")
			return nil
		}

		logf("host is not ready, waiting: %v", err)
		time.Sleep(10 * time.Second)
		waitCount++
		// 18 * 10 = 180s or 3 minutes
		if waitCount > 18 {
			logf("wait time exhausted, bailing out")
			return err
		}
	}
}

// setupForest
func setupForest(host *components.RemoteHost, domainName, domainPassword string, logf func(format string, args ...any)) error {
	machineType, err := PsHost().GetMachineType().Execute(host)
	logf("machine type: %s", machineType)
	if err == nil && machineType == "3" {
		// Already a domain controller, skip
		logf("machine is already a DC, skipping")
		return nil
	}
	_, err = PsHost().
		AddActiveDirectoryDomainServicesWindowsFeature().
		ImportActiveDirectoryDomainServicesModule().
		InstallADDSForest(domainName, domainPassword).
		Execute(host)
	if err != nil {
		logf("error occurred while trying to create the forest")
		return err
	}
	// Still send a reboot, just in case
	logf("fun reboot")
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

	suite.T().Logf("Creating forest")

	err = setupForest(vm, "datadogqalab.com", "TestPassword1234#", suite.T().Logf)
	suite.Require().NoError(err, "should create forest")

	suite.T().Logf("Waiting for host to reboot")

	suite.Require().NoError(waitForHostToReboot(vm, bootTime, suite.T().Logf))

	suite.T().Logf("Adding test user in domain")

	_, err = PsHost().AddActiveDirectoryUser("DatadogTestUser", "TestPassword1234#").Execute(vm)
	suite.Require().NoError(err)

	suite.T().Logf("Host is ready, running tests")

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
