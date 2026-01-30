// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	expectedIssueID = "docker-permission-issue"
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
	}, 3*time.Minute, 10*time.Second, "Health report not received in FakeIntake within timeout")

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

	require.NotNil(suite.T(), dockerIssue, "Docker permission issue not found in health report")
	assert.Equal(suite.T(), "permissions", dockerIssue.Category)
	assert.Contains(suite.T(), dockerIssue.Tags, "integration:docker")

	suite.T().Logf("✅ Test passed! Docker permission issue detected: %s", dockerIssue.Id)
}
