// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ndm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/credentials"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"golang.org/x/crypto/ssh"

	"github.com/vboulineau/pulumi-definitions/aws"
	"github.com/vboulineau/pulumi-definitions/aws/ec2/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
)

const (
	pulumiProjectName = "ci"
	pulumiStackName   = "ci-agent-ndm"

	ec2InstanceName = "agent-ci-docker"
	userData        = `#!/bin/bash

set -ex

export DEBIAN_FRONTEND=noninteractive

apt -y update && apt -y install docker.io
`

	dockerNetworkName          = "ndm-net"
	dockerAgentContainerName   = "dd-agent"
	dockerSnmpsimContainerName = "dd-snmpsim"
)

func TestSetup(t *testing.T) {
	config := auto.ConfigMap{}
	env := aws.NewSandboxEnvironment(config)
	credentialsManager := credentials.NewManager()

	// Retrieving necessary secrets
	sshKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.awssandbox.ssh")
	require.NoError(t, err)

	apiKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	require.NoError(t, err)

	stack, err := infra.NewStack(context.Background(), pulumiProjectName, pulumiStackName, config, func(ctx *pulumi.Context) error {
		instance, err := ec2.CreateEC2Instance(ctx, env, ec2InstanceName, "", "amd64", "t3.large", "agent-ci-sandbox", userData)
		if err != nil {
			return err
		}

		ctx.Export("private-ip", instance.PrivateIp)
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, stack)

	result, err := stack.Up(context.Background())
	require.NoError(t, err)
	instanceIP, found := result.Outputs["private-ip"]
	require.True(t, found)

	// Setup Agent
	t.Logf("Connecting through ssh client to %s", instanceIP.Value.(string))
	client, _, err := clients.GetSSHClient("ubuntu", fmt.Sprintf("%s:%d", instanceIP.Value.(string), 22), sshKey, 2*time.Second, 30)
	require.NoError(t, err)
	defer client.Close()

	// Wait for docker to be installed
	require.NoError(t, waitForDocker(t, client, 5*time.Minute))

	// create docker network
	stdout, err := clients.ExecuteCommand(client, fmt.Sprintf("sudo docker network create %s", dockerNetworkName))
	t.Log(stdout)
	require.NoError(t, err)

	// clone integrations core
	_, err = clients.ExecuteCommand(client, "sudo mkdir -p /repos/dd/integrations-core")
	require.NoError(t, err)

	t.Log("git clone integrations-core")
	stdout, err = clients.ExecuteCommand(client, "sudo git clone https://github.com/DataDog/integrations-core.git /repos/dd/integrations-core")
	t.Log(stdout)
	require.NoError(t, err)

	// run the agent container on the VM
	stdout, err = clients.ExecuteCommand(client, fmt.Sprintf("sudo docker run -d --cgroupns host"+
		" --name %s"+
		" -v /var/run/docker.sock:/var/run/docker.sock:ro"+
		" -v /proc/:/host/proc/:ro"+
		" -v /dd/config/:/etc/datadog-agent/"+
		" -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro"+
		" --network %s"+
		" -e DD_API_KEY=%s datadog/agent-dev:master", dockerAgentContainerName, dockerNetworkName, apiKey))
	t.Log(stdout)
	require.NoError(t, err)

	t.Log("sudo ls /dd/config")
	stdout, err = clients.ExecuteCommand(client, "sudo ls /dd/config")

	t.Log(stdout)

	require.NoError(t, err)

	stdout, err = clients.ExecuteCommand(client, fmt.Sprintf("sudo docker run -d --cgroupns host"+
		" --name %s"+
		" -v /repos/dd/integrations-core/snmp/tests/compose/data:/usr/snmpsim/data/"+
		" --network %s"+
		" datadog/docker-library:snmp", dockerSnmpsimContainerName, dockerNetworkName))

	t.Log(stdout)
	require.NoError(t, err)
}

func TestAgentSNMPWalk(t *testing.T) {
	config := auto.ConfigMap{}
	env := aws.NewSandboxEnvironment(config)
	credentialsManager := credentials.NewManager()

	// Retrieving necessary secrets
	sshKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.awssandbox.ssh")
	require.NoError(t, err)

	// apiKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	// require.NoError(t, err)

	stack, err := infra.NewStack(context.Background(), pulumiProjectName, pulumiStackName, config, func(ctx *pulumi.Context) error {
		instance, err := ec2.CreateEC2Instance(ctx, env, ec2InstanceName, "", "amd64", "t3.large", "agent-ci-sandbox", userData)
		if err != nil {
			return err
		}

		ctx.Export("private-ip", instance.PrivateIp)
		return nil
	})
	// defer stack.Down(context.Background()) < ==================
	require.NoError(t, err)
	require.NotNil(t, stack)

	result, err := stack.Up(context.Background())
	require.NoError(t, err)
	instanceIP, found := result.Outputs["private-ip"]
	require.True(t, found)

	// Setup Agent
	client, _, err := clients.GetSSHClient("ubuntu", fmt.Sprintf("%s:%d", instanceIP.Value.(string), 22), sshKey, 2*time.Second, 30)
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, waitForDocker(t, client, 5*time.Minute))

	t.Log("sudo ls /dd/config")
	stdout, err := clients.ExecuteCommand(client, "sudo ls /dd/config")

	t.Log(stdout)

	require.NoError(t, err)

	stdout, err = clients.ExecuteCommand(client, fmt.Sprintf("sudo docker exec %s sh -c \"agent snmp walk %s:1161 1.3.6.1.2.1.25.6.3.1 --community-string public\"", dockerAgentContainerName, dockerSnmpsimContainerName))
	t.Log(stdout)
	require.NoError(t, err)
}

func waitForDocker(t *testing.T, client *ssh.Client, timeout time.Duration) (err error) {
	// Wait for docker to be installed
	waitForDocker := true
	start := time.Now()
	for waitForDocker {
		if time.Since(start) > timeout {
			return errors.New("Timeout waiting for Docker")
		}
		stdout, _ := clients.ExecuteCommand(client, "docker version")
		waitForDocker = !strings.Contains(stdout, "Version")
		if waitForDocker {
			t.Log("Wait for docker")
			time.Sleep(100 * time.Millisecond)
		} else {
			t.Log(stdout)
		}
	}

	return nil
}
