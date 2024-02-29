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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"testing"
)

var devMode = flag.Bool("devmode", false, "enable devmode")

type agentMSISuite struct {
	windows.BaseAgentInstallerSuite[environments.Host]
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

	s := &agentMSISuite{}

	// Include the agent major version in the test name so junit reports will differentiate the tests
	t.Run(fmt.Sprintf("Agent v%s", majorVersion), func(t *testing.T) {
		e2e.Run(t, s, opts...)
	})
}

func (is *agentMSISuite) prepareHost() {
	vm := is.Env().RemoteHost

	if !is.Run("prepare VM", func() {
		is.Run("disable defender", func() {
			err := windowsCommon.DisableDefender(vm)
			is.Require().NoError(err, "should disable defender")
		})
	}) {
		is.T().Fatal("failed to prepare VM")
	}
}

func (is *agentMSISuite) TestInstall() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	t := is.installAgent(vm, nil)

	if !t.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	t.TestUninstall(is.T(), filepath.Join(is.OutputDir, "uninstall.log"))
}

func (is *agentMSISuite) TestUpgrade() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	_ = is.installLastStable(vm, nil)

	t, err := NewTester(is.T(), vm,
		WithAgentPackage(is.AgentPackage),
	)
	is.Require().NoError(err, "should create tester")

	if !is.Run(fmt.Sprintf("upgrade to %s", t.agentPackage.AgentVersion()), func() {
		err = t.InstallAgent(windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "upgrade.log")))
		is.Require().NoError(err, "should upgrade to agent %s", t.agentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	if !t.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	t.TestUninstall(is.T(), filepath.Join(is.OutputDir, "uninstall.log"))
}

// TC-INS-002
func (is *agentMSISuite) TestUpgradeRollback() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	previousTester := is.installLastStable(vm, nil)

	if !is.Run(fmt.Sprintf("upgrade to %s with rollback", is.AgentPackage.AgentVersion()), func() {
		_, err := windowsAgent.InstallAgent(vm,
			windowsAgent.WithPackage(is.AgentPackage),
			windowsAgent.WithWIXFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "upgrade.log")),
		)
		is.Require().Error(err, "should fail to install agent %s", is.AgentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	// TODO: we shouldn't have to start the agent manually after rollback
	//       but the kitchen tests did too.
	err := windowsCommon.StartService(vm, "DatadogAgent")
	is.Require().NoError(err, "agent service should start after rollback")

	if !previousTester.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	previousTester.TestUninstall(is.T(), filepath.Join(is.OutputDir, "uninstall.log"))
}

// TC-INS-001
func (is *agentMSISuite) TestRepair() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	t := is.installAgent(vm, nil)

	err := windowsCommon.StopService(t.host, "DatadogAgent")
	is.Require().NoError(err)

	// Corrupt the install
	err = t.host.Remove("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe")
	is.Require().NoError(err)
	err = t.host.RemoveAll("C:\\Program Files\\Datadog\\Datadog Agent\\embedded3")
	is.Require().NoError(err)

	if !is.Run("repair install", func() {
		err = windowsAgent.RepairAllAgent(t.host, "", filepath.Join(is.OutputDir, "repair.log"))
		is.Require().NoError(err)
	}) {
		is.T().FailNow()
	}

	if !t.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	t.TestUninstall(is.T(), filepath.Join(is.OutputDir, "uninstall.log"))
}

// TC-INS-006
func (is *agentMSISuite) TestAgentUser() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	hostinfo, err := windowsCommon.GetHostInfo(vm)
	is.Require().NoError(err)

	domainPart := windowsCommon.NameToNetBIOSName(hostinfo.Hostname)

	tcs := []struct {
		testname       string
		builtinaccount bool
		username       string
		expectedDomain string
		expectedUser   string
	}{
		{"user_only", false, "testuser", domainPart, "testuser"},
		{"dotslash_user", false, ".\\testuser", domainPart, "testuser"},
		{"domain_user", false, fmt.Sprintf("%s\\testuser", domainPart), domainPart, "testuser"},
		{"LocalSystem", true, "LocalSystem", "NT AUTHORITY", "SYSTEM"},
		{"SYSTEM", true, "SYSTEM", "NT AUTHORITY", "SYSTEM"},
	}
	for _, tc := range tcs {
		if !is.Run(tc.testname, func() {
			// subtest needs a new output dir
			is.OutputDir, err = runner.GetTestOutputDir(runner.GetProfile(), is.T())
			is.Require().NoError(err, "should get output dir")

			t := is.installAgent(vm, nil,
				WithInstallUser(tc.username),
				WithExpectedAgentUser(tc.expectedDomain, tc.expectedUser),
			)

			if !t.TestExpectations(is.T()) {
				is.T().FailNow()
			}

			t.TestUninstall(is.T(), filepath.Join(is.OutputDir, "uninstall.log"))
		}) {
			is.T().FailNow()
		}
	}
}

func (is *agentMSISuite) installAgentPackage(vm *components.RemoteHost, agentPackage *windowsAgent.Package, installOptions []windowsAgent.InstallAgentOption, testerOptions ...TesterOption) *Tester {
	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "install.log")),
	}
	installOpts = append(installOpts, installOptions...)
	testerOpts := []TesterOption{
		WithAgentPackage(agentPackage),
	}
	testerOpts = append(testerOpts, testerOptions...)
	t, err := NewTester(is.T(), vm, testerOpts...)
	is.Require().NoError(err, "should create tester")

	if !is.Run(fmt.Sprintf("install %s", t.agentPackage.AgentVersion()), func() {
		err = t.InstallAgent(installOpts...)
		is.Require().NoError(err, "should install agent %s", t.agentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	return t
}

// installAgent installs the agent package on the VM and returns the Tester
func (is *agentMSISuite) installAgent(vm *components.RemoteHost, options []windowsAgent.InstallAgentOption, testerOpts ...TesterOption) *Tester {
	return is.installAgentPackage(vm, is.AgentPackage, options, testerOpts...)
}

// installLastStable installs the last stable agent package on the VM, runs tests, and returns the Tester
func (is *agentMSISuite) installLastStable(vm *components.RemoteHost, options []windowsAgent.InstallAgentOption) *Tester {
	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	is.Require().NoError(err, "should get last stable agent package from env")
	t := is.installAgentPackage(vm, previousAgentPackage, options,
		WithPreviousVersion(),
	)

	// Ensure the agent is functioning properly to provide a proper foundation for the test
	if !t.TestExpectations(is.T()) {
		is.T().FailNow()
	}

	return t
}
