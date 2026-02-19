// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package servicetest contains tests for Windows Agent service behavior
package servicetest

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenwindows "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"
)

const (
	// PARServiceName is the Windows service name for the private action runner
	PARServiceName = "datadog-agent-action"

	// PARBinaryName is the name of the private action runner binary on Windows
	PARBinaryName = "privateactionrunner.exe"
)

// parIntegrationSuite tests the integration of the Private Action Runner with
// the core Datadog Agent on Windows.
type parIntegrationSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

// TestPARIntegrationEnabled runs the PAR integration tests with PAR enabled.
func TestPARIntegrationEnabled(t *testing.T) {
	s := &parIntegrationSuite{}
	e2e.Run(t, s, e2e.WithProvisioner(awsHostWindows.ProvisionerNoFakeIntake(
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
	)))
}

// TestPARIntegrationDisabled runs the PAR integration tests with PAR disabled.
func TestPARIntegrationDisabled(t *testing.T) {
	s := &parIntegrationDisabledSuite{}
	e2e.Run(t, s, e2e.WithProvisioner(awsHostWindows.ProvisionerNoFakeIntake(
		awsHostWindows.WithRunOptions(
			scenwindows.WithAgentOptions(
				agentparams.WithAgentConfig(agentConfigPARDisabled),
				agentparams.WithSystemProbeConfig(systemProbeConfig),
				agentparams.WithSecurityAgentConfig(securityAgentConfig),
			),
			scenwindows.WithAgentClientOptions(
				agentclientparams.WithSkipWaitForAgentReady(),
			),
		),
	)))
}

// TestPARServiceIsInstalled verifies that the PAR Windows service is registered
// with the correct configuration (service type, start type, and SCM dependency on
// the core agent).
func (s *parIntegrationSuite) TestPARServiceIsInstalled() {
	host := s.Env().RemoteHost

	// Verify the service exists by querying its configuration
	conf, err := windowsCommon.GetServiceConfig(host, PARServiceName)
	s.Require().NoError(err, "should get service config for %s", PARServiceName)

	s.Assert().Equal(PARServiceName, conf.ServiceName,
		"service name should be %s", PARServiceName)
	s.Assert().Equal(windowsCommon.SERVICE_WIN32_OWN_PROCESS, conf.ServiceType,
		"service type should be SERVICE_WIN32_OWN_PROCESS")
	s.Assert().Equal(windowsCommon.SERVICE_DEMAND_START, conf.StartType,
		"start type should be SERVICE_DEMAND_START")
	s.Assert().Contains(conf.ServicesDependedOn, "datadogagent",
		"PAR service should depend on datadogagent")
}

// TestPARBinaryInstalled verifies that the PAR binary is present in the agent
// installation directory.
func (s *parIntegrationSuite) TestPARBinaryInstalled() {
	host := s.Env().RemoteHost

	installPath, err := windowsAgent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err, "should get install path from registry")

	binaryPath := filepath.Join(installPath, "bin", "agent", PARBinaryName)
	exists, err := host.FileExists(binaryPath)
	s.Require().NoError(err, "should check binary existence at %s", binaryPath)
	s.Assert().True(exists, "PAR binary should exist at %s", binaryPath)
}

// TestPARStartsWithCoreAgent verifies that when the core agent starts with
// private_action_runner.enabled = true, the PAR service is also started.
func (s *parIntegrationSuite) TestPARStartsWithCoreAgent() {
	host := s.Env().RemoteHost

	// Ensure everything is stopped first
	_ = windowsCommon.StopService(host, "datadogagent")
	s.eventuallyServiceStopped(PARServiceName)

	// Start the core agent
	err := windowsCommon.StartService(host, "datadogagent")
	s.Require().NoError(err, "should start datadogagent")

	// PAR should be started by the core agent
	s.eventuallyServiceRunning(PARServiceName)
}

// TestPARStopsWithCoreAgent verifies that stopping the core agent also stops
// the PAR service.
func (s *parIntegrationSuite) TestPARStopsWithCoreAgent() {
	host := s.Env().RemoteHost

	// Start the core agent (PAR should come up too)
	err := windowsCommon.StartService(host, "datadogagent")
	s.Require().NoError(err, "should start datadogagent")
	s.eventuallyServiceRunning(PARServiceName)

	// Stop the core agent
	err = windowsCommon.StopService(host, "datadogagent")
	s.Require().NoError(err, "should stop datadogagent")

	// PAR should also stop
	s.eventuallyServiceStopped(PARServiceName)
}

// TestPARStopsCleanlyWithinTimeout verifies that the PAR service shuts down
// promptly (within 15 s) when requested, avoiding Windows hard-stop events.
func (s *parIntegrationSuite) TestPARStopsCleanlyWithinTimeout() {
	host := s.Env().RemoteHost

	// Ensure the agent (and PAR) is running
	err := windowsCommon.StartService(host, "datadogagent")
	s.Require().NoError(err, "should start datadogagent")
	s.eventuallyServiceRunning(PARServiceName)

	// Stop PAR on its own and measure time
	timeTaken, out, err := windowsCommon.MeasureCommand(host, fmt.Sprintf("Stop-Service -Force -Name '%s'", PARServiceName))
	s.Require().NoError(err, "should stop %s", PARServiceName)
	if strings.TrimSpace(out) != "" {
		s.T().Logf("Stop-Service output for %s:\n%s", PARServiceName, out)
	}
	s.T().Logf("Time taken to stop %s: %v ms", PARServiceName, timeTaken.Milliseconds())
	s.Assert().Less(timeTaken, 15*time.Second,
		"PAR service should stop within 15 seconds, actual %v", timeTaken)
}

// TestPARServiceLogPath verifies that the PAR writes its log file to the
// expected Datadog logs directory.
func (s *parIntegrationSuite) TestPARServiceLogPath() {
	host := s.Env().RemoteHost

	// Ensure PAR is running so it can write logs
	err := windowsCommon.StartService(host, "datadogagent")
	s.Require().NoError(err, "should start datadogagent")
	s.eventuallyServiceRunning(PARServiceName)

	logsFolder, err := host.GetLogsFolder()
	s.Require().NoError(err, "should get logs folder")

	// Give the service a moment to flush its log
	s.EventuallyWithExponentialBackoff(func() error {
		logPath := filepath.Join(logsFolder, "private_action_runner.log")
		exists, err := host.FileExists(logPath)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("log file not found at %s", logPath)
		}
		return nil
	}, 30*time.Second, 5*time.Second, "PAR log file should exist")
}

// ---- disabled-PAR test suite ----

type parIntegrationDisabledSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

// TestPARServiceInstalledWhenDisabled verifies that the PAR service is still
// *registered* in the Windows SCM even when the feature flag is off — it just
// should not be *running*.
func (s *parIntegrationDisabledSuite) TestPARServiceInstalledWhenDisabled() {
	host := s.Env().RemoteHost

	conf, err := windowsCommon.GetServiceConfig(host, PARServiceName)
	s.Require().NoError(err, "PAR service should be registered even when disabled")
	s.Assert().Equal(PARServiceName, conf.ServiceName)
}

// TestPARDoesNotStartWhenDisabled verifies that when private_action_runner.enabled
// is false in the configuration, the PAR service stays stopped even after the core
// agent has started.
func (s *parIntegrationDisabledSuite) TestPARDoesNotStartWhenDisabled() {
	host := s.Env().RemoteHost

	// Start the core agent
	err := windowsCommon.StartService(host, "datadogagent")
	s.Require().NoError(err, "should start datadogagent")

	// Wait for the core agent to stabilise
	s.Require().True(s.EventuallyWithExponentialBackoff(func() error {
		status, err := windowsCommon.GetServiceStatus(host, "datadogagent")
		if err != nil {
			return err
		}
		if status != "Running" {
			return fmt.Errorf("datadogagent is not running yet: %s", status)
		}
		return nil
	}, 2*time.Minute, 10*time.Second, "datadogagent should be running"), "datadogagent should be running")

	// PAR must NOT be running
	status, err := windowsCommon.GetServiceStatus(host, PARServiceName)
	s.Require().NoError(err, "should get PAR service status")
	s.Assert().Equal("Stopped", status,
		"PAR service should remain Stopped when disabled in config")
}

// TestStartingDisabledPARService verifies that manually starting the PAR
// service when it is marked as disabled in the config causes it to exit
// cleanly (returning to Stopped) without writing error events.
func (s *parIntegrationDisabledSuite) TestStartingDisabledPARService() {
	host := s.Env().RemoteHost

	// Verify it starts in the stopped state
	s.Require().True(s.EventuallyWithExponentialBackoff(func() error {
		status, err := windowsCommon.GetServiceStatus(host, PARServiceName)
		if err != nil {
			return err
		}
		if status != "Stopped" {
			return fmt.Errorf("service is not stopped: %s", status)
		}
		return nil
	}, 30*time.Second, 5*time.Second, "PAR should be stopped initially"), "PAR should be stopped initially")

	// Try to start it manually – it should exit cleanly because PAR exits with
	// ErrCleanStopAfterInit when disabled.
	err := windowsCommon.StartService(host, PARServiceName)
	s.Require().NoError(err, "StartService should not return an error even when PAR self-disables")

	// Service should go back to Stopped on its own
	s.Require().True(s.EventuallyWithExponentialBackoff(func() error {
		status, err := windowsCommon.GetServiceStatus(host, PARServiceName)
		if err != nil {
			return err
		}
		if status != "Stopped" {
			return fmt.Errorf("waiting for PAR to stop, current: %s", status)
		}
		return nil
	}, 30*time.Second, 5*time.Second, "PAR should return to Stopped after self-disabling"), "PAR should return to Stopped after self-disabling")

	// Verify no error events were emitted by PAR
	entries, err := windowsCommon.GetEventLogErrorAndWarningEntries(host, "Application")
	s.Require().NoError(err, "should get Application event log")
	parErrors := windowsCommon.Filter(entries, func(e windowsCommon.EventLogEntry) bool {
		return strings.EqualFold(e.ProviderName, PARServiceName)
	})
	s.Assert().Empty(parErrors,
		"PAR should not emit error/warning events when self-disabling cleanly")
}

// ---- helpers ----

func (s *parIntegrationSuite) eventuallyServiceRunning(serviceName string) {
	s.T().Helper()
	s.Require().True(s.EventuallyWithExponentialBackoff(func() error {
		status, err := windowsCommon.GetServiceStatus(s.Env().RemoteHost, serviceName)
		if err != nil {
			return err
		}
		if status != "Running" {
			return fmt.Errorf("%s is not running yet: %s", serviceName, status)
		}
		return nil
	}, 2*time.Minute, 10*time.Second, "%s should be running", serviceName),
		"%s should be running", serviceName)
}

func (s *parIntegrationSuite) eventuallyServiceStopped(serviceName string) {
	s.T().Helper()
	s.Require().True(s.EventuallyWithExponentialBackoff(func() error {
		status, err := windowsCommon.GetServiceStatus(s.Env().RemoteHost, serviceName)
		if err != nil {
			return err
		}
		if status != "Stopped" {
			return fmt.Errorf("%s is not stopped yet: %s", serviceName, status)
		}
		return nil
	}, 2*time.Minute, 10*time.Second, "%s should be stopped", serviceName),
		"%s should be stopped", serviceName)
}

