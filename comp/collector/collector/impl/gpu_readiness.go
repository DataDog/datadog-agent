// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectorimpl

import (
	"context"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// registerGPUReadiness checks the existing runner results; it does not call NVML
// or run a second GPU check. Register before startup so a missing or unloaded
// check cannot make an explicitly enabled GPU setup appear ready.
func (c *collectorImpl) registerGPUReadiness(lc compdef.Lifecycle) {
	if !c.config.GetBool("gpu.enabled") {
		return
	}

	handle := health.RegisterReadiness("gpu-check")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lc.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case now := <-ticker.C:
						c.ackGPUReadiness(handle, now)
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			<-done
			return handle.Deregister()
		},
	})
}

func (c *collectorImpl) ackGPUReadiness(handle *health.Handle, now time.Time) {
	if !c.gpuCheckReady(now) {
		return
	}
	// Only acknowledge health pings while the check is healthy. Otherwise the
	// readiness watchdog times out; liveness is intentionally unaffected.
	for {
		select {
		case <-handle.C:
		default:
			return
		}
	}
}

func (c *collectorImpl) gpuCheckReady(now time.Time) bool {
	if !c.config.GetBool("gpu.enabled") {
		return true
	}
	c.m.RLock()
	defer c.m.RUnlock()

	found := false
	for _, ch := range c.checks {
		if ch.String() != "gpu" {
			continue
		}
		found = true
		stats, ok := expvars.CheckStats(ch.ID())
		if !ok {
			return false
		}
		stats = stats.Clone()
		// Allow two collection intervals, with a one-minute floor, so slower
		// configured intervals work while a stalled check eventually fails.
		maxAge := max(2*ch.Interval(), time.Minute)
		if stats.TotalRuns == 0 || stats.LastError != "" || stats.Cancelling || now.Sub(stats.UpdateTimestamp) > maxAge {
			return false
		}
	}
	return found
}
