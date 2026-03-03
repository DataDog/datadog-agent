// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package sandboxed provides sandboxed shell script execution. It verifies
// scripts using the verifier package and executes them inside an agentfs
// overlay filesystem sandbox via the agentfs CLI.
package sandboxed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/shell/verifier"
	"github.com/google/uuid"
)

const (
	// DefaultTimeout is the default execution timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxOutputBytes is the maximum output size (1MB).
	DefaultMaxOutputBytes = 1 << 20 // 1MB
)

// ErrAgentFSNotFound is returned when the agentfs binary is not on PATH.
var ErrAgentFSNotFound = errors.New("agentfs binary not found in PATH")

// Result contains the output of an executed sandboxed script.
type Result struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	DurationMillis int64  `json:"durationMillis"`
	SessionID      string `json:"sessionId"`
}

// Option configures the executor.
type Option func(*options)

type options struct {
	timeout       time.Duration
	maxOutputSize int
	env           []string
	sessionID     string
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

// WithSession sets an existing agentfs session ID. If not set, a new session
// is created automatically.
func WithSession(id string) Option {
	return func(o *options) {
		o.sessionID = id
	}
}

// CheckAvailability verifies that the agentfs binary is available on PATH.
func CheckAvailability() error {
	_, err := exec.LookPath("agentfs")
	if err != nil {
		return ErrAgentFSNotFound
	}
	return nil
}

// InitSession creates a new agentfs session by running `agentfs init <id>`.
func InitSession(ctx context.Context, sessionID string) error {
	cmd := exec.CommandContext(ctx, "agentfs", "init", sessionID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("agentfs init failed: %w: %s", err, string(output))
	}
	return nil
}

// CloseSession removes the agentfs session database and associated WAL/SHM files.
func CloseSession(sessionID string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	base := filepath.Join(home, ".agentfs", sessionID)
	extensions := []string{".db", ".db-wal", ".db-shm"}

	var errs []error
	for _, ext := range extensions {
		path := base + ext
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", path, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Execute verifies and executes a shell script inside an agentfs sandbox.
// The script is first verified for safety using the verifier package.
// If verification passes, the script is executed via agentfs run --session <id> /bin/sh -c <script>.
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

	// Create or reuse session.
	sessionID := o.sessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
		if err := InitSession(ctx, sessionID); err != nil {
			return nil, fmt.Errorf("failed to create agentfs session: %w", err)
		}
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

	// Execute via agentfs run.
	start := time.Now()
	cmd := exec.CommandContext(execCtx, "agentfs", "run", "--session", sessionID, "/bin/sh", "-c", script)

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
		SessionID:      sessionID,
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
