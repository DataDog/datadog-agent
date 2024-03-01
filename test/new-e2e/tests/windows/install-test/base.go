// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package installtest contains e2e tests for the Windows agent installer
package installtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"testing"
)

type agentInstallerSuite[Env any] interface {
	e2e.Suite[Env]

	GetStackName() (string, error)
	GetAgentMajorVersion() (string, error)
}

type baseAgentMSISuite struct {
	windows.BaseAgentInstallerSuite[environments.Host]
}

// getStackNamePrefix returns the stack name for the test suite.
// Set unique stack names to avoid conflicts with other tests.
func (s *baseAgentMSISuite) GetStackName() (string, error) {
	agentPackage, err := s.GetAgentPackage()
	if err != nil {
		return "", err
	}
	majorVersion := strings.Split(agentPackage.Version, ".")[0]

	// E2E auto includes the pipeline ID in the stack name, so we don't need to do that here.
	stackName := fmt.Sprintf("windows-msi-test-v%s-%s", majorVersion, agentPackage.Arch)

	// If running in CI, append the CI job ID to the stack name to ensure uniqueness between jobs
	ciJobID := os.Getenv("CI_JOB_ID")
	if ciJobID != "" {
		stackName = fmt.Sprintf("%s-%s", stackName, ciJobID)
	}

	return stackName, nil
}

func (s *baseAgentMSISuite) GetAgentMajorVersion() (string, error) {
	agentPackage, err := s.GetAgentPackage()
	if err != nil {
		return "", err
	}
	return strings.Split(agentPackage.Version, ".")[0], nil
}

func (s *baseAgentMSISuite) prepareHost() {
	vm := s.Env().RemoteHost

	if !s.Run("prepare VM", func() {
		s.Run("disable defender", func() {
			err := windowsCommon.DisableDefender(vm)
			s.Require().NoError(err, "should disable defender")
		})
	}) {
		s.T().Fatal("failed to prepare VM")
	}
}

func (s *baseAgentMSISuite) installAgentPackage(vm *components.RemoteHost, agentPackage *windowsAgent.Package, installOptions []windowsAgent.InstallAgentOption, testerOptions ...TesterOption) *Tester {
	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "install.log")),
	}
	installOpts = append(installOpts, installOptions...)
	testerOpts := []TesterOption{
		WithAgentPackage(agentPackage),
	}
	testerOpts = append(testerOpts, testerOptions...)
	t, err := NewTester(s.T(), vm, testerOpts...)
	s.Require().NoError(err, "should create tester")

	if !s.Run(fmt.Sprintf("install %s", t.agentPackage.AgentVersion()), func() {
		err = t.InstallAgent(installOpts...)
		s.Require().NoError(err, "should install agent %s", t.agentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	return t
}

// installAgent installs the agent package on the VM and returns the Tester
func (s *baseAgentMSISuite) installAgent(vm *components.RemoteHost, options []windowsAgent.InstallAgentOption, testerOpts ...TesterOption) *Tester {
	return s.installAgentPackage(vm, s.AgentPackage, options, testerOpts...)
}

// installLastStable installs the last stable agent package on the VM, runs tests, and returns the Tester
func (s *baseAgentMSISuite) installLastStable(vm *components.RemoteHost, options []windowsAgent.InstallAgentOption) *Tester {
	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	s.Require().NoError(err, "should get last stable agent package from env")
	t := s.installAgentPackage(vm, previousAgentPackage, options,
		WithPreviousVersion(),
	)

	// Ensure the agent is functioning properly to provide a proper foundation for the test
	if !t.TestExpectations(s.T()) {
		s.T().FailNow()
	}

	return t
}

func run[Env any, T agentInstallerSuite[Env]](t *testing.T, s T, options ...e2e.SuiteOption) {
	s.SetT(t)

	opts := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(componentos.WindowsDefault)),
	))}

	stackName, err := s.GetStackName()
	if err != nil {
		t.Fatalf("failed to get stack name: %v", err)
	}
	opts = append(opts, e2e.WithStackName(stackName))

	// give precedence to provided options
	opts = append(opts, options...)

	// Include the agent major version in the test name so junit reports will differentiate the tests
	majorVersion, err := s.GetAgentMajorVersion()
	if err != nil {
		t.Fatal(err)
	}
	testName := fmt.Sprintf("Windows Agent v%s", majorVersion)

	t.Run(testName, func(t *testing.T) {
		e2e.Run(t, s, opts...)
	})
}
