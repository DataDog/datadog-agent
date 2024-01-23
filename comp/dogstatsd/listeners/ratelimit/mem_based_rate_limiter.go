// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// MemBasedRateLimiter is a rate limiter based on memory usage.
// While the high memory limit is reached, MayWait() blocks and try to release memory.
// When the low memory limit is reached, MayWait() blocks once and may try to release memory.
// The memory limits are defined as soft limits.
//
// `memoryRateLimiter` provides a way to dynamically update the rate at which the memory limit
// is checked.
// When the soft limit is reached, we would like to wait and release memory but not block too often.
// `freeOSMemoryRateLimiter` provides a way to dynamically update the rate at which `FreeOSMemory` is
// called when the soft limit is reached.
type MemBasedRateLimiter struct {
	telemetry               telemetry
	memoryUsage             memoryUsage
	lowSoftLimitRate        float64
	highSoftLimitRate       float64
	memoryRateLimiter       *geometricRateLimiter
	freeOSMemoryRateLimiter *geometricRateLimiter
	previousMemoryUsageRate float64
}

type memoryUsage interface {
	getMemoryStats() (float64, float64, error)
}

var memBasedRateLimiterTml = newMemBasedRateLimiterTelemetry()

// Ballast is a way to trick the GC. `ballast` is never read or written.
var ballast []byte //nolint:unused
var ballastOnce sync.Once

// BuildMemBasedRateLimiter builds a new instance of *MemBasedRateLimiter
func BuildMemBasedRateLimiter(cfg config.Reader) (*MemBasedRateLimiter, error) {
	panic("not called")
}

func getConfigFloat(cfg config.Reader, subkey string) float64 {
	panic("not called")
}

// NewMemBasedRateLimiter creates a new instance of MemBasedRateLimiter.
func NewMemBasedRateLimiter(
	telemetry telemetry,
	memoryUsage memoryUsage,
	lowSoftLimitRate float64,
	highSoftLimitRate float64,
	goGC int,
	memoryRateLimiter geometricRateLimiterConfig,
	freeOSMemoryRateLimiter geometricRateLimiterConfig) (*MemBasedRateLimiter, error) {
	panic("not called")
}

// MayWait waits and tries to release the memory. See MemBasedRateLimiter for more information.
func (m *MemBasedRateLimiter) MayWait() error {
	panic("not called")
}

func (m *MemBasedRateLimiter) waitWhileHighLimit(rate float64) (float64, error) {
	panic("not called")
}

func (m *MemBasedRateLimiter) getMemoryUsageRate() (float64, error) {
	panic("not called")
}

func (m *MemBasedRateLimiter) waitOnceLowLimit(rate float64) bool {
	panic("not called")
}
