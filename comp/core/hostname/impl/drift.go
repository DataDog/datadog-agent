// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless

package hostnameimpl

import (
	"context"
	"time"

	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// Default timing values for drift detection.
const (
	defaultInitialDelay      = 20 * time.Minute
	defaultRecurringInterval = 6 * time.Hour
)

const (
	driftStateHostname         = "hostname_drift"
	driftStateProvider         = "provider_drift"
	driftStateHostnameProvider = "hostname_provider_drift"
	driftStateNone             = "no_drift"
)

// driftInfo holds the result of a single drift comparison.
type driftInfo struct {
	state    string
	hasDrift bool
}

// driftService manages periodic hostname drift detection.
type driftService struct {
	config              pkgconfigmodel.Reader
	initialDelay        time.Duration
	recurringInterval   time.Duration
	cancel              context.CancelFunc
	driftDetected       coretelemetry.Counter
	driftResolutionTime coretelemetry.Histogram
}

// newDriftService creates a drift service with the given config and default timings.
func newDriftService(cfg pkgconfigmodel.Reader, telComp coretelemetry.Component) *driftService {
	return &driftService{
		config:            cfg,
		initialDelay:      defaultInitialDelay,
		recurringInterval: defaultRecurringInterval,
		driftDetected: telComp.NewCounter("hostname", "drift_detected",
			[]string{"state", "provider"}, "Hostname drift detection status"),
		driftResolutionTime: telComp.NewHistogram("hostname", "drift_resolution_time_ms",
			[]string{"state", "provider"}, "Hostname drift resolution time in seconds",
			[]float64{.5, 1, 2.5, 5, 10, 60}),
	}
}

// start begins drift monitoring in a background goroutine using initialData as the baseline.
// The goroutine runs until stop() is called.
func (ds *driftService) start(initialData hostnamedef.Data) {
	cacheKey := cache.BuildAgentKey("hostname_check")
	cache.Cache.Set(cacheKey, initialData, cache.NoExpiration)

	ctx, cancel := context.WithCancel(context.Background())
	ds.cancel = cancel

	go func() {
		initialDelay := ds.getInitialDelay()
		initialTimer := time.NewTimer(initialDelay)
		defer initialTimer.Stop()

		select {
		case <-initialTimer.C:
			ds.checkDrift(ctx, cacheKey)
		case <-ctx.Done():
			return
		}

		recurringInterval := ds.getRecurringInterval()
		ticker := time.NewTicker(recurringInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ds.checkDrift(ctx, cacheKey)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// stop cancels the drift monitoring goroutine.
func (ds *driftService) stop() {
	if ds.cancel != nil {
		ds.cancel()
	}
}

func (ds *driftService) getInitialDelay() time.Duration {
	if d := ds.config.GetDuration("hostname_drift_initial_delay"); d > 0 {
		return d
	}
	return ds.initialDelay
}

func (ds *driftService) getRecurringInterval() time.Duration {
	if d := ds.config.GetDuration("hostname_drift_recurring_interval"); d > 0 {
		return d
	}
	return ds.recurringInterval
}

func (ds *driftService) checkDrift(ctx context.Context, cacheKey string) {
	var baseline hostnamedef.Data
	if v, found := cache.Cache.Get(cacheKey); found {
		baseline = v.(hostnamedef.Data)
	}

	startTime := time.Now()

	var hostname string
	var providerName string
	for _, p := range getProviderCatalog(false) {
		detected, err := p.cb(ctx, ds.config, hostname)
		if err != nil {
			continue
		}
		hostname = detected
		providerName = p.name
		if p.stopIfSuccessful {
			break
		}
	}

	resolutionTime := time.Since(startTime).Seconds()
	current := hostnamedef.Data{Hostname: hostname, Provider: providerName}
	drift := determineDriftState(baseline, current)

	ds.driftResolutionTime.Observe(float64(resolutionTime), drift.state, providerName)

	if drift.hasDrift {
		ds.driftDetected.Inc(drift.state, providerName)
		cache.Cache.Set(cacheKey, current, cache.NoExpiration)
	}
}

// determineDriftState compares old and new hostname data, returning the drift state.
func determineDriftState(old, current hostnamedef.Data) driftInfo {
	hostnameDiff := old.Hostname != current.Hostname
	providerDiff := old.Provider != current.Provider

	switch {
	case hostnameDiff && providerDiff:
		return driftInfo{state: driftStateHostnameProvider, hasDrift: true}
	case hostnameDiff:
		return driftInfo{state: driftStateHostname, hasDrift: true}
	case providerDiff:
		return driftInfo{state: driftStateProvider, hasDrift: true}
	default:
		return driftInfo{state: driftStateNone, hasDrift: false}
	}
}
