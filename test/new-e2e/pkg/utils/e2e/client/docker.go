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

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/require"
)

var _ pulumiStackInitializer = (*Docker)(nil)

// A Docker client that is connected to an [docker.Deamon].
//
// [docker.Deamon]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent/docker#Deamon
type Docker struct {
	optionalAgent      Agent
	deserializer       utils.RemoteServiceDeserializer[docker.ClientData]
	t                  *testing.T
	client             *client.Client
	agentContainerName string
	os                 os.OS
}

// NewDocker creates a new instance of Docker
func NewDocker(daemon *docker.Daemon) *Docker {
	return &Docker{
		agentContainerName: daemon.GetAgentContainerName(),
		os:                 daemon.GetOS(),
		deserializer:       daemon,
	}
}

// initFromPulumiStack initializes the instance from the data stored in the pulumi stack.
// This method is called by [CallStackInitializers] using reflection.
//
//lint:ignore U1000 Ignore unused function as this function is called using reflection
func (docker *Docker) initFromPulumiStack(t *testing.T, stackResult auto.UpResult) error {
	clientData, err := docker.deserializer.Deserialize(stackResult)
	if err != nil {
		return err
	}
	docker.t = t

	deamonURL := fmt.Sprintf("ssh://%v@%v:22", clientData.Connection.User, clientData.Connection.Host)
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
		docker.optionalAgent = newAgentCommandRunner(t, docker.executeAgentCmdWithError)
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

// GetAgent gets an instance that implements the [Agent] interface. This function panics, if there is no agent container.
func (docker *Docker) GetAgent() Agent {
	require.NotNilf(docker.t, docker.optionalAgent, "there is no agent installed on this docker instance")
	return docker.optionalAgent
}

func (docker *Docker) executeAgentCmdWithError(commands []string) (string, error) {
	wholeCommands := []string{"agent"}
	wholeCommands = append(wholeCommands, commands...)
	return docker.ExecuteCommandWithErr(docker.GetAgentContainerName(), wholeCommands...)
}
