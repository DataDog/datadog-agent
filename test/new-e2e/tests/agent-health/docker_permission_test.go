// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type dockerPermissionSuite struct {
	e2e.BaseSuite[dockerPermissionEnv]
}

// TestDockerPermissionSuite runs the docker permission health check test.
func TestDockerPermissionSuite(t *testing.T) {
	e2e.Run(t, &dockerPermissionSuite{},
		e2e.WithPulumiProvisioner(dockerPermissionEnvProvisioner(), nil),
	)
}

// TestDockerPermissionIssueLifecycle tests the full lifecycle of a docker
// socket permission issue using the standard health issue lifecycle helper:
//
//  1. IssueDetection  – agent detects the issue via `agent diagnose` and fakeintake
//  2. RestartResilience – issue persists as ONGOING after agent restart
//  3. Resolution – chmod 666 + restart makes the issue disappear from diagnose
func (suite *dockerPermissionSuite) TestDockerPermissionIssueLifecycle() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fi := suite.Env().Fakeintake.Client()

	// Verify containers are running before the lifecycle phases start.
	suite.T().Run("PreCondition", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready")

		containers, err := suite.Env().Docker.Client.ListContainers()
		require.NoError(t, err)
		found := false
		for _, name := range containers {
			if strings.Contains(name, "spam") {
				found = true
				break
			}
		}
		assert.True(t, found, "busybox spam containers should be running")
	})

	RunHealthIssueLifecycle(suite.T(),
		HealthIssueTestCase{
			// IssueName is a substring of the Title built by dockerpermissions.Issue.BuildIssue().
			IssueName: "Docker",
			IssueID:   "docker-socket-permissions",

			// TriggerIssue restores the broken state (used by Cleanup after Resolution).
			// The initial broken state comes from the provisioner (agent has no Docker access).
			TriggerIssue: func(t *testing.T, h *components.RemoteHost) {
				h.MustExecute("sudo chmod 660 /var/run/docker.sock")
			},

			// FixIssue grants world-access so dd-agent can reach the socket.
			FixIssue: func(t *testing.T, h *components.RemoteHost) {
				h.MustExecute("sudo chmod 666 /var/run/docker.sock")
				perm := h.MustExecute("stat -c '%a' /var/run/docker.sock")
				assert.Contains(t, strings.TrimSpace(perm), "666", "docker socket should be world-accessible")
			},

			// AssertMetadata validates the fakeintake payload fields.
			AssertMetadata: func(t *testing.T, issue *healthplatform.Issue) {
				assert.Equal(t, "docker-socket-permissions", issue.Id)
				assert.Equal(t, "docker_file_tailing_disabled", issue.IssueName)
				assert.Equal(t, "permissions", issue.Category)
				assert.Equal(t, "logs-agent", issue.Location)
				assert.Equal(t, "logs", issue.Source)
				assert.Contains(t, issue.Tags, "docker")
				assert.Contains(t, issue.Tags, "permissions")
				require.NotNil(t, issue.Remediation, "remediation should be provided")
				assert.NotEmpty(t, issue.Remediation.Summary)
				assert.NotEmpty(t, issue.Remediation.Steps)
			},
		},
		agent,
		host,
		fi,
	)
}
