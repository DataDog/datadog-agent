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
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"

	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type baseAgentMSISuite struct {
	windows.BaseAgentInstallerSuite[environments.WindowsHost]
	beforeInstall      *windowsCommon.FileSystemSnapshot
	beforeInstallPerms map[string]string // path -> SDDL
	dumpFolder         string
}

// packageInstallOptions holds options for the installAgentPackage method
type packageInstallOptions struct {
	skipProcdump bool
}

// PackageInstallOption is a functional option for installAgentPackage
type PackageInstallOption func(*packageInstallOptions)

// WithSkipProcdump skips starting procdump during installation.
// Use this for tests that need to delete agent files after installation.
func WithSkipProcdump() PackageInstallOption {
	return func(o *packageInstallOptions) {
		o.skipProcdump = true
	}
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
	s.beforeInstallPerms, err = SnapshotPermissionsForPaths(vm, SystemPathsForPermissionsValidation())
	s.Require().NoError(err)

	// Clear the event logs before each test
	for _, logName := range []string{"System", "Application"} {
		s.T().Logf("Clearing %s event log", logName)
		err = windowsCommon.ClearEventLog(vm, logName)
		s.Require().NoError(err, "should clear %s event log", logName)
	}

	// Enable crash dumps before each test
	s.dumpFolder = `C:\dumps`
	err = windowsCommon.EnableWERGlobalDumps(vm, s.dumpFolder)
	s.Require().NoError(err, "should enable WER dumps")

	// Set GOTRACEBACK=wer globally so Go processes produce stack traces in crash dumps
	_, err = vm.Execute(`[Environment]::SetEnvironmentVariable('GOTRACEBACK', 'wer', 'Machine')`)
	s.Require().NoError(err, "should set GOTRACEBACK environment variable")

	// Clean dump folder before each test
	s.T().Logf("Clearing dump folder")
	err = windowsCommon.CleanDirectory(vm, s.dumpFolder)
	s.Require().NoError(err, "should clean dump folder")
}

// NOTE: AfterTest is not called after subtests
func (s *baseAgentMSISuite) AfterTest(suiteName, testName string) {
	if afterTest, ok := any(&s.BaseAgentInstallerSuite).(suite.AfterTest); ok {
		afterTest.AfterTest(suiteName, testName)
	}

	vm := s.Env().RemoteHost

	// Look for and download crash dumps
	dumps, err := windowsCommon.DownloadAllWERDumps(vm, s.dumpFolder, s.SessionOutputDir())
	s.Assert().NoError(err, "should download crash dumps")
	if !s.Assert().Empty(dumps, "should not have crash dumps") {
		s.T().Logf("Found crash dumps:")
		for _, dump := range dumps {
			s.T().Logf("  %s", dump)
		}
	}

	if s.T().Failed() {
		// If the test failed, export the event logs for debugging
		for _, logName := range []string{"System", "Application"} {
			// collect the full event log as an evtx file
			s.T().Logf("Exporting %s event log", logName)
			outputPath := filepath.Join(s.SessionOutputDir(), logName+".evtx")
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
	return s.installAgentPackageWithOptions(vm, agentPackage, nil, installOptions...)
}

func (s *baseAgentMSISuite) installAgentPackageWithOptions(vm *components.RemoteHost, agentPackage *windowsAgent.Package, pkgOpts []PackageInstallOption, installOptions ...windowsAgent.InstallAgentOption) string {
	remoteMSIPath := ""
	var err error

	// Apply package install options
	opts := &packageInstallOptions{}
	for _, opt := range pkgOpts {
		opt(opts)
	}

	// install the agent
	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithPackage(agentPackage),
		// default log file, can be overridden
		windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
		// trace-agent requires a valid API key
		windowsAgent.WithValidAPIKey(),
	}
	installOpts = append(installOpts, installOptions...)

	// Check if DD_INSTALL_ONLY is set - if so, services won't start
	skipServiceCheck := s.hasInstallOnlyOption(installOptions...)

	// Start xperf tracing
	s.startXperf(vm)
	defer s.collectXperf(vm)

	// Start procdump in background to capture crash dumps (only if service will start and not skipped)
	if !skipServiceCheck && !opts.skipProcdump {
		ps := s.startProcdump(vm)
		defer s.collectProcdumps(ps, vm)
	}

	if !s.Run("install "+agentPackage.AgentVersion(), func() {
		remoteMSIPath, err = s.InstallAgent(vm, installOpts...)
		s.Require().NoError(err, "should install agent %s", agentPackage.AgentVersion())

		// Wait for the service to start (up to 5 minutes), unless DD_INSTALL_ONLY is set
		if !skipServiceCheck {
			err = s.waitForServiceRunning(vm, "datadogagent", 300)
			s.Require().NoError(err, "service should be running after install")
		}
	}) {
		s.T().FailNow()
	}

	return remoteMSIPath
}

// hasInstallOnlyOption checks if DD_INSTALL_ONLY is set in the install options
func (s *baseAgentMSISuite) hasInstallOnlyOption(installOptions ...windowsAgent.InstallAgentOption) bool {
	params := &windowsAgent.InstallAgentParams{}
	for _, opt := range installOptions {
		_ = opt(params)
	}
	return params.InstallOnly == "true"
}

// waitForServiceRunning waits for a service to reach Running status
func (s *baseAgentMSISuite) waitForServiceRunning(vm *components.RemoteHost, serviceName string, timeoutSeconds int) error {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	for time.Now().Before(deadline) {
		status, err := windowsCommon.GetServiceStatus(vm, serviceName)
		if err == nil && strings.Contains(status, "Running") {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("timeout waiting for service %s to reach Running state after %d seconds", serviceName, timeoutSeconds)
}

// startXperf starts xperf tracing on the remote host
func (s *baseAgentMSISuite) startXperf(vm *components.RemoteHost) {
	err := vm.HostArtifactClient.Get("windows-products/xperf-5.0.8169.zip", "C:/xperf.zip")
	s.Require().NoError(err)

	// extract if C:/xperf dir does not exist
	_, err = vm.Execute("if (-Not (Test-Path -Path C:/xperf)) { Expand-Archive -Path C:/xperf.zip -DestinationPath C:/xperf }")
	s.Require().NoError(err)

	outputPath := "C:/kernel.etl"
	xperfPath := "C:/xperf/xperf.exe"
	_, err = vm.Execute(fmt.Sprintf(`& "%s" -On Base+Latency+CSwitch+PROC_THREAD+LOADER+Profile+DISPATCHER -stackWalk CSwitch+Profile+ReadyThread+ThreadCreate -f %s -MaxBuffers 1024 -BufferSize 1024 -MaxFile 1024 -FileMode Circular`, xperfPath, outputPath))
	s.Require().NoError(err)
}

// collectXperf collects xperf tracing from the remote host
func (s *baseAgentMSISuite) collectXperf(vm *components.RemoteHost) {
	xperfPath := "C:/xperf/xperf.exe"
	outputPath := "C:/full_host_profiles.etl"

	_, err := vm.Execute(fmt.Sprintf(`& "%s" -stop -d %s`, xperfPath, outputPath))
	s.Require().NoError(err)

	// collect xperf if the test failed
	if s.T().Failed() {
		outDir := s.SessionOutputDir()
		err = vm.GetFile(outputPath, filepath.Join(outDir, "full_host_profiles.etl"))
		s.Require().NoError(err)
	}
}

// startProcdump sets up procdump and starts it in the background
func (s *baseAgentMSISuite) startProcdump(vm *components.RemoteHost) *windowsCommon.ProcdumpSession {
	// Setup procdump on remote host
	s.T().Log("Setting up procdump on remote host")
	err := windowsCommon.SetupProcdump(vm)
	s.Require().NoError(err, "should setup procdump")

	// Start procdump - use "agent.exe" as the process name for -w flag
	ps, err := windowsCommon.StartProcdump(vm, "agent.exe")
	s.Require().NoError(err, "should start procdump")

	return ps
}

// collectProcdumps stops procdump and downloads any captured dumps if the test failed.
func (s *baseAgentMSISuite) collectProcdumps(ps *windowsCommon.ProcdumpSession, vm *components.RemoteHost) {
	// Only collect dumps if the test failed
	if !s.T().Failed() {
		ps.Close()
		return
	}

	// Wait for procdump to finish writing dump files BEFORE closing the session.
	// Procdump is configured to capture 5 dumps, so wait until all 5 are created.
	expectedDumpCount := 5
	s.T().Logf("Waiting for procdump to create %d dump files...", expectedDumpCount)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		output, err := vm.Execute(fmt.Sprintf(`(Get-ChildItem -Path '%s' -Filter '*.dmp' -ErrorAction SilentlyContinue | Measure-Object).Count`, windowsCommon.ProcdumpsPath))
		if err == nil {
			countStr := strings.TrimSpace(output)
			count, parseErr := strconv.Atoi(countStr)
			if parseErr == nil && count >= expectedDumpCount {
				s.T().Logf("All %d dump files ready", count)
				break
			}
			s.T().Logf("Found %s dump files, waiting for %d...", countStr, expectedDumpCount)
		}
		time.Sleep(5 * time.Second)
	}

	ps.Close()

	// Download all dump files
	outDir := s.SessionOutputDir()
	if err := vm.GetFolder(windowsCommon.ProcdumpsPath, outDir); err != nil {
		s.T().Logf("Warning: failed to download procdumps %s: %v", windowsCommon.ProcdumpsPath, err)
	} else {
		s.T().Logf("Downloaded procdumps to: %s", outDir)
	}
}

func (s *baseAgentMSISuite) uninstallAgent() bool {
	host := s.Env().RemoteHost
	return s.T().Run("uninstall the agent", func(tt *testing.T) {
		if !tt.Run("uninstall", func(tt *testing.T) {
			err := windowsAgent.UninstallAgent(host, filepath.Join(s.SessionOutputDir(), "uninstall.log"))
			require.NoError(tt, err, "should uninstall the agent")
		}) {
			tt.Fatal("uninstall failed")
		}
	})
}

func (s *baseAgentMSISuite) uninstallAgentAndRunUninstallTests(t *Tester) bool {
	host := s.Env().RemoteHost

	if !s.uninstallAgent() {
		return false
	}

	return s.T().Run("validate uninstall", func(tt *testing.T) {
		AssertDoesNotRemoveSystemFiles(s.T(), host, s.beforeInstall)

		s.Run("uninstall does not change system file permissions", func() {
			AssertDoesNotChangePathPermissions(s.T(), host, s.beforeInstallPerms)
		})

		t.TestUninstallExpectations(tt)
	})
}

func (s *baseAgentMSISuite) installAndTestPreviousAgentVersion(vm *components.RemoteHost, agentPackage *windowsAgent.Package, options ...windowsAgent.InstallAgentOption) {
	_ = s.installAgentPackage(vm, agentPackage, options...)
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), agentPackage.AgentVersion())
}

// installLastStable installs the last stable agent package on the VM, runs tests, and returns the Tester
func (s *baseAgentMSISuite) installAndTestLastStable(vm *components.RemoteHost, options ...windowsAgent.InstallAgentOption) *windowsAgent.Package {
	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	s.Require().NoError(err, "should get last stable agent package from env")

	s.installAndTestPreviousAgentVersion(vm, previousAgentPackage, options...)

	return previousAgentPackage
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
			err = windowsAgent.UninstallAgent(host, filepath.Join(s.SessionOutputDir(), "uninstall.log"))
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

// Run sets some options and runs an install test.
func Run[Env any](t *testing.T, s e2e.Suite[Env]) {
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
		opts = append(opts, e2e.WithStackName("windows-msi-test-"+uuid.NewString()))
	}

	// Include the agent major version in the test name so junit reports will differentiate the tests
	t.Run("Agent v"+majorVersion, func(t *testing.T) {
		e2e.Run(t, s, opts...)
	})
}
