// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"context"
	"errors"
	"fmt"
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

// Runner resolves, validates, and executes an authored script in an isolated session.
type Runner struct {
	resolver    *artifacts.Resolver
	platform    artifacts.Platform
	outputLimit int64
}

// NewRunner creates an authored-script runner for one artifact source and platform.
func NewRunner(resolver *artifacts.Resolver, platform artifacts.Platform) *Runner {
	return &Runner{
		resolver:    resolver,
		platform:    platform,
		outputLimit: defaultOutputLimit,
	}
}

// Run executes the package authorized for an action FQN and removes its session afterward.
func (r *Runner) Run(ctx context.Context, fqn string, parameters interface{}) (result Result, err error) {
	if ctx == nil {
		return Result{}, errors.New("authored-script context is required")
	}
	if r == nil || r.resolver == nil {
		return Result{}, errors.New("authored-script runner is not configured")
	}
	if fqn == "" {
		return Result{}, errors.New("authored-script FQN is required")
	}

	descriptor, artifact, err := r.resolver.Resolve(ctx, fqn, r.platform)
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
	result, executionErr := executeCommand(cmd, r.outputLimit)
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
