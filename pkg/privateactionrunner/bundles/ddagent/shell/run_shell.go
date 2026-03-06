// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// RunShellHandler executes shell commands using the embedded POSIX shell interpreter.
type RunShellHandler struct{}

// NewRunShellHandler creates a new RunShellHandler.
func NewRunShellHandler() *RunShellHandler {
	return &RunShellHandler{}
}

// RunShellInputs defines the inputs for the shell action.
type RunShellInputs struct {
	// Script is the shell script to execute.
	Script string `json:"script"`
	// AllowedCommands is an optional list of external commands the shell is allowed to execute.
	// By default, all external commands are blocked; only built-in commands are available.
	AllowedCommands []string `json:"allowedCommands"`
	// Timeout is the maximum execution time in seconds. 0 means no timeout.
	Timeout int `json:"timeout"`
}

// RunShellOutputs defines the outputs for the shell action.
type RunShellOutputs struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	DurationMillis int    `json:"durationMillis"`
}

// Run executes the shell action.
func (h *RunShellHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunShellInputs](task)
	if err != nil {
		return nil, util.DefaultActionError(err)
	}

	if inputs.Script == "" {
		return nil, util.DefaultActionError(errors.New("script must be provided"))
	}

	if inputs.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(inputs.Timeout)*time.Second)
		defer cancel()
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	opts := []interp.RunnerOption{
		interp.StdIO(nil, &stdoutBuf, &stderrBuf),
	}
	if len(inputs.AllowedCommands) > 0 {
		opts = append(opts, interp.AllowedCommands(inputs.AllowedCommands))
	}

	runner, err := interp.New(opts...)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to create shell runner: %w", err))
	}

	prog, err := syntax.NewParser().Parse(strings.NewReader(inputs.Script), "")
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to parse shell input: %w", err))
	}

	runner.Reset()
	log.Debugf("Executing shell script:\n%s", inputs.Script)
	start := time.Now()
	runErr := runner.Run(ctx, prog)
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, util.DefaultActionError(fmt.Errorf("shell execution timed out after %d seconds", inputs.Timeout))
		} else {
			return nil, util.DefaultActionError(fmt.Errorf("shell execution failed: %w", runErr))
		}
	}

	stdout := strings.TrimSuffix(stdoutBuf.String(), "\n")
	stderr := strings.TrimSuffix(stderrBuf.String(), "\n")
	log.Debugf("Shell script completed: exitCode=%d, durationMs=%d, stdout=%q, stderr=%q",
		exitCode, int(duration.Milliseconds()), stdout, stderr)

	return &RunShellOutputs{
		ExitCode:       exitCode,
		Stdout:         stdout,
		Stderr:         stderr,
		DurationMillis: int(duration.Milliseconds()),
	}, nil
}
