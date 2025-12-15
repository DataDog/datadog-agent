// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TestCalculateEvictionScore_BasicScoring tests the scoring algorithm
func TestCalculateEvictionScore_BasicScoring(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		logCount     int
		createdAt    time.Time
		lastAccessAt time.Time
		decayFactor  float64
		expectedMin  float64
		expectedMax  float64
	}{
		{
			name:         "brand new pattern high frequency",
			logCount:     10000,
			createdAt:    now,
			lastAccessAt: now,
			decayFactor:  0.5,
			expectedMin:  19000, // ~10000 * 1.0 * 2.0 (high recency boost)
			expectedMax:  21000,
		},
		{
			name:         "30 day old pattern with decay",
			logCount:     10000,
			createdAt:    now.Add(-30 * 24 * time.Hour),
			lastAccessAt: now.Add(-1 * time.Hour),
			decayFactor:  0.5,
			expectedMin:  3000, // ~10000 / (1+30)^0.5 * recency
			expectedMax:  4000,
		},
		{
			name:         "low frequency pattern",
			logCount:     100,
			createdAt:    now.Add(-7 * 24 * time.Hour),
			lastAccessAt: now.Add(-6 * 24 * time.Hour),
			decayFactor:  0.5,
			expectedMin:  30, // Low count, week old, not accessed recently
			expectedMax:  50,
		},
		{
			name:         "no decay factor (pure LFU)",
			logCount:     5000,
			createdAt:    now.Add(-60 * 24 * time.Hour),
			lastAccessAt: now,
			decayFactor:  0.0,
			expectedMin:  9000, // 5000 * 1.0 * ~2.0 (no age decay, high recency)
			expectedMax:  11000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := &Pattern{
				LogCount:     tt.logCount,
				CreatedAt:    tt.createdAt,
				LastAccessAt: tt.lastAccessAt,
			}

			score := pattern.calculateEvictionScore(now, tt.decayFactor)

			// Since the score is heavily influenced by the decay factor, we can't use a fixed expected number hence a range instead.
			t.Logf("Actual score: %.2f (expected range: %.0f - %.0f)", score, tt.expectedMin, tt.expectedMax)
			assert.GreaterOrEqual(t, score, tt.expectedMin,
				"Score should be at least %f, got %f", tt.expectedMin, score)
			assert.LessOrEqual(t, score, tt.expectedMax,
				"Score should be at most %f, got %f", tt.expectedMax, score)
		})
	}
}

// TestCalculateEvictionScore_ZeroLogCount tests that a pattern with zero log count should have a score of 0.
func TestCalculateEvictionScore_ZeroLogCount(t *testing.T) {
	now := time.Now()
	pattern := &Pattern{
		LogCount:     0,
		CreatedAt:    now.Add(-10 * 24 * time.Hour),
		LastAccessAt: now,
	}

	// Zero LogCount should still calculate (frequency=0 in formula)
	score := pattern.calculateEvictionScore(now, 0.5)
	assert.Equal(t, 0.0, score, "Pattern with zero LogCount should have score of 0")
}

// TestCalculateEvictionScore_ClockSkewBackward tests that a pattern created in the future (clock skew) should be treated as a brand new pattern.
func TestCalculateEvictionScore_ClockSkewBackward(t *testing.T) {
	now := time.Now()

	// Pattern created "in the future" (clock went backwards)
	pattern := &Pattern{
		LogCount:     1000,
		CreatedAt:    now.Add(5 * time.Minute), // 5 minutes in future
		LastAccessAt: now,
	}

	score := pattern.calculateEvictionScore(now, 0.5)

	// Should treat as brand new pattern (age = 0)
	// Score should be roughly: 1000 * 1.0 * 2.0 = ~2000
	assert.Greater(t, score, 1800.0, "Should not break with negative age")
	assert.Less(t, score, 2200.0, "Should treat as brand new pattern")
}

// TestCalculateEvictionScore_ClockSkewForward tests that a pattern created in the past (clock skew) should be treated as a genuinely old pattern.
func TestCalculateEvictionScore_ClockSkewForward(t *testing.T) {
	now := time.Now()

	// Pattern appears very old due to clock skew forward (500 days)
	pattern := &Pattern{
		LogCount:     10000,
		CreatedAt:    now.Add(-500 * 24 * time.Hour), // 500 days ago
		LastAccessAt: now,
	}

	score := pattern.calculateEvictionScore(now, 0.5)

	// Age should be capped at 365 days to prevent excessive penalty
	// Expected: (10000 / (1+365)^0.5) * (1 + 1/(1+0)) = (10000 / 19.13) * 2 ≈ 1045
	assert.Greater(t, score, 900.0, "Should not be excessively penalized by extreme age")
	assert.Less(t, score, 1200.0, "Should be capped at ~365 days worth of decay")

	// Compare with genuinely old pattern (365 days) - should be same
	pattern365 := &Pattern{
		LogCount:     10000,
		CreatedAt:    now.Add(-365 * 24 * time.Hour),
		LastAccessAt: now,
	}
	score365 := pattern365.calculateEvictionScore(now, 0.5)

	// Both should be effectively the same (capped)
	assert.InDelta(t, score, score365, 50.0, "500-day and 365-day patterns should score similarly due to capping")
}

// TestCalculateEvictionScore_RecencyBoost tests that more recent access gives higher scores.
func TestCalculateEvictionScore_RecencyBoost(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-30 * 24 * time.Hour)

	// Test that more recent access gives higher scores
	justAccessed := &Pattern{
		LogCount:     1000,
		CreatedAt:    createdAt,
		LastAccessAt: now,
	}

	accessed1HourAgo := &Pattern{
		LogCount:     1000,
		CreatedAt:    createdAt,
		LastAccessAt: now.Add(-1 * time.Hour),
	}

	accessed7DaysAgo := &Pattern{
		LogCount:     1000,
		CreatedAt:    createdAt,
		LastAccessAt: now.Add(-7 * 24 * time.Hour),
	}

	scoreJustAccessed := justAccessed.calculateEvictionScore(now, 0.5)
	score1Hour := accessed1HourAgo.calculateEvictionScore(now, 0.5)
	score7Days := accessed7DaysAgo.calculateEvictionScore(now, 0.5)

	// More recent access should give higher scores
	assert.Greater(t, scoreJustAccessed, score1Hour,
		"Pattern accessed now (%.2f) should score higher than 1 hour ago (%.2f)", scoreJustAccessed, score1Hour)
	assert.Greater(t, score1Hour, score7Days,
		"Pattern accessed 1 hour ago (%.2f) should score higher than 7 days ago (%.2f)", score1Hour, score7Days)
}

// TestCalculateEvictionScore_AgeDecay tests that scores decrease as age increases.
func TestCalculateEvictionScore_AgeDecay(t *testing.T) {
	now := time.Now()
	logCount := 10000

	tests := []struct {
		name        string
		ageDays     int
		decayFactor float64
	}{
		{"1 day old", 1, 0.5},
		{"7 days old", 7, 0.5},
		{"30 days old", 30, 0.5},
		{"90 days old", 90, 0.5},
	}

	// Track that scores decrease as age increases
	var previousScore float64 = math.Inf(1)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := &Pattern{
				LogCount:     logCount,
				CreatedAt:    now.Add(-time.Duration(tt.ageDays) * 24 * time.Hour),
				LastAccessAt: now,
			}

			score := pattern.calculateEvictionScore(now, tt.decayFactor)

			assert.Greater(t, score, 0.0, "Score should be positive")
			assert.Less(t, score, previousScore,
				"Older patterns should have lower scores (age decay working)")

			previousScore = score
		})
	}
}

// TestCalculateEvictionScore_DecayFactorComparison tests that different decay factors affect the score.
func TestCalculateEvictionScore_DecayFactorComparison(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-30 * 24 * time.Hour)

	pattern := &Pattern{
		LogCount:     10000,
		CreatedAt:    createdAt,
		LastAccessAt: now,
	}

	// Higher decay factor = more aggressive decay
	scoreNoDecay := pattern.calculateEvictionScore(now, 0.0)
	scoreMediumDecay := pattern.calculateEvictionScore(now, 0.5)
	scoreHighDecay := pattern.calculateEvictionScore(now, 1.0)

	assert.Greater(t, scoreNoDecay, scoreMediumDecay,
		"No decay should give higher score than medium decay")
	assert.Greater(t, scoreMediumDecay, scoreHighDecay,
		"Medium decay should give higher score than high decay")
}

// TestCalculateEvictionScore_FrequencyMatters tests that higher frequency patterns have a higher score.
func TestCalculateEvictionScore_FrequencyMatters(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-7 * 24 * time.Hour)

	highFreq := &Pattern{
		LogCount:     10000,
		CreatedAt:    createdAt,
		LastAccessAt: now,
	}

	lowFreq := &Pattern{
		LogCount:     100,
		CreatedAt:    createdAt,
		LastAccessAt: now,
	}

	highScore := highFreq.calculateEvictionScore(now, 0.5)
	lowScore := lowFreq.calculateEvictionScore(now, 0.5)

	assert.Greater(t, highScore, lowScore,
		"High frequency pattern should have much higher score")
	assert.Greater(t, highScore/lowScore, 50.0,
		"Score ratio should roughly match frequency ratio")
}

// TestCalculateEvictionScore_RealWorldScenario tests that the scoring algorithm works as expected in a real-world scenario.
func TestCalculateEvictionScore_RealWorldScenario(t *testing.T) {
	now := time.Now()

	// Scenario: Two patterns competing for eviction

	// Pattern A: Old but very popular, still active
	patternA := &Pattern{
		LogCount:     50000,
		CreatedAt:    now.Add(-60 * 24 * time.Hour),
		LastAccessAt: now.Add(-1 * time.Hour),
	}

	// Pattern B: Recent but low frequency, dormant
	patternB := &Pattern{
		LogCount:     100,
		CreatedAt:    now.Add(-7 * 24 * time.Hour),
		LastAccessAt: now.Add(-6 * 24 * time.Hour),
	}

	scoreA := patternA.calculateEvictionScore(now, 0.5)
	scoreB := patternB.calculateEvictionScore(now, 0.5)

	assert.Greater(t, scoreA, scoreB,
		"High frequency active pattern (A=%.2f) should beat low frequency dormant pattern (B=%.2f)", scoreA, scoreB)
}

// TestEvictionScoreComparison tests that the eviction score comparison works as expected.
func TestEvictionScoreComparison(t *testing.T) {
	// Integration test: Verify eviction priorities work as expected
	now := time.Now()

	patterns := []*Pattern{
		{
			// Should be kept: high frequency, recent
			LogCount:     10000,
			CreatedAt:    now.Add(-30 * 24 * time.Hour),
			LastAccessAt: now,
			PatternID:    1,
		},
		{
			// Should be evicted: low frequency, dormant
			LogCount:     50,
			CreatedAt:    now.Add(-5 * 24 * time.Hour),
			LastAccessAt: now.Add(-4 * 24 * time.Hour),
			PatternID:    2,
		},
		{
			// Should be kept: medium frequency, active
			LogCount:     5000,
			CreatedAt:    now.Add(-15 * 24 * time.Hour),
			LastAccessAt: now.Add(-1 * time.Hour),
			PatternID:    3,
		},
		{
			// Should be evicted: old, low frequency, dormant
			LogCount:     100,
			CreatedAt:    now.Add(-90 * 24 * time.Hour),
			LastAccessAt: now.Add(-89 * 24 * time.Hour),
			PatternID:    4,
		},
	}

	// Calculate scores
	scores := make(map[uint64]float64)
	for _, p := range patterns {
		scores[p.PatternID] = p.calculateEvictionScore(now, 0.5)
	}

	// Verify expected ordering
	assert.Greater(t, scores[1], scores[2],
		"Pattern 1 should score higher than 2 (p1=%.2f, p2=%.2f)", scores[1], scores[2])
	assert.Greater(t, scores[3], scores[2],
		"Pattern 3 should score higher than 2 (p3=%.2f, p2=%.2f)", scores[3], scores[2])
	assert.Greater(t, scores[2], scores[4],
		"Pattern 2 should score higher than 4 (p2=%.2f, p4=%.2f)", scores[2], scores[4])

	// Pattern 2 and 4 should be evicted first
	assert.Less(t, scores[4], scores[3],
		"Pattern 4 should be evicted before 3 (p4=%.2f, p3=%.2f)", scores[4], scores[3])
	assert.Less(t, scores[2], scores[1],
		"Pattern 2 should be evicted before 1 (p2=%.2f, p1=%.2f)", scores[2], scores[1])
}

// TestCalculateEvictionScore_WithTokenList tests that the scoring algorithm works as expected with a new token list.
func TestCalculateEvictionScore_WithTokenList(t *testing.T) {
	// Test with actual token list to ensure integration works
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "ERROR", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "timeout", token.NotWildcard))

	pattern := newPattern(tl, 12345)

	now := time.Now()
	pattern.LogCount = 1000
	pattern.LastAccessAt = now

	score := pattern.calculateEvictionScore(now, 0.5)
	assert.Greater(t, score, 0.0, "Pattern with token list should calculate valid score")
}
