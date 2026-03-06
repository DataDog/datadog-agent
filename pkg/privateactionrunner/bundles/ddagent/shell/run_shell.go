// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package shell

import (
	"context"
	"fmt"
	"os"
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
	opts = append(opts, executor.WithEnv(safeEnv()))

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

// safeEnvVars is the allowlist of environment variable names that are safe
// to pass through to child processes. Everything else is excluded to prevent
// attacks via LD_PRELOAD, PAGER, BASH_ENV, GREP_OPTIONS, etc.
var safeEnvVars = map[string]bool{
	"PATH":    true,
	"HOME":    true,
	"LANG":    true,
	"LC_ALL":  true,
	"TERM":    true,
	"TMPDIR":  true,
	"TZ":      true,
	"USER":    true,
	"LOGNAME": true,
}

// safeEnv builds a minimal environment from the current process environment,
// keeping only variables in the safeEnvVars allowlist and overriding PATH
// with a hardcoded safe value.
func safeEnv() []string {
	env := []string{"PATH=/usr/bin:/bin:/usr/local/bin"}
	for _, e := range os.Environ() {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				name := e[:i]
				if name != "PATH" && safeEnvVars[name] {
					env = append(env, e)
				}
				break
			}
		}
	}
	return env
}
