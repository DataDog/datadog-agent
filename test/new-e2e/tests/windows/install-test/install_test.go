// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installtest contains e2e tests for the Windows agent installer
package installtest

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/agent"

	"github.com/DataDog/test-infra-definitions/components/os"
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
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
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
	opts = append(opts, e2e.WithStackName(fmt.Sprintf("windows-msi-test-v%s-%s%s", majorVersion, agentPackage.Arch, stackNameChannelPart)))

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

	t, err := NewTester(is.T(), vm, WithExpectedAgentVersion(is.agentPackage.AgentVersion()))
	is.Require().NoError(err, "should create tester")

	if !t.TestInstallAgentPackage(is.T(), is.agentPackage, "", filepath.Join(outputDir, "install.log")) {
		is.T().Fatal("failed to install agent")
	}
	t.TestRuntimeExpectations(is.T())
	t.TestUninstall(is.T(), filepath.Join(outputDir, "uninstall.log"))
}

func (is *agentMSISuite) TestUpgrade() {
	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), is.T())
	is.Require().NoError(err, "should get output dir")
	is.T().Logf("Output dir: %s", outputDir)

	vm := is.Env().RemoteHost
	is.prepareHost()

	t, err := NewTester(is.T(), vm, WithExpectedAgentVersion(is.agentPackage.AgentVersion()))
	is.Require().NoError(err, "should create tester")

	// install old agent
	_ = is.installLastStable(t, filepath.Join(outputDir, "install.log"))

	// upgrade to new agent
	if !t.TestInstallAgentPackage(is.T(), is.agentPackage, "", filepath.Join(outputDir, "upgrade.log")) {
		is.T().Fatal("failed to upgrade agent")
	}

	t.TestRuntimeExpectations(is.T())
	t.TestUninstall(is.T(), filepath.Join(outputDir, "uninstall.log"))
}

// This is separate from TestInstallAgentPackage because previous versions of the agent
// may not conform to the latest test expectations.
func (is *agentMSISuite) installLastStable(t *Tester, logfile string) *windowsAgent.Package {
	var agentPackage *windowsAgent.Package

	if !is.Run("install prev stable agent", func() {
		var err error

		agentPackage, err = windowsAgent.GetLastStablePackageFromEnv()
		is.Require().NoError(err, "should get last stable agent package from env")

		t.InstallAgentPackage(is.T(), agentPackage, "", logfile)

		agentVersion, err := t.InstallTestClient.GetAgentVersion()
		is.Require().NoError(err, "should get agent version")
		windowsAgent.TestAgentVersion(is.T(), agentPackage.AgentVersion(), agentVersion)
	}) {
		is.T().Fatal("failed to install last stable agent")
	}

	return agentPackage
}
