// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentclientparams implements function parameters for [e2e.Agent]
package agentclientparams

// Params defines the parameters for the Agent client.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithSkipWaitForAgentReady]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type Params struct {
	ShouldWaitForReady bool
}

// Option alias to a functional option changing a given Params instance
type Option func(*Params)

// NewParams creates a new instance of Agent client params
// default ShouldWaitForReady: true
func NewParams(options ...Option) *Params {
	p := &Params{
		ShouldWaitForReady: true,
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
