// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procinfo

import "context"

// collectAncestorPIDs walks parent links until the platform-specific terminal
// PID, a missing row, cancellation, or a cycle in the process snapshot.
func collectAncestorPIDs(
	ctx context.Context,
	byPID map[int]ProcInfo,
	startPID, terminalPID int,
) map[int]bool {
	ancestors := make(map[int]bool)
	for current := startPID; current > terminalPID; {
		if ctx.Err() != nil || ancestors[current] {
			break
		}
		ancestors[current] = true
		proc, ok := byPID[current]
		if !ok {
			break
		}
		current = proc.PPID
	}
	return ancestors
}
