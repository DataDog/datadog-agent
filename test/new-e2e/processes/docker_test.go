// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/datadog-api-client-go/v2/api/common"
	"github.com/DataDog/datadog-api-client-go/v2/api/v2/datadog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	containerName = "dd-agent"
	agentImage    = "public.ecr.aws/datadog/agent:7"
)

type DockerTestSuite struct {
	suite.Suite

	ec2           *EC2TestEnv
	ddAPIClient   *common.APIClient
	procAPIClient *datadog.ProcessesApi
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestDockerTestSuite(t *testing.T) {
	suite.Run(t, new(DockerTestSuite))
}

func (s *DockerTestSuite) SetupSuite() {
	var err error

	s.ec2, err = NewEC2TestEnv("process-agent-docker-test")
	require.NoError(s.T(), err)

	s.waitForDocker()

	configuration := common.NewConfiguration()
	s.ddAPIClient = common.NewAPIClient(configuration)
	s.procAPIClient = datadog.NewProcessesApi(s.ddAPIClient)
}

func (s *DockerTestSuite) TearDownSuite() {
	s.ec2.Close()
}

func (s *DockerTestSuite) TearDownTest() {
	s.T().Log("terminating docker containers")
	stdout, err := clients.ExecuteCommand(s.ec2.sshClient, "sudo docker kill $(sudo docker ps -aq)")
	if err != nil {
		s.T().Logf("error terminating docker containers. stdout:%s, err: %v", stdout, err)
	}

	stdout, err = clients.ExecuteCommand(s.ec2.sshClient, "sudo docker rm $(sudo docker ps -aq)")
	if err != nil {
		s.T().Logf("error removing docker containers. stdout:%s, err: %v", stdout, err)
	}
	s.T().Log("docker containers terminated")
}

func (s *DockerTestSuite) TestDockerAgent() {
	// TODO:
	//  - setup a custom hostname with DD_HOSTNAME
	//  - Enable process-agent
	//  - Add public API check for processes and containers

	hostName := createHostName(s.T().Name())

	s.T().Logf("start the datadog agent with hostname: %s", hostName)
	// run the agent container on the VM
	stdout, err := clients.ExecuteCommand(s.ec2.sshClient, fmt.Sprintf("sudo docker run -d --cgroupns host"+
		" --name %s"+
		" -v /var/run/docker.sock:/var/run/docker.sock:ro"+
		" -v /proc/:/host/proc/:ro"+
		" -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro"+
		" -v /dd/config/:/etc/datadog-agent/"+
		" -e DD_PROCESS_AGENT_ENABLED=true"+
		" -e DD_HOSTNAME=%s"+
		" -e DD_API_KEY=%s %s", containerName, hostName, s.ec2.ddAPIKey, agentImage))
	s.T().Log(stdout)
	require.NoError(s.T(), err)

	s.waitForProcessAgent()

	// run "echo hello world" on the agent's container
	stdout, err = clients.ExecuteCommand(s.ec2.sshClient, "sudo docker exec dd-agent sh -c \"echo hello world\"")
	s.T().Log(stdout)
	require.NoError(s.T(), err)

	ctx := common.NewDefaultContext(context.Background())
	assert.Eventually(s.T(), func() bool {
		resp, _, err := s.procAPIClient.ListProcesses(ctx, *datadog.NewListProcessesOptionalParameters().
			//WithSearch("process-agent").
			WithTags(fmt.Sprintf("host:%s", hostName)).
			WithPageLimit(20),
		)
		if err != nil {
			s.T().Logf("Error when calling `ProcessesApi.ListProcesses`: %v\n", err)
			return false
		}
		if procs, ok := resp.GetDataOk(); ok && procs != nil {
			if len(*procs) > 0 {
				s.T().Log("summaries")
				for _, summary := range *procs {
					s.T().Logf("%v", summary)
				}

				responseContent, _ := json.MarshalIndent(resp, "", "  ")
				s.T().Logf("Response from `ProcessesApi.ListProcesses`:\n%s\n", responseContent)

				return true
			}
		}
		return false
	}, 10*time.Minute, 1*time.Second)

}

// TODO: create config helper
// TODO: create tests that customize the config yaml
// TODO: update hosts values on a per test basis

// waitForDocker waits for the docker daemon to startup
func (s *DockerTestSuite) waitForDocker() {
	var stdout string
	require.Eventuallyf(s.T(), func() bool {
		stdout, _ := clients.ExecuteCommand(s.ec2.sshClient, "systemctl is-active docker")
		return !strings.Contains(stdout, "inactive")
	}, 2*time.Minute, 500*time.Millisecond, "docker service is not running")
	s.T().Logf("docker service is active: %s", stdout)

	require.Eventuallyf(s.T(), func() bool {
		stdout, _ = clients.ExecuteCommand(s.ec2.sshClient, "sudo docker info")
		return !strings.Contains(stdout, "docker: command not found")
	}, 2*time.Minute, 500*time.Millisecond, "docker is not running")

	s.T().Logf("docker is running: %s", stdout)
}

func (s *DockerTestSuite) waitForProcessAgent() {
	var stdout string
	require.Eventuallyf(s.T(), func() bool {
		// Get process log
		stdout, _ = clients.ExecuteCommand(s.ec2.sshClient, "sudo docker logs dd-agent | grep PROCESS")
		return strings.Contains(stdout, "Starting process-agent for")
	}, 2*time.Minute, 500*time.Millisecond, "process-agent is not running")

	s.T().Log("process-agent is running")
}

func createHostName(testName string) string {
	sl := strings.Split(testName, "/")
	hostName := fmt.Sprintf("%s-%d", sl[len(sl)-1], time.Now().UnixMilli())
	return hostName
}
