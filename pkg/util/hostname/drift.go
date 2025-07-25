// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const (
	// DefaultInitialDelay is the default delay before the first check (20 minutes)
	DefaultInitialDelay = 20 * time.Minute
	// DefaultRecurringInterval is the default interval for recurring checks (6 hours)
	DefaultRecurringInterval = 6 * time.Hour
)

// Telemetry metrics
var (
	tlmDriftDetected = telemetry.NewCounter("hostname", "drift_detected",
		[]string{"state", "provider"}, "Hostname drift detection status")
	tlmDriftResolutionTime = telemetry.NewHistogram("hostname", "drift_resolution_time_ms",
		[]string{"state", "provider"}, "Hostname drift resolution time in milliseconds", []float64{.5, 1, 2.5, 5, 10, 60})
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

func scheduleHostnameDriftChecks(ctx context.Context, hostnameData Data) {
	cacheHostnameKey := cache.BuildAgentKey("hostname_check")
	cache.Cache.Set(cacheHostnameKey, hostnameData, cache.NoExpiration)

	go func() {
		// Wait for the initial delay before the first check
		initialTimer := time.NewTimer(DefaultInitialDelay)
		defer initialTimer.Stop()

		select {
		case <-initialTimer.C:
			// First check after initial delay
			checkHostnameDrift(ctx, cacheHostnameKey)
		case <-ctx.Done():
			return
		}

		// Then start the recurring checks
		driftTicker := time.NewTicker(DefaultRecurringInterval)
		defer driftTicker.Stop()
		for {
			select {
			case <-driftTicker.C:
				checkHostnameDrift(ctx, cacheHostnameKey)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func checkHostnameDrift(ctx context.Context, cacheHostnameKey string) {
	var hostname string
	var providerName string
	var hostnameData Data

	if cacheHostname, found := cache.Cache.Get(cacheHostnameKey); found {
		hostnameData = cacheHostname.(Data)
	}

	// Start timing the drift resolution
	startTime := time.Now()

	for _, p := range GetProviderCatalog(false) {
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
	resolutionTime := time.Since(startTime).Milliseconds()

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
