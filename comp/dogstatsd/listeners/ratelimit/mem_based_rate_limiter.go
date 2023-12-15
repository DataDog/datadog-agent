// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

import (
	"errors"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	var memoryUsage memoryUsage
	var err error
	if memoryUsage, err = newCgroupMemoryUsage(); err == nil {
		log.Info("cgroup limits detected")
	} else {
		memoryUsage = newHostMemoryUsage()
		log.Infof("cgroup limits not detected")
		log.Debugf("cgroup limits not detected: %v", err)
	}

	ballastOnce.Do(func() {
		ballastSize := cfg.GetInt64("dogstatsd_mem_based_rate_limiter.memory_ballast")
		if ballastSize != 0 {
			ballast = make([]byte, 0, ballastSize)
			log.Infof("ballast size %vMB", ballastSize/1024/1024)
		}
	})

	usage, limit, err := memoryUsage.getMemoryStats()
	if err != nil {
		return nil, err
	}
	log.Infof("Memory usage:%v limit:%v", usage, limit)

	return NewMemBasedRateLimiter(
		memBasedRateLimiterTml,
		memoryUsage,
		getConfigFloat(cfg, "low_soft_limit"),
		getConfigFloat(cfg, "high_soft_limit"),
		cfg.GetInt("dogstatsd_mem_based_rate_limiter.go_gc"),
		geometricRateLimiterConfig{
			getConfigFloat(cfg, "rate_check.min"),
			getConfigFloat(cfg, "rate_check.max"),
			getConfigFloat(cfg, "rate_check.factor")},
		geometricRateLimiterConfig{
			getConfigFloat(cfg, "soft_limit_freeos_check.min"),
			getConfigFloat(cfg, "soft_limit_freeos_check.max"),
			getConfigFloat(cfg, "soft_limit_freeos_check.factor"),
		},
	)
}

func getConfigFloat(cfg config.Reader, subkey string) float64 {
	return cfg.GetFloat64("dogstatsd_mem_based_rate_limiter." + subkey)
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

	// When `SetMemoryLimit` will be available (https://github.com/golang/go/issues/48409),
	//  SetGCPercent, madvdontneed=1 and debug.FreeOSMemory() can be removed.
	if goGC > 0 {
		debug.SetGCPercent(goGC)
	}

	return &MemBasedRateLimiter{
		telemetry:               telemetry,
		memoryUsage:             memoryUsage,
		lowSoftLimitRate:        lowSoftLimitRate,
		highSoftLimitRate:       highSoftLimitRate,
		memoryRateLimiter:       newGeometricRateLimiter(memoryRateLimiter),
		freeOSMemoryRateLimiter: newGeometricRateLimiter(freeOSMemoryRateLimiter),
	}, nil
}

// MayWait waits and tries to release the memory. See MemBasedRateLimiter for more information.
func (m *MemBasedRateLimiter) MayWait() error {
	if !m.memoryRateLimiter.keep() {
		m.telemetry.incNoWait()
		return nil
	}
	m.telemetry.incWait()

	rate, err := m.getMemoryUsageRate()
	if err != nil {
		return err
	}
	m.telemetry.setMemoryUsageRate(rate)

	if rate, err = m.waitWhileHighLimit(rate); err != nil {
		return nil
	}

	if m.waitOnceLowLimit(rate) {
		m.memoryRateLimiter.increaseRate()
	} else {
		m.memoryRateLimiter.decreaseRate()
	}
	m.previousMemoryUsageRate = rate
	return nil
}

func (m *MemBasedRateLimiter) waitWhileHighLimit(rate float64) (float64, error) {
	for rate > m.highSoftLimitRate {
		m.memoryRateLimiter.increaseRate()
		m.telemetry.incHighLimit()
		runtime.GC()
		debug.FreeOSMemory()
		var err error
		if rate, err = m.getMemoryUsageRate(); err != nil {
			return 0, err
		}
	}
	return rate, nil
}

func (m *MemBasedRateLimiter) getMemoryUsageRate() (float64, error) {
	usage, limit, err := m.memoryUsage.getMemoryStats()
	if err != nil {
		return 0, err
	}
	if limit == 0 {
		return 0, errors.New("cgroup memory limit is 0")
	}
	return usage / limit, nil
}

func (m *MemBasedRateLimiter) waitOnceLowLimit(rate float64) bool {
	if rate > m.lowSoftLimitRate {
		m.telemetry.incLowLimit()
		if m.freeOSMemoryRateLimiter.keep() {
			runtime.GC()
			debug.FreeOSMemory()
			m.telemetry.incLowLimitFreeOSMemory()
		} else {
			time.Sleep(1 * time.Millisecond)
		}

		if rate > m.previousMemoryUsageRate {
			m.freeOSMemoryRateLimiter.increaseRate()
		} else {
			m.freeOSMemoryRateLimiter.decreaseRate()
		}
		return true
	}
	return false
}
