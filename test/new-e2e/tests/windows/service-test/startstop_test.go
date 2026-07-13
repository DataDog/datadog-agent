// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package servicetest contains tests for Windows Agent service behavior
package servicetest

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v7"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	scenwindows "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/stretchr/testify/suite"
)

//go:embed fixtures/datadog.yaml
var agentConfig string

//go:embed fixtures/datadog-pa-disabled.yaml
var agentConfigPADisabled string

//go:embed fixtures/datadog-ta-disabled.yaml
var agentConfigTADisabled string

//go:embed fixtures/datadog-di-disabled.yaml
var agentConfigDIDisabled string

//go:embed fixtures/datadog-rc-enabled.yaml
var agentConfigRCEnabled string

//go:embed fixtures/system-probe.yaml
var systemProbeConfig string

//go:embed fixtures/system-probe-nofim.yaml
var systemProbeNoFIMConfig string

//go:embed fixtures/system-probe-disabled.yaml
var systemProbeDisabled string

//go:embed fixtures/security-agent.yaml
var securityAgentConfig string

//go:embed fixtures/security-agent-disabled.yaml
var securityAgentConfigDisabled string

// Folder for WER dumps.
const werCrashDumpFolder = `C:\dumps`

// Path to the system crash dump (BSOD).
const systemCrashDumpFile = `C:\Windows\MEMORY.DMP`

// The name of the downloaded system crash dump file.
const systemCrashDumpOutFileName = `SystemCrash.DMP`

// Default scaling of timeouts based on present E2E flakiness. Adjust this as necessary.
const defaultTimeoutScale = 1

// Default scaling of timeouts for tests with driver verifier. This needs to be generous.
const driverVerifierTimeoutScale = 10

type onServiceStateMismatch func(host *components.RemoteHost, serviceName, actual string)

// TestServiceBehaviorInstallerWithRemoteConfig tests that the installer runs
// when remote_configuration is explicitly enabled, which is required in FIPS mode.
// TODO: remove this test when installer runs fully in FIPS mode.
func TestServiceBehaviorInstallerWithRemoteConfig(t *testing.T) {
	s := &installerWithRemoteConfigSuite{}
	run(t, s, systemProbeConfig, agentConfigRCEnabled, securityAgentConfig)
}

type installerWithRemoteConfigSuite struct {
	powerShellServiceCommandSuite
}

func (s *installerWithRemoteConfigSuite) SetupSuite() {
	s.powerShellServiceCommandSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// With remote_configuration enabled, the installer should run even in FIPS mode
	s.runningUserServices = func() []string {
		return s.filterLegacySCMServices(s.getInstalledUserServices())
	}
	s.runningServices = func() []string {
		user := s.filterLegacySCMServices(s.getInstalledUserServices())
		kernel := s.getInstalledKernelServices()
		return append(slices.Clone(user), kernel...)
	}
}

// TestServiceBehaviorAgentCommandNoFIM tests the service behavior when controlled by Agent commands
func TestNoFIMServiceBehaviorAgentCommand(t *testing.T) {
	s := &agentServiceCommandSuite{}
	run(t, s, systemProbeNoFIMConfig, agentConfig, securityAgentConfig)
}

// TestServiceBehaviorPowerShellNoFIM tests the service behavior when controlled by PowerShell commands
func TestNoFIMServiceBehaviorPowerShell(t *testing.T) {
	s := &powerShellServiceCommandSuite{}
	run(t, s, systemProbeNoFIMConfig, agentConfig, securityAgentConfig)
}

// TestServiceBehaviorAgentCommand tests the service behavior when controlled by Agent commands
func TestServiceBehaviorAgentCommand(t *testing.T) {
	s := &agentServiceCommandSuite{}
	run(t, s, systemProbeConfig, agentConfig, securityAgentConfig)
}

type agentServiceCommandSuite struct {
	baseStartStopSuite
}

func (s *agentServiceCommandSuite) SetupSuite() {
	s.baseStartStopSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	installPath, err := windowsAgent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err, "should get install path from registry")

	s.startAgentCommand = func(host *components.RemoteHost) error {
		cmd := fmt.Sprintf(`& "%s\bin\agent.exe" start-service`, installPath)
		out, err := host.Execute(cmd)
		out = strings.TrimSpace(out)
		if err == nil && out != "" {
			s.T().Logf("agent start-service output:\n%s", out)
		}
		return err
	}
	s.stopAgentCommand = func(host *components.RemoteHost) error {
		cmd := fmt.Sprintf(`& "%s\bin\agent.exe" stop-service`, installPath)
		out, err := host.Execute(cmd)
		out = strings.TrimSpace(out)
		if err == nil && out != "" {
			s.T().Logf("agent stop-service output:\n%s", out)
		}
		return err
	}
}

// TestServiceBehaviorAgentCommand tests the service behavior when controlled by PowerShell commands
func TestServiceBehaviorPowerShell(t *testing.T) {
	s := &powerShellServiceCommandSuite{}
	run(t, s, systemProbeConfig, agentConfig, securityAgentConfig)
}

type powerShellServiceCommandSuite struct {
	baseStartStopSuite
}

func (s *powerShellServiceCommandSuite) SetupSuite() {
	s.baseStartStopSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	s.startAgentCommand = func(host *components.RemoteHost) error {
		cmd := `Start-Service -Name datadogagent`
		out, err := host.Execute(cmd)
		out = strings.TrimSpace(out)
		if err == nil && out != "" {
			s.T().Logf("PowerShell Start-Service output:\n%s", out)
		}
		return err
	}
	s.stopAgentCommand = func(host *components.RemoteHost) error {
		cmd := `Stop-Service -Force -Name datadogagent`
		out, err := host.Execute(cmd)
		out = strings.TrimSpace(out)
		if err == nil && out != "" {
			s.T().Logf("PowerShell Stop-Service output:\n%s", out)
		}
		return err
	}
}

// TestStopTimeout tests that each service stops without hitting its hard stop timeout, which
// results in a message in the Application event log.
func (s *powerShellServiceCommandSuite) TestStopTimeout() {
	host := s.Env().RemoteHost

	// ensure all services are running
	s.startAgent()
	s.requireAllServicesState("Running", nil)

	services := []string{
		// stop dependent services first since stopping them won't affect other services
		"datadog-trace-agent",
		"dd-procmgr-service",
		"datadog-security-agent",
		"datadog-system-probe",
		// stop core agent last since it will trigger stop of other services
		"datadogagent",
	}
	// stop them one by one, measuring the time it takes to stop each one using Measure-Command
	for _, serviceName := range services {
		timeTaken, out, err := windowsCommon.MeasureCommand(host, fmt.Sprintf("Stop-Service -Force -Name '%s'", serviceName))
		s.Require().NoError(err, "should stop %s", serviceName)
		s.T().Logf("Stop-Service output for %s:\n%s", serviceName, out)
		s.T().Logf("Time taken to stop %s: %v ms", serviceName, timeTaken.Milliseconds())
		// check if the time taken is less than the hard stop timeout
		s.Assert().Lessf(timeTaken, 15*time.Second, "should stop %s within 15 seconds", serviceName)
	}

	// test all services are stopped
	s.assertAllServicesState("Stopped", nil)

	// check there are no unexpected exit messages in System event log
	// hard stop timeout should set SERVICE_STOPPED before exiting, so
	// we should not see "terminated unexpectedly" messages in the event log
	entries, err := windowsCommon.GetEventLogErrorAndWarningEntries(host, "System")
	s.Require().NoError(err, "should get errors and warnings from System event log")
	s.Require().Empty(windowsCommon.Filter(entries, func(entry windowsCommon.EventLogEntry) bool {
		return strings.Contains(entry.Message, "terminated unexpectedly")
	}), "should not have unexpected exit messages in the event log")

	// check there are no timeout messages in Application event log
	entries, err = windowsCommon.GetEventLogErrorAndWarningEntries(host, "Application")
	s.Require().NoError(err, "should get errors and warnings from Application event log")
	s.Require().Empty(windowsCommon.Filter(entries, func(entry windowsCommon.EventLogEntry) bool {
		return strings.Contains(entry.Message, "hard stopping service")
	}), "should not have timeout messages in the event log")
}

// TestHardExitEventLogEntry tests that the System event log contains an "unexpectedly terminated" message when a service is killed
func (s *powerShellServiceCommandSuite) TestHardExitEventLogEntry() {
	s.T().Cleanup(func() {
		// stop the drivers that are left running when agents are killed
		s.stopAllServices()
	})
	host := s.Env().RemoteHost
	s.startAgent()
	s.requireAllServicesState("Running", nil)

	// kill the agent
	for _, serviceName := range s.runningUserServices() {
		// get pid
		pid, err := windowsCommon.GetServicePID(host, serviceName)
		s.Require().NoError(err, "should get the PID for %s", serviceName)
		// kill the process
		_, err = host.Execute(fmt.Sprintf("Stop-Process -Force -Id %d", pid))
		s.Require().NoError(err, "should kill the process with PID %d", pid)

		// service should stop
		s.Require().True(s.EventuallyWithExponentialBackoff(func() error {
			status, err := windowsCommon.GetServiceStatus(host, serviceName)
			if err != nil {
				return fmt.Errorf("should get the status for %s: %v", serviceName, err)
			}
			if status != "Stopped" {
				return fmt.Errorf("waiting for %s to stop", serviceName)
			}
			return nil
		}, (2*s.timeoutScale)*time.Minute, 60*time.Second, "%s should be stopped", serviceName))
	}

	// collect display names for services
	displayNames := make([]string, 0, len(s.runningUserServices()))
	for _, serviceName := range s.runningUserServices() {
		conf, err := windowsCommon.GetServiceConfig(host, serviceName)
		s.Require().NoError(err, "should get the configuration for %s", serviceName)
		displayNames = append(displayNames, conf.DisplayName)
	}

	// check the System event log for hard exit messages
	s.EventuallyWithExponentialBackoff(func() error {
		entries, err := windowsCommon.GetEventLogErrorAndWarningEntries(host, "System")
		if err != nil {
			return fmt.Errorf("should get errors and warnings from System event log: %v", err)
		}
		for _, displayName := range displayNames {
			match := fmt.Sprintf("The %s service terminated unexpectedly", displayName)
			matching := windowsCommon.Filter(entries, func(entry windowsCommon.EventLogEntry) bool {
				return strings.Contains(entry.Message, match)
			})
			if len(matching) != 1 {
				return fmt.Errorf("should have hard exit message for %s in the event log", displayName)
			}
		}
		return nil
	}, (1*s.timeoutScale)*time.Minute, 60*time.Second, "should have hard exit messages in the event log")
}

type agentServiceDisabledSuite struct {
	baseStartStopSuite
	disabledServices []string
}

// TestServiceBehaviorWhenDisabled tests the service behavior when disabled in the configuration
func TestServiceBehaviorWhenDisabledSystemProbe(t *testing.T) {
	s := &agentServiceDisabledSystemProbeSuite{}
	s.disabledServices = []string{
		"datadog-security-agent",
		"datadog-system-probe",
		"ddnpm",
		"ddprocmon",
	}
	run(t, s, systemProbeDisabled, agentConfig, securityAgentConfigDisabled)
}

type agentServiceDisabledSystemProbeSuite struct {
	agentServiceDisabledSuite
}

// TestServiceBehaviorWhenDisabledProcessAgent tests the service behavior when disabled in the configuration
func TestServiceBehaviorWhenDisabledProcessAgent(t *testing.T) {
	s := &agentServiceDisabledProcessAgentSuite{}
	s.disabledServices = []string{
		"datadog-process-agent",
		"datadog-security-agent",
		"datadog-system-probe",
		"ddnpm",
		"ddprocmon",
	}
	run(t, s, systemProbeDisabled, agentConfigPADisabled, securityAgentConfigDisabled)
}

type agentServiceDisabledProcessAgentSuite struct {
	agentServiceDisabledSuite
}

func TestServiceBehaviorWhenDisabledTraceAgent(t *testing.T) {
	s := &agentServiceDisabledTraceAgentSuite{}
	s.disabledServices = []string{
		"datadog-trace-agent",
	}
	run(t, s, systemProbeConfig, agentConfigTADisabled, securityAgentConfig)
}

type agentServiceDisabledTraceAgentSuite struct {
	agentServiceDisabledSuite
}

func TestServiceBehaviorWhenDisabledInstaller(t *testing.T) {
	s := &agentServiceDisabledInstallerSuite{}
	s.disabledServices = []string{
		"Datadog Installer",
	}
	run(t, s, systemProbeConfig, agentConfigDIDisabled, securityAgentConfig)
}

type agentServiceDisabledInstallerSuite struct {
	agentServiceDisabledSuite
}

func (s *agentServiceDisabledSuite) SetupSuite() {
	s.baseStartStopSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	// TODO: This service is not supported in FIPS mode yet
	if s.Env().Agent.FIPSEnabled && !slices.Contains(s.disabledServices, "Datadog Installer") {
		s.disabledServices = append(s.disabledServices, "Datadog Installer")
	}

	// set up the expected services before calling the base setup
	s.runningUserServices = func() []string {
		runningServices := []string{}
		for _, service := range s.filterLegacySCMServices(s.getInstalledUserServices()) {
			if !slices.Contains(s.disabledServices, service) {
				runningServices = append(runningServices, service)
			}
		}
		return runningServices
	}
	s.runningServices = func() []string {
		user := s.runningUserServices()
		kernel := s.getInstalledKernelServices()
		runningServices := append(slices.Clone(user), kernel...)
		return slices.DeleteFunc(runningServices, func(service string) bool {
			return slices.Contains(s.disabledServices, service)
		})
	}

	s.startAgentCommand = func(host *components.RemoteHost) error {
		cmd := `Start-Service -Name datadogagent`
		out, err := host.Execute(cmd)
		out = strings.TrimSpace(out)
		if err == nil && out != "" {
			s.T().Logf("PowerShell Start-Service output:\n%s", out)
		}
		return err
	}
	s.stopAgentCommand = func(host *components.RemoteHost) error {
		cmd := `Stop-Service -Force -Name datadogagent`
		out, err := host.Execute(cmd)
		out = strings.TrimSpace(out)
		if err == nil && out != "" {
			s.T().Logf("PowerShell Stop-Service output:\n%s", out)
		}
		return err
	}
}

func (s *agentServiceDisabledSuite) TestStartingDisabledService() {
	kernel := s.getInstalledKernelServices()
	// check that the system probe is not running
	for _, service := range s.disabledServices {
		s.assertServiceState("Stopped", service, nil)

		// verify that we only try user services
		if !slices.Contains(kernel, service) {
			// In FIPS builds the installer does not start correctly
			// when remote configuration is disabled and Start-Service fails.
			// TODO: remove this when installer runs fully in FIPS mode.
			if s.Env().Agent.FIPSEnabled && service == "Datadog Installer" {
				err := windowsCommon.StartService(s.Env().RemoteHost, service)
				s.Require().Error(err, "should fail to start "+service+" in FIPS mode")
				continue
			}

			// try and start it and verify that it does correctly outputs to event log
			err := windowsCommon.StartService(s.Env().RemoteHost, service)
			s.Require().NoError(err, "should start "+service)

			// verify that service returns to stopped state
			s.assertServiceState("Stopped", service, nil)
		}
	}

	// Verify there are not errors in the event log
	entries, err := s.getAgentEventLogErrorsAndWarnings()
	s.Require().NoError(err, "should get errors and warnings from Application event log")
	s.Require().Empty(entries, "should not have errors or warnings from agents in the event log")
}

func run[Env any](t *testing.T, s e2e.Suite[Env], systemProbeConfig string, agentConfig string, securityAgentConfig string) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoFakeIntake(
		awsHostWindows.WithRunOptions(
			scenwindows.WithAgentOptions(
				agentparams.WithAgentConfig(agentConfig),
				agentparams.WithSystemProbeConfig(systemProbeConfig),
				agentparams.WithSecurityAgentConfig(securityAgentConfig),
			),
			scenwindows.WithAgentClientOptions(
				agentclientparams.WithSkipWaitForAgentReady(),
			),
		),
	))}
	e2e.Run(t, s, opts...)
}

type baseStartStopSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	startAgentCommand    func(host *components.RemoteHost) error
	stopAgentCommand     func(host *components.RemoteHost) error
	runningUserServices  func() []string
	runningServices      func() []string
	dumpFolder           string
	enableDriverVerifier bool
	timeoutScale         time.Duration
}

// TestAgentStartsAllServices tests that starting the agent starts all services (as enabled)
func (s *baseStartStopSuite) TestAgentStartsAllServices() {
	s.startAgent()
	s.requireAllServicesState("Running", nil)
}

// TestAgentStopsAllServices tests that stopping the agent stops all services
func (s *baseStartStopSuite) TestAgentStopsAllServices() {
	host := s.Env().RemoteHost
	unexpectedRestartedServices := make(map[string]int)

	// this callback checks whether a service that is suppose to stop unexpectedly restarted.
	onServiceUnexpectedRestart := func(host *components.RemoteHost, serviceName, actual string) {
		if actual == "Running" {
			if _, found := unexpectedRestartedServices[serviceName]; !found {
				// the service is still running or unexpectedly restarted, check again on the next try.
				unexpectedRestartedServices[serviceName] = 0
			} else if unexpectedRestartedServices[serviceName] == 0 {
				// still running, try stop only once.
				unexpectedRestartedServices[serviceName] = 1
				s.T().Errorf(`Service "%s" unexpectedly restarted, explicitly stopping it`, serviceName)
				cmd := fmt.Sprintf(`sc.exe stop "%s"`, serviceName)
				host.Execute(cmd)
			}
		}
	}

	// run the test multiple times to ensure the agent can be started and stopped repeatedly
	N := 10
	if testing.Short() {
		N = 1
	}

	for i := 1; i <= N; i++ {
		s.T().Logf("Test iteration %d/%d", i, N)

		s.startAgent()
		s.requireAllServicesState("Running", nil)

		// stop the agent
		err := s.stopAgentCommand(host)
		s.Require().NoError(err, "should stop the datadogagent service")

		// ensure all services are stopped
		s.requireAllServicesState("Stopped", onServiceUnexpectedRestart)

		// ensure there are no errors in the event log from the agent services
		entries, err := s.getAgentEventLogErrorsAndWarnings()
		s.Require().NoError(err, "should get agent errors and warnings from Application event log")
		s.Require().Empty(entries, "should not have errors or warnings from agents in the event log")
	}

	// check event log for N sets of start and stop messages from each service
	for _, serviceName := range s.runningUserServices() {
		providerName := serviceName
		// skip services that don't register an Application event log provider
		if providerName == "Datadog Installer" || providerName == "dd-procmgr-service" {
			continue
		}
		entries, err := windowsCommon.GetEventLogEntriesFromProvider(host, "Application", providerName)
		s.Require().NoError(err, "should get event log entries from %s", providerName)
		// message IDs from pkg/util/winutil/messagestrings
		startingMessages := windowsCommon.Filter(entries, func(entry windowsCommon.EventLogEntry) bool {
			return entry.ID == 7
		})
		startedMessages := windowsCommon.Filter(entries, func(entry windowsCommon.EventLogEntry) bool {
			return entry.ID == 3
		})
		stoppingMessages := windowsCommon.Filter(entries, func(entry windowsCommon.EventLogEntry) bool {
			return entry.ID == 12
		})
		stoppedMessages := windowsCommon.Filter(entries, func(entry windowsCommon.EventLogEntry) bool {
			return entry.ID == 4
		})
		s.Assert().Len(startingMessages, N, "should have %d starting message in the event log", N)
		s.Assert().Len(startedMessages, N, "should have %d started message in the event log", N)
		s.Assert().Len(stoppingMessages, N, "should have %d stopping message in the event log", N)
		s.Assert().Len(stoppedMessages, N, "should have %d stopped message in the event log", N)
	}
}

func (s *baseStartStopSuite) SetupSuite() {
	// Preserve timeout scales explicitly configured by specialized suites, such as
	// the Driver Verifier suites. The zero value means no scale was configured.
	if s.timeoutScale == 0 {
		s.timeoutScale = defaultTimeoutScale
	}

	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost

	// Disable failure actions (auto restart service) so they don't interfere with the tests
	for _, serviceName := range s.getInstalledServices() {
		cmd := fmt.Sprintf(`sc.exe failure "%s" reset= 0 actions= none`, serviceName)
		_, err := host.Execute(cmd)
		s.Require().NoError(err, "should disable failure actions for %s", serviceName)
	}

	// Enable driver verifier and reboot. Tests will require more generous timeouts.
	if s.enableDriverVerifier {
		// Set Agent to manual start mode so we can control when it starts after the reboot
		cmd := `sc.exe config datadogagent start= demand`
		_, err := host.Execute(cmd)
		s.Require().NoError(err, "should set datadogagent to manual start mode")

		out, err := windowsCommon.EnableDriverVerifier(host, s.getInstalledKernelServices())
		if err != nil {
			s.T().Logf("Driver verifier error output:\n%s", err)
		}
		if out != "" {
			s.T().Logf("Driver verifier output:\n%s", out)
		}

		// Driver Verifier adds system-wide kernel overhead that slows Go runtime and
		// package init, causing user-mode services to exceed the default 30s SCM
		// startup timeout (ServicesPipeTimeout) before reaching StartServiceCtrlDispatcher.
		// Raise the timeout to 120s so security-agent and installer survive the extra load.
		cmd = `Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control' -Name ServicesPipeTimeout -Value 120000 -Type DWORD`
		_, err = host.Execute(cmd)
		s.Require().NoError(err, "should increase SCM ServicesPipeTimeout for driver verifier")

		windowsCommon.RebootAndWait(host, backoff.NewConstantBackOff(10*time.Second))
	}

	// TODO(WINA-1320): mark this crash as flaky while we investigate it
	flake.MarkOnLog(s.T(), "Exception code: 0x40000015")

	// Enable crash dumps
	s.dumpFolder = werCrashDumpFolder
	err := windowsCommon.EnableWERGlobalDumps(host, s.dumpFolder)
	s.Require().NoError(err, "should enable WER dumps")

	// Setup cdb.exe for automated crash dump analysis
	err = windowsCommon.SetupCdb(host)
	if err != nil {
		s.T().Logf("Warning: failed to setup cdb for crash dump analysis: %v", err)
	}
	env := map[string]string{
		"GOTRACEBACK": "wer",
		// Force a crash dump (via WER) on hard stop timeout so we capture goroutine
		// state when a service hangs during shutdown. See servicemain.EnvCrashOnHardStopTimeout.
		"DD_CRASH_ON_HARDSTOP_TIMEOUT": "1",
		// Capture a Go execution trace of each service's startup into the logs folder,
		// which collectAgentLogs() uploads as a CI artifact on failure. See
		// servicemain.EnvStartupTraceDir.
		"DD_STARTUP_TRACE_DIR": `C:\ProgramData\Datadog\logs`,
	}
	for _, svc := range s.getInstalledUserServices() {
		err := windowsCommon.SetServiceEnvironment(host, svc, env)
		s.Require().NoError(err, "should set environment for %s", svc)
	}

	// Setup default expected services
	s.runningUserServices = func() []string {
		services := s.filterLegacySCMServices(s.getInstalledUserServices())
		if s.Env().Agent.FIPSEnabled {
			// TODO: This service is not supported in FIPS mode yet
			services = slices.DeleteFunc(services, func(svc string) bool {
				return svc == "Datadog Installer"
			})
		}
		return services
	}
	s.runningServices = func() []string {
		user := s.runningUserServices()
		kernel := s.getInstalledKernelServices()
		services := append(slices.Clone(user), kernel...)
		if s.Env().Agent.FIPSEnabled {
			// TODO: This service is not supported in FIPS mode yet
			services = slices.DeleteFunc(services, func(svc string) bool {
				return svc == "Datadog Installer"
			})
		}
		return services
	}
}

func (s *baseStartStopSuite) TearDownSuite() {
	s.T().Log("Tearing down environment")
	s.BaseSuite.TearDownSuite()
}

func (s *baseStartStopSuite) BeforeTest(suiteName, testName string) {
	if beforeTest, ok := any(&s.BaseSuite).(suite.BeforeTest); ok {
		beforeTest.BeforeTest(suiteName, testName)
	}

	host := s.Env().RemoteHost

	// Stop all services before each test
	s.stopAllServices()

	// Clear the event logs before each test
	for _, logName := range []string{"System", "Application"} {
		s.T().Logf("Clearing %s event log", logName)
		err := windowsCommon.ClearEventLog(host, logName)
		s.Require().NoError(err, "should clear %s event log", logName)
	}
	// Clear agent logs
	s.T().Logf("Clearing agent logs")
	logsFolder, err := host.GetLogsFolder()
	s.Require().NoError(err, "should get logs folder")
	entries, err := host.ReadDir(logsFolder)
	if s.Assert().NoError(err, "should read log folder") {
		for _, entry := range entries {
			err = host.RemoveAll(filepath.Join(logsFolder, entry.Name()))
			s.Assert().NoError(err, "should remove %s", entry.Name())
		}
	}
	// Clear dump folder
	s.T().Logf("Clearing dump folder")
	err = windowsCommon.CleanDirectory(host, s.dumpFolder)
	s.Require().NoError(err, "should clean dump folder")

	// Start xperf tracing to capture service start/stop timing under Driver Verifier.
	// Two ETW sessions run concurrently (circular buffers, merged on stop):
	//   - NT Kernel Logger: scheduler, loader, CPU profile, context switch with stacks
	//   - scm-trace: Microsoft-Windows-Services SCM events (SetServiceStatus transitions)
	// The merged .etl is only downloaded on test failure. See AfterTest -> collectXperf.
	s.startXperf(host)
}

// xperfSCMSessionName is the user-mode ETW session name that captures Microsoft-Windows-Services
// SCM events (matches the name in the MS-published TSS xperf recipe).
const xperfSCMSessionName = "scm-trace"

// startXperf starts xperf tracing on the remote host with two concurrent sessions
// (NT Kernel Logger + scm-trace user session for the SCM provider). Both use circular
// FileMode so that for tests with multiple start/stop iterations the trace captures
// the tail of activity around whichever iteration fails.
func (s *baseStartStopSuite) startXperf(host *components.RemoteHost) {
	err := host.HostArtifactClient.Get("windows-products/xperf-5.0.8169.zip", "C:/xperf.zip")
	if !s.Assert().NoError(err, "should fetch xperf artifact") {
		return
	}

	// Extract if C:/xperf dir does not exist.
	_, err = host.Execute("if (-Not (Test-Path -Path C:/xperf)) { Expand-Archive -Path C:/xperf.zip -DestinationPath C:/xperf }")
	if !s.Assert().NoError(err, "should expand xperf archive") {
		return
	}

	// Single xperf invocation starts both the NT Kernel Logger (-on <KernelGroups> -f kernel.etl ...)
	// and a named user-mode session (-start scm-trace -on Microsoft-Windows-Services) per the
	// MS TSS xperf SCM-tracing recipe. -d on stop will merge both into a single .etl.
	xperfPath := "C:/xperf/xperf.exe"
	cmd := fmt.Sprintf(
		`& "%s" -on Base+Latency+CSwitch+PROC_THREAD+LOADER+Profile+DISPATCHER -stackWalk CSwitch+Profile+ReadyThread+ThreadCreate -f C:/kernel.etl -MaxBuffers 1024 -BufferSize 1024 -MaxFile 1024 -FileMode Circular -start %s -on Microsoft-Windows-Services`,
		xperfPath, xperfSCMSessionName,
	)
	_, err = host.Execute(cmd)
	s.Assert().NoError(err, "should start xperf tracing (kernel + scm-trace)")
}

// collectXperf stops both xperf sessions, merges them, and downloads the resulting
// .etl to the session output dir if the test failed.
func (s *baseStartStopSuite) collectXperf(host *components.RemoteHost) {
	xperfPath := "C:/xperf/xperf.exe"
	outputPath := "C:/full_host_profiles.etl"

	// Stop kernel logger (-stop) and the named SCM user session (-stop scm-trace), then -d
	// merges both into outputPath. Matches the MS TSS recipe.
	_, err := host.Execute(fmt.Sprintf(`& "%s" -stop -stop %s -d %s`, xperfPath, xperfSCMSessionName, outputPath))
	if !s.Assert().NoError(err, "should stop and merge xperf trace") {
		return
	}

	// Only collect the trace artifact if the test failed. Use a tempfile pattern in the
	// session output dir so multiple failing tests in the same suite don't overwrite each
	// other's traces.
	if s.T().Failed() {
		outDir := s.SessionOutputDir()
		f, err := os.CreateTemp(outDir, "xperf-*.etl")
		if !s.Assert().NoError(err, "should create local xperf trace file") {
			return
		}
		localPath := f.Name()
		_ = f.Close()
		err = host.GetFile(outputPath, localPath)
		s.Assert().NoError(err, "should download xperf trace")
	}
}

func (s *baseStartStopSuite) AfterTest(suiteName, testName string) {
	// Stop xperf and merge to .etl as early as possible after the test body, so the
	// circular trace is preserved before any subsequent diagnostic collection further
	// perturbs system state. .etl is only downloaded if the test failed.
	s.collectXperf(s.Env().RemoteHost)

	// look for and download crashdumps. Dumps from processes in
	// DefaultIgnoredCrashDumpImages are still downloaded as artifacts but do
	// not fail the test.
	dumps, err := windowsCommon.DownloadAllWERDumps(s.Env().RemoteHost, s.dumpFolder, s.SessionOutputDir())
	s.Assert().NoError(err, "should download crash dumps")
	failing, ignored := windowsCommon.PartitionDownloadedWERDumps(dumps, windowsCommon.DefaultIgnoredCrashDumpImages)
	if len(ignored) > 0 {
		s.T().Logf("Ignoring %d crash dumps from known-noisy processes:", len(ignored))
		for _, dump := range ignored {
			s.T().Logf("  %s -> %s", dump.Source.FileName, dump.LocalPath)
		}
	}
	if !s.Assert().Empty(failing, "should not have crash dumps") {
		s.T().Logf("Found unexpected crash dumps:")
		for _, dump := range failing {
			s.T().Logf("  %s -> %s", dump.Source.FileName, dump.LocalPath)
		}
		// Run !analyze -v on each crash dump on the remote VM
		if analyzeErr := windowsCommon.AnalyzeAllWERDumps(s.Env().RemoteHost, s.dumpFolder, s.SessionOutputDir(), s.T()); analyzeErr != nil {
			s.T().Logf("Warning: crash dump analysis errors: %v", analyzeErr)
		}
	}

	if s.T().Failed() {
		// If the test failed, export the event logs for debugging
		host := s.Env().RemoteHost
		for _, logName := range []string{"System", "Application"} {
			// collect the full event log as an evtx file
			s.T().Logf("Exporting %s event log", logName)
			outputPath := filepath.Join(s.SessionOutputDir(), logName+".evtx")
			err := windowsCommon.ExportEventLog(host, logName, outputPath)
			s.Assert().NoError(err, "should export %s event log", logName)
			// Log errors and warnings to the screen for easy access
			out, err := windowsCommon.GetEventLogErrorsAndWarnings(host, logName)
			if s.Assert().NoError(err, "should get errors and warnings from %s event log", logName) && out != "" {
				s.T().Logf("Errors and warnings from %s event log:\n%s", logName, out)
			}
		}
		// collect agent logs
		s.collectAgentLogs()
	}

	// Analyze kernel crash dump on the remote VM before downloading it
	if exists, _ := s.Env().RemoteHost.FileExists(systemCrashDumpFile); exists {
		output, analyzeErr := windowsCommon.AnalyzeKernelDump(s.Env().RemoteHost, systemCrashDumpFile)
		if analyzeErr != nil {
			s.T().Logf("Warning: kernel dump analysis error: %v", analyzeErr)
		} else {
			s.T().Logf("=== Kernel crash dump analysis ===\n%s", output)
			analysisPath := filepath.Join(s.SessionOutputDir(), "kernel-dump-analysis.txt")
			_ = os.WriteFile(analysisPath, []byte(output), 0644)
		}
	}

	// check if the host crashed.
	s.Require().False(s.collectSystemCrashDump(), "should not have system crash dump")

	// Run BaseSuite.AfterTest last: on failure it invokes environment diagnose,
	// which may call require (aborting anything after it) and perturbs system
	// state. Our collection above must complete first.
	s.BaseSuite.AfterTest(suiteName, testName)
}

func (s *baseStartStopSuite) collectAgentLogs() {
	host := s.Env().RemoteHost

	s.T().Logf("Collecting agent logs")
	logsFolder, err := host.GetLogsFolder()
	if !s.Assert().NoError(err, "should get logs folder") {
		return
	}
	entries, err := host.ReadDir(logsFolder)
	if !s.Assert().NoError(err, "should read log folder") {
		return
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(logsFolder, entry.Name())
		destPath := filepath.Join(s.SessionOutputDir(), entry.Name())

		if entry.IsDir() {
			s.T().Logf("Found log directory: %s", entry.Name())
			err = host.GetFolder(sourcePath, destPath)
		} else {
			s.T().Logf("Found log file: %s", entry.Name())
			err = host.GetFile(sourcePath, destPath)
		}
		s.Assert().NoError(err, "should download %s", entry.Name())
	}
}

func (s *baseStartStopSuite) startAgent() {
	host := s.Env().RemoteHost
	err := s.startAgentCommand(host)
	s.Require().NoError(err, "should start the agent")
}

func (s *baseStartStopSuite) requireAllServicesState(expected string, onMismatch onServiceStateMismatch) {
	// ensure all services are running
	s.assertAllServicesState(expected, onMismatch)

	if s.T().Failed() {
		// stop test if not all services are running
		s.FailNowf("not all services are %s", expected)
	}

	// ensure no unexpected services are running
	s.assertNonExpectedServiceState("Stopped")
	if s.T().Failed() {
		// stop test if unexpected services are running
		s.FailNow("unexpected services are running")
	}
}

func (s *baseStartStopSuite) assertNonExpectedServiceState(expected string) {
	expectedServices := s.runningServices()
	for _, serviceName := range s.getInstalledServices() {
		if !slices.Contains(expectedServices, serviceName) {
			s.assertServiceState(expected, serviceName, nil)
		}
	}
}

func (s *baseStartStopSuite) assertAllServicesState(expected string, onMismatch onServiceStateMismatch) {
	for _, serviceName := range s.runningServices() {
		s.assertServiceState(expected, serviceName, onMismatch)
	}
}

func (s *baseStartStopSuite) assertServiceState(expected string, serviceName string, onMismatch onServiceStateMismatch) {
	host := s.Env().RemoteHost
	s.EventuallyWithExponentialBackoff(func() error {
		status, err := windowsCommon.GetServiceStatus(host, serviceName)
		if err != nil {
			return err
		}
		if status != expected {
			if onMismatch != nil {
				onMismatch(host, serviceName, status)
			}

			return fmt.Errorf("%s should be %s, actual %s", serviceName, expected, status)
		}
		return nil
	}, (2*s.timeoutScale)*time.Minute, 60*time.Second, "%s should be in the expected state", serviceName)

	// if a driver service failed to get to the expected state, capture a kernel dump for debugging.
	if s.T().Failed() && slices.Contains(s.getInstalledKernelServices(), serviceName) {
		// the polling may have been affected by noise, check one last time.
		status, err := windowsCommon.GetServiceStatus(host, serviceName)
		if err != nil {
			s.T().Logf("failed to get service status for %s : %s", serviceName, err)
			return
		}

		if expected != status {
			s.T().Logf("capturing live kernel dump, %s service state was %s but expected %s\n",
				serviceName, status, expected)
			s.captureLiveKernelDump(host, s.SessionOutputDir())
			s.logHostDiagnostics()
			return
		}

		s.T().Logf("warning, detected late transition of %s to %s state", serviceName, expected)
	}
}

func (s *baseStartStopSuite) stopAllServices() {
	host := s.Env().RemoteHost

	// stop agent first, it should stop all services
	s.T().Logf("Stopping the agent service...")
	err := s.stopAgentCommand(host)
	s.Require().NoError(err, "should stop the agent")
	s.T().Logf("Agent service stopped")

	// ensure all services are stopped
	for _, serviceName := range s.getInstalledServices() {
		s.EventuallyWithExponentialBackoff(func() error {
			status, err := windowsCommon.GetServiceStatus(host, serviceName)
			if err != nil {
				return err
			}
			if status != "Stopped" {
				return windowsCommon.StopService(host, serviceName)
			}
			return nil
		}, (2*s.timeoutScale)*time.Minute, 60*time.Second, "%s should be in the expected state", serviceName)
	}

	// capture a live dump to help identify why one or more services are still running.
	if s.T().Failed() {
		s.T().Logf("capturing live kernel dump, one or more services failed to stop")
		s.captureLiveKernelDump(host, s.SessionOutputDir())
		s.logHostDiagnostics()
	}
}

// legacySCMServices are SCM shells superseded by dd-procmgr; they stay Stopped while
// dd-procmgr-service supervises the workload.
func (s *baseStartStopSuite) legacySCMServices() []string {
	return []string{
		"datadog-process-agent",
	}
}

func (s *baseStartStopSuite) filterLegacySCMServices(services []string) []string {
	return slices.DeleteFunc(slices.Clone(services), func(svc string) bool {
		return slices.Contains(s.legacySCMServices(), svc)
	})
}

func (s *baseStartStopSuite) getInstalledUserServices() []string {
	return []string{
		"datadogagent",
		"datadog-trace-agent",
		"datadog-process-agent",
		"dd-procmgr-service",
		"datadog-security-agent",
		"datadog-system-probe",
		"Datadog Installer",
	}
}

func (s *baseStartStopSuite) getInstalledKernelServices() []string {
	return []string{
		"ddnpm",
		"ddprocmon",
	}
}

// expectedInstalledServices returns the list of services that should be installed by the agent
func (s *baseStartStopSuite) getInstalledServices() []string {
	user := s.getInstalledUserServices()
	kernel := s.getInstalledKernelServices()
	return append(user, kernel...)
}

// getAgentEventLogErrorsAndWarnings returns the errors and warnings from the agent services in the Application event log
func (s *baseStartStopSuite) getAgentEventLogErrorsAndWarnings() ([]windowsCommon.EventLogEntry, error) {
	host := s.Env().RemoteHost
	providerNames := s.getInstalledUserServices()
	// remove services that do not register an Application event log provider
	providerNames = slices.DeleteFunc(providerNames, func(s string) bool {
		return s == "Datadog Installer" || s == "dd-procmgr-service"
	})
	providerNamesFilter := fmt.Sprintf(`"%s"`, strings.Join(providerNames, `","`))
	filter := fmt.Sprintf(`@{ LogName='Application'; ProviderName=%s; Level=1,2,3 }`, providerNamesFilter)
	return windowsCommon.GetEventLogEntriesWithFilterHashTable(host, filter)
}

// captureLiveKernelDump sends a command to the host to create a live kernel dump and downloads it.
func (s *baseStartStopSuite) captureLiveKernelDump(host *components.RemoteHost, dumpDir string) {
	tempDumpDir := `C:\Windows\Temp`
	sourceDumpDir := filepath.Join(tempDumpDir, `localhost`)

	// The live kernel dump will be placed under subdirectory named "localhost."
	// Make sure the subdirectory where the dump will be generated is empty.
	if exists, _ := host.FileExists(sourceDumpDir); exists {
		err := host.RemoveAll(sourceDumpDir)
		if err != nil {
			s.T().Logf("failed to cleanup %s: %s\n", sourceDumpDir, err)
			return
		}
	}

	// This Powershell command is originally tailored for storage cluster environments.
	getSubsystemCmd := `$ss = Get-CimInstance -ClassName MSFT_StorageSubSystem -Namespace Root\Microsoft\Windows\Storage`
	createLiveDumpCmd := fmt.Sprintf(`Invoke-CimMethod -InputObject $ss -MethodName "GetDiagnosticInfo" -Arguments @{DestinationPath="%s"; IncludeLiveDump=$true}`, tempDumpDir)
	dumpCmd := fmt.Sprintf("%s;%s", getSubsystemCmd, createLiveDumpCmd)

	s.T().Logf("creating live kernel dump under %s\n", tempDumpDir)
	out, err := host.Execute(dumpCmd)
	out = strings.TrimSpace(out)
	if out != "" {
		s.T().Logf("PowerShell live kernel dump output:\n%s", out)
	}

	if err != nil {
		s.T().Logf("remote execute error: %s\n", err)
		return
	}

	// Check if the dump is present.
	sourceDumpFile := filepath.Join(sourceDumpDir, `LiveDump.dmp`)
	if exists, _ := host.FileExists(sourceDumpFile); !exists {
		s.T().Logf("live kernel dump not found at %s: %s\n", sourceDumpFile, err)
		return
	}

	// Download the dump file.
	destDumpFile := filepath.Join(dumpDir, `LiveDump.dmp`)
	err = host.GetFile(sourceDumpFile, destDumpFile)
	if err != nil {
		s.T().Logf("failed to download live kernel dump to %s: %s\n", destDumpFile, err)
	} else {
		s.T().Logf("live kernel dump downloaded to %s\n", destDumpFile)
	}

	// Cleanup the "localhost" subdirectory.
	host.RemoveAll(sourceDumpDir)
}

func (s *baseStartStopSuite) collectSystemCrashDump() bool {
	// Look for a system crash dump. These may be triggered by Driver Verifier.
	// Stop the test immediately if one is found.

	s.T().Log("Checking for system crash dump")
	systemCrashDumpOutPath := filepath.Join(s.SessionOutputDir(), systemCrashDumpOutFileName)

	// Check if a system crash dump was already downloaded.
	if _, err := os.Stat(systemCrashDumpOutPath); err != nil {
		if !os.IsNotExist(err) {
			s.T().Logf("Found existing system crash dump %s", systemCrashDumpOutPath)
			return true
		}
	}

	systemDump, err := windowsCommon.DownloadSystemCrashDump(
		s.Env().RemoteHost, systemCrashDumpFile, systemCrashDumpOutPath)
	s.Assert().NoError(err, "should download system crash dump")

	return systemDump != ""
}

// logHostDiagnostics captures diagnostics from the remote host to help troubleshoot timeouts.
func (s *baseStartStopSuite) logHostDiagnostics() {
	host := s.Env().RemoteHost

	s.T().Logf("Querying I/O diagnostics")

	out, err := queryProcessesWithActiveIo(host)
	if err == nil {
		s.T().Logf("Processes with active I/O:\n%s\n", out)
	}

	out, err = queryDiskQueueLength(host)
	if err == nil {
		s.T().Logf("Sampled disk queue length:\n%s\n", out)
	}

	out, err = queryAllHandleCounts(host)
	if err == nil {
		s.T().Logf("Handle count for all processes:\n%s\n", out)
	}
}

// Driver verifier tests start

type dvAgentServiceCommandSuite struct {
	agentServiceCommandSuite
}
type dvPowerShellServiceCommandSuite struct {
	powerShellServiceCommandSuite
}
type dvAgentServiceDisabledSystemProbeSuite struct {
	agentServiceDisabledSystemProbeSuite
}
type dvAgentServiceDisabledProcessAgentSuite struct {
	agentServiceDisabledProcessAgentSuite
}
type dvAgentServiceDisabledTraceAgentSuite struct {
	agentServiceDisabledTraceAgentSuite
}
type dvAgentServiceDisabledInstallerSuite struct {
	agentServiceDisabledInstallerSuite
}

// TestDriverVerifierOnServiceBehaviorAgentCommand tests the same as TestServiceBehaviorAgentCommand
// with driver verifier enabled.
func TestDriverVerifierOnServiceBehaviorAgentCommand(t *testing.T) {
	s := &dvAgentServiceCommandSuite{}
	s.enableDriverVerifier = true
	s.timeoutScale = driverVerifierTimeoutScale
	run(t, s, systemProbeConfig, agentConfig, securityAgentConfig)
}

// TestDriverVerifierOnServiceBehaviorPowerShell tests the the same as TestServiceBehaviorPowerShell
// with driver verifier enabled.
func TestDriverVerifierOnServiceBehaviorPowerShell(t *testing.T) {
	s := &dvPowerShellServiceCommandSuite{}
	s.enableDriverVerifier = true
	s.timeoutScale = driverVerifierTimeoutScale
	run(t, s, systemProbeConfig, agentConfig, securityAgentConfig)
}

// TestDriverVerifierOnServiceBehaviorWhenDisabledSystemProbe tests the same as TestServiceBehaviorWhenDisabledSystemProbe
// with driver verifier enabled.
func TestDriverVerifierOnServiceBehaviorWhenDisabledSystemProbe(t *testing.T) {
	s := &dvAgentServiceDisabledSystemProbeSuite{}
	s.disabledServices = []string{
		"datadog-security-agent",
		"datadog-system-probe",
		"ddnpm",
		"ddprocmon",
	}
	s.enableDriverVerifier = true
	s.timeoutScale = driverVerifierTimeoutScale
	run(t, s, systemProbeDisabled, agentConfig, securityAgentConfigDisabled)
}

// TestDriverVerifierOnServiceBehaviorWhenDisabledProcessAgent tests the same as TestServiceBehaviorWhenDisabledProcessAgent
// with driver verifier enabled.
func TestDriverVerifierOnServiceBehaviorWhenDisabledProcessAgent(t *testing.T) {
	s := &dvAgentServiceDisabledProcessAgentSuite{}
	s.disabledServices = []string{
		"datadog-process-agent",
		"datadog-security-agent",
		"datadog-system-probe",
		"ddnpm",
		"ddprocmon",
	}
	s.enableDriverVerifier = true
	s.timeoutScale = driverVerifierTimeoutScale
	run(t, s, systemProbeDisabled, agentConfigPADisabled, securityAgentConfigDisabled)
}

// TestDriverVerifierOnServiceBehaviorWhenDisabledTraceAgent tests the same as TestServiceBehaviorWhenDisabledTraceAgent
// with driver verifier enabled.
func TestDriverVerifierOnServiceBehaviorWhenDisabledTraceAgent(t *testing.T) {
	s := &dvAgentServiceDisabledTraceAgentSuite{}
	s.disabledServices = []string{
		"datadog-trace-agent",
	}
	s.enableDriverVerifier = true
	s.timeoutScale = driverVerifierTimeoutScale
	run(t, s, systemProbeConfig, agentConfigTADisabled, securityAgentConfig)
}

// TestDriverVerifierOnServiceBehaviorWhenDisabledInstaller tests the same as TestServiceBehaviorWhenDisabledInstaller
// with driver verifier enabled.
func TestDriverVerifierOnServiceBehaviorWhenDisabledInstaller(t *testing.T) {
	s := &dvAgentServiceDisabledInstallerSuite{}
	s.disabledServices = []string{
		"Datadog Installer",
	}
	s.enableDriverVerifier = true
	s.timeoutScale = driverVerifierTimeoutScale
	run(t, s, systemProbeConfig, agentConfigDIDisabled, securityAgentConfig)
}

// Driver verifier tests end
