// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	workflowjsonschema "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/workflowjsonschema"
)

const processWaitDelay = 5 * time.Second

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
	if parameters == nil {
		parameters = map[string]interface{}{}
	}
	if pkg.Manifest.Config.ParameterSchema != nil {
		if err := workflowjsonschema.ValidateParameters(pkg.Manifest.Config.ParameterSchema, parameters); err != nil {
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

	cmd := exec.CommandContext(ctx, pkg.Command[0], pkg.Command[1:]...)
	cmd.Dir = session.WorkDirectory
	cmd.Env = environment
	configureCommand(cmd)
	return cmd, nil
}

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = processWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
}
