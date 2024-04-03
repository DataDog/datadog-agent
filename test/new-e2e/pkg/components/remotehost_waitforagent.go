// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"fmt"
	"strings"
	"time"

	osComp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agentReadyParams struct {
	authToken         string
	authTokenPath     string
	processAgentPort  int
	traceAgentPort    int
	securityAgentPort int
	waitFor           time.Duration
	tick              time.Duration
}

// AgentReadyOption is a functional option for configuring the WaitForAgentsReady function.
//
// Options are:
// - WithAuthToken: sets the auth token directly.
// - WithAuthTokenPath: sets the path to the auth token file.
// - WithProcessAgentOnPort: enables waiting for the Process Agent, using the given port for the API.
// - WithProcessAgent: enables waiting for the Process Agent, using the default API port.
// - WithTraceAgentOnPort: enables waiting for the Trace Agent, using the given port for the API.
// - WithTraceAgent: enables waiting for the Trace Agent, using the default API port.
// - WithSecurityAgentOnPort: enables waiting for the Security Agent, using the given port for the API.
// - WithSecurityAgent: enables waiting for the Security Agent, using the default API port.
// - WithWaitFor: sets the max duration to wait for the agents to be ready.
// - WithTick: sets the duration between checks for the agents to be ready.
type AgentReadyOption func(*agentReadyParams)

// WaitForAgentsReady waits for the given agents to be ready.
// The given options configure which Agents to wait for, and how long to wait.
//
// Under the hood, this function checks the readiness of the agents by querying their configuration endpoints.
// The function will wait until all agents are ready, or until the timeout is reached.
// If the timeout is reached, the function will fail the test.
// The function is only implemented for Linux.
//
// This is used as follows:
// h.WaitForAgentsReady(WithTraceAgent(), WithProcessAgentOnPort(1234), WithTick(10*time.Second))
func (h *RemoteHost) WaitForAgentsReady(opts ...AgentReadyOption) {
	if h.OSFamily != osComp.LinuxFamily {
		h.context.T().Error("WaitForAgentsReady is only implemented on Linux")
	}

	params := agentReadyParams{
		authTokenPath: "/etc/datadog-agent/auth_token",
		waitFor:       2 * time.Minute,
		tick:          20 * time.Second,
	}
	for _, opt := range opts {
		opt(&params)
	}

	require.EventuallyWithT(h.context.T(), func(t *assert.CollectT) {
		if params.authToken == "" {
			authToken, err := h.Execute("sudo cat " + params.authTokenPath)
			if !assert.NoError(t, err, "reading auth token") {
				return
			}
			params.authToken = strings.TrimSpace(string(authToken))
		}

		agents := map[string]func() (string, bool){
			"process-agent":  params.processAgentConfigEndpoint,
			"trace-agent":    params.traceAgentConfigEndpoint,
			"security-agent": params.securityAgentConfigEndpoint,
		}

		for name, ep := range agents {
			endpoint, ok := ep()
			if !ok {
				continue
			}

			cmd := curlCommand(h.OSFamily, endpoint, params.authToken)
			_, err := h.Execute(cmd)
			assert.NoErrorf(t, err, "%s is not ready", name)
		}
	}, params.waitFor, params.tick, "Waiting for agents to be ready")
}

// WithAuthToken sets the auth token.
func WithAuthToken(authToken string) AgentReadyOption {
	return func(p *agentReadyParams) {
		p.authToken = authToken
	}
}

// WithAuthTokenPath sets the path to the auth token file.
// The file is read from the remote host.
// This is not used if the auth token is provided directly with WithAuthToken.
func WithAuthTokenPath(path string) AgentReadyOption {
	return func(p *agentReadyParams) {
		p.authTokenPath = path
	}
}

// WithProcessAgentOnPort enables waiting for the Process Agent, using the given port for the API.
func WithProcessAgentOnPort(port int) AgentReadyOption {
	return func(p *agentReadyParams) {
		p.processAgentPort = port
	}
}

// WithProcessAgent enables waiting for the Process Agent, using the default API port.
func WithProcessAgent() AgentReadyOption {
	return WithProcessAgentOnPort(6162)
}

// WithTraceAgentOnPort enables waiting for the Trace Agent, using the given port for the API.
func WithTraceAgentOnPort(port int) AgentReadyOption {
	return func(p *agentReadyParams) {
		p.traceAgentPort = port
	}
}

// WithTraceAgent enables waiting for the Trace Agent, using the default API port.
func WithTraceAgent() AgentReadyOption {
	return WithTraceAgentOnPort(5012)
}

// WithSecurityAgentOnPort enables waiting for the Security Agent, using the given port for the API.
func WithSecurityAgentOnPort(port int) AgentReadyOption {
	return func(p *agentReadyParams) {
		p.securityAgentPort = port
	}
}

// WithSecurityAgent enables waiting for the Security Agent, using the default API port.
func WithSecurityAgent() AgentReadyOption {
	return WithSecurityAgentOnPort(5010)
}

// WithWaitFor sets the duration to wait for the agents to be ready.
func WithWaitFor(d time.Duration) AgentReadyOption {
	return func(p *agentReadyParams) {
		p.waitFor = d
	}
}

// WithTick sets the duration between checks for the agents to be ready.
func WithTick(d time.Duration) AgentReadyOption {
	return func(p *agentReadyParams) {
		p.tick = d
	}
}

func (p *agentReadyParams) processAgentConfigEndpoint() (string, bool) {
	if p.processAgentPort == 0 {
		return "", false
	}
	return fmt.Sprintf("http://localhost:%d/config/all", p.processAgentPort), true
}

func (p *agentReadyParams) traceAgentConfigEndpoint() (string, bool) {
	if p.traceAgentPort == 0 {
		return "", false
	}
	return fmt.Sprintf("http://localhost:%d/config", p.traceAgentPort), true
}

func (p *agentReadyParams) securityAgentConfigEndpoint() (string, bool) {
	if p.securityAgentPort == 0 {
		return "", false
	}
	return fmt.Sprintf("https://localhost:%d/agent/config", p.securityAgentPort), true
}

func curlCommand(_ osComp.Family, endpoint string, authToken string) string {
	return fmt.Sprintf(
		`curl -L -s -k -H "authorization: Bearer %s" "%s"`,
		authToken,
		endpoint,
	)
}
