// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/tmpl"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunPredefinedScriptHandler struct {
}

func NewRunPredefinedScriptHandler() *RunPredefinedScriptHandler {
	return &RunPredefinedScriptHandler{}
}

type RunPredefinedScriptInputs struct {
	ScriptName             string      `json:"scriptName"`
	Parameters             interface{} `json:"parameters"`
	Timeout                int         `json:"timeout"`
	NoFailOnError          bool        `json:"noFailOnError"`
	NoStripTrailingNewline bool        `json:"noStripTrailingNewline"`
}

type RunPredefinedScriptOutputs struct {
	ExecutedCommand string `json:"executedCommand"`
	ExitCode        int    `json:"exitCode"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	DurationMillis  int    `json:"durationMillis"`
}

func (h *RunPredefinedScriptHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunPredefinedScriptInputs](task)
	if err != nil {
		return nil, err
	}

	scriptConfig, err := parseCredentials(credentials)
	if err != nil {
		return nil, err
	}

	script, ok := scriptConfig.RunPredefinedScript[inputs.ScriptName]
	if !ok {
		return nil, fmt.Errorf("script %s not found", inputs.ScriptName)
	}

	evaluatedCommand, err := evaluateScriptWithParameters(script, inputs.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate command templates: %w", err)
	}

	timeout := time.Duration(inputs.Timeout) * time.Second
	if inputs.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd, err := NewPredefinedScriptCommand(ctx, evaluatedCommand, script.AllowedEnvVars)
	if err != nil {
		return nil, fmt.Errorf("invalid command arguments: %w", err)
	}
	stdoutWriter, stderrWriter := newLimitedWriterPair(maxOutputSize)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	start := time.Now()
	err = cmd.Run()

	if stdoutWriter.LimitReached() || stderrWriter.LimitReached() {
		return nil, errOutputLimitExceeded
	}

	if err != nil && !inputs.NoFailOnError {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("script execution timed out after %d seconds", inputs.Timeout)
		}
		return nil, fmt.Errorf("failed to execute command: %w, stderr %s", err, stderrWriter.String())
	}

	return &RunPredefinedScriptOutputs{
		ExecutedCommand: cmd.String(),
		ExitCode:        cmd.ProcessState.ExitCode(),
		Stdout:          formatOutput(stdoutWriter.String(), inputs.NoStripTrailingNewline),
		Stderr:          formatOutput(stderrWriter.String(), inputs.NoStripTrailingNewline),
		DurationMillis:  int(time.Since(start).Milliseconds()),
	}, nil
}

func evaluateScriptWithParameters(scriptConfig RunPredefinedScriptConfig, parameters interface{}) ([]string, error) {
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

func formatOutput(output string, noStripTrailingNewline bool) string {
	if noStripTrailingNewline {
		return output
	}
	return strings.TrimSuffix(output, "\n")
}
