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

	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/stretchr/testify/require"
)

// A Docker client that is connected to an [docker.Deamon].
//
// [docker.Deamon]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent/docker#Deamon
type Docker struct {
	t      *testing.T
	client *client.Client
}

// NewDocker creates a new instance of Docker
func NewDocker(t *testing.T, host remote.HostOutput) (*Docker, error) {
	deamonURL := fmt.Sprintf("ssh://%v@%v", host.Username, host.Address)

	helper, err := connhelper.GetConnectionHelperWithSSHOpts(deamonURL, []string{"-o", "StrictHostKeyChecking no"})
	if err != nil {
		return nil, fmt.Errorf("cannot connect to docker %v: %v", deamonURL, err)
	}

	opts := []client.Opt{
		client.WithDialContext(helper.Dialer),
		client.WithAPIVersionNegotiation(),
	}

	client, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("cannot create docker client: %v", err)
	}

	return &Docker{
		t:      t,
		client: client,
	}, nil
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
