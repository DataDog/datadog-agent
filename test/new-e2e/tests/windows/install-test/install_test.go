// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installtest contains e2e tests for the Windows agent installer
package installtest

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/agent"

	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"testing"
)

var devMode = flag.Bool("devmode", false, "enable devmode")

type agentMSISuite struct {
	e2e.BaseSuite[environments.Host]

	agentPackage *windowsAgent.Package
	majorVersion string
}

func TestMSI(t *testing.T) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(componentos.WindowsDefault)),
	))}
	if *devMode {
		opts = append(opts, e2e.WithDevMode())
	}

	agentPackage, err := windowsAgent.GetPackageFromEnv()
	if err != nil {
		t.Fatalf("failed to get MSI URL from env: %v", err)
	}
	t.Logf("Using Agent: %#v", agentPackage)

	majorVersion := strings.Split(agentPackage.Version, ".")[0]

	// Set stack name to avoid conflicts with other tests
	// Include channel if we're not running in a CI pipeline.
	// E2E auto includes the pipeline ID in the stack name, so we don't need to do that here.
	stackNameChannelPart := ""
	if agentPackage.PipelineID == "" && agentPackage.Channel != "" {
		stackNameChannelPart = fmt.Sprintf("-%s", agentPackage.Channel)
	}
	stackNameCIJobPart := ""
	ciJobID := os.Getenv("CI_JOB_ID")
	if ciJobID != "" {
		stackNameCIJobPart = fmt.Sprintf("-%s", os.Getenv("CI_JOB_ID"))
	}
	opts = append(opts, e2e.WithStackName(fmt.Sprintf("windows-msi-test-v%s-%s%s%s", majorVersion, agentPackage.Arch, stackNameChannelPart, stackNameCIJobPart)))

	s := &agentMSISuite{
		agentPackage: agentPackage,
		majorVersion: majorVersion,
	}

	// Include the agent major version in the test name so junit reports will differentiate the tests
	t.Run(fmt.Sprintf("Agent v%s", majorVersion), func(t *testing.T) {
		e2e.Run(t, s, opts...)
	})
}

func (is *agentMSISuite) prepareHost() {
	vm := is.Env().RemoteHost

	if !is.Run("prepare VM", func() {
		is.Run("disable defender", func() {
			err := windows.DisableDefender(vm)
			is.Require().NoError(err, "should disable defender")
		})
	}) {
		is.T().Fatal("failed to prepare VM")
	}
}

func (is *agentMSISuite) TestInstall() {
	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), is.T())
	is.Require().NoError(err, "should get output dir")
	is.T().Logf("Output dir: %s", outputDir)

	vm := is.Env().RemoteHost
	is.prepareHost()

	t := is.installAgent(vm, "", filepath.Join(outputDir, "install.log"))

	if !t.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	t.TestUninstall(is.T(), filepath.Join(outputDir, "uninstall.log"))
}

func (is *agentMSISuite) TestUpgrade() {
	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), is.T())
	is.Require().NoError(err, "should get output dir")
	is.T().Logf("Output dir: %s", outputDir)

	vm := is.Env().RemoteHost
	is.prepareHost()

	_ = is.installLastStable(vm, "", filepath.Join(outputDir, "install.log"))

	t, err := NewTester(is.T(), vm,
		WithAgentPackage(is.agentPackage),
	)
	is.Require().NoError(err, "should create tester")

	if !is.Run(fmt.Sprintf("upgrade to %s", t.agentPackage.AgentVersion()), func() {
		err = t.InstallAgent(is.T(), "", filepath.Join(outputDir, "upgrade.log"))
		is.Require().NoError(err, "should upgrade to agent %s", t.agentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	if !t.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	t.TestUninstall(is.T(), filepath.Join(outputDir, "uninstall.log"))
}

// TC-INS-002
func (is *agentMSISuite) TestUpgradeRollback() {
	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), is.T())
	is.Require().NoError(err, "should get output dir")
	is.T().Logf("Output dir: %s", outputDir)

	vm := is.Env().RemoteHost
	is.prepareHost()

	previousTester := is.installLastStable(vm, "", filepath.Join(outputDir, "install.log"))

	t, err := NewTester(is.T(), vm,
		WithAgentPackage(is.agentPackage),
	)
	is.Require().NoError(err, "should create tester")

	if !is.Run(fmt.Sprintf("upgrade to %s with rollback", t.agentPackage.AgentVersion()), func() {
		err = t.InstallAgent(is.T(), "WIXFAILWHENDEFERRED=1", filepath.Join(outputDir, "upgrade.log"))
		is.Require().Error(err, "should fail to install agent %s", t.agentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	// TODO: we shouldn't have to start the agent manually after rollback
	//       but the kitchen tests did too.
	err = windows.StartService(t.host, "DatadogAgent")
	is.Require().NoError(err, "agent service should start after rollback")

	if !previousTester.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	previousTester.TestUninstall(is.T(), filepath.Join(outputDir, "uninstall.log"))
}

// TC-INS-001
func (is *agentMSISuite) TestRepair() {
	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), is.T())
	is.Require().NoError(err, "should get output dir")
	is.T().Logf("Output dir: %s", outputDir)

	vm := is.Env().RemoteHost
	is.prepareHost()

	t := is.installAgent(vm, "", filepath.Join(outputDir, "install.log"))

	err = windows.StopService(t.host, "DatadogAgent")
	is.Require().NoError(err)

	// Corrupt the install
	err = t.host.Remove("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe")
	is.Require().NoError(err)
	err = t.host.RemoveAll("C:\\Program Files\\Datadog\\Datadog Agent\\embedded3")
	is.Require().NoError(err)

	if !is.Run("repair install", func() {
		err = windowsAgent.RepairAllAgent(t.host, "", filepath.Join(outputDir, "repair.log"))
		is.Require().NoError(err)
	}) {
		is.T().FailNow()
	}

	if !t.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	t.TestUninstall(is.T(), filepath.Join(outputDir, "uninstall.log"))
}

// installAgent installs the agent package on the VM and returns the Tester
func (is *agentMSISuite) installAgent(vm *components.RemoteHost, args string, logfile string, testerOpts ...TesterOption) *Tester {
	opts := []TesterOption{
		WithAgentPackage(is.agentPackage),
	}
	opts = append(opts, testerOpts...)
	t, err := NewTester(is.T(), vm, opts...)
	is.Require().NoError(err, "should create tester")

	if !is.Run(fmt.Sprintf("install %s", t.agentPackage.AgentVersion()), func() {
		err = t.InstallAgent(is.T(), args, logfile)
		is.Require().NoError(err, "should install agent %s", t.agentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	return t
}

// installLastStable installs the last stable agent package on the VM, runs tests, and returns the Tester
func (is *agentMSISuite) installLastStable(vm *components.RemoteHost, args string, logfile string) *Tester {
	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	is.Require().NoError(err, "should get last stable agent package from env")

	t, err := NewTester(is.T(), vm,
		WithAgentPackage(previousAgentPackage),
		WithPreviousVersion(),
	)
	is.Require().NoError(err, "should create tester")

	if !is.Run(fmt.Sprintf("install %s", previousAgentPackage.AgentVersion()), func() {
		err = t.InstallAgent(is.T(), args, logfile)
		is.Require().NoError(err, "should install agent %s", previousAgentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	if !t.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	return t
}
