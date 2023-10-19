// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemBasedRateLimiter(t *testing.T) {
	config := geometricRateLimiterConfig{minRate: 1, maxRate: 1, factor: 1}
	memoryUsage := &memoryUsageMock{}
	telemetry := &telemetryMock{}
	rateLimiter, err := NewMemBasedRateLimiter(telemetry, memoryUsage, 0.6, 0.8, 0, config, config)
	r := require.New(t)
	r.NoError(err)

	memoryUsage.setRates(0.5)
	rateLimiter.MayWait()
	r.Equal(1, telemetry.wait)
	r.Equal(0, telemetry.highLimit)
	r.Equal(0, telemetry.lowLimit)

	memoryUsage.setRates(0.7)
	rateLimiter.MayWait()
	r.Equal(2, telemetry.wait)
	r.Equal(0, telemetry.highLimit)
	r.Equal(1, telemetry.lowLimit)

	memoryUsage.setRates(0.9, 0.9, 0.9, 0.7)
	rateLimiter.MayWait()
	r.Equal(3, telemetry.wait)
	r.Equal(3, telemetry.highLimit)
	r.Equal(2, telemetry.lowLimit)
}

type memoryUsageMock struct {
	memStats []float64 // Store memUsage1, memLimit1, memUsage2, memLimit2, ...
}

func (m *memoryUsageMock) setRates(rates ...float64) {
	m.memStats = nil
	for _, rate := range rates {
		m.memStats = append(m.memStats, 1, 1/rate)
	}
}

func (m *memoryUsageMock) getMemoryStats() (float64, float64, error) {
	if len(m.memStats) < 2 {
		return 0, 0, errors.New("memoryUsageMock: not enough values")
	}
	memUsage := m.memStats[0]
	memLimit := m.memStats[1]
	m.memStats = m.memStats[2:]
	return memUsage, memLimit, nil
}

type telemetryMock struct {
	wait      int
	highLimit int
	lowLimit  int
}

func (t *telemetryMock) incWait() {
	t.wait++
}

func (t *telemetryMock) incHighLimit() {
	t.highLimit++
}

func (t *telemetryMock) incLowLimit() {
	t.lowLimit++
}

func (t *telemetryMock) incNoWait()                      {}
func (t *telemetryMock) incLowLimitFreeOSMemory()        {}
func (t *telemetryMock) setMemoryUsageRate(rate float64) {} //nolint:revive // TODO fix revive unused-parameter
