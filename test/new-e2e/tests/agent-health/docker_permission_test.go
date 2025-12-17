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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

type dockerPermissionSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestDockerPermissionSuite runs the docker permission health check test
// Uses Amazon Linux ECS AMI which comes with Docker pre-installed
func TestDockerPermissionSuite(t *testing.T) {
	e2e.Run(t, &dockerPermissionSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(os.AmazonLinuxECSDefault)), // ECS AMI has Docker pre-installed
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

	// Docker is pre-installed on the ECS AMI, just create containers
	suite.T().Log("Creating Docker containers to trigger log collection...")
	host.MustExecute(`
sudo docker pull public.ecr.aws/docker/library/busybox:latest

for i in {1..5}; do
  sudo docker run -d \
    --name "spam$i" \
    --log-opt max-size=10m \
    --log-opt max-file=2 \
    busybox:latest \
    sh -c "while true; do echo container-$i: \$(date); sleep 0.5; done"
done

# Restart agent to pick up the new containers
sudo systemctl restart datadog-agent
`)

	// Wait for health report to be sent to fake intake
	var healthReports []*aggregator.AgentHealthPayload
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		var err error
		healthReports, err = fakeIntake.GetAgentHealth()
		assert.NoError(t, err)
		assert.NotEmpty(t, healthReports)
	}, 3*time.Minute, 10*time.Second, "Health report not received within timeout")

	// Get the latest health report
	report := healthReports[len(healthReports)-1]

	// Verify docker permission issue is present
	dockerIssue, found := report.Issues["docker-permission-check"]
	require.True(suite.T(), found, "Docker permission issue not found in health report")
	require.NotNil(suite.T(), dockerIssue)

	assert.Equal(suite.T(), "docker-permission-issue", dockerIssue.ID)
	assert.Equal(suite.T(), "permissions", dockerIssue.Category)
	assert.Contains(suite.T(), dockerIssue.Tags, "integration:docker")

	// Cleanup
	host.MustExecute("sudo docker stop $(sudo docker ps -aq) 2>/dev/null || true")
	host.MustExecute("sudo docker rm $(sudo docker ps -aq) 2>/dev/null || true")
}
