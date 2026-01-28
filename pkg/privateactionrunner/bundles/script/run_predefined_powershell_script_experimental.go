// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build private_runner_experimental

package com_datadoghq_script

import (
	"bytes"
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

var requiredWindowsEnvVars = []string{
	"SYSTEMROOT",
	"COMSPEC",
	"PATHEXT",
	"WINDIR",
	"TEMP",
	"TMP",
}

type RunPredefinedPowershellScriptHandler struct {
}

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

	evaluatedCommand, err := evaluatePowershellScriptWithParameters(script, inputs.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate command templates: %w", err)
	}

	if len(evaluatedCommand) == 0 {
		return nil, errors.New("command cannot be empty")
	}

	timeout := time.Duration(inputs.Timeout) * time.Second
	if inputs.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := newPredefinedPowershellScriptCommand(ctx, evaluatedCommand, script.AllowedEnvVars)
	var stdoutBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	var stderrBuffer bytes.Buffer
	cmd.Stderr = &stderrBuffer
	start := time.Now()
	err = cmd.Run()

	const maxOutputSize = 10 * 1024 * 1024 // 10MB
	if stdoutBuffer.Len()+stderrBuffer.Len() > maxOutputSize {
		return nil, errors.New("script output exceeded 10MB limit")
	}

	if err != nil && !inputs.NoFailOnError {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("script execution timed out after %d seconds", inputs.Timeout)
		}
		return nil, fmt.Errorf("failed to execute command: %w, stderr %s", err, stderrBuffer.String())
	}

	return &RunPredefinedPowershellScriptOutputs{
		ExecutedCommand: cmd.String(),
		ExitCode:        cmd.ProcessState.ExitCode(),
		Stdout:          formatOutput(stdoutBuffer.String(), inputs.NoStripTrailingNewline),
		Stderr:          formatOutput(stderrBuffer.String(), inputs.NoStripTrailingNewline),
		DurationMillis:  int(time.Since(start).Milliseconds()),
	}, nil
}

func evaluatePowershellScriptWithParameters(scriptConfig RunPredefinedPowershellScriptConfig, parameters interface{}) ([]string, error) {
	if parameters == nil {
		parameters = map[string]interface{}{}
	}
	if scriptConfig.ParameterSchema != nil {
		if err := validateParameters(parameters, scriptConfig.ParameterSchema); err != nil {
			return nil, err
		}
	}
	templateContext := map[string]interface{}{"parameters": parameters}
	evaluatedCommand := make([]string, len(scriptConfig.Command))
	for i, arg := range scriptConfig.Command {
		template, err := tmpl.Parse(arg)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template '%s': %w", arg, err)
		}

		rendered, err := template.Render(templateContext)
		if err != nil {
			return nil, fmt.Errorf("failed to render template '%s': %w", arg, err)
		}

		evaluatedCommand[i] = rendered
	}

	return evaluatedCommand, nil
}

func newPredefinedPowershellScriptCommand(ctx context.Context, command []string, envVarNames []string) *exec.Cmd {
	firstArg := command[0]

	var cmd *exec.Cmd
	if isPowershellScript(firstArg) {
		psArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File"}
		psArgs = append(psArgs, command...)
		cmd = exec.CommandContext(ctx, "powershell.exe", psArgs...)
	} else {
		commandStr := buildPowershellCommand(command)
		psArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", commandStr}
		cmd = exec.CommandContext(ctx, "powershell.exe", psArgs...)
	}

	// Build restricted environment from allowlist
	cmd.Env = buildAllowedEnv(envVarNames)

	return cmd
}

func buildAllowedEnv(envVarNames []string) []string {
	allowed := make(map[string]bool)

	for _, name := range requiredWindowsEnvVars {
		allowed[strings.ToUpper(name)] = true
	}

	for _, name := range envVarNames {
		allowed[strings.ToUpper(name)] = true
	}

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

func buildPowershellCommand(command []string) string {
	parts := make([]string, len(command))
	for i := 0; i < len(command); i++ {
		parts[i] = "'" + strings.ReplaceAll(command[i], "'", "''") + "'"
	}

	return "& " + strings.Join(parts, " ")
}

func isPowershellScript(command string) bool {
	return strings.HasSuffix(strings.ToLower(command), ".ps1")
}
