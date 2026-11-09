// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procinfo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectAncestorPIDsStopsAtTerminalMissingCycleAndCancellation(t *testing.T) {
	t.Run("terminal pid", func(t *testing.T) {
		byPID := map[int]ProcInfo{
			42: {PID: 42, PPID: 7},
			7:  {PID: 7, PPID: 1},
			1:  {PID: 1, PPID: 0},
		}

		require.Equal(
			t,
			map[int]bool{42: true, 7: true},
			collectAncestorPIDs(context.Background(), byPID, 42, 1),
		)
	})

	t.Run("missing parent", func(t *testing.T) {
		byPID := map[int]ProcInfo{
			42: {PID: 42, PPID: 7},
		}

		require.Equal(
			t,
			map[int]bool{42: true, 7: true},
			collectAncestorPIDs(context.Background(), byPID, 42, 1),
		)
	})

	t.Run("cycle", func(t *testing.T) {
		byPID := map[int]ProcInfo{
			42: {PID: 42, PPID: 7},
			7:  {PID: 7, PPID: 42},
		}

		require.Equal(
			t,
			map[int]bool{42: true, 7: true},
			collectAncestorPIDs(context.Background(), byPID, 42, 1),
		)
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		require.Empty(t, collectAncestorPIDs(ctx, map[int]ProcInfo{
			42: {PID: 42, PPID: 7},
		}, 42, 1))
	})
}
