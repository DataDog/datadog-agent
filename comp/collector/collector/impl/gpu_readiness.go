// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectorimpl

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// Allow the same startup grace period as the NVML availability telemetry.
const nvmlReadinessTimeout = 5 * time.Minute

func (c *collectorImpl) registerNVMLReadiness(lc compdef.Lifecycle, initialized func() bool, clk clock.Clock) {
	if !c.config.GetBool("gpu.enabled") {
		return
	}

	// Register before startup, including before autodiscovery loads the GPU check.
	// Leave the watchdog unanswered until NVML initializes or the timeout expires.
	handle := health.RegisterReadiness("gpu-nvml")
	stop := make(chan struct{})
	done := make(chan struct{})
	lc.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			timeout := clk.Timer(nvmlReadinessTimeout)
			ticker := clk.Ticker(time.Second)
			go func() {
				defer close(done)
				defer timeout.Stop()
				defer ticker.Stop()
				defer func() {
					if err := handle.Deregister(); err != nil {
						c.log.Warnf("Unable to deregister NVML readiness check: %v", err)
					}
				}()

				// initialized only reads an atomic flag: a slow or stuck NVML
				// call cannot prevent the timeout or Agent shutdown.
				for !initialized() {
					select {
					case <-stop:
						return
					case <-timeout.C:
						c.log.Warnf("NVML has not initialized after %s; no longer blocking Agent readiness", nvmlReadinessTimeout)
						return
					case <-ticker.C:
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			close(stop)
			<-done
			return nil
		},
	})
}
