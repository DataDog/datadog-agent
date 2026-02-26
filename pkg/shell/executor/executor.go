// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package executor provides safe shell script execution. It verifies scripts
// using the verifier package before executing them via /bin/sh.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/DataDog/datadog-agent/pkg/shell/verifier"
)

const (
	// DefaultTimeout is the default execution timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxOutputBytes is the maximum output size (1MB).
	DefaultMaxOutputBytes = 1 << 20 // 1MB
)

// Result contains the output of an executed script.
type Result struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	DurationMillis int64  `json:"durationMillis"`
}

// Option configures the executor.
type Option func(*options)

type options struct {
	timeout       time.Duration
	maxOutputSize int
	env           []string
}

// WithTimeout sets the execution timeout.
func WithTimeout(d time.Duration) Option {
	return func(o *options) {
		o.timeout = d
	}
}

// WithMaxOutputSize sets the maximum output size in bytes.
func WithMaxOutputSize(n int) Option {
	return func(o *options) {
		o.maxOutputSize = n
	}
}

// WithEnv sets the environment variables for the command.
func WithEnv(env []string) Option {
	return func(o *options) {
		o.env = env
	}
}

// Execute verifies and executes a shell script.
// The script is first verified for safety using the verifier package.
// If verification passes, the script is executed via /bin/sh -c.
func Execute(ctx context.Context, script string, opts ...Option) (*Result, error) {
	// Verify the script first.
	if err := verifier.Verify(script); err != nil {
		return nil, fmt.Errorf("script verification failed: %w", err)
	}

	// Apply options.
	o := &options{
		timeout:       DefaultTimeout,
		maxOutputSize: DefaultMaxOutputBytes,
	}
	for _, opt := range opts {
		opt(o)
	}

	// Create a timeout context if the parent doesn't already have a deadline
	// that's sooner than our timeout.
	execCtx := ctx
	if o.timeout > 0 {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > o.timeout {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, o.timeout)
			defer cancel()
		}
	}

	// Execute via /bin/sh.
	start := time.Now()
	cmd := exec.CommandContext(execCtx, "/bin/sh", "-c", script)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdout, limit: o.maxOutputSize}
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: o.maxOutputSize}

	if o.env != nil {
		cmd.Env = o.env
	}

	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:         stdout.String(),
		Stderr:         stderr.String(),
		DurationMillis: duration.Milliseconds(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, fmt.Errorf("execution failed: %w", err)
		}
	}

	return result, nil
}

// limitedWriter wraps a bytes.Buffer and stops writing after a limit is reached.
type limitedWriter struct {
	buf     *bytes.Buffer
	limit   int
	written int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		// Silently discard — we've hit the cap.
		return len(p), nil
	}
	toWrite := p
	if len(p) > remaining {
		toWrite = p[:remaining]
	}
	n, err := w.buf.Write(toWrite)
	w.written += n
	if err != nil {
		return n, err
	}
	// Always report all bytes as consumed to satisfy io.Writer contract
	// and prevent io.Copy from returning ErrShortWrite.
	return len(p), nil
}
