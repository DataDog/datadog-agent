// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	"golang.org/x/crypto/ssh"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/credentials"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"

	"github.com/vboulineau/pulumi-definitions/aws"
	"github.com/vboulineau/pulumi-definitions/aws/ec2/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const initScript = `#!/bin/bash

set -ex

export DEBIAN_FRONTEND=noninteractive

apt -y update && apt -y install docker.io
`

const (
	containerName = "dd-agent"
	agentImage    = "public.ecr.aws/datadog/agent:7"
)

type EC2TestSuite struct {
	suite.Suite

	ddAPIKey   string
	instanceIP string

	stack     *infra.Stack
	sshClient *ssh.Client
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestEC2TestSuite(t *testing.T) {
	suite.Run(t, new(EC2TestSuite))
}

func (s *EC2TestSuite) SetupSuite() {
	config := auto.ConfigMap{}
	env := aws.NewSandboxEnvironment(config)
	credentialsManager := credentials.NewManager()

	// Retrieving necessary secrets
	sshKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.awssandbox.ssh")
	require.NoError(s.T(), err)

	s.ddAPIKey, err = credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	require.NoError(s.T(), err)

	s.stack, err = infra.NewStack(context.Background(),
		"ci",
		"ci-agent-docker-vm-hello-world",
		config,
		func(ctx *pulumi.Context) error {
			instance, err := ec2.CreateEC2Instance(ctx, env, "process-agent-ci-docker", "", "amd64",
				"t3.large", "agent-ci-sandbox", initScript)
			if err != nil {
				return err
			}

			ctx.Export("private-ip", instance.PrivateIp)
			return nil
		})
	require.NoError(s.T(), err)
	require.NotNil(s.T(), s.stack)

	result, err := s.stack.Up(context.Background())
	require.NoError(s.T(), err)

	output, found := result.Outputs["private-ip"]
	require.True(s.T(), found)
	s.instanceIP = output.Value.(string)

	s.sshClient, _, err = clients.GetSSHClient("ubuntu", fmt.Sprintf("%s:%d", s.instanceIP, 22), sshKey, 2*time.Second, 30)
	require.NoError(s.T(), err)

	s.waitForDocker()
}

func (s *EC2TestSuite) TearDownSuite() {
	s.T().Log("tear down stack")
	_ = s.sshClient.Close()
	_ = s.stack.Down(context.Background())
}

func (s *EC2TestSuite) TearDownTest() {
	s.T().Log("terminating docker containers")
	stdout, err := clients.ExecuteCommand(s.sshClient, "sudo docker kill $(sudo docker ps -aq)")
	if err != nil {
		s.T().Logf("error terminating docker containers. stdout:%s, err: %v", stdout, err)
	}

	stdout, err = clients.ExecuteCommand(s.sshClient, "sudo docker rm $(sudo docker ps -aq)")
	if err != nil {
		s.T().Logf("error removing docker containers. stdout:%s, err: %v", stdout, err)
	}
	s.T().Log("docker containers terminated")
}

func (s *EC2TestSuite) TestDockerAgent() {
	//docker run -d
	// -v /var/run/docker.sock:/var/run/docker.sock:ro \
	// -v /proc/:/host/proc/:ro \
	// -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
	// -v /etc/datadog-agent/datadog.yaml:/etc/datadog-agent/datadog.yaml \
	// --name datadog-agent \
	// -e DD_API_KEY=$API_KEY \
	//$AGENT_TAG

	s.T().Log("run datadog agent")
	// run the agent container on the VM
	stdout, err := clients.ExecuteCommand(s.sshClient, fmt.Sprintf("sudo docker run -d --cgroupns host"+
		" --name %s"+
		" -v /var/run/docker.sock:/var/run/docker.sock:ro"+
		" -v /proc/:/host/proc/:ro"+
		" -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro"+
		" -v /dd/config/:/etc/datadog-agent/"+
		" -e DD_API_KEY=%s %s", containerName, s.ddAPIKey, agentImage))
	s.T().Log(stdout)
	require.NoError(s.T(), err)

	// run "echo hello world" on the agent's container
	stdout, err = clients.ExecuteCommand(s.sshClient, "sudo docker exec dd-agent sh -c \"echo hello world\"")
	s.T().Log(stdout)
	require.NoError(s.T(), err)
}

// TODO: create config helper
// TODO: create tests that customize the config yaml
// TODO: update hosts values on a per test basis

// waitForDocker waits for the docker daemon to startup
func (s *EC2TestSuite) waitForDocker() {
	var stdout string
	require.Eventuallyf(s.T(), func() bool {
		stdout, _ := clients.ExecuteCommand(s.sshClient, "systemctl is-active docker")
		return !strings.Contains(stdout, "inactive")
	}, 2*time.Minute, 500*time.Millisecond, "docker service is not running")
	s.T().Logf("docker service is active: %s", stdout)

	require.Eventuallyf(s.T(), func() bool {
		stdout, _ = clients.ExecuteCommand(s.sshClient, "sudo docker info")
		return !strings.Contains(stdout, "docker: command not found")
	}, 2*time.Minute, 500*time.Millisecond, "docker is not running")

	s.T().Logf("docker is running: %s", stdout)
}
