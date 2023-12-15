// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package executeparams implements function parameters for [client.vmClient]
package executeparams

// Params defines the parameters for the VM client.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithEnvVariables]
//
// [Functional options pattern]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
// [client.vmClient]: https://pkg.go.dev/github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/vm_client.go
import (
	"fmt"
	"regexp"
)

// Params contains par for VM.Execute commands
type Params struct {
	EnvVariables EnvVar
}

// Option alias to a functional option changing a given Params instance
type Option func(*Params) error

// EnvVar alias to map representing env variables
type EnvVar map[string]string

// NewParams creates a new instance of Execute params
// default Env: map[string]string{}
func NewParams(options ...Option) (*Params, error) {
	p := &Params{
		EnvVariables: map[string]string{},
	}
	return applyOption(p, options...)
}

func applyOption(instance *Params, options ...Option) (*Params, error) {
	for _, o := range options {
		if err := o(instance); err != nil {
			return nil, err
		}
	}
	return instance, nil
}

// WithEnvVariables allows to set env variable for the command that will be executed
func WithEnvVariables(env EnvVar) Option {
	envVarRegex := regexp.MustCompile("^[a-zA-Z_]+[a-zA-Z0-9_]*$")
	return func(p *Params) error {
		for envName, envVar := range env {
			if match := envVarRegex.MatchString(envName); match {
				p.EnvVariables[envName] = envVar
			} else {
				return fmt.Errorf("variable name %s does not have a valid format", envName)
			}
		}
		return nil
	}
}
