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
	"errors"
	"os/exec"
	"syscall"
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
	binaryPath    string
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

// WithBinaryExec enables subprocess mode: the script is run by invoking the
// safe-shell binary at path instead of the in-process interpreter. The
// subprocess is launched with platform-specific SysProcAttr sandboxing.
func WithBinaryExec(path string) Option {
	return func(o *options) {
		o.binaryPath = path
	}
}

// Execute interprets a shell script using the restricted interpreter.
// Only allowed commands, pipes, &&/||, and for-in loops are supported.
// If WithBinaryExec was used, the script is run as a subprocess instead.
func Execute(ctx context.Context, script string, opts ...Option) (*Result, error) {
	o := &options{
		timeout:       DefaultTimeout,
		maxOutputSize: DefaultMaxOutputBytes,
	}
	for _, opt := range opts {
		opt(o)
	}

	if o.binaryPath != "" {
		return executeSubprocess(ctx, script, o)
	}
	return executeInProcess(ctx, script, o)
}

// executeInProcess runs the script using the in-process restricted interpreter.
func executeInProcess(ctx context.Context, script string, o *options) (*Result, error) {
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

// executeSubprocess runs the script by invoking the safe-shell binary as a
// subprocess with platform-specific SysProcAttr sandboxing. If the sandbox
// fails to start (e.g., namespace creation denied), it falls back to running
// the subprocess without SysProcAttr.
func executeSubprocess(ctx context.Context, script string, o *options) (*Result, error) {
	execCtx := ctx
	if o.timeout > 0 {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > o.timeout {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, o.timeout)
			defer cancel()
		}
	}

	// Try with sandbox first.
	result, err := runSubprocess(execCtx, script, o, sandboxSysProcAttr())
	if err != nil && !errors.As(err, new(*exec.ExitError)) {
		// The process failed to start (not a script exit code error).
		// Retry without sandbox as a fallback.
		result, err = runSubprocess(execCtx, script, o, nil)
	}
	return result, err
}

// runSubprocess executes the safe-shell binary with the given SysProcAttr.
func runSubprocess(ctx context.Context, script string, o *options, sysProcAttr *syscall.SysProcAttr) (*Result, error) {
	var stdout, stderr bytes.Buffer
	stdoutW := &limitedWriter{buf: &stdout, limit: o.maxOutputSize}
	stderrW := &limitedWriter{buf: &stderr, limit: o.maxOutputSize}

	cmd := exec.CommandContext(ctx, o.binaryPath, "-c", script)
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW
	cmd.Env = o.env
	cmd.SysProcAttr = sysProcAttr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			err = nil // non-zero exit is not an error
		}
	}

	return &Result{
		ExitCode:       exitCode,
		Stdout:         stdout.String(),
		Stderr:         stderr.String(),
		DurationMillis: duration.Milliseconds(),
	}, err
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
