// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package windows contains the code to run the e2e tests on Windows
package windows

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

//go:embed fixtures/datadog.yaml
var agentConfig string

//go:embed fixtures/system-probe.yaml
var systemProbeConfig string

//go:embed fixtures/security-agent.yaml
var securityAgentConfig string

func TestServiceBehavior(t *testing.T) {
	s := &startStopTestSuite{}
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

type startStopTestSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

// TestStopTimeout tests that each service stops without hitting its hard stop timeout, which
// results in a message in the Application event log.
func (s *startStopTestSuite) TestStopTimeout() {
	host := s.Env().RemoteHost

	// ensure all services are running
	s.startAgent()
	s.requireAllServicesRunning()

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
	s.assertAllServicesStopped()

	// check the System event log for unexpected exit messages
	// hard stop timeout should set SERVICE_STOPPED before exiting, so
	// we should not see "terminated unexpectedly" messages in the event log
	out, err := windowsCommon.GetEventLogErrorsAndWarnings(host, "System")
	s.Require().NoError(err, "should get errors and warnings from System event log")
	s.T().Logf("Errors and warnings from System event log:\n%s", out)
	s.Assert().NotContains(out, "terminated unexpectedly", "should not have unexpected exit messages in the event log")
	// check the Application event log for timeout messages
	out, err = windowsCommon.GetEventLogErrorsAndWarnings(host, "Application")
	s.Require().NoError(err, "should get errors and warnings from Application event log")
	s.T().Logf("Errors and warnings from Application event log:\n%s", out)
	s.Assert().NotContains(out, "hard stopping service", "should not have timeout messages in the event log")
}

// TestAgentStartsAllServices tests that starting the agent starts all services
func (s *startStopTestSuite) TestAgentStartsAllServices() {
	s.startAgent()
	s.requireAllServicesRunning()
}

// TestAgentStopsAllServices tests that stopping the agent stops all services
func (s *startStopTestSuite) TestAgentStopsAllServices() {
	host := s.Env().RemoteHost
	s.startAgent()
	s.requireAllServicesRunning()

	// stop the agent
	err := windowsCommon.StopService(host, "datadogagent")
	s.Require().NoError(err, "should stop the datadogagent service")

	// ensure all services are stopped
	s.assertAllServicesStopped()
}

// TestHardExitEventLogEntry tests that the System event log contains an "unexpectedly terminated" message when a service is killed
func (s *startStopTestSuite) TestHardExitEventLogEntry() {
	host := s.Env().RemoteHost
	s.startAgent()
	s.requireAllServicesRunning()

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
		out, err := windowsCommon.GetEventLogErrorsAndWarnings(host, "System")
		require.NoError(c, err, "should get errors and warnings from System event log")
		s.T().Logf("Errors and warnings from System event log:\n%s", out)
		for _, displayName := range displayNames {
			match := fmt.Sprintf("The %s service terminated unexpectedly", displayName)
			assert.Contains(c, out, match, "should have hard exit messages in the event log")
		}
	}, 1*time.Minute, 1*time.Second, "should have hard exit messages in the event log")
}

func (s *startStopTestSuite) SetupSuite() {
	if setupSuite, ok := any(&s.BaseSuite).(suite.SetupAllSuite); ok {
		setupSuite.SetupSuite()
	}

	// Disable failure actions (auto restart service) so they don't interfere with the tests
	host := s.Env().RemoteHost
	for _, serviceName := range s.expectedInstalledServices() {
		cmd := fmt.Sprintf(`sc.exe failure "%s" reset= 0 actions= none`, serviceName)
		_, err := host.Execute(cmd)
		s.Require().NoError(err, "should disable failure actions for %s", serviceName)
	}
}

func (s *startStopTestSuite) BeforeTest(suiteName, testName string) {
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
}

func (s *startStopTestSuite) AfterTest(suiteName, testName string) {
	if afterTest, ok := any(&s.BaseSuite).(suite.AfterTest); ok {
		afterTest.AfterTest(suiteName, testName)
	}

	if s.T().Failed() {
		// If the test failed, export the event logs for debugging
		outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), s.T())
		if err != nil {
			s.T().Fatalf("should get output dir")
		}
		s.T().Logf("Output dir: %s", outputDir)
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

func (s *startStopTestSuite) collectAgentLogs() {
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

func (s *startStopTestSuite) startAgent() {
	host := s.Env().RemoteHost
	err := windowsCommon.StartService(host, "datadogagent")
	s.Require().NoError(err, "should start the datadogagent service")
}

func (s *startStopTestSuite) requireAllServicesRunning() {
	// ensure all services are running
	s.allServicesRunning()

	if s.T().Failed() {
		// stop test if not all services are running
		s.FailNow("not all services are running")
	}
}

func (s *startStopTestSuite) allServicesRunning() {
	host := s.Env().RemoteHost

	for _, serviceName := range s.expectedInstalledServices() {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			status, err := windowsCommon.GetServiceStatus(host, serviceName)
			require.NoError(c, err)
			if !assert.Equal(c, "Running", status, "%s should be running", serviceName) {
				s.T().Logf("waiting for %s to start, status %s", serviceName, status)
			}
		}, 1*time.Minute, 1*time.Second, "%s should be in the expected state", serviceName)
	}
}

func (s *startStopTestSuite) assertAllServicesStopped() {
	host := s.Env().RemoteHost

	for _, serviceName := range s.expectedInstalledServices() {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			status, err := windowsCommon.GetServiceStatus(host, serviceName)
			require.NoError(c, err)
			if !assert.Equal(c, "Stopped", status, "%s should be stopped", serviceName) {
				s.T().Logf("waiting for %s to stop, status %s", serviceName, status)
			}
		}, 1*time.Minute, 1*time.Second, "%s should be in the expected state", serviceName)
	}
}

func (s *startStopTestSuite) stopAllServices() {
	host := s.Env().RemoteHost

	// stop agent first, it should stop all services
	s.T().Logf("Stopping the agent service...")
	err := windowsCommon.StopService(host, "datadogagent")
	s.Require().NoError(err, "should stop the datadogagent service")
	s.T().Logf("Agent service stopped")

	// ensure all services are stopped
	for _, serviceName := range s.expectedInstalledServices() {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			status, err := windowsCommon.GetServiceStatus(host, serviceName)
			require.NoError(c, err)
			if !assert.Equal(c, "Stopped", status, "%s should be stopped", serviceName) {
				s.T().Logf("%s still running, sending stop cmd", serviceName)
				err := windowsCommon.StopService(host, serviceName)
				assert.NoError(c, err, "should stop %s", serviceName)
			}
		}, 1*time.Minute, 1*time.Second, "%s should be in the expected state", serviceName)
	}
}

// expectedUserServices returns the list of user-mode services
func (s *startStopTestSuite) expectedUserServices() []string {
	return []string{
		"datadogagent",
		"datadog-trace-agent",
		"datadog-process-agent",
		"datadog-security-agent",
		"datadog-system-probe",
	}
}

// expectedInstalledServices returns the list of services that should be installed by the agent
func (s *startStopTestSuite) expectedInstalledServices() []string {
	user := s.expectedUserServices()
	kernel := []string{
		"ddnpm",
		"ddprocmon",
	}
	return append(user, kernel...)
}
