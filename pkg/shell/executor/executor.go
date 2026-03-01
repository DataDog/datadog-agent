// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package executor provides safe shell script execution using a restricted
// interpreter that directly executes allowed commands without invoking /bin/sh.
package executor

import (
	"bytes"
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
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

// Execute interprets a shell script using the restricted interpreter.
// Only allowed commands, pipes, &&/||, and for-in loops are supported.
func Execute(ctx context.Context, script string, opts ...Option) (*Result, error) {
	o := &options{
		timeout:       DefaultTimeout,
		maxOutputSize: DefaultMaxOutputBytes,
	}
	for _, opt := range opts {
		opt(o)
	}

	// Create a timeout context.
	execCtx := ctx
	if o.timeout > 0 {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > o.timeout {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, o.timeout)
			defer cancel()
		}
	}

	var stdout, stderr bytes.Buffer
	runner := interp.New(
		interp.WithStdout(&limitedWriter{buf: &stdout, limit: o.maxOutputSize}),
		interp.WithStderr(&limitedWriter{buf: &stderr, limit: o.maxOutputSize}),
		interp.WithEnv(o.env),
	)

	start := time.Now()
	err := runner.Run(execCtx, script)
	duration := time.Since(start)

	result := &Result{
		ExitCode:       runner.ExitCode(),
		Stdout:         stdout.String(),
		Stderr:         stderr.String(),
		DurationMillis: duration.Milliseconds(),
	}

	if err != nil {
		return result, err
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
	return len(p), nil
}
