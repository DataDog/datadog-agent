// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
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
)

var userDataHelloWorld = `#!/bin/bash

set -ex

export DEBIAN_FRONTEND=noninteractive

apt -y update && apt -y install docker.io
`

func TestAgentHelloWorld(t *testing.T) {
	config := auto.ConfigMap{}
	env := aws.NewSandboxEnvironment(config)
	credentialsManager := credentials.NewManager()

	// Retrieving necessary secrets
	sshKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.awssandbox.ssh")
	require.NoError(t, err)

	apiKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	require.NoError(t, err)

	stack, err := infra.NewStack(context.Background(), "ci", "ci-agent-docker-vm-hello-world", config, func(ctx *pulumi.Context) error {
		instance, err := ec2.CreateEC2Instance(ctx, env, "agent-ci-docker", "", "amd64", "t3.large", "agent-ci-sandbox", userDataHelloWorld)
		if err != nil {
			return err
		}

		ctx.Export("private-ip", instance.PrivateIp)
		return nil
	})
	defer stack.Down(context.Background())
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

	waitForDocker := true
	for waitForDocker {
		stdout, _ := clients.ExecuteCommand(client, "docker info")
		waitForDocker = strings.Contains(stdout, "docker: command not found")
	}

	// run the agent container on the VM
	_, err = clients.ExecuteCommand(client, fmt.Sprintf("sudo docker run -d --cgroupns host"+
		" --name dd-agent"+
		" -v /var/run/docker.sock:/var/run/docker.sock:ro"+
		" -v /proc/:/host/proc/:ro"+
		" -v /dd/config/:/etc/datadog-agent/"+
		" -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro"+
		" -e DD_API_KEY=%s public.ecr.aws/datadog/agent:7", apiKey))
	require.NoError(t, err)

	// run "echo hello world" on the agent's container
	stdout, err := clients.ExecuteCommand(client, "sudo docker exec dd-agent sh -c \"echo hello world\"")
	t.Log(stdout)
	require.NoError(t, err)
}
