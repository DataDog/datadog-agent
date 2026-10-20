// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/stretchr/testify/require"
)

// Pause at the cancellation check after the first chunk has been written.
// The outer abandonment path's own Err call must remain nonblocking.
type pausedWriteContext struct {
	context.Context
	calls   atomic.Int32
	release <-chan struct{}
}

func (c *pausedWriteContext) Err() error {
	if c.calls.Add(1) == 3 {
		<-c.release
	}
	return c.Context.Err()
}

func TestWriteWithOutcomeResolvesPartialWriteBeforeCallerDeadline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	original := bytes.Repeat([]byte("a"), 65536)
	require.NoError(t, os.WriteFile(path, original, 0600))
	sb, _, err := allowedpaths.New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mutated, err := writeWithOutcome(ctx, func(writeCtx context.Context) (bool, error) {
		release := make(chan struct{})
		defer close(release)
		paused := &pausedWriteContext{Context: writeCtx, release: release}
		mutated, err := sb.WriteRegularFile(paused, path, dir, bytes.Repeat([]byte("b"), len(original)), nil)
		require.True(t, allowedpaths.IsWriteOutcomeUnknown(err), "the worker must still be paused when the write deadline fires")
		return mutated, err // release lets the worker report its final partial-write failure
	})
	require.True(t, mutated)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, builtins.ErrWriteOutcomeUnknown, "the reserved window must resolve the writer before returning")
	require.NoError(t, ctx.Err(), "outcome resolution must finish inside the caller's budget")

	partial, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, byte('b'), partial[0], "the write must really have changed part of the file")
	require.Equal(t, byte('a'), partial[len(partial)-1], "the write must have stopped before replacing the whole file")

	// A resolved partial-write error permits the caller's best-effort restore.
	_, err = sb.WriteRegularFile(context.Background(), path, dir, original, nil)
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, got)
}
