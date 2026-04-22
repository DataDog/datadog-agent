// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

const (
	rateLimitInterval  = 1 * time.Minute
	rateLimitBurst     = 1
	maxRateLimiterKeys = 10000
)

// rateLimitedDetector wraps a Detector and drops anomalies for the same
// Source.Key() that exceed rateLimitBurst per rateLimitInterval.
// The key set is LRU-evicted at maxRateLimiterKeys.
type rateLimitedDetector struct {
	inner    observerdef.Detector
	mu       sync.RWMutex
	limiters *lru.Cache[string, *rate.Limiter]
}

func newRateLimitedDetector(inner observerdef.Detector) *rateLimitedDetector {
	cache, _ := lru.New[string, *rate.Limiter](maxRateLimiterKeys)
	return &rateLimitedDetector{inner: inner, limiters: cache}
}

func (d *rateLimitedDetector) Name() string { return d.inner.Name() }

func (d *rateLimitedDetector) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	result := d.inner.Detect(storage, dataTime)
	filtered := result.Anomalies[:0:0]
	var dropped int
	for _, a := range result.Anomalies {
		if d.allow(a.Source.Key()) {
			filtered = append(filtered, a)
		} else {
			dropped++
		}
	}
	result.Anomalies = filtered
	if dropped > 0 {
		result.Telemetry = append(result.Telemetry,
			newTelemetryGauge(
				[]string{"detector:" + d.inner.Name()},
				telemetryRateLimitDropped,
				float64(dropped),
				dataTime,
			),
		)
	}
	return result
}

func (d *rateLimitedDetector) allow(key string) bool {
	d.mu.RLock()
	if l, ok := d.limiters.Get(key); ok {
		allowed := l.Allow()
		d.mu.RUnlock()
		return allowed
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.limiters.Get(key); ok {
		return l.Allow()
	}
	l := rate.NewLimiter(rate.Every(rateLimitInterval), rateLimitBurst)
	d.limiters.Add(key, l)
	return l.Allow()
}
