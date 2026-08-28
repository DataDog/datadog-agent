// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

const (
	defaultOutputLimit = 10 * 1024 * 1024
	maxErrorStderrSize = 16 * 1024
)

var errOutputLimitExceeded = errors.New("authored-script output limit exceeded")

// Result contains the observable result of an authored-script execution.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

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
	if cmd == nil {
		return Result{}, errors.New("authored-script command is required")
	}
	if outputLimit <= 0 {
		return Result{}, errors.New("authored-script output limit must be positive")
	}

	stdout, stderr := util.NewLimitedStdoutStderrWritersPair(outputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{ExitCode: -1, Duration: time.Since(start)}, err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	var cancellationErr error
	var runErr error
	select {
	case runErr = <-waitCh:
	case <-ctx.Done():
		cancellationErr = cancelCommand(cmd)
		runErr = <-waitCh
	case <-stdout.LimitReachedSignal():
		cancellationErr = cancelCommand(cmd)
		runErr = <-waitCh
	}

	terminationErr := terminateCommand(cmd)
	result := Result{
		ExitCode: -1,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if stdout.LimitReached() || stderr.LimitReached() {
		return result, fmt.Errorf("authored-script output exceeded %d bytes: %w", outputLimit, errOutputLimitExceeded)
	}
	if cancellationErr != nil {
		if runErr != nil {
			return result, errors.Join(runErr, fmt.Errorf("could not cancel authored-script command: %w", cancellationErr))
		}
		return result, fmt.Errorf("could not cancel authored-script command: %w", cancellationErr)
	}
	if runErr != nil && terminationErr != nil {
		return result, errors.Join(runErr, fmt.Errorf("could not terminate authored-script process group: %w", terminationErr))
	}
	if runErr != nil {
		return result, runErr
	}
	if terminationErr != nil {
		return result, fmt.Errorf("could not terminate authored-script process group: %w", terminationErr)
	}
	return result, nil
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
