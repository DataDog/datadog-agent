// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package shell

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/shell/executor"
)

// RunShellHandler handles safe shell execution requests from the PAR.
type RunShellHandler struct{}

// NewRunShellHandler creates a new RunShellHandler.
func NewRunShellHandler() *RunShellHandler {
	return &RunShellHandler{}
}

// RunShellInputs defines the input contract for the shell action.
type RunShellInputs struct {
	Script  string `json:"script"`
	Timeout int    `json:"timeout"` // timeout in seconds, 0 = default
}

// RunShellOutputs defines the output contract for the shell action.
type RunShellOutputs struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	DurationMillis int64  `json:"durationMillis"`
}

// Run executes a safe shell command.
func (h *RunShellHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunShellInputs](task)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to extract inputs: %w", err))
	}

	if inputs.Script == "" {
		return nil, util.DefaultActionError(fmt.Errorf("script is required"))
	}

	var opts []executor.Option
	if inputs.Timeout > 0 {
		opts = append(opts, executor.WithTimeout(time.Duration(inputs.Timeout)*time.Second))
	}

	result, err := executor.Execute(ctx, inputs.Script, opts...)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("shell execution failed: %w", err))
	}

	return &RunShellOutputs{
		ExitCode:       result.ExitCode,
		Stdout:         result.Stdout,
		Stderr:         result.Stderr,
		DurationMillis: result.DurationMillis,
	}, nil
}
