// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processes

import (
	"context"
	"fmt"
	"golang.org/x/crypto/ssh"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

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
	ddAPIClient   *datadog.APIClient
	procAPIClient *datadogV2.ProcessesApi
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

	waitForDocker(s.T(), s.ec2.sshClient)

	configuration := datadog.NewConfiguration()
	s.ddAPIClient = datadog.NewAPIClient(configuration)
	s.procAPIClient = datadogV2.NewProcessesApi(s.ddAPIClient)
}

func (s *DockerTestSuite) TearDownSuite() {
	s.ec2.Close()
}

func (s *DockerTestSuite) TearDownTest() {
	killAndRemoveContainers(s.T(), s.ec2.sshClient)
}

func (s *DockerTestSuite) TestProcessAgentOnDocker() {
	hostName := createHostName(s.T().Name())
	hostTag := fmt.Sprintf("host:%s", hostName)

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

	// Start containers
	stdout, err = clients.ExecuteCommand(s.ec2.sshClient,
		"docker run --name spring-hello -p 8080:8080 -d springio/gs-spring-boot-docker")
	s.T().Log(stdout)
	assert.NoError(s.T(), err)

	stdout, err = clients.ExecuteCommand(s.ec2.sshClient,
		"docker run --name sleepy  -d busybox sleep 9999")
	s.T().Log(stdout)
	assert.NoError(s.T(), err)

	// Check processes are
	ctx := datadog.NewDefaultContext(context.Background())
	assert.Eventually(s.T(), func() bool {
		resp, _, err := s.procAPIClient.ListProcesses(ctx, *datadogV2.NewListProcessesOptionalParameters().
			WithSearch("java").
			WithTags(fmt.Sprintf("%s,container_name:spring-hello", hostTag)).
			WithPageLimit(2),
		)
		if err != nil {
			s.T().Logf("Error when calling `ProcessesApi.ListProcesses`go: %v\n", err)
			return false
		}
		if procs, ok := resp.GetDataOk(); ok && procs != nil {
			return len(*procs) == 1
		}
		return false
	}, 2*time.Minute, 1*time.Second)

	assert.Eventually(s.T(), func() bool {
		resp, _, err := s.procAPIClient.ListProcesses(ctx, *datadogV2.NewListProcessesOptionalParameters().
			WithTags(fmt.Sprintf("%s,container_name:sleepy", hostTag)).
			WithPageLimit(2),
		)
		if err != nil {
			s.T().Logf("Error when calling `ProcessesApi.ListProcesses`: %v\n", err)
			return false
		}
		if procs, ok := resp.GetDataOk(); ok && procs != nil {
			return len(*procs) == 1
		}
		return false
	}, 2*time.Minute, 1*time.Second)

}

func (s *DockerTestSuite) waitForProcessAgent() {
	var stdout string
	require.Eventuallyf(s.T(), func() bool {
		// Get process log
		stdout, _ = clients.ExecuteCommand(s.ec2.sshClient, "docker logs dd-agent | grep PROCESS")
		return strings.Contains(stdout, "Starting process-agent for")
	}, 2*time.Minute, 500*time.Millisecond, "process-agent is not running")

	s.T().Log("process-agent is running")
}

// waitForDocker waits for the docker daemon to startup
func waitForDocker(t *testing.T, sshClient *ssh.Client) {
	var stdout string
	require.Eventuallyf(t, func() bool {
		stdout, _ := clients.ExecuteCommand(sshClient, "systemctl is-active docker")
		return !strings.Contains(stdout, "inactive")
	}, 2*time.Minute, 500*time.Millisecond, "docker service is not running")
	t.Logf("docker service is active: %s", stdout)

	_, err := clients.ExecuteCommand(sshClient, "sudo chmod o+rw /var/run/docker.sock")
	require.NoError(t, err)

	require.Eventuallyf(t, func() bool {
		stdout, _ = clients.ExecuteCommand(sshClient, "docker info")
		return !strings.Contains(stdout, "docker: command not found")
	}, 2*time.Minute, 500*time.Millisecond, "docker is not running")
	t.Logf("docker is running: %s", stdout)
}

func killAndRemoveContainers(t *testing.T, sshClient *ssh.Client) {
	t.Log("terminating docker containers")
	stdout, err := clients.ExecuteCommand(sshClient, "docker kill $(sudo docker ps -aq)")
	if err != nil {
		t.Logf("error terminating docker containers. stdout:%s, err: %v", stdout, err)
	}

	stdout, err = clients.ExecuteCommand(sshClient, "docker rm $(sudo docker ps -aq)")
	if err != nil {
		t.Logf("error removing docker containers. stdout:%s, err: %v", stdout, err)
	}
	t.Log("docker containers terminated")
}
