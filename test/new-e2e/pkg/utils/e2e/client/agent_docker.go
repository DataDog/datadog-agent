// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

type agentDockerExecutor struct {
	dockerClient       *Docker
	agentContainerName string
}

var _ agentCommandExecutor = &agentDockerExecutor{}

func newAgentDockerExecutor(dockerClient *Docker, agentContainerName string) *agentDockerExecutor {
	return &agentDockerExecutor{
		dockerClient:       dockerClient,
		agentContainerName: agentContainerName,
	}
}

func (ae agentDockerExecutor) execute(arguments []string) (string, error) {
	// We consider that in a container, "agent" is always in path and is the single entrypoint.
	// It's mostly incorrect but it's what we have ATM.
	// TODO: Support all agents and Windows
	arguments = append([]string{"agent"}, arguments...)
	return ae.dockerClient.ExecuteCommandWithErr(ae.agentContainerName, arguments...)
}
