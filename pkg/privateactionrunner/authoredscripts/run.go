// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/artifacts"
)

const maxErrorStderrSize = 16 * 1024

// Result contains the observable result of an authored-script execution.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// Execute runs the cached package authorized for an action FQN in an isolated session.
func Execute(ctx context.Context, fqn string, parameters interface{}) (result Result, err error) {
	if ctx == nil {
		return Result{}, errors.New("authored-script context is required")
	}
	if fqn == "" {
		return Result{}, errors.New("authored-script FQN is required")
	}

	provider, err := NewStaticProvider()
	if err != nil {
		return Result{}, err
	}
	resolver := artifacts.NewResolver(NewStaticCatalog(), provider)
	descriptor, artifact, err := resolver.Resolve(ctx, fqn, artifacts.Platform{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	})
	if err != nil {
		return Result{}, err
	}
	loadedPackage, err := LoadPackage(fqn, descriptor, artifact)
	if err != nil {
		return Result{}, err
	}

	session, err := NewSession()
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if cleanupErr := session.Cleanup(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()

	cmd, err := NewCommand(ctx, loadedPackage, session, parameters)
	if err != nil {
		return Result{}, err
	}
	result, executionErr := executeCommand(cmd, defaultOutputLimit)
	if executionErr != nil {
		return result, formatExecutionError(ctx, result, executionErr)
	}
	return result, nil
}

func formatExecutionError(ctx context.Context, result Result, err error) error {
	if errors.Is(err, errOutputLimitExceeded) {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
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
