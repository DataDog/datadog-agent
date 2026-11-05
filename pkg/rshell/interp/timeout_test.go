// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTimeoutRunner(t *testing.T, opts ...RunnerOption) *Runner {
	t.Helper()
	allOpts := append([]RunnerOption{allowAllCommandsOpt()}, opts...)
	r, err := New(allOpts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Reset()
	return r
}

func TestMaxExecutionTimeRejectsNegative(t *testing.T) {
	_, err := New(MaxExecutionTime(-time.Second))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MaxExecutionTime")
}

func TestMaxExecutionTimeStopsRun(t *testing.T) {
	r := newTimeoutRunner(t, MaxExecutionTime(20*time.Millisecond))
	r.execHandler = func(ctx context.Context, _ []string) error {
		<-ctx.Done()
		return ctx.Err()
	}

	err := r.Run(context.Background(), parseScript(t, "slowcmd"))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMaxExecutionTimeRespectsEarlierParentDeadline(t *testing.T) {
	r := newTimeoutRunner(t, MaxExecutionTime(time.Second))
	var got time.Time
	r.execHandler = func(ctx context.Context, _ []string) error {
		var ok bool
		got, ok = ctx.Deadline()
		require.True(t, ok, "expected deadline on exec handler context")
		return nil
	}

	parent, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	parentDeadline, ok := parent.Deadline()
	require.True(t, ok)

	err := r.Run(parent, parseScript(t, "slowcmd"))
	require.NoError(t, err)
	// context.WithTimeout takes the earlier of the two deadlines, so the runner's 1s
	// MaxExecutionTime must not override the parent's tighter 25ms deadline.
	assert.WithinDuration(t, parentDeadline, got, 5*time.Millisecond)
}

func TestMaxExecutionTimeStopsForLoop(t *testing.T) {
	// Exercises the interpreter's own ctx.Err() check inside the for-loop body
	// (runner_exec.go), not just the execHandler cooperative-cancellation path.
	// We use a for loop with an execHandler that sleeps per iteration to make
	// the loop outlast the timeout.
	r := newTimeoutRunner(t, MaxExecutionTime(50*time.Millisecond))
	r.execHandler = func(ctx context.Context, _ []string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
			return nil
		}
	}

	// 10 iterations × 20ms each = 200ms total, well beyond the 50ms timeout.
	err := r.Run(context.Background(), parseScript(t, "for x in 1 2 3 4 5 6 7 8 9 10; do cmd; done"))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMaxExecutionTimeIsRefreshedPerRun(t *testing.T) {
	r := newTimeoutRunner(t, MaxExecutionTime(100*time.Millisecond))
	var deadlines []time.Time
	r.execHandler = func(ctx context.Context, _ []string) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "expected deadline on exec handler context")
		deadlines = append(deadlines, deadline)
		return nil
	}

	prog := parseScript(t, "slowcmd")

	err := r.Run(context.Background(), prog)
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)

	err = r.Run(context.Background(), prog)
	require.NoError(t, err)

	require.Len(t, deadlines, 2)
	assert.True(t, deadlines[1].After(deadlines[0]), "expected a fresh deadline on each Run")
}

// raceCtx reports a deadline but never fires: it reproduces the window where a
// builtin's own kernel read deadline (derived from ctx.Deadline) expires before
// the context's timer goroutine has set ctx.Err().
type raceCtx struct {
	context.Context
	deadline time.Time
}

func (c raceCtx) Deadline() (time.Time, bool) { return c.deadline, true }

// TestReadHittingRunDeadlineIsNotSuccess guards against a run that hit its
// deadline reporting success: read turns the run deadline into its own
// status-142 timeout, and the script must neither continue nor return nil.
func TestReadHittingRunDeadlineIsNotSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows pipes have no read deadlines, so read waits on ctx.Done()
		// there and cannot race ahead of it; this ctx never fires Done.
		t.Skip("read cannot install a kernel read deadline on Windows pipes")
	}
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })

	var out bytes.Buffer
	r := newTimeoutRunner(t, StdIO(pr, &out, &out))

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := raceCtx{Context: parent, deadline: time.Now().Add(20 * time.Millisecond)}

	err = r.Run(ctx, parseScript(t, "read x; echo after"))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotContains(t, out.String(), "after")
}
