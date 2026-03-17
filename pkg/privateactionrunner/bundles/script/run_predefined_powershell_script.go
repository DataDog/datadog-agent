// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package com_datadoghq_script

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// requiredWindowsEnvVars are environment variables that must always be available
// for PowerShell and Windows to function correctly
var requiredWindowsEnvVars = []string{
	"SYSTEMROOT",
	"COMSPEC",
	"PATHEXT",
	"WINDIR",
	"TEMP",
	"TMP",
}

type RunPredefinedPowershellScriptHandler struct{}

func NewRunPredefinedPowershellScriptHandler() *RunPredefinedPowershellScriptHandler {
	return &RunPredefinedPowershellScriptHandler{}
}

type RunPredefinedPowershellScriptInputs struct {
	ScriptName             string      `json:"scriptName"`
	Parameters             interface{} `json:"parameters"`
	Timeout                int         `json:"timeout"`
	NoFailOnError          bool        `json:"noFailOnError"`
	NoStripTrailingNewline bool        `json:"noStripTrailingNewline"`
}

type RunPredefinedPowershellScriptOutputs struct {
	ExecutedCommand string `json:"executedCommand"`
	ExitCode        int    `json:"exitCode"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	DurationMillis  int    `json:"durationMillis"`
}

func (h *RunPredefinedPowershellScriptHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunPredefinedPowershellScriptInputs](task)
	if err != nil {
		return nil, err
	}

	scriptConfig, err := parseCredentials(credentials)
	if err != nil {
		return nil, err
	}

	script, ok := scriptConfig.RunPredefinedPowershellScript[inputs.ScriptName]
	if !ok {
		return nil, fmt.Errorf("powershell script %s not found", inputs.ScriptName)
	}

	// Validate that either Script or File is provided, but not both
	if script.Script == "" && script.File == "" {
		return nil, errors.New("either 'script' or 'file' must be specified in the configuration")
	}
	if script.Script != "" && script.File != "" {
		return nil, errors.New("cannot specify both 'script' and 'file' - use one or the other")
	}

	// Evaluate templates in the script/file/arguments
	evaluatedScript, err := evaluatePowershellScript(script, inputs.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate script templates: %w", err)
	}

	timeout := time.Duration(inputs.Timeout) * time.Second
	if inputs.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := newPowershellCommand(ctx, evaluatedScript, script.AllowedEnvVars)
	stdoutWriter, stderrWriter := newLimitedStdoutStderrWritersPair(defaultMaxOutputSize)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	start := time.Now()
	err = cmd.Run()

	if stdoutWriter.LimitReached() || stderrWriter.LimitReached() {
		return nil, newOutputLimitError(defaultMaxOutputSize)
	}

	if err != nil && !inputs.NoFailOnError {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("script execution timed out after %d seconds", inputs.Timeout)
		}
		return nil, fmt.Errorf("failed to execute script: %w, stderr %s", err, stderrWriter.String())
	}

	return &RunPredefinedPowershellScriptOutputs{
		ExecutedCommand: cmd.String(),
		ExitCode:        cmd.ProcessState.ExitCode(),
		Stdout:          formatPowershellOutput(stdoutWriter.String(), inputs.NoStripTrailingNewline),
		Stderr:          formatPowershellOutput(stderrWriter.String(), inputs.NoStripTrailingNewline),
		DurationMillis:  int(time.Since(start).Milliseconds()),
	}, nil
}

// newPowershellCommand creates an exec.Cmd for running PowerShell.
//
// For inline scripts the script body is wrapped in a scriptblock ({ ... }) and
// parameter values are appended as separate OS-level named arguments.  PowerShell
// binds them to the param() variables declared at the top of the script body.
// Because values arrive as independent arguments they can never escape into the
// script code, regardless of their content.
func newPowershellCommand(ctx context.Context, script *evaluatedPowershellScript, envVarNames []string) *exec.Cmd {
	// Base PowerShell arguments for security and consistency
	baseArgs := []string{
		"-NoProfile",                 // Don't load user profile (faster, more predictable)
		"-NonInteractive",            // No interactive prompts
		"-ExecutionPolicy", "Bypass", // Allow script execution
	}

	var cmd *exec.Cmd

	if script.File != "" {
		// File mode: run a .ps1 script file
		args := append(baseArgs, "-File", script.File)
		args = append(args, script.Arguments...)
		cmd = exec.CommandContext(ctx, "powershell.exe", args...)
	} else {
		// Inline script mode: wrap the script in a scriptblock so that
		// ScriptArgs are bound as named parameters rather than interpolated.
		//
		//   powershell.exe ... -Command { param($__par_name = $null) ... } -__par_name "Alice"
		//
		// PowerShell passes the trailing "-name value" pairs to the scriptblock's
		// param() binder; they are never parsed as PowerShell code.
		scriptblock := "{\n" + script.Script + "\n}"
		args := append(baseArgs, "-Command", scriptblock)
		args = append(args, script.ScriptArgs...)
		cmd = exec.CommandContext(ctx, "powershell.exe", args...)
	}

	// Build restricted environment from allowlist
	cmd.Env = buildAllowedEnv(envVarNames)

	return cmd
}

// buildAllowedEnv constructs an environment variable list containing only
// the required Windows env vars plus any explicitly allowed vars
func buildAllowedEnv(envVarNames []string) []string {
	allowed := make(map[string]bool)

	// Always include required Windows environment variables
	for _, name := range requiredWindowsEnvVars {
		allowed[strings.ToUpper(name)] = true
	}

	// Add user-specified allowed env vars
	for _, name := range envVarNames {
		allowed[strings.ToUpper(name)] = true
	}

	// Filter current environment to only allowed variables
	var env []string
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.ToUpper(parts[0])
		if allowed[name] {
			env = append(env, e)
		}
	}

	return env
}
