// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// ============================================================================
// Environment definition
// ============================================================================

type dockerPermissionEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

var _ common.Diagnosable = (*dockerPermissionEnv)(nil)

func (e *dockerPermissionEnv) Diagnose(outputDir string) (string, error) {
	var parts []string
	if e.Agent != nil {
		parts = append(parts, "==== Agent ====")
		dst, err := e.generateAndDownloadAgentFlare(outputDir)
		if err != nil {
			parts = append(parts, fmt.Sprintf("flare error: %v", err))
		} else {
			parts = append(parts, "flare: "+dst)
		}
	}
	if e.Docker != nil {
		parts = append(parts, "==== Docker ====")
		diag, err := e.diagnoseDocker()
		if err != nil {
			parts = append(parts, fmt.Sprintf("docker diag error: %v", err))
		} else {
			parts = append(parts, diag)
		}
	}
	return strings.Join(parts, "\n"), nil
}

func (e *dockerPermissionEnv) generateAndDownloadAgentFlare(outputDir string) (string, error) {
	if e.Agent == nil || e.RemoteHost == nil {
		return "", errors.New("agent or host not initialized")
	}
	out, err := e.Agent.Client.FlareWithError(agentclient.WithArgs([]string{"--email", "e2e-tests@datadog-agent", "--send"}))
	allOut := out
	if err != nil {
		allOut = out + "\n" + err.Error()
	}
	re := regexp.MustCompile(`(?m)^(.+\.zip) is going to be uploaded to Datadog$`)
	m := re.FindStringSubmatch(allOut)
	if len(m) < 2 {
		return "", fmt.Errorf("no flare archive path in output: %s", allOut)
	}
	flarePath := m[1]
	info, err := e.RemoteHost.Lstat(flarePath)
	if err != nil {
		return "", fmt.Errorf("stat flare: %w", err)
	}
	dst := filepath.Join(outputDir, info.Name())
	if err = e.RemoteHost.EnsureFileIsReadable(flarePath); err != nil {
		return "", fmt.Errorf("chmod flare: %w", err)
	}
	if err = e.RemoteHost.GetFile(flarePath, dst); err != nil {
		return "", fmt.Errorf("download flare: %w", err)
	}
	return dst, nil
}

func (e *dockerPermissionEnv) diagnoseDocker() (string, error) {
	var sb strings.Builder
	for _, c := range []struct{ label, cmd string }{
		{"containers", "docker ps -a"},
		{"socket perms", "ls -l /var/run/docker.sock"},
		{"dd-agent groups", "groups dd-agent"},
	} {
		out, err := e.RemoteHost.Execute(c.cmd)
		if err != nil {
			sb.WriteString(fmt.Sprintf("[%s] error: %v\n", c.label, err))
		} else {
			sb.WriteString(fmt.Sprintf("[%s]\n%s\n", c.label, out))
		}
	}
	return sb.String(), nil
}

// ============================================================================
// Test suite
// ============================================================================

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
// socket permission issue:
//
//  1. IssueDetection  – chmod 660 on the docker socket triggers the issue; it
//     appears in `agent diagnose` and in fakeintake.
//  2. RestartResilience – the issue persists as ONGOING after an agent restart.
//  3. Resolution – chmod 666 + restart makes the issue disappear from diagnose.
func (suite *dockerPermissionSuite) TestDockerPermissionIssueLifecycle() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fi := suite.Env().Fakeintake.Client()

	const (
		issueName = "Docker"
		issueID   = "docker-socket-permissions"
	)

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

	var initialFirstSeen string

	// =========================================================================
	// Phase 1: Issue Detection
	// =========================================================================
	suite.T().Run("IssueDetection", func(t *testing.T) {
		host.MustExecute("sudo chmod 660 /var/run/docker.sock")

		AssertIssueDetectedViaDiagnose(t, agent, issueName)

		issues := waitForIssuesInFakeintake(t, fi, issueID)
		require.NotEmpty(t, issues)
		issue := issues[0]
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

		for _, iss := range issues {
			if iss.PersistedIssue != nil && initialFirstSeen == "" {
				initialFirstSeen = iss.PersistedIssue.FirstSeen
			}
		}
	})

	// =========================================================================
	// Phase 2: Restart Resilience
	// =========================================================================
	suite.T().Run("RestartResilience", func(t *testing.T) {
		require.NoError(t, fi.FlushServerAndResetAggregators())
		require.NoError(t, agent.Client.Restart())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready after restart")

		AssertIssueDetectedViaDiagnose(t, agent, issueName)

		if initialFirstSeen != "" {
			issues := waitForIssuesInFakeintake(t, fi, issueID)
			require.NotEmpty(t, issues)
			require.NotNil(t, issues[0].PersistedIssue)
			assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_ONGOING, issues[0].PersistedIssue.State)
			assert.Equal(t, initialFirstSeen, issues[0].PersistedIssue.FirstSeen, "first_seen should be preserved across restart")
		}
	})

	// =========================================================================
	// Phase 3: Resolution
	// =========================================================================
	suite.T().Run("Resolution", func(t *testing.T) {
		// Restore broken state on cleanup so infra can be re-used for re-runs.
		t.Cleanup(func() {
			host.MustExecute("sudo chmod 660 /var/run/docker.sock")
			_ = agent.Client.Restart()
		})

		host.MustExecute("sudo chmod 666 /var/run/docker.sock")
		perm := host.MustExecute("stat -c '%a' /var/run/docker.sock")
		assert.Contains(t, strings.TrimSpace(perm), "666", "docker socket should be world-accessible")

		require.NoError(t, fi.FlushServerAndResetAggregators())
		require.NoError(t, agent.Client.Restart())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready after fix restart")

		AssertIssueAbsentViaDiagnose(t, agent, issueName)
	})
}
