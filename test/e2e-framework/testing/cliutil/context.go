// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cliutil provides helpers for using the e2e installer layer outside a
// test binary (e.g. in the e2e-install CLI). It satisfies common.Context
// without requiring *testing.T or any test framework bootstrap.
package cliutil

import (
	"fmt"
	"os"
	"sync"
)

// NewContext returns a common.Context implementation suitable for CLI programs.
// On FailNow the process exits with code 1. Cleanup functions registered via
// Cleanup are called in LIFO order when RunCleanup is invoked.
func NewContext() *CLIContext {
	return &CLIContext{}
}

// CLIContext implements common.Context for use in non-test programs.
type CLIContext struct {
	mu       sync.Mutex
	cleanups []func()
}

// Errorf logs a formatted error message to stderr.
func (c *CLIContext) Errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}

// FailNow prints a failure indicator and exits the process with code 1.
func (c *CLIContext) FailNow() {
	fmt.Fprintln(os.Stderr, "[FATAL] installer reported a failure, exiting")
	c.RunCleanup()
	os.Exit(1)
}

// Logf logs a formatted informational message to stdout.
func (c *CLIContext) Logf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// Helper is a no-op outside test binaries.
func (c *CLIContext) Helper() {}

// Cleanup registers fn to be called when RunCleanup is invoked (LIFO).
func (c *CLIContext) Cleanup(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanups = append(c.cleanups, fn)
}

// RunCleanup executes all registered cleanup functions in LIFO order.
// It is called automatically on FailNow; callers should also defer it in main.
func (c *CLIContext) RunCleanup() {
	c.mu.Lock()
	fns := make([]func(), len(c.cleanups))
	copy(fns, c.cleanups)
	c.cleanups = nil
	c.mu.Unlock()

	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}

// SessionOutputDir returns an empty string; CLI runs do not have a test output
// directory. Set this to a real directory if artifact output is needed.
func (c *CLIContext) SessionOutputDir() string {
	return ""
}
