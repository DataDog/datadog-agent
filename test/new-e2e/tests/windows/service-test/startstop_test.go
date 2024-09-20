// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package servicetest contains tests for Windows Agent service behavior
package servicetest

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

//go:embed fixtures/datadog.yaml
var agentConfig string

//go:embed fixtures/system-probe.yaml
var systemProbeConfig string

//go:embed fixtures/system-probe-nofim.yaml
var systemProbeNoFIMConfig string

//go:embed fixtures/security-agent.yaml
var securityAgentConfig string

// TestServiceBehaviorAgentCommandNoFIM tests the service behavior when controlled by Agent commands
func TestNoFIMServiceBehaviorAgentCommand(t *testing.T) {
	s := &agentServiceCommandSuite{}
	run(t, s, systemProbeNoFIMConfig)
}

// TestServiceBehaviorPowerShellNoFIM tests the service behavior when controlled by PowerShell commands
func TestNoFIMServiceBehaviorPowerShell(t *testing.T) {
	s := &powerShellServiceCommandSuite{}
	run(t, s, systemProbeNoFIMConfig)
}

// TestServiceBehaviorAgentCommand tests the service behavior when controlled by Agent commands
func TestServiceBehaviorAgentCommand(t *testing.T) {
	s := &agentServiceCommandSuite{}
	run(t, s, systemProbeConfig)
}

type agentServiceCommandSuite struct {
	baseStartStopSuite
}

func (s *agentServiceCommandSuite) SetupSuite() {
	s.baseStartStopSuite.SetupSuite()

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
	run(t, s, systemProbeConfig)
}

type powerShellServiceCommandSuite struct {
	baseStartStopSuite
}

func (s *powerShellServiceCommandSuite) SetupSuite() {
	s.baseStartStopSuite.SetupSuite()

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
	s.requireAllServicesState("Running")

	services := []string{
		// stop dependent services first since stopping them won't affect other services
		"datadog-trace-agent",
		"datadog-process-agent",
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
	s.assertAllServicesState("Stopped")

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
	s.requireAllServicesState("Running")

	// kill the agent
	for _, serviceName := range s.expectedUserServices() {
		// get pid
		pid, err := windowsCommon.GetServicePID(host, serviceName)
		s.Require().NoError(err, "should get the PID for %s", serviceName)
		// kill the process
		_, err = host.Execute(fmt.Sprintf("Stop-Process -Force -Id %d", pid))
		s.Require().NoError(err, "should kill the process with PID %d", pid)
		// service should stop
		status, err := windowsCommon.GetServiceStatus(host, serviceName)
		s.Require().NoError(err, "should get the status for %s", serviceName)
		s.Require().Equal("Stopped", status, "%s should be stopped", serviceName)
	}

	// collect display names for services
	displayNames := make([]string, 0, len(s.expectedUserServices()))
	for _, serviceName := range s.expectedUserServices() {
		conf, err := windowsCommon.GetServiceConfig(host, serviceName)
		s.Require().NoError(err, "should get the configuration for %s", serviceName)
		displayNames = append(displayNames, conf.DisplayName)
	}

	// check the System event log for hard exit messages
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		entries, err := windowsCommon.GetEventLogErrorAndWarningEntries(host, "System")
		if !assert.NoError(c, err, "should get errors and warnings from System event log") {
			return
		}
		for _, displayName := range displayNames {
			match := fmt.Sprintf("The %s service terminated unexpectedly", displayName)
			matching := windowsCommon.Filter(entries, func(entry windowsCommon.EventLogEntry) bool {
				return strings.Contains(entry.Message, match)
			})
			assert.Len(c, matching, 1, "should have hard exit message for %s in the event log", displayName)
		}
	}, 1*time.Minute, 1*time.Second, "should have hard exit messages in the event log")
}

func run[Env any](t *testing.T, s e2e.Suite[Env], systemProbeConfig string) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoFakeIntake(
		awsHostWindows.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithSystemProbeConfig(systemProbeConfig),
			agentparams.WithSecurityAgentConfig(securityAgentConfig),
		),
		awsHostWindows.WithAgentClientOptions(
			agentclientparams.WithSkipWaitForAgentReady(),
		),
	))}
	e2e.Run(t, s, opts...)
}

type baseStartStopSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	startAgentCommand func(host *components.RemoteHost) error
	stopAgentCommand  func(host *components.RemoteHost) error
	dumpFolder        string
}

// TestAgentStartsAllServices tests that starting the agent starts all services (as enabled)
func (s *baseStartStopSuite) TestAgentStartsAllServices() {
	s.startAgent()
	s.requireAllServicesState("Running")
}

// TestAgentStopsAllServices tests that stopping the agent stops all services
func (s *baseStartStopSuite) TestAgentStopsAllServices() {
	host := s.Env().RemoteHost

	// run the test multiple times to ensure the agent can be started and stopped repeatedly
	N := 10
	if testing.Short() {
		N = 1
	}

	for i := 1; i <= N; i++ {
		s.T().Logf("Test iteration %d/%d", i, N)

		s.startAgent()
		s.requireAllServicesState("Running")

		// stop the agent
		err := s.stopAgentCommand(host)
		s.Require().NoError(err, "should stop the datadogagent service")

		// ensure all services are stopped
		s.requireAllServicesState("Stopped")

		// ensure there are no errors in the event log from the agent services
		entries, err := s.getAgentEventLogErrorsAndWarnings()
		s.Require().NoError(err, "should get agent errors and warnings from Application event log")
		s.Require().Empty(entries, "should not have errors or warnings from agents in the event log")
	}

	// check event log for N sets of start and stop messages from each service
	for _, serviceName := range s.expectedUserServices() {
		providerName := serviceName
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
	s.BaseSuite.SetupSuite()

	// Enable crash dumps
	s.dumpFolder = `C:\dumps`
	err := windowsCommon.EnableWERGlobalDumps(s.Env().RemoteHost, s.dumpFolder)
	s.Require().NoError(err, "should enable WER dumps")
	env := map[string]string{
		"GOTRACEBACK": "wer",
	}
	for _, svc := range s.expectedUserServices() {
		err := windowsCommon.SetServiceEnvironment(s.Env().RemoteHost, svc, env)
		s.Require().NoError(err, "should set environment for %s", svc)
	}

	// Disable failure actions (auto restart service) so they don't interfere with the tests
	host := s.Env().RemoteHost
	for _, serviceName := range s.expectedInstalledServices() {
		cmd := fmt.Sprintf(`sc.exe failure "%s" reset= 0 actions= none`, serviceName)
		_, err := host.Execute(cmd)
		s.Require().NoError(err, "should disable failure actions for %s", serviceName)
	}
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
			err = host.Remove(filepath.Join(logsFolder, entry.Name()))
			s.Assert().NoError(err, "should remove %s", entry.Name())
		}
	}
	// Clear dump folder
	s.T().Logf("Clearing dump folder")
	err = windowsCommon.CleanDirectory(host, s.dumpFolder)
	s.Require().NoError(err, "should clean dump folder")
}

func (s *baseStartStopSuite) AfterTest(suiteName, testName string) {
	s.BaseSuite.AfterTest(suiteName, testName)

	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), s.T())
	if err != nil {
		s.T().Fatalf("should get output dir")
	}
	s.T().Logf("Output dir: %s", outputDir)

	// look for and download crashdumps
	dumps, err := windowsCommon.DownloadAllWERDumps(s.Env().RemoteHost, s.dumpFolder, outputDir)
	s.Assert().NoError(err, "should download crash dumps")
	if !s.Assert().Empty(dumps, "should not have crash dumps") {
		s.T().Logf("Found crash dumps:")
		for _, dump := range dumps {
			s.T().Logf("  %s", dump)
		}
	}

	if s.T().Failed() {
		// If the test failed, export the event logs for debugging
		host := s.Env().RemoteHost
		for _, logName := range []string{"System", "Application"} {
			// collect the full event log as an evtx file
			s.T().Logf("Exporting %s event log", logName)
			outputPath := filepath.Join(outputDir, fmt.Sprintf("%s.evtx", logName))
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
}

func (s *baseStartStopSuite) collectAgentLogs() {
	host := s.Env().RemoteHost
	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), s.T())
	if err != nil {
		s.T().Fatalf("should get output dir")
	}

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
		s.T().Logf("Found log file: %s", entry.Name())
		err = host.GetFile(
			filepath.Join(logsFolder, entry.Name()),
			filepath.Join(outputDir, entry.Name()),
		)
		s.Assert().NoError(err, "should download %s", entry.Name())
	}
}

func (s *baseStartStopSuite) startAgent() {
	host := s.Env().RemoteHost
	err := s.startAgentCommand(host)
	s.Require().NoError(err, "should start the agent")
}

func (s *baseStartStopSuite) requireAllServicesState(expected string) {
	// ensure all services are running
	s.assertAllServicesState(expected)

	if s.T().Failed() {
		// stop test if not all services are running
		s.FailNowf("not all services are %s", expected)
	}
}

func (s *baseStartStopSuite) assertAllServicesState(expected string) {
	host := s.Env().RemoteHost

	for _, serviceName := range s.expectedInstalledServices() {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			status, err := windowsCommon.GetServiceStatus(host, serviceName)
			if !assert.NoError(c, err) {
				return
			}
			if !assert.Equal(c, expected, status, "%s should be %s", serviceName, expected) {
				s.T().Logf("waiting for %s to be %s, status %s", serviceName, expected, status)
			}
		}, 1*time.Minute, 1*time.Second, "%s should be in the expected state", serviceName)
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
	for _, serviceName := range s.expectedInstalledServices() {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			status, err := windowsCommon.GetServiceStatus(host, serviceName)
			if !assert.NoError(c, err) {
				return
			}
			if !assert.Equal(c, "Stopped", status, "%s should be stopped", serviceName) {
				s.T().Logf("%s still running, sending stop cmd", serviceName)
				err := windowsCommon.StopService(host, serviceName)
				assert.NoError(c, err, "should stop %s", serviceName)
			}
		}, 1*time.Minute, 1*time.Second, "%s should be in the expected state", serviceName)
	}
}

// expectedUserServices returns the list of user-mode services
func (s *baseStartStopSuite) expectedUserServices() []string {
	return []string{
		"datadogagent",
		"datadog-trace-agent",
		"datadog-process-agent",
		"datadog-security-agent",
		"datadog-system-probe",
	}
}

// expectedInstalledServices returns the list of services that should be installed by the agent
func (s *baseStartStopSuite) expectedInstalledServices() []string {
	user := s.expectedUserServices()
	kernel := []string{
		"ddnpm",
		"ddprocmon",
	}
	return append(user, kernel...)
}

// getAgentEventLogErrorsAndWarnings returns the errors and warnings from the agent services in the Application event log
func (s *baseStartStopSuite) getAgentEventLogErrorsAndWarnings() ([]windowsCommon.EventLogEntry, error) {
	host := s.Env().RemoteHost
	providerNames := s.expectedUserServices()
	providerNamesFilter := fmt.Sprintf(`"%s"`, strings.Join(providerNames, `","`))
	filter := fmt.Sprintf(`@{ LogName='Application'; ProviderName=%s; Level=1,2,3 }`, providerNamesFilter)
	return windowsCommon.GetEventLogEntriesWithFilterHashTable(host, filter)
}
