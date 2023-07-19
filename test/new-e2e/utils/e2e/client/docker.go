// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/stretchr/testify/require"
)

var _ clientService[docker.ClientData] = (*Docker)(nil)

// A docker client that is connected to an [docker.Deamon].
//
// [docker.Deamon]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent/docker#Deamon
type Docker struct {
	agent *AgentCommandRunner
	*UpResultDeserializer[docker.ClientData]
	t                  *testing.T
	client             *client.Client
	agentContainerName string
	os                 os.OS
}

// Create a new instance of Docker
func NewDocker(daemon *docker.Daemon) *Docker {
	dockerInstance := &Docker{
		agentContainerName: daemon.GetAgentContainerName(),
		os:                 daemon.GetOS(),
	}
	dockerInstance.UpResultDeserializer = NewUpResultDeserializer[docker.ClientData](daemon, dockerInstance)
	return dockerInstance
}

//lint:ignore U1000 Ignore unused function as this function is call using reflection
func (docker *Docker) initService(t *testing.T, data *docker.ClientData) error {
	docker.t = t

	deamonURL := fmt.Sprintf("ssh://%v@%v:22", data.Connection.User, data.Connection.Host)
	helper, err := connhelper.GetConnectionHelperWithSSHOpts(deamonURL, []string{"-o", "StrictHostKeyChecking no"})

	if err != nil {
		return fmt.Errorf("cannot connect to docker %v: %v", deamonURL, err)
	}

	opts := []client.Opt{
		client.WithDialContext(helper.Dialer),
		client.WithAPIVersionNegotiation(),
	}

	docker.client, err = client.NewClientWithOpts(opts...)
	if docker.agentContainerName != "" {
		docker.agent = newAgentCommandRunner(t, &dockerAgentCommand{docker: docker})
	}
	return err
}

// ExecuteCommand executes a command on containerName and returns the output.
func (docker *Docker) ExecuteCommand(containerName string, commands ...string) string {
	output, err := docker.ExecuteCommandWithErr(containerName, commands...)
	require.NoErrorf(docker.t, err, "%v: %v", output, err)
	return output
}

// ExecuteCommandWithErr executes a command on containerName and returns the output and an error.
func (docker *Docker) ExecuteCommandWithErr(containerName string, commands ...string) (string, error) {
	output, errOutput, err := docker.ExecuteCommandStdoutStdErr(containerName, commands...)
	if len(errOutput) != 0 {
		output += " " + errOutput
	}
	return output, err
}

// ExecuteCommandStdoutStdErr executes a command on containerName and returns the output, the error output and an error.
func (docker *Docker) ExecuteCommandStdoutStdErr(containerName string, commands ...string) (string, string, error) {
	context := context.Background()
	execConfig := types.ExecConfig{Cmd: commands, AttachStderr: true, AttachStdout: true}
	execCreateResp, err := docker.client.ContainerExecCreate(context, containerName, execConfig)
	require.NoError(docker.t, err)

	execAttachResp, err := docker.client.ContainerExecAttach(context, execCreateResp.ID, types.ExecStartCheck{})
	require.NoError(docker.t, err)
	defer execAttachResp.Close()

	var outBuf, errBuf bytes.Buffer
	// Use stdcopy.StdCopy to remove prefix for stdout and stderr
	// See https://stackoverflow.com/questions/52774830/docker-exec-command-from-golang-api for additional context
	_, err = stdcopy.StdCopy(&outBuf, &errBuf, execAttachResp.Reader)
	require.NoError(docker.t, err)

	execInspectResp, err := docker.client.ContainerExecInspect(context, execCreateResp.ID)
	require.NoError(docker.t, err)

	output := outBuf.String()
	errOutput := errBuf.String()

	if execInspectResp.ExitCode != 0 {
		return "", "", fmt.Errorf("error when running command %v on container %v: %v %v", commands, containerName, output, errOutput)
	}

	return output, errOutput, err
}

// GetClient gets the [docker client].
//
// [docker client]: https://pkg.go.dev/github.com/docker/docker/client
func (docker *Docker) GetClient() *client.Client {
	return docker.client
}

// GetAgentContainerName gets the agent container name
func (docker *Docker) GetAgentContainerName() string {
	require.NotEmptyf(docker.t, docker.agentContainerName, "agent container not found")
	return docker.agentContainerName
}

// GetAgentCommandRunner gets an agent that provides high level methods to run Agent commands.
func (docker *Docker) GetAgentCommandRunner() *AgentCommandRunner {
	require.NotNilf(docker.t, docker.agent, "there is no agent installed on this docker instance")
	return docker.agent
}

var _ agentRawCommandRunner = (*dockerAgentCommand)(nil)

// dockerAgentCommand is a wrapper to execute Agent commands on Docker.
type dockerAgentCommand struct {
	docker *Docker
}

func (agentCmd *dockerAgentCommand) ExecuteWithError(commands []string) (string, error) {
	wholeCommands := []string{"agent"}
	wholeCommands = append(wholeCommands, commands...)
	return agentCmd.docker.ExecuteCommandWithErr(agentCmd.docker.GetAgentContainerName(), wholeCommands...)
}
