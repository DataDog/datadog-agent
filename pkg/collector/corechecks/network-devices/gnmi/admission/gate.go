// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package admission limits concurrent gNMI connection dials across check instances.
package admission

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
)

const (
	maxConfiguredDevices = 1000
	fdHoldThreshold      = 0.7

	defaultPaceInterval    = 100 * time.Millisecond
	defaultPaceBurst       = 10
	defaultFDRetryInterval = 1 * time.Second
)

// ErrDeviceCapExceeded is returned when the process-wide device admission cap is reached.
var ErrDeviceCapExceeded = errors.New("gNMI device admission cap exceeded")

// AdmissionGate limits concurrent gNMI connection dials across all check instances.
type AdmissionGate struct {
	mu sync.Mutex

	configured int

	paceInterval time.Duration
	paceBurst    int
	tokens       int
	lastRefill   time.Time

	fdRetryInterval time.Duration

	clock        clock.Clock
	getFileStats func() (*procfilestats.ProcessFileStats, error)
}

var (
	gateInstance *AdmissionGate
	gateOnce     sync.Once
)

// Gate returns the process-wide admission gate singleton.
func Gate() *AdmissionGate {
	gateOnce.Do(func() {
		gateInstance = newGate(clock.New())
	})
	return gateInstance
}

func newGate(clk clock.Clock) *AdmissionGate {
	return &AdmissionGate{
		paceInterval:    defaultPaceInterval,
		paceBurst:       defaultPaceBurst,
		tokens:          defaultPaceBurst,
		fdRetryInterval: defaultFDRetryInterval,
		clock:           clk,
		getFileStats:    procfilestats.GetProcessFileStats,
	}
}

// Admit blocks until a new connection dial may proceed, the context is cancelled,
// or the configured device cap is reached.
func (g *AdmissionGate) Admit(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		g.mu.Lock()
		admitted, wait, err := g.tryAdmitLocked()
		g.mu.Unlock()
		if err != nil {
			return err
		}
		if admitted {
			if err := ctx.Err(); err != nil {
				g.Release()
				return err
			}
			return nil
		}

		if err := g.waitForRetry(ctx, wait); err != nil {
			return err
		}
	}
}

func (g *AdmissionGate) tryAdmitLocked() (bool, time.Duration, error) {
	if g.configured >= maxConfiguredDevices {
		return false, 0, ErrDeviceCapExceeded
	}

	stats, statsErr := g.getFileStats()
	if statsErr != nil && !errors.Is(statsErr, procfilestats.ErrNotImplemented) {
		return false, 0, fmt.Errorf("process file stats: %w", statsErr)
	}
	if stats != nil && fileDescriptorRatio(stats) > fdHoldThreshold {
		return false, g.fdRetryInterval, nil
	}

	now := g.clock.Now()
	g.refillTokens(now)
	if g.tokens <= 0 {
		return false, g.timeUntilNextPaceToken(now), nil
	}

	g.tokens--
	g.configured++
	return true, 0, nil
}

func (g *AdmissionGate) waitForRetry(ctx context.Context, wait time.Duration) error {
	timer := g.clock.Timer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Release decrements the configured device count when a connection closes.
func (g *AdmissionGate) Release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.configured > 0 {
		g.configured--
	}
}

// ConfiguredCount returns the number of devices currently holding admission slots.
func (g *AdmissionGate) ConfiguredCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.configured
}

func (g *AdmissionGate) refillTokens(now time.Time) {
	if g.paceInterval <= 0 {
		g.tokens = g.paceBurst
		return
	}

	if g.lastRefill.IsZero() {
		g.lastRefill = now
		if g.tokens == 0 {
			g.tokens = g.paceBurst
		}
		return
	}

	elapsed := now.Sub(g.lastRefill)
	ticks := int(elapsed / g.paceInterval)
	if ticks <= 0 {
		return
	}

	g.tokens += ticks * g.paceBurst
	if g.tokens > g.paceBurst {
		g.tokens = g.paceBurst
	}
	g.lastRefill = now
}

func (g *AdmissionGate) timeUntilNextPaceToken(now time.Time) time.Duration {
	if g.paceInterval <= 0 {
		return 0
	}
	if g.lastRefill.IsZero() {
		return g.paceInterval
	}
	elapsed := now.Sub(g.lastRefill)
	if elapsed >= g.paceInterval {
		return 0
	}
	return g.paceInterval - elapsed
}

func fileDescriptorRatio(stats *procfilestats.ProcessFileStats) float64 {
	if stats.OsFileLimit == 0 {
		return 0
	}
	return float64(stats.AgentOpenFiles) / float64(stats.OsFileLimit)
}
