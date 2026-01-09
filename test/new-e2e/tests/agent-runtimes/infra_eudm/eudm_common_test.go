// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infraeudm provides e2e tests for EUDM (End User Device Monitoring) mode functionality
package infraeudm

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	agentclient "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// ============================================================================
// Type Definitions
// ============================================================================

type eudmSuite struct {
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
// Configuration
// ============================================================================

func (s *eudmSuite) getSuiteOptions() []e2e.SuiteOption {
	// Build agent options with EUDM mode configuration
	// The wlan check is automatically loaded via the embedded config provider
	// (configured in integration.end_user_device.inject_embedded)
	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(`infrastructure_mode: "end_user_device"`),
	}

	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(s.descriptor)),
				ec2.WithAgentOptions(agentOptions...),
			),
		),
	))

	return suiteOptions
}

// ============================================================================
// Utility Functions
// ============================================================================

// getScheduledChecks retrieves the map of scheduled checks from the agent status
func (s *eudmSuite) getScheduledChecks() (map[string]map[string]check.Runner, error) {
	status := s.Env().Agent.Client.Status(agentclient.WithArgs([]string{"collector", "--json"}))

	var statusMap AgentStatusJSON
	err := json.Unmarshal([]byte(status.Content), &statusMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent status: %w", err)
	}

	return statusMap.RunnerStats.Checks, nil
}

// isCheckScheduled returns true if the check is scheduled and has run at least once
func (s *eudmSuite) isCheckScheduled(checkName string, checks map[string]map[string]check.Runner) bool {
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
func (s *eudmSuite) verifyCheckRuns(checkName string) bool {
	// Run the check using the cross-platform Agent client helper
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

// verifyCheckSchedulingViaStatusAPI verifies that a check is in the expected scheduling state
func (s *eudmSuite) verifyCheckSchedulingViaStatusAPI(c *assert.CollectT, checkName string, shouldBeScheduled bool) {
	scheduledChecks, err := s.getScheduledChecks()
	if !assert.NoError(c, err, "Failed to get scheduled checks") {
		s.T().Logf("Failed to retrieve scheduled checks, will retry...")
		return
	}

	scheduled := s.isCheckScheduled(checkName, scheduledChecks)

	if scheduled {
		s.T().Logf("Check %s is scheduled", checkName)
	} else {
		s.T().Logf("Check %s is not scheduled", checkName)
	}

	assert.Equal(c, shouldBeScheduled, scheduled, "Check %s scheduling state mismatch", checkName)
}

// verifyCheckEmitsMetrics verifies that a check outputs metrics when run
func (s *eudmSuite) verifyCheckEmitsMetrics(checkName string) (bool, []string) {
	output, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{checkName, "--json"}))
	if err != nil {
		s.T().Logf("Check %s failed to execute: %v", checkName, err)
		return false, nil
	}

	data := check.ParseJSONOutput(s.T(), []byte(output))
	if len(data) == 0 {
		s.T().Logf("Check %s produced no output data", checkName)
		return false, nil
	}

	// Extract metric names from the check output
	var metricNames []string
	for _, d := range data {
		for _, metric := range d.Aggregator.Metrics {
			metricNames = append(metricNames, metric.Metric)
		}
	}

	s.T().Logf("Check %s emitted %d metrics: %v", checkName, len(metricNames), metricNames)
	return len(metricNames) > 0, metricNames
}

// ============================================================================
// Test Functions
// ============================================================================

// TestWLANCheckInEUDMMode verifies that the wlan check is scheduled and runs
// in EUDM (end_user_device) infrastructure mode.
// Note: On EC2 instances without WiFi hardware, the check will emit a warning
// status (system.wlan.status = 1) with reason:interface_inactive tag.
func (s *eudmSuite) TestWLANCheckInEUDMMode() {
	s.T().Run("wlan_check_scheduled", func(t *testing.T) {
		t.Logf("Verifying wlan check is scheduled in EUDM mode...")

		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			s.verifyCheckSchedulingViaStatusAPI(c, "wlan", true)
		}, 2*time.Minute, 10*time.Second, "wlan check should be scheduled in EUDM mode")
	})

	s.T().Run("wlan_check_runs", func(t *testing.T) {
		t.Logf("Verifying wlan check runs successfully...")

		ran := s.verifyCheckRuns("wlan")
		assert.True(t, ran, "wlan check must run in EUDM mode")
	})

	s.T().Run("wlan_check_emits_metrics", func(t *testing.T) {
		t.Logf("Verifying wlan check emits metrics...")

		hasMetrics, metricNames := s.verifyCheckEmitsMetrics("wlan")
		assert.True(t, hasMetrics, "wlan check must emit metrics")

		// On EC2 without WiFi, we expect at least the status metric
		// system.wlan.status should be emitted with value 1 (warning) and reason:interface_inactive
		found := false
		for _, name := range metricNames {
			if name == "system.wlan.status" {
				found = true
				break
			}
		}
		assert.True(t, found, "wlan check should emit system.wlan.status metric")
	})
}
