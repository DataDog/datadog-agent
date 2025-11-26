// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type dockerPermissionSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestDockerPermissionSuite runs the docker permission health check test
func TestDockerPermissionSuite(t *testing.T) {
	e2e.Run(t, &dockerPermissionSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithAgentOptions(
				agentparams.WithAgentConfig(`health_platform:
  enabled: true
  forwarder:
    interval_minutes: 1
logs_enabled: true
logs_config:
  container_collect_all: true
logs:
  - type: docker
    service: docker
    container_all: true
`),
			),
		)),
	)
}

// TestDockerPermissionIssue tests that the agent detects and reports docker permission issues
func (suite *dockerPermissionSuite) TestDockerPermissionIssue() {
	host := suite.Env().RemoteHost
	fakeIntake := suite.Env().FakeIntake.Client()

	// Install Docker CE on the host
	suite.T().Log("Installing Docker CE...")
	installDockerScript := `
# Update & tools
sudo apt-get update -y
sudo apt-get install -y curl ca-certificates gnupg lsb-release

# Install Docker CE (official repo)
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update -y
sudo apt-get install -y docker-ce docker-ce-cli containerd.io

# Verify Docker is installed and running
sudo docker --version
sudo systemctl enable --now docker
sudo systemctl status docker --no-pager
`
	host.MustExecute(installDockerScript)

	// Verify agent cannot access docker containers directory
	accessCheck, _ := host.Execute("sudo -u dd-agent bash -c 'ls /var/lib/docker/containers 2>&1'")
	if !strings.Contains(accessCheck, "Permission denied") {
		suite.T().Log("Warning: dd-agent can access docker directory, test may not trigger the issue")
	}

	// Create containers to generate logs and trigger Docker log collection
	suite.T().Log("Creating 5 containers and restarting agent...")
	for i := 1; i <= 5; i++ {
		host.MustExecute(fmt.Sprintf(
			`sudo docker run -d --name spam%d --log-opt max-size=10m --log-opt max-file=2 alpine sh -c 'while true; do echo "container-%d: $(date)"; sleep 0.5; done'`,
			i, i))
	}

	host.MustExecute("sudo systemctl restart datadog-agent")
	time.Sleep(30 * time.Second)

	// Wait for health report to be sent to fake intake (with retry)
	suite.T().Log("Waiting for health report...")
	var healthReports []api.Payload
	var getErr error

	maxRetries := 18
	retryInterval := 10 * time.Second

	for i := 0; i < maxRetries; i++ {
		healthReports, getErr = getAgentHealthPayloads(fakeIntake)
		if getErr == nil && len(healthReports) > 0 {
			break
		}
		time.Sleep(retryInterval)
	}

	require.NoError(suite.T(), getErr, "Failed to get health reports from fake intake")
	require.NotEmpty(suite.T(), healthReports, "No health reports received")

	var report HealthReport
	err := json.Unmarshal(healthReports[len(healthReports)-1].Data, &report)
	require.NoError(suite.T(), err, "Failed to parse health report")

	// Verify the report structure
	assert.Equal(suite.T(), "1.0", report.SchemaVersion)
	assert.Equal(suite.T(), "agent-health-issues", report.EventType)
	assert.NotEmpty(suite.T(), report.EmittedAt)
	assert.NotEmpty(suite.T(), report.Host.Hostname)
	assert.NotEmpty(suite.T(), report.Host.AgentVersion)

	// Verify docker permission issue is present
	dockerIssueFound := false
	for _, issue := range report.Issues {
		if issue.ID == "docker-file-tailing-disabled" {
			dockerIssueFound = true
			assert.Equal(suite.T(), "docker", issue.Category)
			assert.Contains(suite.T(), issue.Title, "Docker File Tailing Disabled")
			assert.NotEmpty(suite.T(), issue.Description)
			assert.Contains(suite.T(), issue.Tags, "integration:docker")
			suite.T().Log("✓ Docker permission issue found and validated")
			break
		}
	}

	assert.True(suite.T(), dockerIssueFound, "Docker permission issue not found in health report")

	// Cleanup
	host.MustExecute("sudo docker stop $(sudo docker ps -aq) 2>/dev/null || true")
	host.MustExecute("sudo docker rm $(sudo docker ps -aq) 2>/dev/null || true")
}
