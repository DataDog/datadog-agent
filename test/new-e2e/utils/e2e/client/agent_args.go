// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

// agentArgs contains the arguments for the Agent commands.
// Its value is populated using the functional options pattern.
type agentArgs struct {
	Args string
}

type AgentArgsOption = func(*agentArgs)

// WithArgs sets the Agent arguments
func WithArgs(args string) AgentArgsOption {
	return func(a *agentArgs) {
		a.Args = args
	}
}

func newAgentArgs(commandArgs ...AgentArgsOption) *agentArgs {
	args := &agentArgs{}
	for _, argsFunc := range commandArgs {
		argsFunc(args)
	}
	return args
}
