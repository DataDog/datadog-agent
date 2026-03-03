// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"

	"github.com/docker/docker/api/types/container"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

type agentDockerExecutor struct {
	dockerClient       *Docker
	agentContainerName string
}

var _ agentCommandExecutor = &agentDockerExecutor{}

func newAgentDockerExecutor(context common.Context, dockerAgentOutput agent.DockerAgentOutput) *agentDockerExecutor {
	dockerClient, err := NewDocker(context.T(), dockerAgentOutput.DockerManager)
	if err != nil {
		panic(err)
	}
	return &agentDockerExecutor{
		dockerClient:       dockerClient,
		agentContainerName: dockerAgentOutput.ContainerName,
	}
}

func (ae agentDockerExecutor) execute(arguments []string) (string, error) {
	// We consider that in a container, "agent" is always in path and is the single entrypoint.
	// It's mostly incorrect but it's what we have ATM.
	// TODO: Support all agents and Windows
	arguments = append([]string{"agent"}, arguments...)
	return ae.dockerClient.ExecuteCommandWithErr(ae.agentContainerName, arguments...)
}

func (ae agentDockerExecutor) restart() error {
	ctx := context.Background()
	timeout := 30
	return ae.dockerClient.client.ContainerRestart(ctx, ae.agentContainerName, container.StopOptions{Timeout: &timeout})
}
