// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// driftService contains configuration for hostname drift detection
type driftService struct {
	initialDelay      time.Duration
	recurringInterval time.Duration
}

// Default timing values
const (
	defaultInitialDelay      = 20 * time.Minute
	defaultRecurringInterval = 6 * time.Hour
)

// Telemetry metrics
var (
	tlmDriftDetected = telemetry.NewCounter("hostname", "drift_detected",
		[]string{"state", "provider"}, "Hostname drift detection status")
	tlmDriftResolutionTime = telemetry.NewHistogram("hostname", "drift_resolution_time_ms",
		[]string{"state", "provider"}, "Hostname drift resolution time in seconds", []float64{.5, 1, 2.5, 5, 10, 60})
)

var (
	hostnameChanged         = "hostname_drift"
	providerChanged         = "provider_drift"
	hostnameProviderChanged = "hostname_provider_drift"
	noDrift                 = "no_drift"
)

// driftInfo contains information about hostname drift detection
type driftInfo struct {
	state    string
	hasDrift bool
}

// getInitialDelay returns the initial delay for drift checks, with config override
func (ds *driftService) getInitialDelay() time.Duration {
	// Check if config override is set
	if configDelay := setup.Datadog().GetDuration("hostname_drift_initial_delay"); configDelay > 0 {
		return configDelay
	}
	return ds.initialDelay
}

// getRecurringInterval returns the recurring interval for drift checks, with config override
func (ds *driftService) getRecurringInterval() time.Duration {
	// Check if config override is set
	if configInterval := setup.Datadog().GetDuration("hostname_drift_recurring_interval"); configInterval > 0 {
		return configInterval
	}
	return ds.recurringInterval
}

// determineDriftState determines the drift state and whether any drift occurred
func determineDriftState(oldData, newData Data) driftInfo {
	hostnameDiff := oldData.Hostname != newData.Hostname
	providerDiff := oldData.Provider != newData.Provider

	if hostnameDiff && providerDiff {
		return driftInfo{state: hostnameProviderChanged, hasDrift: true}
	} else if hostnameDiff {
		return driftInfo{state: hostnameChanged, hasDrift: true}
	} else if providerDiff {
		return driftInfo{state: providerChanged, hasDrift: true}
	}
	return driftInfo{state: noDrift, hasDrift: false}
}

func (ds *driftService) scheduleHostnameDriftChecks(ctx context.Context, hostnameData Data) {
	cacheHostnameKey := cache.BuildAgentKey("hostname_check")
	cache.Cache.Set(cacheHostnameKey, hostnameData, cache.NoExpiration)

	go func() {
		// Wait for the initial delay before the first check
		initialDelay := ds.getInitialDelay()
		initialTimer := time.NewTimer(initialDelay)
		defer initialTimer.Stop()

		select {
		case <-initialTimer.C:
			// First check after initial delay
			ds.checkHostnameDrift(ctx, cacheHostnameKey)
		case <-ctx.Done():
			return
		}

		// Then start the recurring checks
		recurringInterval := ds.getRecurringInterval()
		driftTicker := time.NewTicker(recurringInterval)
		defer driftTicker.Stop()
		for {
			select {
			case <-driftTicker.C:
				ds.checkHostnameDrift(ctx, cacheHostnameKey)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (ds *driftService) checkHostnameDrift(ctx context.Context, cacheHostnameKey string) {
	var hostname string
	var providerName string
	var hostnameData Data

	if cacheHostname, found := cache.Cache.Get(cacheHostnameKey); found {
		hostnameData = cacheHostname.(Data)
	}

	// Start timing the drift resolution
	startTime := time.Now()

	// Use test variable if available, otherwise use the real function
	providers := getProviderCatalog(false)

	for _, p := range providers {
		detectedHostname, err := p.cb(ctx, hostname)
		if err != nil {
			continue
		}

		hostname = detectedHostname
		providerName = p.name

		if p.stopIfSuccessful {
			break
		}
	}

	// Calculate resolution time in milliseconds
	resolutionTime := time.Since(startTime).Seconds()

	// Determine drift state
	newData := Data{Hostname: hostname, Provider: providerName}
	drift := determineDriftState(hostnameData, newData)

	// Emit resolution time metric
	tlmDriftResolutionTime.Observe(float64(resolutionTime), drift.state, providerName)

	if drift.hasDrift {
		tlmDriftDetected.Inc(drift.state, providerName)
		cache.Cache.Set(cacheHostnameKey, newData, cache.NoExpiration)
	}
}
