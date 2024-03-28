// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type agentMSISuite struct {
	windows.BaseAgentInstallerSuite[environments.Host]
	beforeInstall *windowsCommon.FileSystemSnapshot
}

func TestMSI(t *testing.T) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(componentos.WindowsDefault)),
	))}

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

// TODO: this isn't called before tabular tests (e.g. TestAgentUser/...)
func (is *agentMSISuite) BeforeTest(suiteName, testName string) {
	if beforeTest, ok := any(&is.BaseAgentInstallerSuite).(suite.BeforeTest); ok {
		beforeTest.BeforeTest(suiteName, testName)
	}

	vm := is.Env().RemoteHost
	var err error
	// If necessary (for example for parallelization), store the snapshot per suite/test in a map
	is.beforeInstall, err = windowsCommon.NewFileSystemSnapshot(vm, SystemPaths())
	is.Require().NoError(err)

	// Clear the event logs before each test
	for _, logName := range []string{"System", "Application"} {
		is.T().Logf("Clearing %s event log", logName)
		err = windowsCommon.ClearEventLog(vm, logName)
		is.Require().NoError(err, "should clear %s event log", logName)
	}
}

// TODO: this isn't called after tabular tests (e.g. TestAgentUser/...)
func (is *agentMSISuite) AfterTest(suiteName, testName string) {
	if afterTest, ok := any(&is.BaseAgentInstallerSuite).(suite.AfterTest); ok {
		afterTest.AfterTest(suiteName, testName)
	}

	if is.T().Failed() {
		// If the test failed, export the event logs for debugging
		vm := is.Env().RemoteHost
		for _, logName := range []string{"System", "Application"} {
			// collect the full event log as an evtx file
			is.T().Logf("Exporting %s event log", logName)
			outputPath := filepath.Join(is.OutputDir, fmt.Sprintf("%s.evtx", logName))
			err := windowsCommon.ExportEventLog(vm, logName, outputPath)
			is.Assert().NoError(err, "should export %s event log", logName)
			// Log errors and warnings to the screen for easy access
			out, err := windowsCommon.GetEventLogErrorsAndWarnings(vm, logName)
			if is.Assert().NoError(err, "should get errors and warnings from %s event log", logName) && out != "" {
				is.T().Logf("Errors and warnings from %s event log:\n%s", logName, out)
			}
		}
	}
}

func (is *agentMSISuite) TestInstall() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	// initialize test helper
	t := is.newTester(vm)

	// install the agent
	remoteMSIPath := is.installAgentPackage(vm, is.AgentPackage)

	// run tests
	if !t.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	// Test the code signatures of the installed files.
	// The same MSI is used in all tests so only check it once here.
	root, err := windowsAgent.GetInstallPathFromRegistry(t.host)
	is.Require().NoError(err)
	paths := getExpectedSignedFilesForAgentMajorVersion(t.expectedAgentMajorVersion)
	for i, path := range paths {
		paths[i] = root + path
	}
	if remoteMSIPath != "" {
		paths = append(paths, remoteMSIPath)
	}
	windowsAgent.TestValidDatadogCodeSignatures(is.T(), t.host, paths)

	is.uninstallAgentAndRunUninstallTests(t)
}

func (is *agentMSISuite) TestUpgrade() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	// install previous version
	_ = is.installLastStable(vm)

	// upgrade to the new version
	if !is.Run(fmt.Sprintf("upgrade to %s", is.AgentPackage.AgentVersion()), func() {
		_, err := is.InstallAgent(vm,
			windowsAgent.WithPackage(is.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "upgrade.log")),
		)
		is.Require().NoError(err, "should upgrade to agent %s", is.AgentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	// run tests
	t := is.newTester(vm)
	if !t.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	is.uninstallAgentAndRunUninstallTests(t)
}

// TC-INS-002
func (is *agentMSISuite) TestUpgradeRollback() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	// install previous version
	previousTester := is.installLastStable(vm)

	// upgrade to the new version, but intentionally fail
	if !is.Run(fmt.Sprintf("upgrade to %s with rollback", is.AgentPackage.AgentVersion()), func() {
		_, err := windowsAgent.InstallAgent(vm,
			windowsAgent.WithPackage(is.AgentPackage),
			windowsAgent.WithWixFailWhenDeferred(),
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

	// the previous version should be functional
	if !previousTester.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	is.uninstallAgentAndRunUninstallTests(previousTester)
}

// TC-INS-001
func (is *agentMSISuite) TestRepair() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	// initialize test helper
	t := is.newTester(vm)

	// install the agent
	_ = is.installAgentPackage(vm, is.AgentPackage)

	err := windowsCommon.StopService(t.host, "DatadogAgent")
	is.Require().NoError(err)

	// Corrupt the install
	installPath, err := windowsAgent.GetInstallPathFromRegistry(t.host)
	is.Require().NoError(err)
	err = t.host.Remove(filepath.Join(installPath, "bin", "agent.exe"))
	is.Require().NoError(err)
	err = t.host.RemoveAll(filepath.Join(installPath, "embedded3"))
	is.Require().NoError(err)

	// Run Repair through the MSI
	if !is.Run("repair install", func() {
		err = windowsAgent.RepairAllAgent(t.host, "", filepath.Join(is.OutputDir, "repair.log"))
		is.Require().NoError(err)
	}) {
		is.T().FailNow()
	}

	// run tests, agent should function normally after repair
	if !t.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	is.uninstallAgentAndRunUninstallTests(t)
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

			// initialize test helper
			t := is.newTester(vm,
				WithExpectedAgentUser(tc.expectedDomain, tc.expectedUser),
			)

			// install the agent
			_ = is.installAgentPackage(vm, is.AgentPackage,
				windowsAgent.WithAgentUser(tc.username),
			)

			// run tests
			if !t.TestInstallExpectations(is.T()) {
				is.T().FailNow()
			}

			is.uninstallAgentAndRunUninstallTests(t)
		}) {
			is.T().FailNow()
		}
	}
}

func (is *agentMSISuite) uninstallAgentAndRunUninstallTests(t *Tester) bool {
	return is.T().Run("uninstall the agent", func(tt *testing.T) {
		if !tt.Run("uninstall", func(tt *testing.T) {
			err := windowsAgent.UninstallAgent(t.host, filepath.Join(is.OutputDir, "uninstall.log"))
			require.NoError(tt, err, "should uninstall the agent")
		}) {
			tt.Fatal("uninstall failed")
		}

		AssertDoesNotRemoveSystemFiles(is.T(), is.Env().RemoteHost, is.beforeInstall)

		t.TestUninstallExpectations(tt)
	})
}

func (is *agentMSISuite) newTester(vm *components.RemoteHost, options ...TesterOption) *Tester {
	testerOpts := []TesterOption{
		WithAgentPackage(is.AgentPackage),
	}
	testerOpts = append(testerOpts, options...)
	t, err := NewTester(is.T(), vm, testerOpts...)
	is.Require().NoError(err, "should create tester")
	return t
}

func (is *agentMSISuite) installAgentPackage(vm *components.RemoteHost, agentPackage *windowsAgent.Package, installOptions ...windowsAgent.InstallAgentOption) string {
	remoteMSIPath := ""
	var err error

	// install the agent
	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithPackage(agentPackage),
		// default log file, can be overridden
		windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "install.log")),
		// trace-agent requires a valid API key
		windowsAgent.WithValidAPIKey(),
	}
	installOpts = append(installOpts, installOptions...)
	if !is.Run(fmt.Sprintf("install %s", agentPackage.AgentVersion()), func() {
		remoteMSIPath, err = is.InstallAgent(vm, installOpts...)
		is.Require().NoError(err, "should install agent %s", agentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	return remoteMSIPath
}

// installLastStable installs the last stable agent package on the VM, runs tests, and returns the Tester
func (is *agentMSISuite) installLastStable(vm *components.RemoteHost, options ...windowsAgent.InstallAgentOption) *Tester {

	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	is.Require().NoError(err, "should get last stable agent package from env")

	// create the tester
	t := is.newTester(vm,
		WithAgentPackage(previousAgentPackage),
		WithPreviousVersion(),
	)

	_ = is.installAgentPackage(vm, previousAgentPackage, options...)

	// Ensure the agent is functioning properly to provide a proper foundation for the test
	if !t.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	return t
}
