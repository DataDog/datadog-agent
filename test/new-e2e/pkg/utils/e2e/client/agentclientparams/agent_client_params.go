// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentclientparams implements function parameters for [e2e.Agent]
package agentclientparams

import (
	"fmt"
	"time"

	osComp "github.com/DataDog/test-infra-definitions/components/os"
)

// Params defines the parameters for the Agent client.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithSkipWaitForAgentReady]
//   - [WithAgentInstallPath]
//   - [WithAuthToken]
//   - [WithAuthTokenPath]
//   - [WithProcessAgentOnPort]
//   - [WithProcessAgent]
//   - [WithTraceAgentOnPort]
//   - [WithTraceAgent]
//   - [WithSecurityAgentOnPort]
//   - [WithSecurityAgent]
//   - [WithWaitForDuration]
//   - [WithWaitForTick]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type Params struct {
	ShouldWaitForReady bool
	AgentInstallPath   string

	AuthToken         string
	AuthTokenPath     string
	ProcessAgentPort  int
	TraceAgentPort    int
	SecurityAgentPort int
	WaitForDuration   time.Duration
	WaitForTick       time.Duration
}

// Option alias to a functional option changing a given Params instance
type Option func(*Params)

// NewParams creates a new instance of Agent client params
// default ShouldWaitForReady: true
func NewParams(osfam osComp.Family, options ...Option) *Params {
	p := &Params{
		ShouldWaitForReady: true,
		AuthTokenPath:      defaultAuthTokenPath(osfam),
		WaitForDuration:    1 * time.Minute,
		WaitForTick:        5 * time.Second,
	}
	return applyOption(p, options...)
}

func applyOption(instance *Params, options ...Option) *Params {
	for _, o := range options {
		o(instance)
	}
	return instance
}

// WithSkipWaitForAgentReady skips waiting for agent readiness after setting up the agent client
// Use it to testagent configuration that are expected to have an unhealthy agent
func WithSkipWaitForAgentReady() Option {
	return func(p *Params) {
		p.ShouldWaitForReady = false
	}
}

// WithAgentInstallPath sets the agent installation path
func WithAgentInstallPath(path string) Option {
	return func(p *Params) {
		p.AgentInstallPath = path
	}
}

// WithAuthToken sets the auth token.
func WithAuthToken(authToken string) Option {
	return func(p *Params) {
		p.AuthToken = authToken
	}
}

// WithAuthTokenPath sets the path to the auth token file.
// The file is read from the remote host.
// This is not used if the auth token is provided directly with WithAuthToken.
func WithAuthTokenPath(path string) Option {
	return func(p *Params) {
		p.AuthTokenPath = path
	}
}

// WithProcessAgentOnPort enables waiting for the Process Agent, using the given port for the API.
func WithProcessAgentOnPort(port int) Option {
	return func(p *Params) {
		p.ProcessAgentPort = port
	}
}

// WithProcessAgent enables waiting for the Process Agent, using the default API port.
func WithProcessAgent() Option {
	return WithProcessAgentOnPort(6162)
}

// WithTraceAgentOnPort enables waiting for the Trace Agent, using the given port for the API.
func WithTraceAgentOnPort(port int) Option {
	return func(p *Params) {
		p.TraceAgentPort = port
	}
}

// WithTraceAgent enables waiting for the Trace Agent, using the default API port.
func WithTraceAgent() Option {
	return WithTraceAgentOnPort(5012)
}

// WithSecurityAgentOnPort enables waiting for the Security Agent, using the given port for the API.
func WithSecurityAgentOnPort(port int) Option {
	return func(p *Params) {
		p.SecurityAgentPort = port
	}
}

// WithSecurityAgent enables waiting for the Security Agent, using the default API port.
func WithSecurityAgent() Option {
	return WithSecurityAgentOnPort(5010)
}

// WithWaitForDuration sets the duration to wait for the agents to be ready.
func WithWaitForDuration(d time.Duration) Option {
	return func(p *Params) {
		p.WaitForDuration = d
	}
}

// WithWaitForTick sets the duration between checks for the agents to be ready.
func WithWaitForTick(d time.Duration) Option {
	return func(p *Params) {
		p.WaitForTick = d
	}
}

func defaultAuthTokenPath(osfam osComp.Family) string {
	switch osfam {
	case osComp.LinuxFamily:
		return "/etc/datadog-agent/auth_token"
	case osComp.WindowsFamily:
		return "C:\\ProgramData\\Datadog\\auth_token"
	case osComp.MacOSFamily:
		return "/opt/datadog-agent/etc/auth_token"
	}
	panic(fmt.Sprintf("unsupported OS family %d", osfam))
}
