// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/mock"
)

func newMockStats(updateTimestamp time.Time, lastExecutionTime time.Duration, interval time.Duration) *Stats {
	mockStatsCheck := newMockCheckWithInterval(interval)
	mockStats := NewStats(mockStatsCheck, healthplatformmock.Mock(nil))

	mockStats.UpdateTimestamp = updateTimestamp
	mockStats.LastExecutionTime = lastExecutionTime

	return mockStats
}

func TestCalculateCheckDelay(t *testing.T) {
	mockNow := time.Now()

	tests := []struct {
		name          string
		prevRunStats  *Stats
		execTime      time.Duration
		checkInterval time.Duration
		delay         float64
	}{
		{
			name:         "First Check Run",
			prevRunStats: newMockStats(time.Time{}, 0*time.Second, 15*time.Second),
			execTime:     2 * time.Second,
			delay:        0,
		},
		{
			name:         "Long Running Check",
			prevRunStats: newMockStats(time.Time{}, 999999*time.Second, 0),
			execTime:     999999 * time.Second,
			delay:        0,
		},
		{
			name:         "Regular Running Delayed Check",
			prevRunStats: newMockStats(mockNow.Add(-16*time.Second), 17*time.Second, 15*time.Second),
			delay:        18, // Check ran 33 seconds after the previous run started
		},
		{
			name:         "Regular Running Delayed Check With Execution Time In Decimal Seconds ",
			prevRunStats: newMockStats(mockNow.Add(-16*time.Second), 17320*time.Millisecond, 15*time.Second),
			delay:        18.32, // Check ran 33 seconds after the previous run started
		},
		{
			name:         "Recovery from delay",
			prevRunStats: newMockStats(mockNow.Add(-6*time.Second), 1*time.Second, 15*time.Second),
			delay:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.delay, calculateCheckDelay(mockNow, tt.prevRunStats, tt.execTime))
		})
	}
}
