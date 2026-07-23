// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stagedstart provides a shared pacer used to spread expensive
// subsystem startup across time so the Agent's startup memory high-water-mark
// stays close to its steady-state footprint.
//
// A Pacer is called between startup steps. In "interval" mode it inserts a
// fixed delay. In "adaptive" mode it waits until this process's memory settles
// (the previous step has finished allocating and any transient scratch has been
// reclaimed) before releasing the next step, bounded by a hard maximum so a
// process under memory pressure fails loudly rather than hanging. Either way it
// then returns transient memory to the OS.
//
// The signal is this process's own Go-runtime memory retained from the OS,
// read via the stdlib runtime/metrics package — portable across every OS and
// non-stop-the-world. It is a proxy for the process's RSS contribution; the
// sampler is injectable so a truer RSS source (or a test double) can be used.
package stagedstart

import (
	"context"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"time"
)

// configReader is the subset of the Agent config interface the pacer needs.
// Any config.Component / model.Reader satisfies it structurally.
type configReader interface {
	GetBool(string) bool
	GetString(string) string
	GetDuration(string) time.Duration
	GetInt64(string) int64
}

// Config controls the pacer. Zero value is a disabled no-op pacer.
type Config struct {
	Enabled      bool
	FreeOSMemory bool
	Adaptive     bool // true => memory-feedback pacing; false => fixed interval

	// Interval mode.
	Interval time.Duration

	// Adaptive mode.
	StepMin         time.Duration // minimum wait per step (let async work start)
	StepMax         time.Duration // hard cap per step; on hit, proceed and warn
	SettleWindow    time.Duration // memory growth is measured over this window
	SettleThreshold uint64        // growth below this (bytes) over the window => settled
	PollInterval    time.Duration // sampling cadence
}

// ConfigFromReader builds a Config from staged_start.* Agent config keys.
func ConfigFromReader(c configReader) Config {
	threshold := c.GetInt64("staged_start.settle_threshold_bytes")
	if threshold < 0 {
		threshold = 0
	}
	return Config{
		Enabled:         c.GetBool("staged_start.enabled"),
		FreeOSMemory:    c.GetBool("staged_start.free_os_memory"),
		Adaptive:        c.GetString("staged_start.mode") == "adaptive",
		Interval:        c.GetDuration("staged_start.stage_interval"),
		StepMin:         c.GetDuration("staged_start.step_min"),
		StepMax:         c.GetDuration("staged_start.step_max"),
		SettleWindow:    c.GetDuration("staged_start.settle_window"),
		SettleThreshold: uint64(threshold),
		PollInterval:    c.GetDuration("staged_start.poll_interval"),
	}
}

// Pacer paces startup steps. Construct with NewPacer; the zero value is not usable.
type Pacer struct {
	cfg Config

	// Injectable for testing; defaulted by NewPacer.
	sample  func() uint64
	reclaim func()
	now     func() time.Time
	// sleep waits for d or until ctx is done; returns false if ctx was cancelled.
	sleep func(ctx context.Context, d time.Duration) bool

	infof func(string, ...interface{})
	warnf func(string, ...interface{})
}

// NewPacer returns a Pacer for the given config. infof/warnf may be nil.
func NewPacer(cfg Config, infof, warnf func(string, ...interface{})) *Pacer {
	if infof == nil {
		infof = func(string, ...interface{}) {}
	}
	if warnf == nil {
		warnf = func(string, ...interface{}) {}
	}
	return &Pacer{
		cfg:     cfg,
		sample:  retainedFromOS,
		reclaim: func() { runtime.GC(); debug.FreeOSMemory() },
		now:     time.Now,
		sleep:   sleepCtx,
		infof:   infof,
		warnf:   warnf,
	}
}

// Enabled reports whether the pacer will do anything.
func (p *Pacer) Enabled() bool { return p != nil && p.cfg.Enabled }

// Pace is called between startup steps. It waits (fixed interval, or until
// memory settles in adaptive mode), then returns transient memory to the OS.
// name identifies the step just completed, for logging. It returns early if ctx
// is cancelled. It is a no-op when the pacer is disabled.
func (p *Pacer) Pace(ctx context.Context, name string) {
	if !p.Enabled() {
		return
	}
	if p.cfg.Adaptive {
		p.waitUntilSettled(ctx, name)
	} else if p.cfg.Interval > 0 {
		p.sleep(ctx, p.cfg.Interval)
	}
	if ctx.Err() != nil {
		return
	}
	if p.cfg.FreeOSMemory {
		p.reclaim()
	}
}

// waitUntilSettled blocks until this process's retained memory stops growing
// (growth over SettleWindow < SettleThreshold) after at least StepMin, or until
// StepMax elapses — at which point it proceeds anyway and warns loudly, so a
// process under sustained memory pressure makes progress (and fails loudly)
// instead of hanging.
func (p *Pacer) waitUntilSettled(ctx context.Context, name string) {
	start := p.now()
	deadline := start.Add(p.cfg.StepMax)

	type point struct {
		t time.Time
		v uint64
	}
	var window []point

	for {
		if ctx.Err() != nil {
			return
		}
		now := p.now()
		window = append(window, point{now, p.sample()})
		// Drop samples older than the settle window.
		cutoff := now.Add(-p.cfg.SettleWindow)
		i := 0
		for i < len(window) && window[i].t.Before(cutoff) {
			i++
		}
		window = window[i:]

		elapsed := now.Sub(start)
		if elapsed >= p.cfg.StepMin && now.Sub(window[0].t) >= p.cfg.SettleWindow {
			var lo, hi uint64 = window[0].v, window[0].v
			for _, pt := range window {
				if pt.v < lo {
					lo = pt.v
				}
				if pt.v > hi {
					hi = pt.v
				}
			}
			if hi-lo < p.cfg.SettleThreshold {
				p.infof("staged startup: %q settled after %s", name, elapsed.Round(time.Millisecond))
				return
			}
		}

		if !now.Before(deadline) {
			p.warnf("staged startup: %q did not settle within %s; proceeding (memory may still be growing)", name, p.cfg.StepMax)
			return
		}

		if !p.sleep(ctx, p.cfg.PollInterval) {
			return
		}
	}
}

// retainedFromOS returns the bytes this process's Go runtime currently holds
// from the OS (total mapped minus released). Non-stop-the-world and portable.
func retainedFromOS() uint64 {
	samples := []metrics.Sample{
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
	}
	metrics.Read(samples)
	total := samples[0].Value.Uint64()
	released := samples[1].Value.Uint64()
	if released > total {
		return 0
	}
	return total - released
}

// sleepCtx waits for d or until ctx is done; returns false if ctx was cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
