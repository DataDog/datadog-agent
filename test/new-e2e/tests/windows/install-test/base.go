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
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type baseAgentMSISuite struct {
	windows.BaseAgentInstallerSuite[environments.WindowsHost]
	beforeInstall *windowsCommon.FileSystemSnapshot
}

// NOTE: BeforeTest is not called before subtests
func (s *baseAgentMSISuite) BeforeTest(suiteName, testName string) {
	if beforeTest, ok := any(&s.BaseAgentInstallerSuite).(suite.BeforeTest); ok {
		beforeTest.BeforeTest(suiteName, testName)
	}

	vm := s.Env().RemoteHost
	var err error
	// If necessary (for example for parallelization), store the snapshot per suite/test in a map
	s.beforeInstall, err = windowsCommon.NewFileSystemSnapshot(vm, SystemPaths())
	s.Require().NoError(err)

	// Clear the event logs before each test
	for _, logName := range []string{"System", "Application"} {
		s.T().Logf("Clearing %s event log", logName)
		err = windowsCommon.ClearEventLog(vm, logName)
		s.Require().NoError(err, "should clear %s event log", logName)
	}
}

// NOTE: AfterTest is not called after subtests
func (s *baseAgentMSISuite) AfterTest(suiteName, testName string) {
	if afterTest, ok := any(&s.BaseAgentInstallerSuite).(suite.AfterTest); ok {
		afterTest.AfterTest(suiteName, testName)
	}

	if s.T().Failed() {
		// If the test failed, export the event logs for debugging
		vm := s.Env().RemoteHost
		for _, logName := range []string{"System", "Application"} {
			// collect the full event log as an evtx file
			s.T().Logf("Exporting %s event log", logName)
			outputPath := filepath.Join(s.OutputDir, fmt.Sprintf("%s.evtx", logName))
			err := windowsCommon.ExportEventLog(vm, logName, outputPath)
			s.Assert().NoError(err, "should export %s event log", logName)
			// Log errors and warnings to the screen for easy access
			out, err := windowsCommon.GetEventLogErrorsAndWarnings(vm, logName)
			if s.Assert().NoError(err, "should get errors and warnings from %s event log", logName) && out != "" {
				s.T().Logf("Errors and warnings from %s event log:\n%s", logName, out)
			}
		}
	}
}

func (s *baseAgentMSISuite) newTester(vm *components.RemoteHost, options ...TesterOption) *Tester {
	testerOpts := []TesterOption{
		WithAgentPackage(s.AgentPackage),
	}
	testerOpts = append(testerOpts, options...)
	t, err := NewTester(s, vm, testerOpts...)
	s.Require().NoError(err, "should create tester")
	return t
}

func (s *baseAgentMSISuite) installAgentPackage(vm *components.RemoteHost, agentPackage *windowsAgent.Package, installOptions ...windowsAgent.InstallAgentOption) string {
	remoteMSIPath := ""
	var err error

	// install the agent
	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithPackage(agentPackage),
		// default log file, can be overridden
		windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "install.log")),
		// trace-agent requires a valid API key
		windowsAgent.WithValidAPIKey(),
	}
	installOpts = append(installOpts, installOptions...)
	if !s.Run(fmt.Sprintf("install %s", agentPackage.AgentVersion()), func() {
		remoteMSIPath, err = s.InstallAgent(vm, installOpts...)
		s.Require().NoError(err, "should install agent %s", agentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	return remoteMSIPath
}

func (s *baseAgentMSISuite) uninstallAgentAndRunUninstallTests(t *Tester) bool {
	return s.T().Run("uninstall the agent", func(tt *testing.T) {
		// Get config dir from registry before uninstalling
		configDir, err := windowsAgent.GetConfigRootFromRegistry(t.host)
		require.NoError(tt, err)
		tt.Cleanup(func() {
			// remove the agent config for a cleaner uninstall
			tt.Logf("Removing agent configuration files")
			err = t.host.RemoveAll(configDir)
			require.NoError(tt, err)
		})

		if !tt.Run("uninstall", func(tt *testing.T) {
			err := windowsAgent.UninstallAgent(t.host, filepath.Join(s.OutputDir, "uninstall.log"))
			require.NoError(tt, err, "should uninstall the agent")
		}) {
			tt.Fatal("uninstall failed")
		}

		AssertDoesNotRemoveSystemFiles(s.T(), s.Env().RemoteHost, s.beforeInstall)

		t.TestUninstallExpectations(tt)
	})
}

func (s *baseAgentMSISuite) installPreviousAgentVersion(vm *components.RemoteHost, agentPackage *windowsAgent.Package, options ...windowsAgent.InstallAgentOption) *Tester {
	// create the tester
	t := s.newTester(vm,
		WithAgentPackage(agentPackage),
		WithPreviousVersion(),
	)

	_ = s.installAgentPackage(vm, agentPackage, options...)

	// Ensure the agent is functioning properly to provide a proper foundation for the test
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	return t
}

// installLastStable installs the last stable agent package on the VM, runs tests, and returns the Tester
func (s *baseAgentMSISuite) installLastStable(vm *components.RemoteHost, options ...windowsAgent.InstallAgentOption) *Tester {

	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	s.Require().NoError(err, "should get last stable agent package from env")

	return s.installPreviousAgentVersion(vm, previousAgentPackage, options...)
}

func (s *baseAgentMSISuite) readYamlConfig(host *components.RemoteHost, path string) (map[string]any, error) {
	config, err := host.ReadFile(path)
	if err != nil {
		return nil, err
	}
	confYaml := make(map[string]any)
	err = yaml.Unmarshal(config, &confYaml)
	if err != nil {
		return nil, err
	}
	return confYaml, nil
}

func (s *baseAgentMSISuite) readAgentConfig(host *components.RemoteHost) (map[string]any, error) {
	confDir, err := windowsAgent.GetConfigRootFromRegistry(host)
	if err != nil {
		return nil, err
	}
	configFilePath := filepath.Join(confDir, "datadog.yaml")
	return s.readYamlConfig(host, configFilePath)
}

func (s *baseAgentMSISuite) writeYamlConfig(host *components.RemoteHost, path string, config map[string]any) error {
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	_, err = host.WriteFile(path, configYaml)
	return err
}

// cleanupAgent fully removes the agent from the VM, MSI, config, agent user, etc,
// to create a clean slate for the next test to run on the same host/stack.
func (s *baseAgentMSISuite) cleanupAgent() {
	host := s.Env().RemoteHost
	t := s.T()

	// The uninstaller removes the registry keys that point to the install parameters,
	// so we collect them prior to uninstalling and then use defer to perform the actions.

	// remove the agent user
	_, agentUserName, err := windowsAgent.GetAgentUserFromRegistry(host)
	if err == nil {
		defer func() {
			t.Logf("Removing agent user: %s", agentUserName)
			err = windowsCommon.RemoveLocalUser(host, agentUserName)
			require.NoError(t, err)
		}()
	}

	// remove the agent config
	configDir, err := windowsAgent.GetConfigRootFromRegistry(host)
	if err == nil {
		defer func() {
			t.Logf("Removing agent configuration files: %s", configDir)
			err = host.RemoveAll(configDir)
			require.NoError(t, err)
		}()
	}

	// uninstall the agent
	_, err = windowsAgent.GetDatadogAgentProductCode(host)
	if err == nil {
		defer func() {
			t.Logf("Uninstalling Datadog Agent")
			err = windowsAgent.UninstallAgent(host, filepath.Join(s.OutputDir, "uninstall.log"))
			require.NoError(t, err)
		}()
	}
}

// cleanupOnSuccessInDevMode runs clean tasks on the host when running in DevMode. This makes it
// easier to run subsequent tests on the same host without having to manually clean up.
// This is not necessary outside of DevMode because the VM is destroyed after each test run.
// Cleanup is not run if the test failed, to allow for manual inspection of the VM.
func (s *baseAgentMSISuite) cleanupOnSuccessInDevMode() {
	if !s.IsDevMode() || s.T().Failed() {
		return
	}
	s.T().Log("Running DevMode cleanup tasks")
	s.cleanupAgent()
}

// isDevMode returns true when DevMode is enabled.
// We can't always use e2e.Suite.IsDevMode since that's only available once the suite is initialized
func isDevMode(t *testing.T) bool {
	devMode, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevMode, false)
	if err != nil {
		t.Logf("Unable to get DevMode value, DevMode will be disabled, error: %v", err)
		return false
	}
	return devMode
}

func isCI() bool {
	return os.Getenv("CI") != ""
}

func run[Env any](t *testing.T, s e2e.Suite[Env]) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake())}

	agentPackage, err := windowsAgent.GetPackageFromEnv()
	if err != nil {
		t.Fatalf("failed to get MSI URL from env: %v", err)
	}
	t.Logf("Using Agent: %#v", agentPackage)

	majorVersion := strings.Split(agentPackage.Version, ".")[0]

	// E2E auto includes the job ID in the stack name in CI tests. We only run one test per CI job
	// so in the CI this is sufficient for unique stack names.
	if isDevMode(t) {
		// if running in dev mode, set a stack name so we can re-use the same resource for each test.
		opts = append(opts, e2e.WithStackName("windows-msi-test"))
	} else if !isCI() {
		// if running locally and not in dev mode, run tests in parallel
		t.Parallel()
		// use a UUID to generate a unique name for the stack
		opts = append(opts, e2e.WithStackName(fmt.Sprintf("windows-msi-test-%s", uuid.NewString())))
	}

	// Include the agent major version in the test name so junit reports will differentiate the tests
	t.Run(fmt.Sprintf("Agent v%s", majorVersion), func(t *testing.T) {
		e2e.Run(t, s, opts...)
	})
}
