// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Tests added by vuln-hunt campaign 2026-05-18-initial-audit / signal-handling
// to pin the executor's behaviour under in-process pipeline back-pressure.
//
// Background: rshell installs no OS signal handlers (no signal.Notify), so the
// only "signal-like" mechanism is context cancellation. Pipelines run in the
// same process; the writer side's stdout is the write-end of an os.Pipe (see
// interp/runner_exec.go:159-219). If the reader side closes early (e.g.
// `head -n 1` after the first line), the writer's next Write to that pipe
// could in principle deliver SIGPIPE. Go's documented behaviour is that
// SIGPIPE on a file descriptor other than stdout/stderr is converted into an
// EPIPE error rather than process termination — but only if the runtime's
// default SIGPIPE disposition is intact, i.e. no caller has called
// signal.Notify(..., syscall.SIGPIPE). These tests verify both invariants
// hold for the rshell pipeline path.

package interp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVulnHuntSubsystemSignalHandling_PipelineEpipeDoesNotKillProcess
// asserts that a pipeline whose reader exits before the writer is done
// (`while true; do echo x; done | head -n 1`) terminates via context
// deadline rather than crashing the rshell process with SIGPIPE.
//
// If a regression caused the writer to receive SIGPIPE without an installed
// handler, the entire test binary would die and this test would never reach
// its assertions — the test passing at all is part of the evidence.
func TestVulnHuntSubsystemSignalHandling_PipelineEpipeDoesNotKillProcess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		MaxExecutionTime(500*time.Millisecond),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	prog := parseScript(t, "while true; do echo x; done | head -n 1")

	start := time.Now()
	err = r.Run(context.Background(), prog)
	elapsed := time.Since(start)

	// Outcome must be a clean cancellation from the runner's MaxExecutionTime,
	// not a process crash or panic. The test would not run to here if the
	// process had taken SIGPIPE.
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded,
		"expected deadline exceeded from MaxExecutionTime, got: %v", err)

	// head consumed exactly one line before closing its stdin; that line must
	// have reached our stdout buffer. If the pipeline tore down before the
	// first byte made it through, this would be empty.
	assert.Equal(t, "x\n", stdout.String(),
		"head should have captured the first line before closing the pipe")

	// Sanity: the test must complete within a reasonable bound of the timeout.
	// A bound far in excess would suggest the loop is not being interrupted
	// promptly, which would itself be a (separate) DoS concern.
	assert.Less(t, elapsed, 2*time.Second,
		"pipeline did not stop promptly after timeout: %s", elapsed)
}

// TestVulnHuntSubsystemSignalHandling_ParentCtxCancelStopsPipeline
// asserts that parent-context cancellation propagates through the pipeline
// goroutines, not just the runner's own MaxExecutionTime. This exercises the
// embedding-as-a-library path where the caller controls cancellation.
func TestVulnHuntSubsystemSignalHandling_ParentCtxCancelStopsPipeline(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	prog := parseScript(t, "while true; do echo x; done | head -n 1")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = r.Run(ctx, prog)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded,
		"expected parent-ctx deadline exceeded, got: %v", err)
	assert.Equal(t, "x\n", stdout.String(),
		"head should have captured the first line before closing the pipe")
	assert.Less(t, elapsed, 2*time.Second,
		"pipeline did not stop promptly after parent-ctx cancel: %s", elapsed)
}

// TestVulnHuntSubsystemSignalHandling_PipelineLeftPanicRecovered asserts
// that the pipeline-left goroutine's panic recovery path converts a panic
// into a controlled runner error. Without the goroutine-local recovery, this
// test process would crash before the right side of the pipeline returned.
func TestVulnHuntSubsystemSignalHandling_PipelineLeftPanicRecovered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	r.Reset()
	r.execHandler = func(ctx context.Context, args []string) error {
		if len(args) > 0 && args[0] == "paniccmd" {
			panic("left pipeline panic sentinel")
		}
		return nil
	}

	err = r.Run(context.Background(), parseScript(t, "paniccmd | true"))
	require.Error(t, err)
	assert.Equal(t, "internal error", err.Error())
	assert.NotContains(t, fmt.Sprint(err), "left pipeline panic sentinel",
		"the returned error must not leak the panic payload")
	assert.Contains(t, stderr.String(), "left pipeline panic sentinel",
		"panic details should stay within the configured stderr writer")
	assert.False(t, errors.Is(err, context.Canceled),
		"panic recovery must not be misclassified as context cancellation")
	assert.Empty(t, stdout.String())
}
