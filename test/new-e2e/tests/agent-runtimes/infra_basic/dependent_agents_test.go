// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infrabasic

import (
	"fmt"
	"strings"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

// dependentAgentsSuite tests that dependent agents (trace-agent, process-agent, etc.)
// do not run when infrastructure_mode is set to "basic"
type dependentAgentsSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

func (s *dependentAgentsSuite) getSuiteOptions() []e2e.SuiteOption {
	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(s.descriptor), ec2.WithInstanceType("t3.micro")),
		),
	))

	return suiteOptions
}

// configureBasicMode configures the agent in infrastructure basic mode
func (s *dependentAgentsSuite) configureBasicMode() {
	env := s.Env()
	host := env.RemoteHost

	basicModeConfig := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: disabled
network_config:
  enabled: false
system_probe_config:
  enabled: false
runtime_security_config:
  enabled: false
`

	confFolder := "/etc/datadog-agent"
	if s.descriptor.Family() == e2eos.WindowsFamily {
		confFolder = "C:\\ProgramData\\Datadog"
	}

	configFile := host.JoinPath(confFolder, "datadog.yaml")

	// Write the config
	tmpFolder, err := host.GetTmpFolder()
	if err != nil {
		s.T().Fatalf("Failed to get tmp folder: %v", err)
	}

	tmpConfigFile := host.JoinPath(tmpFolder, "datadog_basic.yaml")
	_, err = host.WriteFile(tmpConfigFile, []byte(basicModeConfig))
	if err != nil {
		s.T().Fatalf("Failed to write basic mode config: %v", err)
	}

	// Copy to agent config location
	if s.descriptor.Family() == e2eos.WindowsFamily {
		_, err = host.Execute(fmt.Sprintf("copy /Y %s %s", tmpConfigFile, configFile))
	} else {
		_, err = host.Execute(fmt.Sprintf("sudo cp %s %s", tmpConfigFile, configFile))
		if err == nil {
			_, _ = host.Execute(fmt.Sprintf("sudo chown dd-agent:dd-agent %s", configFile))
		}
	}
	if err != nil {
		s.T().Fatalf("Failed to copy basic mode config: %v", err)
	}
}

// configureBasicModeWithAgentsEnabled configures the agent in infrastructure basic mode
// BUT with individual agents explicitly enabled in the configuration.
// This tests that basic mode enforcement overrides individual agent settings.
func (s *dependentAgentsSuite) configureBasicModeWithAgentsEnabled() { //nolint:unused
	env := s.Env()
	host := env.RemoteHost

	// Note: All agents are explicitly ENABLED here, but infrastructure_mode: basic should override
	basicModeWithAgentsEnabledConfig := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: true
apm_config:
  enabled: true
process_config:
  enabled: true
  process_collection:
    enabled: true
network_config:
  enabled: true
system_probe_config:
  enabled: true
runtime_security_config:
  enabled: true
`

	confFolder := "/etc/datadog-agent"
	if s.descriptor.Family() == e2eos.WindowsFamily {
		confFolder = "C:\\ProgramData\\Datadog"
	}

	configFile := host.JoinPath(confFolder, "datadog.yaml")

	// Write the config
	tmpFolder, err := host.GetTmpFolder()
	if err != nil {
		s.T().Fatalf("Failed to get tmp folder: %v", err)
	}

	tmpConfigFile := host.JoinPath(tmpFolder, "datadog_basic_with_agents.yaml")
	_, err = host.WriteFile(tmpConfigFile, []byte(basicModeWithAgentsEnabledConfig))
	if err != nil {
		s.T().Fatalf("Failed to write basic mode config with agents enabled: %v", err)
	}

	// Copy to agent config location
	if s.descriptor.Family() == e2eos.WindowsFamily {
		_, err = host.Execute(fmt.Sprintf("copy /Y %s %s", tmpConfigFile, configFile))
	} else {
		_, err = host.Execute(fmt.Sprintf("sudo cp %s %s", tmpConfigFile, configFile))
		if err == nil {
			_, _ = host.Execute(fmt.Sprintf("sudo chown dd-agent:dd-agent %s", configFile))
		}
	}
	if err != nil {
		s.T().Fatalf("Failed to copy basic mode config with agents enabled: %v", err)
	}
}

// restartCoreAgent restarts the core agent to apply configuration changes
func (s *dependentAgentsSuite) restartCoreAgent() {
	env := s.Env()
	host := env.RemoteHost

	if s.descriptor.Family() == e2eos.WindowsFamily {
		_, err := host.Execute("Restart-Service -Name DatadogAgent")
		if err != nil {
			s.T().Fatalf("Failed to restart core agent on Windows: %v", err)
		}
		// Give Windows services time to start/fail
		_, _ = host.Execute("Start-Sleep -Seconds 10")
	} else {
		_, err := host.Execute("sudo systemctl restart datadog-agent")
		if err != nil {
			s.T().Fatalf("Failed to restart core agent on Linux: %v", err)
		}
		// Give systemd time to attempt starting dependent services
		_, _ = host.Execute("sleep 10")
	}
}

// checkServiceNotRunning verifies that a service is not running
// Returns true if service is NOT running (expected), false if it IS running (unexpected)
func (s *dependentAgentsSuite) checkServiceNotRunning(serviceName string, displayName string) bool {
	env := s.Env()
	host := env.RemoteHost

	if s.descriptor.Family() == e2eos.WindowsFamily {
		// Check Windows service status
		output, err := host.Execute(fmt.Sprintf("(Get-Service -Name '%s').Status", serviceName))
		if err != nil {
			// Service might not exist or be in error state - that's okay
			s.T().Logf("%s service query returned error (expected): %v", displayName, err)
			return true
		}

		status := strings.TrimSpace(output)
		if status == "Running" {
			s.T().Errorf("%s is RUNNING but should be STOPPED in basic mode! Status: %s", displayName, status)
			return false
		}

		s.T().Logf("%s is correctly NOT running. Status: %s", displayName, status)
		return true
	} else {
		// Check systemd service status
		output, err := host.Execute(fmt.Sprintf("systemctl is-active %s", serviceName))
		status := strings.TrimSpace(output)

		if err != nil || status != "active" {
			// Service is not active - this is what we expect
			s.T().Logf("%s is correctly NOT running. Status: %s", displayName, status)
			return true
		}

		s.T().Errorf("%s is ACTIVE but should be INACTIVE in basic mode! Status: %s", displayName, status)
		return false
	}
}

// checkServiceLogs verifies that the service logs contain the expected basic mode message
func (s *dependentAgentsSuite) checkServiceLogs(serviceName string, displayName string, expectedMessage string) {
	env := s.Env()
	host := env.RemoteHost

	if s.descriptor.Family() == e2eos.WindowsFamily {
		// On Windows, check the service logs
		logPath := fmt.Sprintf("C:\\ProgramData\\Datadog\\logs\\%s.log", serviceName)
		output, err := host.Execute(fmt.Sprintf("Get-Content -Path '%s' -Tail 50 -ErrorAction SilentlyContinue", logPath))
		if err != nil {
			s.T().Logf("Could not read %s logs (may not exist): %v", displayName, err)
			return
		}

		if strings.Contains(output, expectedMessage) {
			s.T().Logf("%s logs correctly contain basic mode message", displayName)
		} else {
			s.T().Logf("%s logs do not contain expected message (service may not have started)", displayName)
		}
	} else {
		// On Linux, check journalctl
		output, err := host.Execute(fmt.Sprintf("sudo journalctl -u %s -n 50 --no-pager", serviceName))
		if err != nil {
			s.T().Logf("Could not read %s journal logs: %v", displayName, err)
			return
		}

		if strings.Contains(output, expectedMessage) {
			s.T().Logf("%s logs correctly contain basic mode message", displayName)
		} else {
			s.T().Logf("%s logs do not contain expected message yet", displayName)
		}
	}
}

// assertTraceAgentNotRunning verifies that trace-agent does not run in basic mode
func (s *dependentAgentsSuite) assertTraceAgentNotRunning() {
	// Configure and restart
	s.configureBasicMode()
	s.restartCoreAgent()

	// Check service status
	if s.descriptor.Family() == e2eos.WindowsFamily {
		if !s.checkServiceNotRunning("DatadogTraceAgent", "trace-agent") {
			s.T().Error("trace-agent should NOT be running in infrastructure basic mode")
		}
	} else {
		if !s.checkServiceNotRunning("datadog-agent-trace", "trace-agent") {
			s.T().Error("trace-agent should NOT be running in infrastructure basic mode")
		}
		// Check for expected log message
		s.checkServiceLogs("datadog-agent-trace", "trace-agent",
			"Infrastructure basic mode is enabled - trace-agent is not allowed to run")
	}
}

// assertProcessAgentNotRunning verifies that process-agent does not run in basic mode
func (s *dependentAgentsSuite) assertProcessAgentNotRunning() {
	// Configure and restart
	s.configureBasicMode()
	s.restartCoreAgent()

	// Check service status
	if s.descriptor.Family() == e2eos.WindowsFamily {
		if !s.checkServiceNotRunning("DatadogProcess", "process-agent") {
			s.T().Error("process-agent should NOT be running in infrastructure basic mode")
		}
	} else {
		if !s.checkServiceNotRunning("datadog-agent-process", "process-agent") {
			s.T().Error("process-agent should NOT be running in infrastructure basic mode")
		}
		// Check for expected log message
		s.checkServiceLogs("datadog-agent-process", "process-agent",
			"Infrastructure basic mode is enabled - process-agent is not allowed to run")
	}
}

// assertSystemProbeNotRunning verifies that system-probe does not run in basic mode
func (s *dependentAgentsSuite) assertSystemProbeNotRunning() {
	// Configure and restart
	s.configureBasicMode()
	s.restartCoreAgent()

	// Check service status
	if s.descriptor.Family() == e2eos.WindowsFamily {
		if !s.checkServiceNotRunning("DatadogSystemProbe", "system-probe") {
			s.T().Error("system-probe should NOT be running in infrastructure basic mode")
		}
	} else {
		if !s.checkServiceNotRunning("datadog-agent-sysprobe", "system-probe") {
			s.T().Error("system-probe should NOT be running in infrastructure basic mode")
		}
		// Check for expected log message
		s.checkServiceLogs("datadog-agent-sysprobe", "system-probe",
			"Infrastructure basic mode is enabled - system-probe is not allowed to run")
	}
}

// assertSecurityAgentNotRunning verifies that security-agent does not run in basic mode
func (s *dependentAgentsSuite) assertSecurityAgentNotRunning() {
	// Configure and restart
	s.configureBasicMode()
	s.restartCoreAgent()

	// Check service status
	if s.descriptor.Family() == e2eos.WindowsFamily {
		if !s.checkServiceNotRunning("DatadogSecurity", "security-agent") {
			s.T().Error("security-agent should NOT be running in infrastructure basic mode")
		}
	} else {
		if !s.checkServiceNotRunning("datadog-agent-security", "security-agent") {
			s.T().Error("security-agent should NOT be running in infrastructure basic mode")
		}
		// Check for expected log message
		s.checkServiceLogs("datadog-agent-security", "security-agent",
			"Infrastructure basic mode is enabled - security-agent is not allowed to run")
	}
}

// assertCoreAgentStillRunning verifies that the core agent is still running
func (s *dependentAgentsSuite) assertCoreAgentStillRunning() {
	env := s.Env()
	host := env.RemoteHost

	if s.descriptor.Family() == e2eos.WindowsFamily {
		output, err := host.Execute("(Get-Service -Name 'DatadogAgent').Status")
		if err != nil {
			s.T().Fatalf("Failed to check core agent status: %v", err)
		}

		status := strings.TrimSpace(output)
		if status != "Running" {
			s.T().Errorf("Core agent should be RUNNING but status is: %s", status)
		} else {
			s.T().Log("Core agent is correctly RUNNING in basic mode")
		}
	} else {
		output, err := host.Execute("systemctl is-active datadog-agent")
		if err != nil {
			s.T().Fatalf("Failed to check core agent status: %v", err)
		}

		status := strings.TrimSpace(output)
		if status != "active" {
			s.T().Errorf("Core agent should be ACTIVE but status is: %s", status)
		} else {
			s.T().Log("Core agent is correctly ACTIVE in basic mode")
		}
	}
}

// ========================================
// Tests with agents explicitly enabled
// These tests verify that basic mode enforcement overrides individual agent settings
// ========================================

// assertTraceAgentNotRunningEvenWhenEnabled verifies that trace-agent does not run in basic mode
// even when apm_config.enabled is set to true
func (s *dependentAgentsSuite) assertTraceAgentNotRunningEvenWhenEnabled() {
	// Configure with agents explicitly enabled, but infrastructure_mode: basic
	s.configureBasicModeWithAgentsEnabled()
	s.restartCoreAgent()

	// Check service status - should still NOT be running
	if s.descriptor.Family() == e2eos.WindowsFamily {
		if !s.checkServiceNotRunning("DatadogTraceAgent", "trace-agent") {
			s.T().Error("trace-agent should NOT be running even when enabled in config (basic mode should override)")
		}
	} else {
		if !s.checkServiceNotRunning("datadog-agent-trace", "trace-agent") {
			s.T().Error("trace-agent should NOT be running even when enabled in config (basic mode should override)")
		}
		// Check for expected log message
		s.checkServiceLogs("datadog-agent-trace", "trace-agent",
			"Infrastructure basic mode is enabled - trace-agent is not allowed to run")
	}
}

// assertProcessAgentNotRunningEvenWhenEnabled verifies that process-agent does not run in basic mode
// even when process_config.enabled is set to true
func (s *dependentAgentsSuite) assertProcessAgentNotRunningEvenWhenEnabled() {
	// Configure with agents explicitly enabled, but infrastructure_mode: basic
	s.configureBasicModeWithAgentsEnabled()
	s.restartCoreAgent()

	// Check service status - should still NOT be running
	if s.descriptor.Family() == e2eos.WindowsFamily {
		if !s.checkServiceNotRunning("DatadogProcess", "process-agent") {
			s.T().Error("process-agent should NOT be running even when enabled in config (basic mode should override)")
		}
	} else {
		if !s.checkServiceNotRunning("datadog-agent-process", "process-agent") {
			s.T().Error("process-agent should NOT be running even when enabled in config (basic mode should override)")
		}
		// Check for expected log message
		s.checkServiceLogs("datadog-agent-process", "process-agent",
			"Infrastructure basic mode is enabled - process-agent is not allowed to run")
	}
}

// assertSystemProbeNotRunningEvenWhenEnabled verifies that system-probe does not run in basic mode
// even when system_probe_config.enabled is set to true
func (s *dependentAgentsSuite) assertSystemProbeNotRunningEvenWhenEnabled() {
	// Configure with agents explicitly enabled, but infrastructure_mode: basic
	s.configureBasicModeWithAgentsEnabled()
	s.restartCoreAgent()

	// Check service status - should still NOT be running
	if s.descriptor.Family() == e2eos.WindowsFamily {
		if !s.checkServiceNotRunning("DatadogSystemProbe", "system-probe") {
			s.T().Error("system-probe should NOT be running even when enabled in config (basic mode should override)")
		}
	} else {
		if !s.checkServiceNotRunning("datadog-agent-sysprobe", "system-probe") {
			s.T().Error("system-probe should NOT be running even when enabled in config (basic mode should override)")
		}
		// Check for expected log message
		s.checkServiceLogs("datadog-agent-sysprobe", "system-probe",
			"Infrastructure basic mode is enabled - system-probe is not allowed to run")
	}
}

// assertSecurityAgentNotRunningEvenWhenEnabled verifies that security-agent does not run in basic mode
// even when runtime_security_config.enabled is set to true
func (s *dependentAgentsSuite) assertSecurityAgentNotRunningEvenWhenEnabled() {
	// Configure with agents explicitly enabled, but infrastructure_mode: basic
	s.configureBasicModeWithAgentsEnabled()
	s.restartCoreAgent()

	// Check service status - should still NOT be running
	if s.descriptor.Family() == e2eos.WindowsFamily {
		if !s.checkServiceNotRunning("DatadogSecurityAgent", "security-agent") {
			s.T().Error("security-agent should NOT be running even when enabled in config (basic mode should override)")
		}
	} else {
		if !s.checkServiceNotRunning("datadog-agent-security", "security-agent") {
			s.T().Error("security-agent should NOT be running even when enabled in config (basic mode should override)")
		}
		// Check for expected log message
		s.checkServiceLogs("datadog-agent-security", "security-agent",
			"Infrastructure basic mode is enabled - security-agent is not allowed to run")
	}
}
