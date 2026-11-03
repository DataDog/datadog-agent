// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteOutcomeDeadlineClampsToShorterCtxDeadline(t *testing.T) {
	now := time.Now()
	ctxDeadline := now.Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), ctxDeadline)
	defer cancel()
	require.Equal(t, ctxDeadline, writeOutcomeDeadline(ctx, now))
}

func TestWriteOutcomeDeadlineUsesTimeoutWhenCtxDeadlineIsLater(t *testing.T) {
	now := time.Now()
	require.Equal(t, now.Add(writeOutcomeWaitTimeout), writeOutcomeDeadline(context.Background(), now))
	ctx, cancel := context.WithDeadline(context.Background(), now.Add(time.Hour))
	defer cancel()
	require.Equal(t, now.Add(writeOutcomeWaitTimeout), writeOutcomeDeadline(ctx, now))
}

func TestWriteWithOutcomeReservesTimeWithinBudget(t *testing.T) {
	for _, budget := range []time.Duration{200 * time.Millisecond, 5 * time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			start := time.Now()
			deadline := start.Add(budget)
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			defer cancel()
			_, err := writeWithOutcome(ctx, func(writeCtx context.Context) (bool, error) {
				writeDeadline, ok := writeCtx.Deadline()
				require.True(t, ok)
				require.True(t, writeDeadline.After(start))
				require.True(t, writeDeadline.Before(deadline))
				require.LessOrEqual(t, deadline.Sub(writeDeadline), min(time.Second, budget/2))
				return false, nil
			})
			require.NoError(t, err)
		})
	}
}

func TestWriteWithOutcomePropagatesExplicitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := writeWithOutcome(ctx, func(writeCtx context.Context) (bool, error) {
		return false, writeCtx.Err()
	})
	require.ErrorIs(t, err, context.Canceled)
}
