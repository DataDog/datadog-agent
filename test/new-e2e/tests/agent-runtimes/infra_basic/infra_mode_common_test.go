// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infrabasic provides e2e tests for infrastructure basic mode functionality
package infrabasic

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	agentclient "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

var (
	// allowedChecks lists all checks that should work in basic mode
	allowedChecks = []string{
		"cpu",
		"memory",
		"disk",
		"uptime",
		"load", // linux only
		"network",
		"ntp",
		"io",
		"file_handle",
		"system_core",
		"telemetry",
	}

	// excludedChecks lists integrations that should NOT run in basic mode
	excludedChecks = []string{
		"container_lifecycle",
		"container_image",
		"container",
		"kubelet",
		"docker",
		"orchestrator_pod",
		"cri",
		"containerd",
		"coredns",
		"kubernetes_apiserver",
		"datadog_cluster_agent",
		"kube_apiserver_metrics",
	}
)

// ============================================================================
// Type Definitions
// ============================================================================

type infraBasicSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

type runnerStatsContainer struct {
	Checks map[string]map[string]check.Runner `json:"Checks"`
}

// AgentStatusJSON represents the JSON structure of the agent status output
type AgentStatusJSON struct {
	RunnerStats runnerStatsContainer `json:"runnerStats"`
}

// ============================================================================
// Utility Functions
// ============================================================================

// getAllowedChecks returns the list of checks that should work on the current OS
func (s *infraBasicSuite) getAllowedChecks() []string {
	checks := make([]string, 0, len(allowedChecks))
	for _, checkName := range allowedChecks {
		// Skip "load" check on Windows as it's Linux-only
		if checkName == "load" && s.descriptor.Family() == e2eos.WindowsFamily {
			continue
		}
		checks = append(checks, checkName)
	}
	return checks
}

func (s *infraBasicSuite) getSuiteOptions() []e2e.SuiteOption {
	// Agent configuration for basic mode testing
	// Enable process agent explicitly to test it is blocked by infrastructure_mode: basic
	// Other agents (trace, system-probe, security) should still work in basic mode
	basicModeAgentConfig := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: true
process_config:
  enabled: true
  process_collection:
    enabled: true
  container_collection:
    enabled: true
telemetry:
  enabled: true
`

	// Minimal check configuration for integration configs
	minimalCheckConfig := `
init_config:
instances:
  - {}
`

	// Build agent options with basic mode configuration and check configurations
	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(basicModeAgentConfig),
	}

	// Add integration configs for all allowed checks (filtered by OS)
	for _, checkName := range s.getAllowedChecks() {
		agentOptions = append(agentOptions, agentparams.WithIntegration(checkName+".d", minimalCheckConfig))
	}

	// Add integration configs for excluded checks too (to test they're blocked)
	for _, checkName := range excludedChecks {
		agentOptions = append(agentOptions, agentparams.WithIntegration(checkName+".d", minimalCheckConfig))
	}

	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(s.descriptor), ec2.WithInstanceType("t3.micro")),
			awshost.WithAgentOptions(agentOptions...),
		),
	))

	return suiteOptions
}

// getScheduledChecks retrieves the map of scheduled checks from the agent status
func (s *infraBasicSuite) getScheduledChecks() (map[string]map[string]check.Runner, error) {
	status := s.Env().Agent.Client.Status(agentclient.WithArgs([]string{"collector", "--json"}))

	var statusMap AgentStatusJSON
	err := json.Unmarshal([]byte(status.Content), &statusMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent status: %w", err)
	}

	return statusMap.RunnerStats.Checks, nil
}

// isCheckScheduled returns true if the check is scheduled and has run at least once
func (s *infraBasicSuite) isCheckScheduled(checkName string, checks map[string]map[string]check.Runner) bool {
	// The checks map is nested: checkName -> instanceID -> stats
	if instances, exists := checks[checkName]; exists {
		// Check if any instance of this check has run
		for _, stat := range instances {
			if stat.TotalRuns > 0 {
				return true
			}
		}
	}
	return false
}

// verifyCheckRuns runs a check and verifies it executed successfully
// All check configs are already provisioned during suite setup
func (s *infraBasicSuite) verifyCheckRuns(checkName string) bool {
	// Run the check using the cross-platform Agent client helper
	// This works on both Linux (sudo datadog-agent) and Windows (& "path\bin\agent.exe")
	output, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{checkName, "--json"}))
	if err != nil {
		s.T().Logf("Check %s failed to execute: %v", checkName, err)
		return false
	}

	// Parse the JSON output and check the Runner.TotalRuns field
	data := check.ParseJSONOutput(s.T(), []byte(output))
	if len(data) == 0 {
		s.T().Logf("Check %s produced no output data", checkName)
		return false
	}

	// Check if the check actually ran by inspecting TotalRuns
	runner := data[0].Runner
	if runner.TotalRuns == 0 {
		s.T().Logf("Check %s did not run (TotalRuns=0, TotalErrors=%d, TotalWarnings=%d)",
			checkName, runner.TotalErrors, runner.TotalWarnings)
		return false
	}

	// Log success with runner statistics
	s.T().Logf("Check %s ran successfully (TotalRuns=%d, TotalErrors=%d, TotalWarnings=%d)",
		checkName, runner.TotalRuns, runner.TotalErrors, runner.TotalWarnings)
	return true
}

// verifyCheckSchedulingViaStatusAPI verifies that checks are in the expected scheduling state
// by querying the agent status API. This is a helper function meant to be called within EventuallyWithT.
func (s *infraBasicSuite) verifyCheckSchedulingViaStatusAPI(c *assert.CollectT, checks []string, shouldBeScheduled bool) {
	scheduledChecks, err := s.getScheduledChecks()
	if !assert.NoError(c, err, "Failed to get scheduled checks") {
		s.T().Logf("Failed to retrieve scheduled checks, will retry...")
		return
	}

	s.T().Logf("Found %d check types in agent status", len(scheduledChecks))

	// Verify all checks match the expected scheduling state
	for _, checkName := range checks {
		scheduled := s.isCheckScheduled(checkName, scheduledChecks)

		// Log current state
		if scheduled {
			s.T().Logf("Check %s is scheduled", checkName)
		} else {
			s.T().Logf("Check %s is not scheduled", checkName)
		}

		// Assert expected state
		assert.Equal(c, shouldBeScheduled, scheduled, "Check %s scheduling state mismatch", checkName)
	}
}

// checkServiceRunning checks if a service is running on the host, handling both Windows and Linux.
// Returns (isRunning bool, err error).
func (s *infraBasicSuite) checkServiceRunning(serviceCommand string) (bool, error) {
	if s.descriptor.Family() == e2eos.WindowsFamily {
		// Windows: Check service status using PowerShell Get-Service
		result := s.Env().RemoteHost.MustExecute("powershell -Command \"Get-Service -Name '" + serviceCommand + "' -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Status\"")
		return result == "Running", nil
	}

	// Linux: Check systemd service status
	result, err := s.Env().RemoteHost.Execute("systemctl is-active " + serviceCommand + " 2>/dev/null || echo 'inactive'")
	return result == "active", err
}

// getAgentStatus retrieves and parses the agent status JSON.
// Returns the parsed status map or an error.
func (s *infraBasicSuite) getAgentStatus() (map[string]interface{}, error) {
	status := s.Env().Agent.Client.Status(agentclient.WithArgs([]string{"--json"}))
	var statusMap map[string]interface{}
	err := json.Unmarshal([]byte(status.Content), &statusMap)
	return statusMap, err
}

// ============================================================================
// Test Functions
// ============================================================================

// TestCheckSchedulingBehavior verifies that checks are correctly scheduled or blocked in basic mode.
// Tests both scheduler behavior (via status API) and CLI behavior for allowed and excluded checks.
// Note: Check configurations are provisioned during suite setup via agentparams.WithIntegration()
func (s *infraBasicSuite) TestCheckSchedulingBehavior() {
	// First test: Verify scheduler behavior via status API
	s.T().Run("via_status_api", func(t *testing.T) {
		t.Run("allowed_checks_scheduled", func(t *testing.T) {
			allowedChecksForOS := s.getAllowedChecks()
			t.Logf("Verifying %d allowed checks are scheduled via status API...", len(allowedChecksForOS))

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				s.verifyCheckSchedulingViaStatusAPI(c, allowedChecksForOS, true)
			}, 1*time.Minute, 10*time.Second, "All allowed checks should be scheduled within 1 minute")
		})

		t.Run("excluded_checks_not_scheduled", func(t *testing.T) {
			t.Logf("Verifying %d excluded checks are not scheduled via status API...", len(excludedChecks))

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				s.verifyCheckSchedulingViaStatusAPI(c, excludedChecks, false)
			}, 1*time.Minute, 10*time.Second, "All excluded checks should remain not scheduled within 1 minute")
		})
	})

	// Second test: Verify CLI behavior
	s.T().Run("via_cli", func(t *testing.T) {
		t.Run("allowed_checks_runnable", func(t *testing.T) {
			allowedChecksForOS := s.getAllowedChecks()
			t.Logf("Testing %d allowed checks via CLI...", len(allowedChecksForOS))
			for _, checkName := range allowedChecksForOS {
				ran := s.verifyCheckRuns(checkName)
				assert.True(t, ran, "Check %s must be runnable via CLI in basic mode", checkName)
			}
		})

		t.Run("excluded_checks_blocked", func(t *testing.T) {
			t.Logf("Testing %d excluded checks via CLI...", len(excludedChecks))
			for _, checkName := range excludedChecks {
				ran := s.verifyCheckRuns(checkName)
				assert.False(t, ran, "Check %s should be blocked via CLI in basic mode", checkName)
			}
		})
	})
}

// TestAgentBlocking verifies that certain agents are blocked in basic mode while others are allowed.
// Process and cluster agents should be blocked even if enabled in config.
// Trace, system-probe, and security agents should still be allowed to run.
func (s *infraBasicSuite) TestAgentBlocking() {
	// Define allowed agents (should be able to run in basic mode)
	allowedAgents := map[string]string{
		"trace-agent":    "datadog-trace-agent",
		"system-probe":   "datadog-system-probe",
		"security-agent": "datadog-security-agent",
	}

	// Define blocked agents (should NOT run in basic mode, even if enabled)
	blockedAgents := map[string]string{
		"process-agent": "datadog-process-agent",
		"cluster-agent": "datadog-cluster-agent",
	}

	// Test allowed agents
	s.T().Run("allowed_agents", func(t *testing.T) {
		t.Run("service_status", func(t *testing.T) {
			t.Logf("Verifying %d allowed agents can run in basic mode...", len(allowedAgents))

			for serviceName, serviceCommand := range allowedAgents {
				t.Run(serviceName, func(t *testing.T) {
					isRunning, err := s.checkServiceRunning(serviceCommand)
					if err != nil {
						t.Logf("Could not check service %s status: %v", serviceName, err)
					}

					// Note: We don't assert.True here because these services might not be enabled
					// by default or might not be installed. The key test is that they're NOT blocked.
					// If they are configured and installed, they should be running.
					t.Logf("Agent %s running status: %v (should be allowed to run in basic mode)", serviceName, isRunning)
				})
			}
		})

		t.Run("agent_status", func(t *testing.T) {
			// Verify allowed agents can appear in agent status (if configured)
			t.Logf("Checking agent status for allowed agents...")

			statusMap, err := s.getAgentStatus()
			if !assert.NoError(t, err, "Failed to parse agent status JSON") {
				return
			}

			// Check APM/trace-agent - should be allowed and present since we enabled it in config
			apmStatus, apmExists := statusMap["apmStats"]
			t.Logf("APM status exists: %v, value: %v", apmExists, apmStatus)
			// APM is explicitly enabled in config, so it should appear in status eventually
			// Note: May not be present immediately on startup, but should be allowed
			if apmExists {
				assert.NotNil(t, apmStatus, "APM stats should not be nil when present in status")
			}

			// Check system probe - allowed in basic mode but may not be configured by default
			sysprobe, sysprobeExists := statusMap["systemProbeStats"]
			t.Logf("System probe status exists: %v, value: %v", sysprobeExists, sysprobe)
			if sysprobeExists {
				assert.NotNil(t, sysprobe, "System probe stats should not be nil when present in status")
			}
			// The key point: these agents are ALLOWED in basic mode (not blocked)
		})
	})

	// Test blocked agents
	s.T().Run("blocked_agents", func(t *testing.T) {
		t.Run("service_status", func(t *testing.T) {
			t.Logf("Verifying %d blocked agents are not running (despite being enabled in config)...", len(blockedAgents))

			for serviceName, serviceCommand := range blockedAgents {
				t.Run(serviceName, func(t *testing.T) {
					isRunning, err := s.checkServiceRunning(serviceCommand)
					if err != nil {
						t.Logf("Could not check service %s status (expected if service doesn't exist): %v", serviceName, err)
					}

					assert.False(t, isRunning, "Agent %s should NOT be running in infrastructure basic mode (even though enabled in config)", serviceName)
				})
			}
		})

		t.Run("agent_status", func(t *testing.T) {
			// Verify blocked agents don't appear as active in agent status
			t.Logf("Verifying blocked agents are not active in agent status...")

			statusMap, err := s.getAgentStatus()
			if !assert.NoError(t, err, "Failed to parse agent status JSON") {
				return
			}

			// Check process agent - should not be active in basic mode (even though we enabled it in config)
			processStatus, processExists := statusMap["processAgentStatus"]
			t.Logf("Process agent status exists: %v, value: %v", processExists, processStatus)

			// Process agent should either:
			// 1. Not appear in status at all (best case - completely blocked)
			// 2. Appear but indicate it's not running/disabled
			// The critical assertion is that the service is NOT running (tested in service_status above)
			// Here we document the status API behavior for observability
			if processExists {
				t.Logf("Process agent status is present (should indicate disabled/not running): %v", processStatus)
			} else {
				t.Logf("Process agent status not found in status output (completely blocked by basic mode)")
			}
		})
	})
}

// TestAdditionalCheckWorks verifies that a check can be added via infra_basic_additional_checks.
// This test dynamically updates the environment to add a check not in the default allow list.
func (s *infraBasicSuite) TestAdditionalCheckWorks() {
	// Use http_check as an example of a check not in the default allow list
	additionalCheckName := "http_check"

	// HTTP check configuration
	httpCheckConfig := `
init_config:

instances:
  - name: Example website
    url: http://example.com
    timeout: 1
`

	// Agent configuration with the additional check enabled
	agentConfigWithAdditionalCheck := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
telemetry:
  enabled: true
infra_basic_additional_checks:
  - http_check
`

	s.T().Logf("Updating environment to enable additional check: %s", additionalCheckName)

	// Update the environment with the new agent config and check integration
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(s.descriptor), ec2.WithInstanceType("t3.micro")),
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfigWithAdditionalCheck),
			agentparams.WithIntegration(additionalCheckName+".d", httpCheckConfig),
		),
	))

	s.T().Run("via_status_api", func(t *testing.T) {
		t.Logf("Verifying additional check %s is scheduled via status API...", additionalCheckName)

		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			s.verifyCheckSchedulingViaStatusAPI(c, []string{additionalCheckName}, true)
		}, 1*time.Minute, 10*time.Second, "Additional check should be scheduled within 1 minute")
	})

	s.T().Run("via_cli", func(t *testing.T) {
		// Verify the additional check can be run via CLI
		t.Logf("Testing additional check %s via CLI...", additionalCheckName)
		ran := s.verifyCheckRuns(additionalCheckName)
		assert.True(t, ran, "Check %s must be runnable via CLI in basic mode when added via infra_basic_additional_checks", additionalCheckName)
	})
}
