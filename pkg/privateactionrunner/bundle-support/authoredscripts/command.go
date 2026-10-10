// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"context"
	"errors"
	"os/exec"

	workflowjsonschema "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/workflowjsonschema"
)

// NewCommand prepares an authored-script process with validated inputs and an isolated environment.
func NewCommand(ctx context.Context, pkg *Package, session *Session, parameters interface{}) (*exec.Cmd, error) {
	if ctx == nil {
		return nil, errors.New("authored-script command context is required")
	}
	if pkg == nil {
		return nil, errors.New("authored-script package is required")
	}
	if session == nil {
		return nil, errors.New("authored-script session is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(pkg.Command) == 0 || pkg.Command[0] == "" {
		return nil, errors.New("authored-script command is required")
	}
	if parameters == nil {
		parameters = map[string]interface{}{}
	}
	if pkg.Manifest.ParameterSchema != nil {
		if err := workflowjsonschema.ValidateParameters(pkg.Manifest.ParameterSchema, parameters); err != nil {
			return nil, err
		}
	}

	parameterMap, ok := parameters.(map[string]interface{})
	if !ok {
		return nil, errors.New("authored-script parameters must be an object")
	}

	environment, err := pkg.BuildEnvironment(session, parameterMap)
	if err != nil {
		return nil, err
	}

	command, err := platformCommand(pkg.Command)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = session.WorkDirectory
	cmd.Env = environment
	return cmd, nil
}
