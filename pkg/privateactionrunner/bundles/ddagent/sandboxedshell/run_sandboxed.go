// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sandboxedshell

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/shell/sandboxed"
)

// RunSandboxedHandler handles sandboxed shell execution requests from the PAR.
type RunSandboxedHandler struct{}

// NewRunSandboxedHandler creates a new RunSandboxedHandler.
func NewRunSandboxedHandler() *RunSandboxedHandler {
	return &RunSandboxedHandler{}
}

// RunSandboxedInputs defines the input contract for the runSandboxed action.
type RunSandboxedInputs struct {
	Script    string `json:"script"`
	Timeout   int    `json:"timeout"`   // timeout in seconds, 0 = default (30s)
	SessionID string `json:"sessionId"` // optional; empty = auto-create new session
}

// RunSandboxedOutputs defines the output contract for the runSandboxed action.
type RunSandboxedOutputs struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	DurationMillis int64  `json:"durationMillis"`
	SessionID      string `json:"sessionId"` // always returned (created or reused)
}

// Run executes a sandboxed shell command with agentfs session tracking.
func (h *RunSandboxedHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunSandboxedInputs](task)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to extract inputs: %w", err))
	}

	if inputs.Script == "" {
		return nil, util.DefaultActionError(fmt.Errorf("script is required"))
	}

	var opts []sandboxed.Option
	if inputs.Timeout > 0 {
		opts = append(opts, sandboxed.WithTimeout(time.Duration(inputs.Timeout)*time.Second))
	}
	if inputs.SessionID != "" {
		opts = append(opts, sandboxed.WithSession(inputs.SessionID))
	}

	result, err := sandboxed.Execute(ctx, inputs.Script, opts...)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("sandboxed execution failed: %w", err))
	}

	return &RunSandboxedOutputs{
		ExitCode:       result.ExitCode,
		Stdout:         result.Stdout,
		Stderr:         result.Stderr,
		DurationMillis: result.DurationMillis,
		SessionID:      result.SessionID,
	}, nil
}
