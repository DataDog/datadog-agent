// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package utils holds utils related files
package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimiter_Allow(t *testing.T) {
	type testCase[K comparable] struct {
		name                          string
		numOfUniqueTokens             int
		numOfAllowedTokensPerDuration int
		duration                      time.Duration
		numOfTokensEachToGenerate     int
		wantStats                     []LimiterStat
	}
	tests := []testCase[string]{
		{
			name:                          "More events than limit",
			numOfUniqueTokens:             3,
			numOfAllowedTokensPerDuration: 1,
			duration:                      time.Minute * 2, // Test will not exceed this period
			numOfTokensEachToGenerate:     5,
			wantStats: []LimiterStat{
				{Allowed: 3, Dropped: 12}, // 15 'events' are generated (numOfTokensEachToGenerate * numOfUniqueTokens). Allow 3 because each unique token is allowed 'numOfAllowedTokensPerDuration' times in the 'period'.
			},
		},
		{
			name:                          "More events than limit but spaced over time",
			numOfUniqueTokens:             3,
			numOfAllowedTokensPerDuration: 1,
			duration:                      time.Nanosecond, // Test will exceed this period
			numOfTokensEachToGenerate:     10,
			wantStats: []LimiterStat{
				{Allowed: 30, Dropped: 0}, // Allow all (numOfTokensEachToGenerate * numOfUniqueTokens) because they are spaced more than the 'period'.
			},
		},
		{
			name:                          "Same number of events as limit",
			numOfUniqueTokens:             3,
			numOfAllowedTokensPerDuration: 2,
			duration:                      time.Minute * 2,
			numOfTokensEachToGenerate:     2,
			wantStats: []LimiterStat{
				{Allowed: 6, Dropped: 0}, // Allow all (numOfTokensEachToGenerate * numOfUniqueTokens) because the count is <= to the limit.
			},
		},
		{
			name:                          "Fewer events than limit",
			numOfUniqueTokens:             3,
			numOfAllowedTokensPerDuration: 2,
			duration:                      time.Minute * 2,
			numOfTokensEachToGenerate:     1,
			wantStats: []LimiterStat{
				{Allowed: 3, Dropped: 0}, // Allow all (numOfTokensEachToGenerate * numOfUniqueTokens) because the count is <= to the limit.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := NewLimiter[int](tt.numOfUniqueTokens, tt.numOfAllowedTokensPerDuration, tt.duration)
			assert.NoError(t, err)

			for i := 0; i < tt.numOfUniqueTokens; i++ {
				for j := 0; j < tt.numOfTokensEachToGenerate; j++ {
					limiter.Allow(i)
				}
			}

			stats := limiter.SwapStats()
			assert.Equal(t, tt.wantStats, stats)
		})
	}
}
