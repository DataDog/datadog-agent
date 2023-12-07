// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentclient

// AgentArgs contains the arguments for the Agent commands.
// Its value is populated using the functional options pattern.
type AgentArgs struct {
	Args []string
}

// AgentArgsOption is an optional function parameter type for Agent arguments
type AgentArgsOption = func(*AgentArgs) error

// WithArgs sets the Agent arguments
func WithArgs(args []string) AgentArgsOption {
	return func(a *AgentArgs) error {
		a.Args = args
		return nil
	}
}
