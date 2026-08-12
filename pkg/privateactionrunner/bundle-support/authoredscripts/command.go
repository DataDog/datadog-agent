// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/tmpl"
	workflowjsonschema "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/workflowjsonschema"
)

// NewCommand prepares an authored-script process with validated inputs and an isolated environment.
func NewCommand(ctx context.Context, pkg *Package, session *Session, parameters interface{}) (*exec.Cmd, error) {
	if ctx == nil {
		return nil, errors.New("authored-script command context is required")
	}
	if pkg == nil || pkg.Manifest == nil {
		return nil, errors.New("authored-script package is required")
	}
	if len(pkg.Command) == 0 || pkg.Command[0] == "" {
		return nil, errors.New("authored-script command is required")
	}
	if session == nil {
		return nil, errors.New("authored-script session is required")
	}
	if parameters == nil {
		parameters = map[string]interface{}{}
	}
	if pkg.Manifest.Config.ParameterSchema != nil {
		if err := workflowjsonschema.ValidateParameters(pkg.Manifest.Config.ParameterSchema, parameters); err != nil {
			return nil, err
		}
	}

	command := make([]string, len(pkg.Command))
	command[0] = pkg.Command[0]
	templateContext := map[string]interface{}{"parameters": parameters}
	for i, argument := range pkg.Command[1:] {
		template, err := tmpl.Parse(argument)
		if err != nil {
			return nil, fmt.Errorf("failed to parse authored-script command argument %q: %w", argument, err)
		}
		rendered, err := template.Render(templateContext)
		if err != nil {
			return nil, fmt.Errorf("failed to render authored-script command argument %q: %w", argument, err)
		}
		command[i+1] = rendered
	}

	environment, err := BuildEnvironment(pkg, session)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = session.WorkDirectory
	cmd.Env = environment
	configureCommand(cmd)
	return cmd, nil
}
