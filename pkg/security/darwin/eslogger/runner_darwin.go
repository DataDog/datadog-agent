// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eslogger

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Path is the system eslogger binary. It ships pre-entitled for Endpoint
// Security, which is what lets a proof of concept run without an Apple
// entitlement of our own.
const Path = "/usr/bin/eslogger"

// Runner spawns eslogger and exposes its stdout.
type Runner struct {
	events []string
	cmd    *exec.Cmd

	mu     sync.Mutex
	stderr []string
}

// NewRunner returns a Runner subscribing to the given ES event names.
func NewRunner(events []string) *Runner {
	return &Runner{events: events}
}

// Start launches eslogger and returns its stdout.
//
// Two preconditions are easy to get wrong and fail in confusingly different
// ways. The process must run as root, which fails loudly with
// ES_NEW_CLIENT_RESULT_ERR_NOT_PRIVILEGED. The calling context must also hold
// Full Disk Access, which fails with
// ES_NEW_CLIENT_RESULT_ERR_NOT_PERMITTED — or, worse, presents as eslogger
// starting normally and emitting nothing at all. Stderr is therefore captured
// and surfaced by Stderr() rather than discarded.
func (r *Runner) Start(ctx context.Context) (io.ReadCloser, error) {
	if len(r.events) == 0 {
		return nil, errors.New("no events requested")
	}

	r.cmd = exec.CommandContext(ctx, Path, r.events...)

	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := r.cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := r.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", Path, err)
	}

	go r.drainStderr(stderr)

	return stdout, nil
}

// drainStderr accumulates eslogger's diagnostics so a permission failure can be
// reported instead of looking like an empty event stream.
func (r *Runner) drainStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		r.mu.Lock()
		r.stderr = append(r.stderr, line)
		r.mu.Unlock()
	}
}

// Stderr returns whatever eslogger wrote to stderr so far.
func (r *Runner) Stderr() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.stderr...)
}

// Wait waits for eslogger to exit.
func (r *Runner) Wait() error {
	if r.cmd == nil {
		return nil
	}
	return r.cmd.Wait()
}
