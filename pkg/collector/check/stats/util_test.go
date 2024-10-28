// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newMockStats(updateTimestamp, lastExecutionTime int64, interval time.Duration) *Stats {
	mockStatsCheck := newMockCheckWithInterval(interval)
	mockStats := NewStats(mockStatsCheck)

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
		delay         int64
	}{
		{
			name:         "First Check Run",
			prevRunStats: newMockStats(0, 0, 15*time.Second),
			execTime:     2 * time.Second,
			delay:        0,
		},
		{
			name:         "Long Runnig Check",
			prevRunStats: newMockStats(0, 9999999999, 0),
			execTime:     999999 * time.Second,
			delay:        0,
		},
		{
			name:         "Regular Running Delayed Check",
			prevRunStats: newMockStats(mockNow.Add(-16*time.Second).Unix(), 17*1000, 15*time.Second),
			delay:        18, // Check ran 33 seconds after the previous run started
		},
		{
			name:         "Regular Running Delayed Check With Execution Time In Decimal Seconds ",
			prevRunStats: newMockStats(mockNow.Add(-16*time.Second).Unix(), 17.32*1000, 15*time.Second),
			delay:        18, // Check ran 33 seconds after the previous run started
		},
		{
			name:         "Recovery from delay",
			prevRunStats: newMockStats(mockNow.Add(-6*time.Second).Unix(), 1*1000, 15*time.Second),
			delay:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.delay, calculateCheckDelay(mockNow, tt.prevRunStats, tt.execTime))
		})
	}
}
