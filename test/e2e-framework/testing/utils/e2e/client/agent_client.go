// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	osComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

const (
	agentReadyTimeout = 1 * time.Minute
)

// NewHostAgentClient creates an Agent client for host install
func NewHostAgentClient(context Context, hostOutput remote.HostOutput, waitForAgentReady bool) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(hostOutput.OSFamily)
	params.ShouldWaitForReady = waitForAgentReady

	host, err := NewHost(context, hostOutput)
	if err != nil {
		return nil, err
	}

	ae := newAgentHostExecutor(hostOutput.OSFamily, host, params)
	commandRunner := newAgentCommandRunner(context, ae)

	if params.ShouldWaitForReady {
		if err := waitForReadyTimeout(commandRunner, agentReadyTimeout); err != nil {
			return nil, err
		}
		commandRunner.isReady = true
	}

	return commandRunner, nil
}

// NewHostAgentClientWithParams creates an Agent client for host install with custom parameters
func NewHostAgentClientWithParams(context Context, hostOutput remote.HostOutput, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(hostOutput.OSFamily, options...)

	host, err := NewHost(context, hostOutput)
	if err != nil {
		return nil, err
	}

	ae := newAgentHostExecutor(hostOutput.OSFamily, host, params)
	commandRunner := newAgentCommandRunner(context, ae)

	if params.ShouldWaitForReady {
		if err := waitForReadyTimeout(commandRunner, agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	if err := waitForAgentsReady(context, host, params); err != nil {
		return nil, err
	}

	return commandRunner, nil
}

// NewDockerAgentClient creates an Agent client for a Docker install
func NewDockerAgentClient(context Context, dockerAgentOutput agent.DockerAgentOutput, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(dockerAgentOutput.DockerManager.Host.OSFamily, options...)
	ae := newAgentDockerExecutor(context, dockerAgentOutput)
	commandRunner := newAgentCommandRunner(context, ae)

	if params.ShouldWaitForReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

// NewK8sAgentClient creates an Agent client for a Kubernetes install, passing the ListOptions
// to select the pod that runs the agent. There are some helper functions to create common selectors,
// such as AgentSelectorAnyPod that will select any pod that runs the agent.
func NewK8sAgentClient(context Context, podSelector metav1.ListOptions, clusterClient *KubernetesClient, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(osComp.LinuxFamily, options...)
	ae, err := newAgentK8sExecutor(podSelector, clusterClient)
	if err != nil {
		return nil, fmt.Errorf("could not create k8s agent executor: %w", err)
	}

	commandRunner := newAgentCommandRunner(context, ae)

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
func waitForAgentsReady(ctx Context, host *Host, params *agentclientparams.Params) error {
	agentReadyCmds := map[string]func(*agentclientparams.Params, *Host) (*http.Request, bool, error){
		"process-agent":  processAgentRequest,
		"trace-agent":    traceAgentRequest,
		"security-agent": securityAgentRequest,
	}

	hostHTTPClient := host.NewHTTPClient()
	deadline := time.Now().Add(params.WaitForDuration)
	var lastErr error

	for time.Now().Before(deadline) {
		allReady := true
		for name, getReadyRequest := range agentReadyCmds {
			req, ok, err := getReadyRequest(params, host)
			if err != nil {
				allReady = false
				lastErr = fmt.Errorf("could not build ready command for %s: %w", name, err)
				continue
			}
			if !ok {
				continue
			}
			ctx.Logf("Checking if %s is ready...", name)
			resp, err := hostHTTPClient.Do(req)
			if err != nil {
				allReady = false
				lastErr = fmt.Errorf("%s did not become ready: %w", name, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				allReady = false
				lastErr = fmt.Errorf("%s returned status %d", name, resp.StatusCode)
			}
		}
		if allReady {
			return nil
		}
		time.Sleep(params.WaitForTick)
	}

	return fmt.Errorf("agents not ready after %v: %w", params.WaitForDuration, lastErr)
}

func processAgentRequest(params *agentclientparams.Params, host *Host) (*http.Request, bool, error) {
	return makeStatusEndpointRequest(params, host, "https://localhost:%d/agent/status", params.ProcessAgentPort)
}

func traceAgentRequest(params *agentclientparams.Params, host *Host) (*http.Request, bool, error) {
	return makeStatusEndpointRequest(params, host, "http://localhost:%d/info", params.TraceAgentPort)
}

func securityAgentRequest(params *agentclientparams.Params, host *Host) (*http.Request, bool, error) {
	return makeStatusEndpointRequest(params, host, "https://localhost:%d/agent/status", params.SecurityAgentPort)
}

func makeStatusEndpointRequest(params *agentclientparams.Params, host *Host, url string, port int) (*http.Request, bool, error) {
	if port == 0 {
		return nil, false, nil
	}

	// we want to fetch the auth token only if we actually need it
	if err := ensureAuthToken(params, host); err != nil {
		return nil, true, err
	}

	statusEndpoint := fmt.Sprintf(url, port)
	req, err := http.NewRequest(http.MethodGet, statusEndpoint, nil)
	if err != nil {
		return nil, true, err
	}

	req.Header.Set("Authorization", "Bearer "+params.AuthToken)
	return req, true, nil
}

func ensureAuthToken(params *agentclientparams.Params, host *Host) error {
	if params.AuthToken != "" {
		return nil
	}

	getAuthTokenCmd := fetchAuthTokenCommand(params.AuthTokenPath, host.osFamily)
	authToken, err := host.Execute(getAuthTokenCmd)
	if err != nil {
		return fmt.Errorf("could not read auth token file: %v", err)
	}
	params.AuthToken = strings.TrimSpace(authToken)

	return nil
}

func fetchAuthTokenCommand(authTokenPath string, osFamily osComp.Family) string {
	if osFamily == osComp.WindowsFamily {
		return "Get-Content -Raw -Path " + authTokenPath
	}

	return "sudo cat " + authTokenPath
}

func waitForReadyTimeout(commandRunner *agentCommandRunner, timeout time.Duration) error {
	return commandRunner.waitForReadyTimeout(timeout)
}
