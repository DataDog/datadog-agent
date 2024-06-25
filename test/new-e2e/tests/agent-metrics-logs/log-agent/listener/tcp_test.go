// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package listener

import (
	"context"
	_ "embed"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metrics-logs/log-agent/utils"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"

	appslogger "github.com/DataDog/test-infra-definitions/components/datadog/apps/logger"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"

	"github.com/docker/docker/api/types"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/tcp-compose.yaml
var tcpCompose string

type dockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestTCPListener(t *testing.T) {
	t.Skip("Skipping as don't currently support using labels to spin up TCP/UDP listener")
	e2e.Run(t, &dockerSuite{}, e2e.WithProvisioner(
		awsdocker.Provisioner(
			awsdocker.WithAgentOptions(
				dockeragentparams.WithExtraComposeManifest("logger", appslogger.DockerComposeManifest.Content),
				dockeragentparams.WithExtraComposeManifest("logger-tcp", pulumi.String(tcpCompose)),
			))))
}

func (d *dockerSuite) TestLogsReceived() {
	d.EventuallyWithT(func(c *assert.CollectT) {
		agentReady := d.Env().Agent.Client.IsReady()
		assert.True(c, agentReady)
	}, 1*time.Minute, 5*time.Second, "Agent was not ready")
	agentVersion := d.Env().Agent.Client.Version()
	d.T().Logf("Testing Agent Version '%v'\n", agentVersion)
	statusOutput := d.Env().Agent.Client.Status().Content
	d.T().Logf("Agent status:\n %v", statusOutput)
	containerID, err := d.getLoggerContainerID()
	require.NoError(d.T(), err)
	assert.NotEmpty(d.T(), containerID)

	dc := d.Env().Docker.GetClient()

	// Command to execute inside the container
	cmd := []string{
		"curl", "-v",
		"--header", "\"Content-Type: application/json\"",
		"--request", "POST",
		"--data", "'{\"message\":\"bob\"}'",
		"localhost:3333/",
	}

	// Prepare the execution configuration
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	// Create the execution request
	execResponse, err := dc.ContainerExecCreate(context.Background(), containerID, execConfig)
	require.NoError(d.T(), err)

	// Run the command in the container
	execID := execResponse.ID
	execStartCheck := types.ExecStartCheck{}
	respID, err := dc.ContainerExecAttach(context.Background(), execID, execStartCheck)
	require.NoError(d.T(), err)

	// Wait for command execution to complete
	_, err = dc.ContainerExecInspect(context.Background(), execID)
	assert.NoError(d.T(), err)

	all, err := io.ReadAll(respID.Reader)
	require.NoError(d.T(), err)
	d.T().Log(string(all))
	utils.CheckLogsExpected(d.T(), d.Env().FakeIntake, "test-app", "bob", []string{})
}

// getLoggerContainerID returns the container ID of the logger app container
func (d *dockerSuite) getLoggerContainerID() (string, error) {
	containers, err := d.Env().Docker.GetClient().ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		return "", err
	}
	for _, ctr := range containers {
		d.T().Logf("Got container: %s %s | %v\n", ctr.ID, ctr.Image, ctr.Names)
		if ctr.Image == "ghcr.io/datadog/apps-logger:main" {
			return ctr.ID, nil
		}
	}
	return "", errors.New("could not find logger app container")
}
