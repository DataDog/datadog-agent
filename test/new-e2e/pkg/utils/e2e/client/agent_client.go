// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"strings"
	"testing"
	"time"

	osComp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

const (
	agentReadyTimeout = 1 * time.Minute
)

// NewHostAgentClient creates an Agent client for host install
func NewHostAgentClient(t *testing.T, host *components.RemoteHost, waitForAgentReady bool) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(host.OSFamily)
	params.ShouldWaitForReady = waitForAgentReady

	ae := newAgentHostExecutor(host, params)
	commandRunner := newAgentCommandRunner(t, ae)

	if params.ShouldWaitForReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

// NewHostAgentClientWithParams creates an Agent client for host install with custom parameters
func NewHostAgentClientWithParams(t *testing.T, host *components.RemoteHost, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(host.OSFamily, options...)

	ae := newAgentHostExecutor(host, params)
	commandRunner := newAgentCommandRunner(t, ae)

	if params.ShouldWaitForReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}
	waitForAgentsReady(t, host, params)

	return commandRunner, nil
}

// NewDockerAgentClient creates an Agent client for a Docker install
func NewDockerAgentClient(t *testing.T, docker *Docker, agentContainerName string, waitForAgentReady bool) (agentclient.Agent, error) {
	commandRunner := newAgentCommandRunner(t, newAgentDockerExecutor(docker, agentContainerName))

	if waitForAgentReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

// waitForAgentsReady waits for the given non-core agents to be ready.
// The given options configure which Agents to wait for, and how long to wait.
//
// Under the hood, this function checks the readiness of the agents by querying their status endpoints.
// The function will wait until all agents are ready, or until the timeout is reached.
// If the timeout is reached, an error is returned.
//
// As of now this is only implemented for Linux.
func waitForAgentsReady(tt *testing.T, host *components.RemoteHost, params *agentclientparams.Params) {
	require.EventuallyWithT(tt, func(t *assert.CollectT) {
		agentReadyCmds := map[string]func(*agentclientparams.Params, *components.RemoteHost) (string, bool, error){
			"process-agent":  processAgentCommand,
			"trace-agent":    traceAgentCommand,
			"security-agent": securityAgentCommand,
		}

		for name, getReadyCmd := range agentReadyCmds {
			cmd, ok, err := getReadyCmd(params, host)
			if !assert.NoErrorf(t, err, "could not build ready command for %s", name) {
				continue
			}

			if !ok {
				continue
			}

			tt.Logf("Checking if %s is ready...", name)
			_, err = host.Execute(cmd)
			assert.NoErrorf(t, err, "%s did not become ready", name)
		}
	}, params.WaitForDuration, params.WaitForTick)
}

func ensureAuthToken(params *agentclientparams.Params, host *components.RemoteHost) error {
	if params.AuthToken != "" {
		return nil
	}

	authToken, err := host.Execute("sudo cat " + params.AuthTokenPath)
	if err != nil {
		return fmt.Errorf("could not read auth token file: %v", err)
	}
	params.AuthToken = strings.TrimSpace(string(authToken))

	return nil
}

func processAgentCommand(params *agentclientparams.Params, host *components.RemoteHost) (string, bool, error) {
	return makeStatusEndpointCommand(params, host, "http://localhost:%d/agent/status", params.ProcessAgentPort)
}

func traceAgentCommand(params *agentclientparams.Params, host *components.RemoteHost) (string, bool, error) {
	return makeStatusEndpointCommand(params, host, "http://localhost:%d/info", params.TraceAgentPort)
}

func securityAgentCommand(params *agentclientparams.Params, host *components.RemoteHost) (string, bool, error) {
	return makeStatusEndpointCommand(params, host, "https://localhost:%d/agent/status", params.SecurityAgentPort)
}

func makeStatusEndpointCommand(params *agentclientparams.Params, host *components.RemoteHost, url string, port int) (string, bool, error) {
	if port == 0 {
		return "", false, nil
	}

	if host.OSFamily != osComp.LinuxFamily {
		return "", true, fmt.Errorf("waiting for non-core agents is not implemented for OS family %d", host.OSFamily)
	}

	// we want to fetch the auth token only if we actually need it
	if err := ensureAuthToken(params, host); err != nil {
		return "", true, err
	}

	statusEndpoint := fmt.Sprintf(url, port)
	return curlCommand(statusEndpoint, params.AuthToken), true, nil
}

func curlCommand(endpoint string, authToken string) string {
	return fmt.Sprintf(
		`curl -L -s -k -H "authorization: Bearer %s" "%s"`,
		authToken,
		endpoint,
	)
}
