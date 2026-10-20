// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package vmstat

import (
	"bytes"
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	ivmstat "github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/vmstat"
)

func TestSubClamp(t *testing.T) {
	assert.EqualValues(t, 5, subClamp(10, 5))
	assert.EqualValues(t, 0, subClamp(5, 10), "a counter reset (prev > cur) must clamp to 0, not underflow")
	assert.EqualValues(t, 0, subClamp(5, 5))
}

func TestSatUint64(t *testing.T) {
	assert.EqualValues(t, 0, satUint64(0))
	assert.EqualValues(t, 0, satUint64(-1), "negative input must clamp to 0")
	assert.EqualValues(t, 0, satUint64(math.NaN()), "NaN must not reach an implementation-defined integer conversion")
	assert.EqualValues(t, 42, satUint64(42.4))
	assert.EqualValues(t, uint64(math.MaxUint64), satUint64(math.MaxUint64))
	assert.EqualValues(t, uint64(math.MaxUint64), satUint64(math.MaxUint64*2), "overflowing float must saturate, not wrap")
}

func TestPct(t *testing.T) {
	assert.EqualValues(t, 50, pct(5, 10))
	assert.EqualValues(t, 100, pct(10, 10))
	assert.EqualValues(t, 0, pct(0, 10))
	assert.EqualValues(t, 0, pct(1, 0), "zero total must not convert NaN to an integer")
	assert.EqualValues(t, 33, pct(1, 3), "rounds to nearest, not truncates")
}

func TestCPUPercents_SinceBoot(t *testing.T) {
	cur := ivmstat.Stats{CPUUser: 60, CPUIdle: 40}
	us, sy, id, wa, st := cpuPercents(cur, nil)
	assert.EqualValues(t, 60, us)
	assert.EqualValues(t, 0, sy)
	assert.EqualValues(t, 40, id)
	assert.EqualValues(t, 0, wa)
	assert.EqualValues(t, 0, st)
}

func TestCPUPercents_Delta(t *testing.T) {
	prev := ivmstat.Stats{CPUUser: 100, CPUIdle: 900}
	cur := ivmstat.Stats{CPUUser: 150, CPUIdle: 950}
	us, sy, id, wa, st := cpuPercents(cur, &prev)
	assert.EqualValues(t, 50, us)
	assert.EqualValues(t, 0, sy)
	assert.EqualValues(t, 50, id)
	assert.EqualValues(t, 0, wa)
	assert.EqualValues(t, 0, st)
}

func TestCPUPercents_CounterReset(t *testing.T) {
	// A counter reset (cur < prev, e.g. an adversarial or momentarily
	// inconsistent read) must not underflow into a bogus huge percentage;
	// every tick delta clamps to 0 via subClamp, leaving total == 0.
	prev := ivmstat.Stats{CPUUser: 1000, CPUIdle: 5000}
	cur := ivmstat.Stats{CPUUser: 10, CPUIdle: 10}
	us, sy, id, wa, st := cpuPercents(cur, &prev)
	assert.EqualValues(t, 0, us)
	assert.EqualValues(t, 0, sy)
	assert.EqualValues(t, 100, id)
	assert.EqualValues(t, 0, wa)
	assert.EqualValues(t, 0, st)
}

func TestCPUPercents_ZeroTotal(t *testing.T) {
	us, sy, id, wa, st := cpuPercents(ivmstat.Stats{}, nil)
	assert.EqualValues(t, 0, us)
	assert.EqualValues(t, 0, sy)
	assert.EqualValues(t, 100, id, "no elapsed ticks must report fully idle, not divide by zero")
	assert.EqualValues(t, 0, wa)
	assert.EqualValues(t, 0, st)
}

func TestCPUPercents_LargeCountersDoNotOverflow(t *testing.T) {
	cur := ivmstat.Stats{
		CPUUser: math.MaxUint64, CPUNice: math.MaxUint64,
		CPUSystem: math.MaxUint64, CPUIRQ: math.MaxUint64, CPUSoftIRQ: math.MaxUint64,
		CPUIdle: math.MaxUint64, CPUIOWait: math.MaxUint64, CPUSteal: math.MaxUint64,
	}
	us, sy, id, wa, st := cpuPercents(cur, nil)
	assert.EqualValues(t, 100, us+sy+id+wa+st)
	assert.GreaterOrEqual(t, id, int64(0))
}

func TestRateSwap(t *testing.T) {
	cur := ivmstat.Stats{SwapInPages: 100, SwapOutPages: 50, PageSize: 4096}
	prev := ivmstat.Stats{SwapInPages: 50, SwapOutPages: 20, PageSize: 4096}
	si, so := rateSwap(cur, &prev, 2, 1024)
	assert.EqualValues(t, (100-50)*4/2, si)
	assert.EqualValues(t, (50-20)*4/2, so)
}

func TestRateSwap_RespectsSelectedUnit(t *testing.T) {
	cur := ivmstat.Stats{SwapInPages: 1000, PageSize: 4096}
	k, _ := rateSwap(cur, nil, 1, 1000)
	capitalK, _ := rateSwap(cur, nil, 1, 1024)
	assert.Greater(t, k, capitalK)
}

func TestRateSwap_CounterReset(t *testing.T) {
	cur := ivmstat.Stats{SwapInPages: 5, PageSize: 4096}
	prev := ivmstat.Stats{SwapInPages: 500, PageSize: 4096}
	si, _ := rateSwap(cur, &prev, 1, 1024)
	assert.EqualValues(t, 0, si, "prev > cur must clamp to 0 via subClamp, not underflow")
}

func TestRateIO(t *testing.T) {
	cur := ivmstat.Stats{PagesInKB: 1000, PagesOutKB: 500}
	prev := ivmstat.Stats{PagesInKB: 200, PagesOutKB: 100}
	bi, bo := rateIO(cur, &prev, 4)
	assert.EqualValues(t, (1000-200)/4, bi)
	assert.EqualValues(t, (500-100)/4, bo)
}

func TestRateHelpers_ZeroElapsed(t *testing.T) {
	cur := ivmstat.Stats{SwapInPages: 1, PagesInKB: 1, Interrupts: 1, PageSize: 4096}
	si, so := rateSwap(cur, nil, 0, 1024)
	bi, bo := rateIO(cur, nil, 0)
	in, cs := rateSystem(cur, nil, 0)
	assert.Zero(t, si)
	assert.Zero(t, so)
	assert.Zero(t, bi)
	assert.Zero(t, bo)
	assert.Zero(t, in)
	assert.Zero(t, cs)
}

func TestRateSystem(t *testing.T) {
	cur := ivmstat.Stats{Interrupts: 1000, ContextSwitches: 500}
	prev := ivmstat.Stats{Interrupts: 200, ContextSwitches: 100}
	in, cs := rateSystem(cur, &prev, 4)
	assert.EqualValues(t, (1000-200)/4, in)
	assert.EqualValues(t, (500-100)/4, cs)
}

func TestParseSamplingArgsRequiresBoundedCount(t *testing.T) {
	_, _, err := parseSamplingArgs([]string{"1"})
	assert.EqualError(t, err, "count is required when delay is specified")

	delay, count, err := parseSamplingArgs([]string{"1", "30"})
	assert.NoError(t, err)
	assert.Equal(t, time.Second, delay)
	assert.Equal(t, 30, count)

	_, _, err = parseSamplingArgs([]string{"2", "30"})
	assert.EqualError(t, err, "sampling duration exceeds 29s")
}

func TestValidateStatsArgsAllowsIgnoredDelay(t *testing.T) {
	assert.NoError(t, validateStatsArgs([]string{"1"}))
	assert.NoError(t, validateStatsArgs([]string{"1", "2"}))
	assert.EqualError(t, validateStatsArgs([]string{"invalid"}), "invalid delay 'invalid'")
}

// A long delay combined with a short-lived context proves the select on
// ctx.Done() returns promptly during a bounded sampling invocation.
func TestRunSamplingStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	var stdout, stderr bytes.Buffer
	callCtx := &builtins.CallContext{Stdout: &stdout, Stderr: &stderr}

	start := time.Now()
	res := runSampling(ctx, callCtx, false, false, 1, time.Hour, 2)
	elapsed := time.Since(start)

	assert.EqualValues(t, 1, res.Code, "a cancelled context must abort the sampling loop with a non-zero exit")
	assert.Less(t, elapsed, 5*time.Second, "the loop must return promptly on cancellation, not wait out the delay")
}
