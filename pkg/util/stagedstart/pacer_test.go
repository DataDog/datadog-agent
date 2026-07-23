// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stagedstart

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock drives now() and sleep() deterministically: sleeping just advances
// virtual time, so tests run instantly and reproducibly on any OS.
type fakeClock struct {
	t time.Time
}

func (c *fakeClock) now() time.Time { return c.t }

func (c *fakeClock) sleep(ctx context.Context, d time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	c.t = c.t.Add(d)
	return true
}

func newTestPacer(cfg Config, clock *fakeClock, sample func() uint64) (*Pacer, *int32, *int32) {
	var reclaims, warns int32
	p := NewPacer(cfg, nil, func(string, ...interface{}) { atomic.AddInt32(&warns, 1) })
	p.now = clock.now
	p.sleep = clock.sleep
	p.sample = sample
	p.reclaim = func() { atomic.AddInt32(&reclaims, 1) }
	return p, &reclaims, &warns
}

func TestPaceDisabledIsNoop(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	p, reclaims, _ := newTestPacer(Config{Enabled: false}, clock, func() uint64 { return 0 })
	p.Pace(context.Background(), "x")
	assert.Equal(t, int32(0), atomic.LoadInt32(reclaims), "disabled pacer must not reclaim")
	assert.Equal(t, time.Unix(0, 0), clock.t, "disabled pacer must not sleep")
}

func TestPaceIntervalMode(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	cfg := Config{Enabled: true, FreeOSMemory: true, Adaptive: false, Interval: 5 * time.Second}
	p, reclaims, _ := newTestPacer(cfg, clock, func() uint64 { return 0 })

	p.Pace(context.Background(), "x")

	assert.Equal(t, 5*time.Second, clock.t.Sub(time.Unix(0, 0)), "interval mode should wait exactly the interval")
	assert.Equal(t, int32(1), atomic.LoadInt32(reclaims), "should reclaim once after the interval")
}

func TestAdaptiveSettlesWhenMemoryFlattens(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	start := clock.t
	cfg := Config{
		Enabled: true, FreeOSMemory: true, Adaptive: true,
		StepMin: 1 * time.Second, StepMax: 30 * time.Second,
		SettleWindow: 2 * time.Second, SettleThreshold: 2 << 20, PollInterval: 250 * time.Millisecond,
	}
	// Memory grows fast for the first 3s, then holds flat.
	sample := func() uint64 {
		elapsed := clock.t.Sub(start)
		if elapsed < 3*time.Second {
			return 100<<20 + uint64(elapsed/(250*time.Millisecond))*(10<<20)
		}
		return 220 << 20
	}
	p, reclaims, warns := newTestPacer(cfg, clock, sample)

	p.Pace(context.Background(), "heavy")

	elapsed := clock.t.Sub(start)
	assert.GreaterOrEqual(t, elapsed, 3*time.Second, "must not settle while still growing")
	assert.Less(t, elapsed, cfg.StepMax, "should settle well before the hard cap")
	assert.Equal(t, int32(1), atomic.LoadInt32(reclaims), "should reclaim once after settling")
	assert.Equal(t, int32(0), atomic.LoadInt32(warns), "clean settle should not warn")
}

// The bounded worst case: memory grows forever (memory pressure). The pacer must
// NOT hang — it proceeds at StepMax and warns loudly.
func TestAdaptiveBoundedWorstCase(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	start := clock.t
	cfg := Config{
		Enabled: true, FreeOSMemory: true, Adaptive: true,
		StepMin: 1 * time.Second, StepMax: 10 * time.Second,
		SettleWindow: 2 * time.Second, SettleThreshold: 2 << 20, PollInterval: 250 * time.Millisecond,
	}
	sample := func() uint64 {
		return 100<<20 + uint64(clock.t.Sub(start)/(250*time.Millisecond))*(10<<20) // never flattens
	}
	p, reclaims, warns := newTestPacer(cfg, clock, sample)

	p.Pace(context.Background(), "runaway")

	elapsed := clock.t.Sub(start)
	assert.GreaterOrEqual(t, elapsed, cfg.StepMax, "must wait at least the hard cap before giving up")
	assert.Less(t, elapsed, cfg.StepMax+time.Second, "must not exceed the hard cap by more than one poll")
	assert.Equal(t, int32(1), atomic.LoadInt32(warns), "hitting the cap must warn loudly")
	assert.Equal(t, int32(1), atomic.LoadInt32(reclaims), "should still reclaim after giving up")
}

func TestAdaptiveRespectsStepMin(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	start := clock.t
	cfg := Config{
		Enabled: true, Adaptive: true,
		StepMin: 5 * time.Second, StepMax: 30 * time.Second,
		SettleWindow: 1 * time.Second, SettleThreshold: 1 << 20, PollInterval: 250 * time.Millisecond,
	}
	// Perfectly flat from the start — but StepMin must still be honored.
	p, _, _ := newTestPacer(cfg, clock, func() uint64 { return 42 << 20 })

	p.Pace(context.Background(), "flat")

	assert.GreaterOrEqual(t, clock.t.Sub(start), cfg.StepMin, "must wait at least StepMin even if already flat")
}

func TestAdaptiveStopsOnContextCancel(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	cfg := Config{
		Enabled: true, Adaptive: true,
		StepMin: 1 * time.Second, StepMax: time.Hour,
		SettleWindow: 2 * time.Second, SettleThreshold: 1 << 20, PollInterval: 250 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p, reclaims, _ := newTestPacer(cfg, clock, func() uint64 { return 0 })

	require.NotPanics(t, func() { p.Pace(ctx, "cancelled") })
	assert.Equal(t, int32(0), atomic.LoadInt32(reclaims), "cancelled pace must not reclaim")
}
