// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infrabasic provides e2e tests for infrastructure basic mode functionality
package infrabasic

import (
	_ "embed"
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

//go:embed fixtures/custom_mycheck.py
var customCheckPython []byte

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
	basicModeAgentConfig := `
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

// TestAdditionalCheckWorks verifies that checks can be added via infra_basic_additional_checks
// and that regex patterns work. This test also verifies the static custom_.* pattern.
func (s *infraBasicSuite) TestAdditionalCheckWorks() {
	// HTTP check configuration
	httpCheckConfig := `
init_config:

instances:
  - name: Example website
    url: http://example.com
    timeout: 1
`

	// Custom check configuration (tests static custom_.* pattern)
	customCheckConfig := `
init_config:
instances:
  - {}
`

	// Agent configuration with additional checks including a regex pattern
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
  - "^http_.*"
`

	s.T().Logf("Updating environment to test regex patterns and custom checks")

	// Determine the correct path for the custom check Python file based on OS
	customCheckPath := "/etc/datadog-agent/checks.d/custom_mycheck.py"
	if s.descriptor.Family() == e2eos.WindowsFamily {
		customCheckPath = "C:/ProgramData/Datadog/checks.d/custom_mycheck.py"
	}

	// Update the environment with the new agent config and check integrations
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(s.descriptor), ec2.WithInstanceType("t3.micro")),
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfigWithAdditionalCheck),
			agentparams.WithIntegration("http_check.d", httpCheckConfig),
			agentparams.WithIntegration("custom_mycheck.d", customCheckConfig),
			agentparams.WithFile(customCheckPath, string(customCheckPython), true),
		),
	))

	s.T().Run("regex_pattern_in_config", func(t *testing.T) {
		t.Run("via_status_api", func(t *testing.T) {
			t.Logf("Verifying http_check matches ^http_.* pattern via status API...")

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				s.verifyCheckSchedulingViaStatusAPI(c, []string{"http_check"}, true)
			}, 1*time.Minute, 10*time.Second, "http_check should match ^http_.* pattern")
		})

		t.Run("via_cli", func(t *testing.T) {
			t.Logf("Testing http_check matches ^http_.* pattern via CLI...")
			ran := s.verifyCheckRuns("http_check")
			assert.True(t, ran, "http_check must match ^http_.* regex pattern")
		})
	})

	s.T().Run("static_custom_pattern", func(t *testing.T) {
		t.Run("via_status_api", func(t *testing.T) {
			t.Logf("Verifying custom_mycheck matches static custom_.* pattern via status API...")

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				s.verifyCheckSchedulingViaStatusAPI(c, []string{"custom_mycheck"}, true)
			}, 1*time.Minute, 10*time.Second, "custom_mycheck should match static custom_.* pattern")
		})

		t.Run("via_cli", func(t *testing.T) {
			t.Logf("Testing custom_mycheck matches static custom_.* pattern via CLI...")
			ran := s.verifyCheckRuns("custom_mycheck")
			assert.True(t, ran, "custom_mycheck must match static custom_.* pattern")
		})
	})
}
