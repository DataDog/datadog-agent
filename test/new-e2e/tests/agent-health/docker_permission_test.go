// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type dockerPermissionSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestDockerPermissionSuite runs the docker permission health check test
func TestDockerPermissionSuite(t *testing.T) {
	e2e.Run(t, &dockerPermissionSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(`health_platform:
  enabled: true
  forwarder:
    interval: 1
logs_enabled: true
logs_config:
  container_collect_all: true
logs:
  - type: docker
    service: docker
    container_all: true
`),
				),
			),
		)),
	)
}

// TestDockerPermissionIssue tests that the agent detects and reports docker permission issues
func (suite *dockerPermissionSuite) TestDockerPermissionIssue() {
	host := suite.Env().RemoteHost
	fakeIntake := suite.Env().FakeIntake.Client()

	// Install Docker CE, create containers, and restart agent
	suite.T().Log("Setting up Docker and creating containers...")
	host.MustExecute(`
sudo apt-get update -y
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update -y
sudo apt-get install -y docker-ce docker-ce-cli containerd.io
sudo systemctl enable --now docker
sudo docker pull public.ecr.aws/docker/library/busybox:latest

for i in {1..5}; do
  docker run -d \
    --name "spam$i" \
    --log-opt max-size=10m \
    --log-opt max-file=2 \
    busybox:latest \
    sh -c "while true; do echo container-$i: \$(date); sleep 0.5; done"
done

sudo systemctl restart datadog-agent
`)

	// Wait for health report to be sent to fake intake
	var healthReports []api.Payload
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		var err error
		healthReports, err = getAgentHealthPayloads(fakeIntake)
		assert.NoError(t, err)
		assert.NotEmpty(t, healthReports)
	}, 3*time.Minute, 10*time.Second, "Health report not received within timeout")

	var report HealthReport
	err := json.Unmarshal(healthReports[len(healthReports)-1].Data, &report)
	require.NoError(suite.T(), err, "Failed to parse health report")

	// Verify docker permission issue is present
	var dockerIssue *IssueReport
	for _, issue := range report.Issues {
		if issue.ID == "docker-permission-issue" {
			dockerIssue = issue
			break
		}
	}

	require.NotNil(suite.T(), dockerIssue, "Docker permission issue not found in health report")
	assert.Equal(suite.T(), "permissions", dockerIssue.Category)
	assert.Contains(suite.T(), dockerIssue.Tags, "integration:docker")

	// Cleanup
	host.MustExecute("sudo docker stop $(sudo docker ps -aq) 2>/dev/null || true")
	host.MustExecute("sudo docker rm $(sudo docker ps -aq) 2>/dev/null || true")
}
