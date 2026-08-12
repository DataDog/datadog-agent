// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"errors"
	"fmt"
	"os/exec"
	"time"

	commandsupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/command"
)

const defaultOutputLimit = 10 * 1024 * 1024

var errOutputLimitExceeded = errors.New("authored-script output limit exceeded")

func executeCommand(cmd *exec.Cmd, outputLimit int64) (Result, error) {
	if cmd == nil {
		return Result{}, errors.New("authored-script command is required")
	}
	if outputLimit <= 0 {
		return Result{}, errors.New("authored-script output limit must be positive")
	}

	stdout, stderr := commandsupport.NewLimitedOutputPair(outputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	runErr := cmd.Run()
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
