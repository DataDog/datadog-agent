// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

import (
	tlm "github.com/DataDog/datadog-agent/pkg/telemetry"
)

type telemetry interface {
	incWait()
	incNoWait()
	incHighLimit()
	incLowLimit()
	incLowLimitFreeOSMemory()
	setMemoryUsageRate(rate float64)
}

type memBasedRateLimiterTelemetry struct {
	wait              tlm.Counter
	noWait            tlm.Counter
	highLimit         tlm.Counter
	lowLimit          tlm.Counter
	lowLimitFreeOSMem tlm.Counter
	memoryUsageRate   tlm.Gauge
}

func newMemBasedRateLimiterTelemetry() *memBasedRateLimiterTelemetry {
	return &memBasedRateLimiterTelemetry{
		wait:              tlm.NewCounter("dogstatsd", "mem_based_rate_limiter_wait", []string{}, "The number of times the rate limiter wait"),
		noWait:            tlm.NewCounter("dogstatsd", "mem_based_rate_limiter_no_wait", []string{}, "The number of times the rate limiter doesn't wait"),
		highLimit:         tlm.NewCounter("dogstatsd", "mem_based_rate_limiter_high_limit", []string{}, "The number of times the high limit is reached"),
		lowLimit:          tlm.NewCounter("dogstatsd", "mem_based_rate_limiter_low_limit", []string{}, "The number of times the soft limit is reached"),
		lowLimitFreeOSMem: tlm.NewCounter("dogstatsd", "mem_based_rate_limiter_low_limit_freeos_mem", []string{}, "The number of times FreeOSMemory is called when the soft limit is reached"),
		memoryUsageRate:   tlm.NewGauge("dogstatsd", "mem_based_rate_limiter_mem_rate", []string{}, "The memory usage rate based on cgroup memory limit if it exists, otherwise based on the memory available"),
	}
}

func (t *memBasedRateLimiterTelemetry) incWait() {
	panic("not called")
}

func (t *memBasedRateLimiterTelemetry) incNoWait() {
	panic("not called")
}

func (t *memBasedRateLimiterTelemetry) incHighLimit() {
	panic("not called")
}

func (t *memBasedRateLimiterTelemetry) incLowLimit() {
	panic("not called")
}

func (t *memBasedRateLimiterTelemetry) incLowLimitFreeOSMemory() {
	panic("not called")
}

func (t *memBasedRateLimiterTelemetry) setMemoryUsageRate(rate float64) {
	panic("not called")
}
