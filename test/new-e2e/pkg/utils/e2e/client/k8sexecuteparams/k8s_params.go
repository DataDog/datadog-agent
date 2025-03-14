// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package k8sexecuteparams contains the parameters for the k8s exec command.
package k8sexecuteparams

// Options is a function that modifies the Params.
type Options func(*Params) error

// Params is the struct that contains the parameters for the k8s exec command.
type Params struct {
	EnvVariables map[string]string
}

// WithEnvVariables sets the environment variables for the command.
func WithEnvVariables(envVariables map[string]string) Options {
	return func(p *Params) error {
		p.EnvVariables = envVariables
		return nil
	}
}
