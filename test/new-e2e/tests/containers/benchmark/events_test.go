// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPhaseEvent(t *testing.T) {
	windows := testWindows(4, 15*time.Minute)
	baseline := []string{"agent:1-linux", "cluster-agent:1"}
	comparison := []string{"agent:2-linux", "cluster-agent:2"}

	start := PhaseEvent{Boundary: PhaseStart, Phase: 0, Phases: 4, Window: windows[0], Baseline: baseline, Comparison: comparison}
	assert.Equal(t, windows[0].From, start.Time())
	assert.Equal(t, "benchmarkeks phase 1/4 started: baseline on installation a, comparison on installation b", start.Title())
	assert.Equal(t, "%%% \n"+
		"Measurement phase 1 of 4, from 2026-10-13 10:00 UTC to 2026-10-13 10:15 UTC (15m0s):\n\n"+
		"- installation a runs the baseline images: `agent:1-linux`, `cluster-agent:1`\n"+
		"- installation b runs the comparison images: `agent:2-linux`, `cluster-agent:2`\n"+
		" %%%", start.Text())
	assert.Equal(t, []string{"benchmark_phase:1", "benchmark_phase_boundary:start"}, start.Tags())

	// In the second phase, the installations swap their images.
	end := PhaseEvent{Boundary: PhaseEnd, Phase: 1, Phases: 4, Window: windows[1], Baseline: baseline, Comparison: comparison}
	assert.Equal(t, windows[1].To, end.Time())
	assert.Equal(t, "benchmarkeks phase 2/4 ended: baseline on installation b, comparison on installation a", end.Title())
	assert.Equal(t, "%%% \n"+
		"Measurement phase 2 of 4, from 2026-10-13 11:00 UTC to 2026-10-13 11:15 UTC (15m0s):\n\n"+
		"- installation b runs the baseline images: `agent:1-linux`, `cluster-agent:1`\n"+
		"- installation a runs the comparison images: `agent:2-linux`, `cluster-agent:2`\n"+
		" %%%", end.Text())
	assert.Equal(t, []string{"benchmark_phase:2", "benchmark_phase_boundary:end"}, end.Tags())
}
