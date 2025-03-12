// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dockerexecuteparams contains the set of parameters for the DockerExecuteParams.
package dockerexecuteparams

// Options is the set of options for the DockerExecuteParams.
type Options func(*Params) error

// Params is the set of parameters for the DockerExecuteParams.
type Params struct {
	EnvVariables map[string]string
}

// WithEnvVariables sets the environment variables for the command.
func WithEnvVariables(env map[string]string) Options {
	return func(p *Params) error {
		p.EnvVariables = env
		return nil
	}
}
