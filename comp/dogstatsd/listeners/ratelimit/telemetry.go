// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

type telemetryInterface interface {
	incWait()
	incNoWait()
	incHighLimit()
	incLowLimit()
	incLowLimitFreeOSMemory()
	setMemoryUsageRate(rate float64)
}

type memBasedRateLimiterTelemetry struct {
	wait              telemetry.Counter
	noWait            telemetry.Counter
	highLimit         telemetry.Counter
	lowLimit          telemetry.Counter
	lowLimitFreeOSMem telemetry.Counter
	memoryUsageRate   telemetry.Gauge
}

func newMemBasedRateLimiterTelemetry(telemetry telemetry.Component) *memBasedRateLimiterTelemetry {
	return &memBasedRateLimiterTelemetry{
		wait:              telemetry.NewCounter("dogstatsd", "mem_based_rate_limiter_wait", []string{}, "The number of times the rate limiter wait"),
		noWait:            telemetry.NewCounter("dogstatsd", "mem_based_rate_limiter_no_wait", []string{}, "The number of times the rate limiter doesn't wait"),
		highLimit:         telemetry.NewCounter("dogstatsd", "mem_based_rate_limiter_high_limit", []string{}, "The number of times the high limit is reached"),
		lowLimit:          telemetry.NewCounter("dogstatsd", "mem_based_rate_limiter_low_limit", []string{}, "The number of times the soft limit is reached"),
		lowLimitFreeOSMem: telemetry.NewCounter("dogstatsd", "mem_based_rate_limiter_low_limit_freeos_mem", []string{}, "The number of times FreeOSMemory is called when the soft limit is reached"),
		memoryUsageRate:   telemetry.NewGauge("dogstatsd", "mem_based_rate_limiter_mem_rate", []string{}, "The memory usage rate based on cgroup memory limit if it exists, otherwise based on the memory available"),
	}
}

func (t *memBasedRateLimiterTelemetry) incWait() {
	t.wait.Inc()
}

func (t *memBasedRateLimiterTelemetry) incNoWait() {
	t.noWait.Inc()
}

func (t *memBasedRateLimiterTelemetry) incHighLimit() {
	t.highLimit.Inc()
}

func (t *memBasedRateLimiterTelemetry) incLowLimit() {
	t.lowLimit.Inc()
}

func (t *memBasedRateLimiterTelemetry) incLowLimitFreeOSMemory() {
	t.lowLimitFreeOSMem.Inc()
}

func (t *memBasedRateLimiterTelemetry) setMemoryUsageRate(rate float64) {
	t.memoryUsageRate.Set(rate)
}
