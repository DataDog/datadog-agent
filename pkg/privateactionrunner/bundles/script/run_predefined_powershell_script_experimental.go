// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build private_runner_experimental && windows

package com_datadoghq_script

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/tmpl"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

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

	if script.Script == "" && script.File == "" {
		return nil, errors.New("either 'script' or 'file' must be specified in the configuration")
	}
	if script.Script != "" && script.File != "" {
		return nil, errors.New("cannot specify both 'script' and 'file' - use one or the other")
	}

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

	cmd, cleanup, err := newPowershellCommand(ctx, evaluatedScript, script.AllowedEnvVars)
	if err != nil {
		return nil, err
	}
	defer cleanup()

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
		return nil, fmt.Errorf("failed to execute script: %w, stderr %s", err, stderrBuffer.String())
	}

	return &RunPredefinedPowershellScriptOutputs{
		ExecutedCommand: cmd.String(),
		ExitCode:        cmd.ProcessState.ExitCode(),
		Stdout:          formatPowershellOutput(stdoutBuffer.String(), inputs.NoStripTrailingNewline),
		Stderr:          formatPowershellOutput(stderrBuffer.String(), inputs.NoStripTrailingNewline),
		DurationMillis:  int(time.Since(start).Milliseconds()),
	}, nil
}

type evaluatedPowershellScript struct {
	Script    string   // inline script content
	File      string   // path to .ps1 file
	Arguments []string // arguments for file mode
}

func evaluatePowershellScript(config RunPredefinedPowershellScriptConfig, parameters interface{}) (*evaluatedPowershellScript, error) {
	if parameters == nil {
		parameters = map[string]interface{}{}
	}

	if config.ParameterSchema != nil {
		if err := validateParameters(parameters, config.ParameterSchema); err != nil {
			return nil, err
		}
	}

	templateContext := map[string]interface{}{"parameters": parameters}

	result := &evaluatedPowershellScript{}

	if config.Script != "" {
		rendered, err := renderTemplate(config.Script, templateContext)
		if err != nil {
			return nil, fmt.Errorf("failed to render script template: %w", err)
		}
		if strings.TrimSpace(rendered) == "" {
			return nil, errors.New("script cannot be empty")
		}
		result.Script = rendered
	} else {
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

func renderTemplate(templateStr string, context map[string]interface{}) (string, error) {
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

// newPowershellCommand creates an exec.Cmd that always runs as dd-scriptuser.
// The returned cleanup func must be deferred to release the user token.
func newPowershellCommand(ctx context.Context, script *evaluatedPowershellScript, envVarNames []string) (*exec.Cmd, func(), error) {
	baseArgs := []string{
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
	}

	var cmd *exec.Cmd

	if script.File != "" {
		args := append(baseArgs, "-File", script.File)
		args = append(args, script.Arguments...)
		cmd = exec.CommandContext(ctx, "powershell.exe", args...)
	} else {
		args := append(baseArgs, "-Command", script.Script)
		cmd = exec.CommandContext(ctx, "powershell.exe", args...)
	}

	token, err := logonScriptUser()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to obtain restricted user token for %s: %w", ScriptUserName, err)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Token: token,
	}
	cleanup := func() { token.Close() }
	cmd.Env = buildAllowedEnv(envVarNames)

	return cmd, cleanup, nil
}

func formatPowershellOutput(output string, noStripTrailingNewline bool) string {
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	if noStripTrailingNewline {
		return normalized
	}
	normalized = strings.TrimRight(normalized, "\n")
	normalized = strings.TrimLeft(normalized, "\n")
	return normalized
}
