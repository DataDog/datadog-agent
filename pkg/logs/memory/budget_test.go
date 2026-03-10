// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetAcquireRelease(t *testing.T) {
	budget := New(Config{
		Enabled:        true,
		MaxBytes:       10,
		OverflowPolicy: OverflowPolicyBlock,
	})

	require.NoError(t, budget.Acquire("processor", 4))
	require.NoError(t, budget.Acquire("sender", 3))

	snapshot := budget.Snapshot()
	assert.Equal(t, int64(7), snapshot.UsedBytes)
	assert.Equal(t, int64(4), snapshot.ComponentBytes["processor"])
	assert.Equal(t, int64(3), snapshot.ComponentBytes["sender"])

	require.NoError(t, budget.Release("processor", 4))
	require.NoError(t, budget.Release("sender", 3))

	snapshot = budget.Snapshot()
	assert.Equal(t, int64(0), snapshot.UsedBytes)
	assert.Empty(t, snapshot.ComponentBytes)
}

func TestBudgetTryAcquire(t *testing.T) {
	budget := New(Config{
		Enabled:        true,
		MaxBytes:       5,
		OverflowPolicy: OverflowPolicyBlock,
	})

	ok, err := budget.TryAcquire("processor", 5)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = budget.TryAcquire("sender", 1)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestBudgetAcquireBlocksUntilRelease(t *testing.T) {
	budget := New(Config{
		Enabled:        true,
		MaxBytes:       5,
		OverflowPolicy: OverflowPolicyBlock,
	})

	require.NoError(t, budget.Acquire("processor", 5))

	acquired := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- budget.Acquire("sender", 1)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("acquire returned before bytes were released")
	case <-time.After(50 * time.Millisecond):
	}

	require.NoError(t, budget.Release("processor", 1))

	select {
	case <-acquired:
		require.NoError(t, <-errCh)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for acquire to unblock")
	}
}

func TestBudgetReleaseRejectsOverRelease(t *testing.T) {
	budget := New(Config{
		Enabled:        true,
		MaxBytes:       5,
		OverflowPolicy: OverflowPolicyBlock,
	})

	require.NoError(t, budget.Acquire("processor", 2))
	assert.ErrorIs(t, budget.Release("processor", 3), ErrReleaseExceedsReservation)
}
