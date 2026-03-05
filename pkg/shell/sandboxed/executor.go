// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package sandboxed provides sandboxed shell script execution with agentfs
// session tracking via the Go SDK. The agentfs overlay filesystem provides
// sandboxing, so no AST-based script verification is performed.
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

	"github.com/google/uuid"
	agentfs "github.com/tursodatabase/agentfs/sdk/go"
)

const (
	// DefaultTimeout is the default execution timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxOutputBytes is the maximum output size (1MB).
	DefaultMaxOutputBytes = 1 << 20 // 1MB
)

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

// Execute executes a shell script with agentfs session tracking.
// The script is executed via /bin/sh -c with the execution recorded in the
// agentfs session's audit trail. The agentfs overlay filesystem provides
// sandboxing; no AST-based script verification is performed.
func Execute(ctx context.Context, script string, opts ...Option) (*Result, error) {
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
	}

	// Open the agentfs session (creates DB if it doesn't exist).
	afs, err := agentfs.Open(ctx, agentfs.AgentFSOptions{ID: sessionID})
	if err != nil {
		return nil, fmt.Errorf("failed to open agentfs session: %w", err)
	}
	defer afs.Close()

	// Start tracking this execution in the audit trail.
	pending, err := afs.Tools.Start(ctx, "sandboxed_shell", map[string]string{
		"script": script,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start tool call tracking: %w", err)
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

	// Execute via /bin/sh -c.
	start := time.Now()
	cmd := exec.CommandContext(execCtx, "/bin/sh", "-c", script)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdout, limit: o.maxOutputSize}
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: o.maxOutputSize}

	if o.env != nil {
		cmd.Env = o.env
	}

	err = cmd.Run()
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
			// Record the non-zero exit as a successful tool call (non-zero exit is not an execution failure).
			pending.Success(ctx, result) //nolint:errcheck
		} else {
			// Record the execution failure in the audit trail.
			pending.Error(ctx, err) //nolint:errcheck
			return result, fmt.Errorf("execution failed: %w", err)
		}
	} else {
		// Record successful execution in the audit trail.
		pending.Success(ctx, result) //nolint:errcheck
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
