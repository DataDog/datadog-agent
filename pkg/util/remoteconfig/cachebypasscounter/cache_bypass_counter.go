// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cachebypasscounter provides a counter for RC cache bypass telemetry metrics.
package cachebypasscounter

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	subsystem     = "remoteconfig"
	bypassSkipped = "cache_bypass_ratelimiter_skip"
	bypassTimeout = "cache_bypass_timeout"
)

// CacheBypassMetricCounter is a datadog-agent telemetry counter for RC cache bypass metrics.
type CacheBypassMetricCounter struct {
	BypassRateLimitCounter telemetry.Counter
	BypassTimeoutCounter   telemetry.Counter
}

// NewCacheBypassMetricCounter returns a new CacheBypassMetricCounter that uses the datadog-agent telemetry package to emit metrics.
func NewCacheBypassMetricCounter() *CacheBypassMetricCounter {
	commonOpts := telemetry.Options{NoDoubleUnderscoreSep: true}
	return &CacheBypassMetricCounter{
		BypassRateLimitCounter: telemetry.NewCounterWithOpts(
			subsystem,
			bypassSkipped,
			[]string{},
			"Number of Remote Configuration cache bypass requests skipped by rate limiting.",
			commonOpts,
		),
		BypassTimeoutCounter: telemetry.NewCounterWithOpts(
			subsystem,
			bypassTimeout,
			[]string{},
			"Number of Remote Configuration cache bypass requests that timeout.",
			commonOpts,
		),
	}
}

// IncRateLimit increments the CacheBypassMetricCounter BypassRateLimitCounter counter.
func (c *CacheBypassMetricCounter) IncRateLimit() {
	c.BypassRateLimitCounter.Inc()
}

// IncTimeout increments the CacheBypassMetricCounter BypassTimeoutCounter counter.
func (c *CacheBypassMetricCounter) IncTimeout() {
	c.BypassTimeoutCounter.Inc()
}
