// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/agenthealth_bytes
var agenthealthBytes []byte

//go:embed fixtures/agenthealth_multi_bytes
var agenthealthMultiBytes []byte

//go:embed fixtures/agenthealth_noissues_bytes
var agenthealthNoIssuesBytes []byte

func TestAgentHealthAggregator(t *testing.T) {
	t.Run("parseAgentHealth empty JSON object should be ignored", func(t *testing.T) {
		agentHealth, err := ParseAgentHealthPayload(api.Payload{
			Data:     []byte("{}"),
			Encoding: encodingJSON,
		})
		assert.NoError(t, err)
		assert.Empty(t, agentHealth)
	})

	t.Run("parseAgentHealth valid body should parse health report", func(t *testing.T) {
		agentHealth, err := ParseAgentHealthPayload(api.Payload{Data: agenthealthBytes, Encoding: encodingDeflate})
		require.NoError(t, err)
		require.Equal(t, 1, len(agentHealth))

		payload := agentHealth[0]
		require.NotNil(t, payload.HealthReport)

		// Verify basic fields
		assert.Equal(t, "agent-health-issues", payload.EventType)
		assert.NotEmpty(t, payload.EmittedAt)

		// Verify host info
		require.NotNil(t, payload.Host)
		assert.Equal(t, "test-hostname", payload.Host.Hostname)
		assert.Equal(t, "7.50.0", payload.Host.GetAgentVersion())

		// Verify issues
		require.NotNil(t, payload.Issues)
		require.Len(t, payload.Issues, 1)

		// Verify the specific issue
		issue, ok := payload.Issues["check-id-123"]
		require.True(t, ok)
		assert.Equal(t, "docker-permissions-issue", issue.Id)
		assert.Equal(t, "Docker Permissions Issue", issue.IssueName)
		assert.Equal(t, "Docker socket permissions error", issue.Title)
		assert.Contains(t, issue.Description, "Unable to access Docker socket")
		assert.Equal(t, "permissions", issue.Category)
		assert.Equal(t, "core-agent", issue.Location)
		assert.Equal(t, "error", issue.Severity)
		assert.Equal(t, "docker", issue.Source)

		// Verify remediation
		require.NotNil(t, issue.Remediation)
		assert.Equal(t, "Fix Docker socket permissions", issue.Remediation.Summary)
		require.Len(t, issue.Remediation.Steps, 2)
		assert.Equal(t, int32(1), issue.Remediation.Steps[0].Order)
		assert.Contains(t, issue.Remediation.Steps[0].Text, "Add user to docker group")

		// Verify tags
		assert.Contains(t, issue.Tags, "os:linux")
		assert.Contains(t, issue.Tags, "docker:installed")
	})

	t.Run("parseAgentHealth with multiple issues", func(t *testing.T) {
		agentHealth, err := ParseAgentHealthPayload(api.Payload{Data: agenthealthMultiBytes, Encoding: encodingDeflate})
		require.NoError(t, err)
		require.Equal(t, 1, len(agentHealth))

		payload := agentHealth[0]
		require.NotNil(t, payload.Issues)
		assert.Len(t, payload.Issues, 2)
		assert.Contains(t, payload.Issues, "check-1")
		assert.Contains(t, payload.Issues, "check-2")
	})

	t.Run("parseAgentHealth with no issues", func(t *testing.T) {
		agentHealth, err := ParseAgentHealthPayload(api.Payload{Data: agenthealthNoIssuesBytes, Encoding: encodingDeflate})
		require.NoError(t, err)
		require.Equal(t, 1, len(agentHealth))

		payload := agentHealth[0]
		assert.NotNil(t, payload.Issues)
		assert.Empty(t, payload.Issues)
	})
}
