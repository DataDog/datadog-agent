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

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	osComp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

const (
	agentReadyTimeout = 1 * time.Minute
)

// NewHostAgentClient creates an Agent client for host install
func NewHostAgentClient(context e2e.Context, hostOutput remote.HostOutput, waitForAgentReady bool) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(hostOutput.OSFamily)
	params.ShouldWaitForReady = waitForAgentReady

	host, err := NewHost(context, hostOutput)
	if err != nil {
		return nil, err
	}

	ae := newAgentHostExecutor(hostOutput.OSFamily, host, params)
	commandRunner := newAgentCommandRunner(context.T(), ae)

	if params.ShouldWaitForReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

// NewHostAgentClientWithParams creates an Agent client for host install with custom parameters
func NewHostAgentClientWithParams(context e2e.Context, hostOutput remote.HostOutput, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(hostOutput.OSFamily, options...)

	host, err := NewHost(context, hostOutput)
	if err != nil {
		return nil, err
	}

	ae := newAgentHostExecutor(hostOutput.OSFamily, host, params)
	commandRunner := newAgentCommandRunner(context.T(), ae)

	if params.ShouldWaitForReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}
	waitForAgentsReady(context.T(), hostOutput.OSFamily, host, params)

	return commandRunner, nil
}

// NewDockerAgentClient creates an Agent client for a Docker install
func NewDockerAgentClient(context e2e.Context, dockerAgentOutput agent.DockerAgentOutput, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(dockerAgentOutput.DockerManager.Host.OSFamily, options...)
	ae := newAgentDockerExecutor(context, dockerAgentOutput)
	commandRunner := newAgentCommandRunner(context.T(), ae)

	if params.ShouldWaitForReady {
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
func waitForAgentsReady(tt *testing.T, osFamily osComp.Family, host *Host, params *agentclientparams.Params) {
	require.EventuallyWithT(tt, func(t *assert.CollectT) {
		agentReadyCmds := map[string]func(*agentclientparams.Params, osComp.Family, *Host) (string, bool, error){
			"process-agent":  processAgentCommand,
			"trace-agent":    traceAgentCommand,
			"security-agent": securityAgentCommand,
		}

		for name, getReadyCmd := range agentReadyCmds {
			cmd, ok, err := getReadyCmd(params, osFamily, host)
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

func ensureAuthToken(params *agentclientparams.Params, _ osComp.Family, host *Host) error {
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

func processAgentCommand(params *agentclientparams.Params, osFamily osComp.Family, host *Host) (string, bool, error) {
	return makeStatusEndpointCommand(params, osFamily, host, "http://localhost:%d/agent/status", params.ProcessAgentPort)
}

func traceAgentCommand(params *agentclientparams.Params, osFamily osComp.Family, host *Host) (string, bool, error) {
	return makeStatusEndpointCommand(params, osFamily, host, "http://localhost:%d/info", params.TraceAgentPort)
}

func securityAgentCommand(params *agentclientparams.Params, osFamily osComp.Family, host *Host) (string, bool, error) {
	return makeStatusEndpointCommand(params, osFamily, host, "https://localhost:%d/agent/status", params.SecurityAgentPort)
}

func makeStatusEndpointCommand(params *agentclientparams.Params, osFamily osComp.Family, host *Host, url string, port int) (string, bool, error) {
	if port == 0 {
		return "", false, nil
	}

	if osFamily != osComp.LinuxFamily {
		return "", true, fmt.Errorf("waiting for non-core agents is not implemented for OS family %d", osFamily)
	}

	// we want to fetch the auth token only if we actually need it
	if err := ensureAuthToken(params, osFamily, host); err != nil {
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
