// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/scriptprocess"
)

const (
	defaultOutputLimit = 10 * 1024 * 1024
	maxErrorStderrSize = 16 * 1024
)

var errOutputLimitExceeded = errors.New("authored-script output limit exceeded")

// Result contains the observable result of an authored-script execution.
type Result = scriptprocess.Result

// ExecuteCommand runs an authored-script command with bounded output and process cleanup.
func ExecuteCommand(ctx context.Context, cmd *exec.Cmd) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("authored-script context is required")
	}

	result, err := executeCommand(ctx, cmd, defaultOutputLimit)
	if err != nil {
		return result, formatExecutionError(ctx, result, err)
	}
	return result, nil
}

func executeCommand(ctx context.Context, cmd *exec.Cmd, outputLimit int64) (Result, error) {
	result, err := scriptprocess.Run(ctx, cmd, outputLimit)
	if errors.Is(err, scriptprocess.ErrOutputLimitExceeded) {
		return result, fmt.Errorf("authored-script output exceeded %d bytes: %w", outputLimit, errOutputLimitExceeded)
	}
	if startErr, ok := errors.AsType[*scriptprocess.StartError](err); ok && errors.Is(startErr, os.ErrPermission) {
		return result, executionDeniedError(startErr.Err)
	}
	return result, err
}

func formatExecutionError(ctx context.Context, result Result, err error) error {
	if errors.Is(err, errOutputLimitExceeded) {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return fmt.Errorf("authored-script execution timed out: %w", ctxErr)
		}
		return fmt.Errorf("authored-script execution canceled: %w", ctxErr)
	}

	if stderr := errorStderr(result.Stderr); stderr != "" {
		return fmt.Errorf("authored-script command failed with exit code %d: %w; stderr: %s", result.ExitCode, err, stderr)
	}
	return fmt.Errorf("authored-script command failed with exit code %d: %w", result.ExitCode, err)
}

func errorStderr(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if len(stderr) <= maxErrorStderrSize {
		return stderr
	}
	return "[truncated] " + stderr[len(stderr)-maxErrorStderrSize:]
}
