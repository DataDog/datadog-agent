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

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/tmpl"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// requiredWindowsEnvVars are environment variables that must always be available
// for PowerShell and Windows to function correctly
var requiredWindowsEnvVars = []string{
	"SYSTEMROOT",
	"COMSPEC",
	"PATH",
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
	ScriptName             string `json:"scriptName"`
	Parameters             any    `json:"parameters"`
	Timeout                int    `json:"timeout"`
	NoFailOnError          bool   `json:"noFailOnError"`
	NoStripTrailingNewline bool   `json:"noStripTrailingNewline"`
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
) (any, error) {
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

// evaluatePowershellScript prepares the script for execution.
//
// For inline scripts it calls transformInlineScript, which prepends safe
// single-quoted variable assignments for every {{ parameters.X }} reference
// and replaces those expressions in the script body with $__par_X variable
// references — keeping user-supplied data out of any parseable code position.
//
// For file-based scripts the file path and arguments are still rendered via
// the template engine (they are not executed as PowerShell code).
func evaluatePowershellScript(config RunPredefinedPowershellScriptConfig, parameters any) (*evaluatedPowershellScript, error) {
	if parameters == nil {
		parameters = map[string]any{}
	}

	if config.ParameterSchema != nil {
		if err := validateParameters(parameters, config.ParameterSchema); err != nil {
			return nil, err
		}
	}

	result := &evaluatedPowershellScript{}

	if config.Script != "" {
		// Inline script mode: prepend single-quoted variable assignments to prevent injection.
		transformed, err := transformInlineScript(config.Script, parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to transform script template: %w", err)
		}
		if strings.TrimSpace(transformed.Script) == "" {
			return nil, errors.New("script cannot be empty")
		}
		result.Script = transformed.Script
	} else {
		// File mode: render templates in file path and arguments.
		// These values are never executed as PowerShell code; they are passed
		// as arguments to powershell.exe -File, so template rendering is safe here.
		templateContext := map[string]any{"parameters": parameters}

		rendered, err := renderTemplate(config.File, templateContext)
		if err != nil {
			return nil, fmt.Errorf("failed to render file path template: %w", err)
		}
		if strings.TrimSpace(rendered) == "" {
			return nil, errors.New("file path cannot be empty")
		}
		result.File = rendered

		result.Arguments = make([]string, len(config.Arguments))
		for i, arg := range config.Arguments {
			rendered, err := renderTemplate(arg, templateContext)
			if err != nil {
				return nil, fmt.Errorf("failed to render argument template '%s': %w", arg, err)
			}
			result.Arguments[i] = rendered
		}
	}

	return result, nil
}

// renderTemplate parses and renders a template string with the given context.
// Used for file-mode paths and arguments (not for inline script bodies).
func renderTemplate(templateStr string, context map[string]any) (string, error) {
	template, err := tmpl.Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template '%s': %w", templateStr, err)
	}

	rendered, err := template.Render(context)
	if err != nil {
		return "", fmt.Errorf("failed to render template '%s': %w", templateStr, err)
	}

	return rendered, nil
}

// formatPowershellOutput normalises line endings and optionally strips leading/trailing newlines.
func formatPowershellOutput(output string, noStripTrailingNewline bool) string {
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	if noStripTrailingNewline {
		return normalized
	}
	normalized = strings.TrimRight(normalized, "\n")
	normalized = strings.TrimLeft(normalized, "\n")
	return normalized
}

// newPowershellCommand creates an exec.Cmd for running PowerShell.
//
// For inline scripts the script body is passed to -Command as-is.  User-supplied
// parameter values are embedded as PowerShell single-quoted string literals in the
// preamble (generated by transformInlineScript) and never appear in executable
// code positions.
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
		// Inline script mode: script body begins with single-quoted variable assignments
		// for every parameter (generated by transformInlineScript), followed by the
		// original script with {{ parameters.X }} replaced by $__par_X references.
		args := append(baseArgs, "-Command", script.Script)
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
