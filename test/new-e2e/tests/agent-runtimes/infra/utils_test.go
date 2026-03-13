// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	agentclient "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// ============================================================================
// Type Definitions
// ============================================================================

// collectorStatus represents the JSON structure of `agent status collector --json` output
type collectorStatus struct {
	RunnerStats struct {
		Checks map[string]map[string]check.Runner `json:"Checks"`
	} `json:"runnerStats"`
}

// ============================================================================
// Utility Functions
// ============================================================================

// getScheduledChecks retrieves the map of scheduled checks from the agent status
func getScheduledChecks(env *environments.Host) (map[string]map[string]check.Runner, error) {
	status := env.Agent.Client.Status(agentclient.WithArgs([]string{"collector", "--json"}))

	var statusMap collectorStatus
	err := json.Unmarshal([]byte(status.Content), &statusMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent status: %w", err)
	}

	return statusMap.RunnerStats.Checks, nil
}

// isCheckScheduled returns true if the check is scheduled and has run at least once
func isCheckScheduled(checkName string, checks map[string]map[string]check.Runner) bool {
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
func verifyCheckRuns(t *testing.T, env *environments.Host, checkName string) bool {
	// Run the check using the cross-platform Agent client helper
	output, err := env.Agent.Client.CheckWithError(agentclient.WithArgs([]string{checkName, "--json"}))
	if err != nil {
		t.Logf("Check %s failed to execute: %v", checkName, err)
		return false
	}

	// Parse the JSON output and check the Runner.TotalRuns field
	data := check.ParseJSONOutput(t, []byte(output))
	if len(data) == 0 {
		t.Logf("Check %s produced no output data", checkName)
		return false
	}

	// Check if the check actually ran by inspecting TotalRuns
	runner := data[0].Runner
	if runner.TotalRuns == 0 {
		t.Logf("Check %s did not run (TotalRuns=0, TotalErrors=%d, TotalWarnings=%d)",
			checkName, runner.TotalErrors, runner.TotalWarnings)
		return false
	}

	// Log success with runner statistics
	t.Logf("Check %s ran successfully (TotalRuns=%d, TotalErrors=%d, TotalWarnings=%d)",
		checkName, runner.TotalRuns, runner.TotalErrors, runner.TotalWarnings)
	return true
}

// verifyCheckSchedulingViaStatusAPI verifies that checks are in the expected scheduling state
// by querying the agent status API. This is a helper function meant to be called within EventuallyWithT.
func verifyCheckSchedulingViaStatusAPI(t *testing.T, c *assert.CollectT, env *environments.Host, checks []string, shouldBeScheduled bool) {
	scheduledChecks, err := getScheduledChecks(env)
	require.NoError(c, err, "Failed to get scheduled checks")

	t.Logf("Found %d check types in agent status", len(scheduledChecks))

	// Verify all checks match the expected scheduling state
	for _, checkName := range checks {
		scheduled := isCheckScheduled(checkName, scheduledChecks)

		// Log current state
		if scheduled {
			t.Logf("Check %s is scheduled", checkName)
		} else {
			t.Logf("Check %s is not scheduled", checkName)
		}

		// Assert expected state
		assert.Equal(c, shouldBeScheduled, scheduled, "Check %s scheduling state mismatch", checkName)
	}
}
