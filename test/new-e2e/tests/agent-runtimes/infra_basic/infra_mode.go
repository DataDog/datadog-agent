// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infrabasic contains end-to-end tests for infrastructure basic mode functionality.
// These tests verify that core system checks work correctly when the agent is configured
// to run in basic infrastructure mode, which only instantiates essential components.
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
		"load",
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

type infraBasicSuite struct { //nolint:unused
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

// RunnerStats represents the check runner statistics from agent status
type RunnerStats struct {
	CheckName         string `json:"CheckName"`
	TotalRuns         uint64 `json:"TotalRuns"`
	TotalErrors       uint64 `json:"TotalErrors"`
	TotalWarnings     uint64 `json:"TotalWarnings"`
	CheckID           string `json:"CheckID"`
	CheckConfigSource string `json:"CheckConfigSource"`
}

// runnerStatsContainer holds the Checks map
// The structure is nested: check name -> instance ID -> stats
type runnerStatsContainer struct {
	Checks map[string]map[string]RunnerStats `json:"Checks"`
}

// AgentStatusJSON represents the relevant parts of agent status JSON output
type AgentStatusJSON struct {
	RunnerStats runnerStatsContainer `json:"runnerStats"`
}

// ============================================================================
// Utility Functions
// ============================================================================

func (s *infraBasicSuite) getSuiteOptions() []e2e.SuiteOption { //nolint:unused
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

	// Add integration configs for all allowed checks
	for _, checkName := range allowedChecks {
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

// getRunnerStats retrieves the runner statistics from the agent status
func (s *infraBasicSuite) getRunnerStats() (map[string]map[string]RunnerStats, error) { //nolint:unused
	status := s.Env().Agent.Client.Status(agentclient.WithArgs([]string{"collector", "--json"}))

	var statusMap AgentStatusJSON
	err := json.Unmarshal([]byte(status.Content), &statusMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent status: %w", err)
	}

	return statusMap.RunnerStats.Checks, nil
}

// isCheckScheduled returns true if the check is scheduled and has run at least once
func (s *infraBasicSuite) isCheckScheduled(checkName string, checks map[string]map[string]RunnerStats) bool { //nolint:unused
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
func (s *infraBasicSuite) verifyCheckRuns(checkName string) bool { //nolint:unused
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

// ============================================================================
// Test Functions
// ============================================================================

// assertAllowedChecksWork verifies that allowed checks work in basic mode.
// Tests both scheduler behavior (via status API) and CLI behavior.
// Note: Check configurations are provisioned during suite setup via agentparams.WithIntegration()
func (s *infraBasicSuite) assertAllowedChecksWork() { //nolint:unused
	s.T().Run("via_status_api", func(t *testing.T) {
		// Verify all checks are scheduled and running by querying agent status
		t.Logf("Testing %d checks via status API...", len(allowedChecks))

		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			stats, err := s.getRunnerStats()
			if !assert.NoError(c, err, "Failed to get runner stats") {
				t.Logf("Failed to retrieve runner stats, will retry...")
				return
			}

			t.Logf("Found %d check types in runner stats, verifying all allowed checks are present...", len(stats))

			// Verify all allowed checks are scheduled
			for _, checkName := range allowedChecks {
				scheduled := s.isCheckScheduled(checkName, stats)
				if !scheduled {
					t.Logf("Check %s not yet scheduled in basic mode", checkName)
				} else {
					t.Logf("Check %s found in runner stats and has run", checkName)
				}
				assert.True(c, scheduled, "Check %s should be scheduled and running in basic mode", checkName)
			}
		}, 1*time.Minute, 10*time.Second, "All allowed checks should be scheduled within 1 minute")
	})

	s.T().Run("via_cli", func(t *testing.T) {
		// Also verify all checks can be run via CLI
		// No need to setup configs - they're already provisioned during suite setup
		t.Logf("Testing %d checks via CLI...", len(allowedChecks))
		for _, checkName := range allowedChecks {
			ran := s.verifyCheckRuns(checkName)
			assert.True(t, ran, "Check %s must be runnable via CLI in basic mode", checkName)
		}
	})
}

// assertExcludedChecksAreBlocked verifies that excluded checks are blocked in basic mode.
// Tests both scheduler behavior (via status API) and CLI behavior.
// Note: Excluded check configurations are provisioned during suite setup to verify they're blocked
func (s *infraBasicSuite) assertExcludedChecksAreBlocked() { //nolint:unused
	s.T().Run("via_status_api", func(t *testing.T) {
		// Verify checks are NOT scheduled by querying running agent
		t.Logf("Verifying %d excluded checks are not scheduled...", len(excludedChecks))

		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			stats, err := s.getRunnerStats()
			if !assert.NoError(c, err, "Failed to get runner stats") {
				t.Logf("Failed to retrieve runner stats, will retry...")
				return
			}

			t.Logf("Found %d check types in runner stats, verifying excluded checks are not present...", len(stats))

			// Verify all excluded checks are NOT scheduled
			for _, checkName := range excludedChecks {
				scheduled := s.isCheckScheduled(checkName, stats)
				if !scheduled {
					t.Logf("Check %s correctly not scheduled in basic mode", checkName)
				} else {
					t.Logf("Check %s incorrectly found in runner stats!", checkName)
				}
				assert.False(c, scheduled, "Check %s should NOT be scheduled in basic mode", checkName)
			}
		}, 1*time.Minute, 10*time.Second, "All excluded checks should remain not scheduled within 1 minute")
	})

	s.T().Run("via_cli", func(t *testing.T) {
		// Verify checks are blocked via CLI even though configs exist
		t.Logf("Testing %d excluded checks via CLI...", len(excludedChecks))
		for _, checkName := range excludedChecks {
			ran := s.verifyCheckRuns(checkName)
			assert.False(t, ran, "Check %s should be blocked via CLI in basic mode", checkName)
		}
	})
}

// assertAdditionalCheckWorks verifies that a check can be added via infra_basic_additional_checks.
// This test dynamically updates the environment to add a check not in the default allow list.
func (s *infraBasicSuite) assertAdditionalCheckWorks() { //nolint:unused
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
		// Verify the additional check is scheduled and running
		t.Logf("Waiting for additional check %s to appear in runner stats...", additionalCheckName)

		var cachedStats map[string]map[string]RunnerStats
		var statsErr error

		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			cachedStats, statsErr = s.getRunnerStats()
			if !assert.NoError(c, statsErr, "Failed to get runner stats") {
				t.Logf("Failed to retrieve runner stats, will retry...")
				return
			}

			scheduled := s.isCheckScheduled(additionalCheckName, cachedStats)
			if !scheduled {
				t.Logf("Check %s not yet scheduled (found %d check types), refetching...", additionalCheckName, len(cachedStats))
			} else {
				t.Logf("Check %s found in runner stats and has run", additionalCheckName)
			}
			assert.True(c, scheduled, "Check %s should be scheduled in basic mode when added via infra_basic_additional_checks", additionalCheckName)
		}, 1*time.Minute, 10*time.Second, "Check %s did not appear in runner stats within 1 minute", additionalCheckName)
	})

	s.T().Run("via_cli", func(t *testing.T) {
		// Verify the additional check can be run via CLI
		t.Logf("Testing additional check %s via CLI...", additionalCheckName)
		ran := s.verifyCheckRuns(additionalCheckName)
		assert.True(t, ran, "Check %s must be runnable via CLI in basic mode when added via infra_basic_additional_checks", additionalCheckName)
	})
}
