// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build private_runner_experimental && !windows

package com_datadoghq_script

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/tempfile"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func init() {
	log.Warn("Experimental feature: RunShellScript is enabled")
}

type RunShellScriptHandler struct{}

func NewRunShellScriptHandler() *RunShellScriptHandler {
	return &RunShellScriptHandler{}
}

// RunShellScriptInputs follows AWS SSM Run Shell Script specification
type RunShellScriptInputs struct {
	Script                 string   `json:"script"`                 // Required: Shell script content to execute
	Args                   []string `json:"args"`                   // Optional: Arguments to pass to the script
	Timeout                int      `json:"timeout"`                // Optional: Time in seconds for command to complete (default: 3600)
	NoFailOnError          bool     `json:"noFailOnError"`          // Optional: Don't fail on non-zero exit code
	NoStripTrailingNewline bool     `json:"noStripTrailingNewline"` // Optional: Keep trailing newlines in output
}

type RunShellScriptOutputs struct {
	ExecutedCommand string `json:"executedCommand"`
	ExitCode        int    `json:"exitCode"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	DurationMillis  int    `json:"durationMillis"`
}

func (h *RunShellScriptHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunShellScriptInputs](task)
	if err != nil {
		return nil, err
	}

	if inputs.Script == "" {
		return nil, errors.New("script cannot be empty")
	}

	timeout := time.Duration(inputs.Timeout) * time.Second
	if inputs.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	scriptFile, err := tempfile.NewWithContent("*-user-script.sh", []byte(inputs.Script))
	if err != nil {
		return nil, fmt.Errorf("failed to create script file: %w", err)
	}
	defer scriptFile.CloseSafely()

	// Make the script file readable by all, writable only by owner (needed for scriptuser execution)
	if err = scriptFile.Chmod(0o644); err != nil {
		return nil, fmt.Errorf("failed to set script file permissions: %w", err)
	}

	cmd, err := NewShellScriptCommand(ctx, scriptFile.Name(), inputs.Args)
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

	stdErr := sanitizeErrorMessage(scriptFile.Name(), stderrWriter.String())

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("script execution timed out after %d seconds", inputs.Timeout)
		}
		if !inputs.NoFailOnError {
			return nil, fmt.Errorf("failed to execute command: %w, stderr %s", err, stdErr)
		}
	}

	return &RunShellScriptOutputs{
		ExecutedCommand: cmd.String(),
		ExitCode:        cmd.ProcessState.ExitCode(),
		Stdout:          formatOutput(stdoutWriter.String(), inputs.NoStripTrailingNewline),
		Stderr:          formatOutput(stdErr, inputs.NoStripTrailingNewline),
		DurationMillis:  int(time.Since(start).Milliseconds()),
	}, nil
}

func sanitizeErrorMessage(scriptFile, errMsg string) string {
	return strings.ReplaceAll(errMsg, scriptFile+": ", "")
}
