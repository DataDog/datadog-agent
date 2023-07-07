// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
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
			duration:                      time.Minute * 2,
			numOfTokensEachToGenerate:     5,
			wantStats: []LimiterStat{
				{Allowed: 3, Dropped: 12}, // 15 'events' are generated (numOfTokensEachToGenerate * numOfUniqueTokens). Allow 3 because each unique token is allowed 'numOfAllowedTokensPerDuration' times in the 'duration'.
			},
		},
		{
			name:                          "More events than limit but spaced over time",
			numOfUniqueTokens:             3,
			numOfAllowedTokensPerDuration: 1,
			duration:                      time.Nanosecond,
			numOfTokensEachToGenerate:     10,
			wantStats: []LimiterStat{
				{Allowed: 30, Dropped: 0}, // Allow all (numOfTokensEachToGenerate * numOfUniqueTokens) because they are spaced more than the 'duration'.
			},
		},
		//{
		//	name:                          "Over capacity of LRU",
		//	numOfUniqueTokens:             3,
		//	numOfAllowedTokensPerDuration: 8,
		//	duration:                      time.Second,
		//	numOfTokensEachToGenerate:     10,
		//	wantStats: []LimiterStat{
		//		{Allowed: 8, Dropped: 3},
		//	},
		//},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := NewLimiter[int](tt.numOfUniqueTokens, tt.numOfAllowedTokensPerDuration, tt.duration)
			assert.NoError(t, err)

			// TODO: add sleeping

			for i := 0; i < tt.numOfUniqueTokens; i++ {
				for j := 0; j < tt.numOfTokensEachToGenerate; j++ {
					limiter.Allow(i)
				}
			}

			stats := limiter.SwapStats()

			fmt.Printf("Stats: %+v", stats)

			assert.Equal(t, tt.wantStats, stats)
		})
	}
}

//func TestLimiter_Count(t *testing.T) {
//	type args[K comparable] struct {
//		k K
//	}
//	type testCase[K comparable] struct {
//		name string
//		l    Limiter[K]
//		args args[K]
//	}
//	tests := []testCase[ /* TODO: Insert concrete types here */ ]{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			tt.l.Count(tt.args.k)
//		})
//	}
//}
//
//func TestLimiter_SwapStats(t *testing.T) {
//	type testCase[K comparable] struct {
//		name string
//		l    Limiter[K]
//		want []LimiterStat
//	}
//	tests := []testCase[ /* TODO: Insert concrete types here */ ]{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			if got := tt.l.SwapStats(); !reflect.DeepEqual(got, tt.want) {
//				t.Errorf("SwapStats() = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
//
//func TestNewLimiter(t *testing.T) {
//	type args struct {
//		numUniqueTokens             int
//		numAllowedTokensPerDuration int
//		duration                    time.Duration
//	}
//	type testCase[K comparable] struct {
//		name    string
//		args    args
//		want    *Limiter[K]
//		wantErr bool
//	}
//	tests := []testCase[ /* TODO: Insert concrete types here */ ]{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			got, err := NewLimiter(tt.args.numUniqueTokens, tt.args.numAllowedTokensPerDuration, tt.args.duration)
//			if (err != nil) != tt.wantErr {
//				t.Errorf("NewLimiter() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if !reflect.DeepEqual(got, tt.want) {
//				t.Errorf("NewLimiter() got = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
