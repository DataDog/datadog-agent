// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package com_datadoghq_script

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunPredefinedScriptHandler struct {
}

func NewRunPredefinedScriptHandler() *RunPredefinedScriptHandler {
	return &RunPredefinedScriptHandler{}
}

type RunPredefinedScriptInputs struct {
	ScriptName             string `json:"scriptName"`
	Timeout                int    `json:"timeout"`
	NoFailOnError          bool   `json:"noFailOnError"`
	NoStripTrailingNewline bool   `json:"noStripTrailingNewline"`
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
	credentials interface{},
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunPredefinedScriptInputs](task)
	if err != nil {
		return nil, err
	}

	scriptConfig, err := parseCredentials(credentials)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(inputs.ScriptName, "dangerously-run-script-for-poc") {
		return nil, fmt.Errorf("This script implementation is only meant for POC purposes. Please use the 'dangerously-run-script-for-poc' prefix for your script name to indicate that you understand the risks of running arbitrary scripts. This is not intended for production use.")
	}

	script, ok := scriptConfig.RunPredefinedScript[inputs.ScriptName]
	if !ok {
		return nil, fmt.Errorf("script %s not found", inputs.ScriptName)
	}

	// TODO add support for parameters
	//evaluatedCommand, err := evaluateScriptWithParameters(script, task.Data.Attributes.Inputs["parameters"])
	//if err != nil {
	//	return nil, fmt.Errorf("failed to evaluate command templates: %w", err)
	//}

	timeout := time.Duration(inputs.Timeout) * time.Second
	if inputs.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := NewPredefinedScriptCommand(ctx, script.Command)
	var stdoutBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	var stderrBuffer bytes.Buffer
	cmd.Stderr = &stderrBuffer
	start := time.Now()
	err = cmd.Run()

	const maxOutputSize = 10 * 1024 * 1024 // 10MB
	if stdoutBuffer.Len()+stderrBuffer.Len() > maxOutputSize {
		return nil, fmt.Errorf("script output exceeded 10MB limit")
	}

	if err != nil && !inputs.NoFailOnError {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("script execution timed out after %d seconds", inputs.Timeout)
		}
		return nil, fmt.Errorf("failed to execute command: %w, stderr %s", err, stderrBuffer.String())
	}

	return &RunPredefinedScriptOutputs{
		ExecutedCommand: cmd.String(),
		ExitCode:        cmd.ProcessState.ExitCode(),
		Stdout:          formatOutput(stdoutBuffer.String(), inputs.NoStripTrailingNewline),
		Stderr:          formatOutput(stderrBuffer.String(), inputs.NoStripTrailingNewline),
		DurationMillis:  int(time.Since(start).Milliseconds()),
	}, nil
}

func formatOutput(output string, noStripTrailingNewline bool) string {
	if noStripTrailingNewline {
		return output
	}
	return strings.TrimSuffix(output, "\n")
}
