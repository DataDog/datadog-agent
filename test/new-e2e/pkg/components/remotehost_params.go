// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

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

// EnvVar alias to map representing env variables
type EnvVar map[string]string

var envVarNameRegexp = regexp.MustCompile("^[a-zA-Z_]+[a-zA-Z0-9_]*$")

// ExecuteParams contains parameters for VM.Execute commands
type ExecuteParams struct {
	EnvVariables EnvVar
}

// ExecuteOption alias to a functional option changing a given Params instance
type ExecuteOption func(*ExecuteParams) error

// WithEnvVariables allows to set env variable for the command that will be executed
func WithEnvVariables(env EnvVar) ExecuteOption {
	return func(p *ExecuteParams) error {
		p.EnvVariables = make(EnvVar, len(env))
		for envName, envVar := range env {
			if match := envVarNameRegexp.MatchString(envName); match {
				p.EnvVariables[envName] = envVar
			} else {
				return fmt.Errorf("variable name %s does not have a valid format", envName)
			}
		}
		return nil
	}
}
