// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package scriptprocess runs script processes with bounded output and cleans up
// their whole process tree on cancellation, on output exhaustion, and after exit.
package scriptprocess

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

// processWaitDelay bounds how long Wait keeps waiting for output pipes held open
// by descendants once the process has exited or been canceled.
const processWaitDelay = 5 * time.Second

// ErrOutputLimitExceeded is returned when the combined stdout and stderr exceed the output limit.
var ErrOutputLimitExceeded = errors.New("script output limit exceeded")

// StartError wraps an error that prevented the process from starting.
type StartError struct {
	Err error
}

func (e *StartError) Error() string { return e.Err.Error() }

func (e *StartError) Unwrap() error { return e.Err }

// Result contains the observable result of a script execution.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

type commandController interface {
	// cancel stops the running process tree.
	cancel() error
	// terminate stops any descendants left behind and releases resources once the process has exited.
	terminate() error
}

// Run starts cmd isolated from the runner (its own process group on Unix, a
// kill-on-close job object on Windows), captures its output up to outputLimit
// bytes, and waits for it to finish.
//
// The whole process tree is killed when ctx is done or the output limit is
// reached, and any descendants still running after the process exits are killed
// as well. If ctx ends the run, the returned error is non-nil even when the
// process itself exited successfully. cmd.Stdout, cmd.Stderr and cmd.WaitDelay
// are overwritten.
func Run(ctx context.Context, cmd *exec.Cmd, outputLimit int64) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("script context is required")
	}
	if cmd == nil {
		return Result{}, errors.New("script command is required")
	}
	if outputLimit <= 0 {
		return Result{}, errors.New("script output limit must be positive")
	}

	configureCommand(cmd)

	stdout, stderr := util.NewLimitedStdoutStderrWritersPair(outputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	controller, err := startCommand(cmd)
	if err != nil {
		return Result{ExitCode: -1, Duration: time.Since(start)}, &StartError{Err: err}
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
		cancellationErr = controller.cancel()
		runErr = <-waitCh
		if runErr == nil {
			// The script exited cleanly, but descendants held its output until ctx ended the run.
			runErr = ctx.Err()
		}
	case <-stdout.LimitReachedSignal():
		cancellationErr = controller.cancel()
		runErr = <-waitCh
	}

	terminationErr := controller.terminate()
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
		return result, ErrOutputLimitExceeded
	}
	if cancellationErr != nil {
		return result, errors.Join(runErr, fmt.Errorf("could not cancel script command: %w", cancellationErr))
	}
	if terminationErr != nil {
		return result, errors.Join(runErr, fmt.Errorf("could not terminate script process group: %w", terminationErr))
	}
	return result, runErr
}
