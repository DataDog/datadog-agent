package utils

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestLimiter_Allow(t *testing.T) {
	type args[K comparable] struct {
		k K
	}
	type testCase[K comparable] struct {
		name                          string
		numOfUniqueTokens             int
		numOfAllowedTokensPerDuration int
		duration                      time.Duration
		numOfTokensEachToGenerate     int
		args                          args[K]
		wantStats                     []LimiterStat
	}
	tests := []testCase[string]{
		{
			name:                          "More events than limit",
			numOfUniqueTokens:             3,
			numOfAllowedTokensPerDuration: 8,
			duration:                      time.Second,
			numOfTokensEachToGenerate:     10,
			wantStats: []LimiterStat{
				{Allowed: 24, Dropped: 6},
			},
		},
		//{
		//	name:                          "More events than limit but spaced over time",
		//	numOfUniqueTokens:             3,
		//	numOfAllowedTokensPerDuration: 8,
		//	duration:                      time.Second,
		//	numOfTokensEachToGenerate:     10,
		//	wantStats: []LimiterStat{
		//		{Allowed: 10, Dropped: 0},
		//		{Allowed: 10, Dropped: 0},
		//		{Allowed: 10, Dropped: 0},
		//	},
		//},
		//{
		//	name:                          "Over capacity of LRU",
		//	numOfUniqueTokens:             3,
		//	numOfAllowedTokensPerDuration: 8,
		//	duration:                      time.Second,
		//	numOfTokensEachToGenerate:     10,
		//	wantStats: []LimiterStat{
		//		{Allowed: 8, Dropped: 3},
		//		{Allowed: 8, Dropped: 3},
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

			//if got := limiter.Allow(tt.args.k); got != tt.want {
			//	t.Errorf("Allow() = %v, want %v", got, tt.want)
			//}
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
