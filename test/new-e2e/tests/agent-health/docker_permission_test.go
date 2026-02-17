// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agenthealth contains E2E tests for the agent health reporting functionality.
package agenthealth

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	expectedIssueID = "docker-file-tailing-disabled"
)

type dockerPermissionSuite struct {
	e2e.BaseSuite[dockerPermissionEnv]
}

// TestDockerPermissionSuite runs the docker permission health check test
func TestDockerPermissionSuite(t *testing.T) {
	e2e.Run(t, &dockerPermissionSuite{},
		e2e.WithPulumiProvisioner(dockerPermissionEnvProvisioner(), nil),
	)
}

// TestDockerPermissionIssue tests that the agent detects and reports docker permission issues
func (suite *dockerPermissionSuite) TestDockerPermissionIssue() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fakeIntake := suite.Env().Fakeintake.Client()

	// Containers are already deployed by the provisioner via Docker Compose
	// The agent should detect that it cannot access the Docker socket
	suite.T().Log("Verifying agent health reports...")

	// Verify agent is running
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		assert.True(t, agent.Client.IsReady(), "Agent should be ready")
	}, 2*time.Minute, 10*time.Second, "Agent not ready")

	// Verify containers are running
	output := host.MustExecute("docker ps --format '{{.Names}}' | grep spam")
	suite.T().Logf("Running containers: %s", output)
	assert.Contains(suite.T(), output, "spam", "Busybox containers should be running")

	// Wait for health report to be sent to fakeintake
	var healthPayloads []*aggregator.AgentHealthPayload
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		var err error
		healthPayloads, err = fakeIntake.GetAgentHealth()
		assert.NoError(t, err)
		assert.NotEmpty(t, healthPayloads)
	}, 2*time.Minute, 10*time.Second, "Health report not received in FakeIntake within timeout")

	// Get the most recent health report
	require.NotEmpty(suite.T(), healthPayloads, "No health payloads received")
	latestReport := healthPayloads[len(healthPayloads)-1]
	require.NotNil(suite.T(), latestReport.HealthReport, "Health report is nil")

	// Verify docker permission issue is present
	var dockerIssue *healthplatform.Issue
	for _, issue := range latestReport.Issues {
		if issue.Id == expectedIssueID {
			dockerIssue = issue
			break
		}
	}

	// Build debug message with all found issues if expected one is missing
	if dockerIssue == nil {
		var debugMsg strings.Builder
		debugMsg.WriteString(fmt.Sprintf("\nExpected issue not found. Found %d issues:", len(latestReport.Issues)))
		count := 1
		for _, issue := range latestReport.Issues {
			debugMsg.WriteString(fmt.Sprintf("\n  #%d: ID='%s', Category='%s', Tags=%v", count, issue.Id, issue.Category, issue.Tags))
			count++
		}
		require.Fail(suite.T(), fmt.Sprintf("Docker permission issue '%s' not found in health report%s", expectedIssueID, debugMsg.String()))
		return
	}

	// Verify issue metadata
	assert.Equal(suite.T(), "docker-file-tailing-disabled", dockerIssue.Id)
	assert.Equal(suite.T(), "docker_file_tailing_disabled", dockerIssue.IssueName)
	assert.Equal(suite.T(), "permissions", dockerIssue.Category)
	assert.Equal(suite.T(), "medium", dockerIssue.Severity)
	assert.Equal(suite.T(), "logs-agent", dockerIssue.Location)
	assert.Equal(suite.T(), "logs", dockerIssue.Source)

	// Verify issue tags
	assert.Contains(suite.T(), dockerIssue.Tags, "docker")
	assert.Contains(suite.T(), dockerIssue.Tags, "logs")
	assert.Contains(suite.T(), dockerIssue.Tags, "permissions")
	assert.Contains(suite.T(), dockerIssue.Tags, "file-tailing")

	// Verify remediation is provided
	assert.NotNil(suite.T(), dockerIssue.Remediation, "Remediation should be provided")
	assert.NotEmpty(suite.T(), dockerIssue.Remediation.Summary, "Remediation summary should not be empty")
	assert.NotEmpty(suite.T(), dockerIssue.Remediation.Steps, "Remediation steps should not be empty")

	suite.T().Logf("✅ Test passed! Docker permission issue detected: %s", dockerIssue.Id)
}
