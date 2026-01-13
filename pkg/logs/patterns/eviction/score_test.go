// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eviction

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestCalculateScore_BasicScoring tests the scoring algorithm with various inputs
func TestCalculateScore_BasicScoring(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		frequency    float64
		createdAt    time.Time
		lastAccessAt time.Time
		decayFactor  float64
		expectedMin  float64
		expectedMax  float64
	}{
		{
			name:         "brand new item high frequency",
			frequency:    10000,
			createdAt:    now,
			lastAccessAt: now,
			decayFactor:  0.5,
			expectedMin:  19000, // ~10000 * 1.0 * 2.0 (high recency boost)
			expectedMax:  21000,
		},
		{
			name:         "30 day old item with decay",
			frequency:    10000,
			createdAt:    now.Add(-30 * 24 * time.Hour),
			lastAccessAt: now.Add(-1 * time.Hour),
			decayFactor:  0.5,
			expectedMin:  3000, // ~10000 / (1+30)^0.5 * recency
			expectedMax:  4000,
		},
		{
			name:         "low frequency item",
			frequency:    100,
			createdAt:    now.Add(-7 * 24 * time.Hour),
			lastAccessAt: now.Add(-6 * 24 * time.Hour),
			decayFactor:  0.5,
			expectedMin:  30, // Low count, week old, not accessed recently
			expectedMax:  50,
		},
		{
			name:         "no decay factor (pure LFU)",
			frequency:    5000,
			createdAt:    now.Add(-60 * 24 * time.Hour),
			lastAccessAt: now,
			decayFactor:  0.0,
			expectedMin:  9000, // 5000 * 1.0 * ~2.0 (no age decay, high recency)
			expectedMax:  11000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := CalculateScore(tt.frequency, tt.createdAt, tt.lastAccessAt, now, tt.decayFactor)

			t.Logf("Actual score: %.2f (expected range: %.0f - %.0f)", score, tt.expectedMin, tt.expectedMax)
			assert.GreaterOrEqual(t, score, tt.expectedMin,
				"Score should be at least %f, got %f", tt.expectedMin, score)
			assert.LessOrEqual(t, score, tt.expectedMax,
				"Score should be at most %f, got %f", tt.expectedMax, score)
		})
	}
}

// TestCalculateScore_ZeroFrequency tests that zero frequency gives zero score
func TestCalculateScore_ZeroFrequency(t *testing.T) {
	now := time.Now()
	score := CalculateScore(0, now.Add(-10*24*time.Hour), now, now, 0.5)
	assert.Equal(t, 0.0, score, "Zero frequency should give zero score")
}

// TestCalculateScore_ClockSkewBackward tests negative age handling (clock went backwards)
func TestCalculateScore_ClockSkewBackward(t *testing.T) {
	now := time.Now()
	// Item created "in the future" (clock went backwards)
	score := CalculateScore(1000, now.Add(5*time.Minute), now, now, 0.5)

	// Should treat as brand new item (age = 0)
	// Score should be roughly: 1000 * 1.0 * 2.0 = ~2000
	assert.Greater(t, score, 1800.0, "Should not break with negative age")
	assert.Less(t, score, 2200.0, "Should treat as brand new item")
}

// TestCalculateScore_ClockSkewForward tests extreme age capping (clock jumped forward)
func TestCalculateScore_ClockSkewForward(t *testing.T) {
	now := time.Now()

	// Item appears 500 days old (clock jumped forward)
	score500 := CalculateScore(10000, now.Add(-500*24*time.Hour), now, now, 0.5)

	// Compare with 365 days old - should be similar due to capping
	score365 := CalculateScore(10000, now.Add(-365*24*time.Hour), now, now, 0.5)

	// Age should be capped at 365 days
	// Expected: (10000 / (1+365)^0.5) * (1 + 1/(1+0)) = (10000 / 19.13) * 2 ≈ 1045
	assert.Greater(t, score500, 900.0, "Should not be excessively penalized by extreme age")
	assert.Less(t, score500, 1200.0, "Should be capped at ~365 days worth of decay")
	assert.InDelta(t, score500, score365, 50.0, "500-day and 365-day items should score similarly due to capping")
}

// TestCalculateScore_RecencyBoost tests that recent access increases scores
func TestCalculateScore_RecencyBoost(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-30 * 24 * time.Hour)
	frequency := 1000.0

	// Test different recency levels
	scoreJustAccessed := CalculateScore(frequency, createdAt, now, now, 0.5)
	score1Hour := CalculateScore(frequency, createdAt, now.Add(-1*time.Hour), now, 0.5)
	score7Days := CalculateScore(frequency, createdAt, now.Add(-7*24*time.Hour), now, 0.5)

	// More recent access should give higher scores
	assert.Greater(t, scoreJustAccessed, score1Hour,
		"Just accessed (%.2f) should score higher than 1 hour ago (%.2f)", scoreJustAccessed, score1Hour)
	assert.Greater(t, score1Hour, score7Days,
		"1 hour ago (%.2f) should score higher than 7 days ago (%.2f)", score1Hour, score7Days)
}

// TestCalculateScore_AgeDecay tests that scores decrease as age increases
func TestCalculateScore_AgeDecay(t *testing.T) {
	now := time.Now()
	frequency := 10000.0
	decayFactor := 0.5

	tests := []struct {
		ageDays int
	}{
		{1},
		{7},
		{30},
		{90},
	}

	// Track that scores decrease as age increases
	previousScore := math.Inf(1)

	for _, tt := range tests {
		t.Run(string(rune(tt.ageDays)), func(t *testing.T) {
			score := CalculateScore(
				frequency,
				now.Add(-time.Duration(tt.ageDays)*24*time.Hour),
				now,
				now,
				decayFactor,
			)

			assert.Greater(t, score, 0.0, "Score should be positive")
			assert.Less(t, score, previousScore,
				"Older items should have lower scores (age decay working)")

			previousScore = score
		})
	}
}

// TestCalculateScore_DecayFactorComparison tests different decay factors
func TestCalculateScore_DecayFactorComparison(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-30 * 24 * time.Hour)
	frequency := 10000.0

	// Higher decay factor = more aggressive decay
	scoreNoDecay := CalculateScore(frequency, createdAt, now, now, 0.0)
	scoreMediumDecay := CalculateScore(frequency, createdAt, now, now, 0.5)
	scoreHighDecay := CalculateScore(frequency, createdAt, now, now, 1.0)

	assert.Greater(t, scoreNoDecay, scoreMediumDecay,
		"No decay should give higher score than medium decay")
	assert.Greater(t, scoreMediumDecay, scoreHighDecay,
		"Medium decay should give higher score than high decay")
}

// TestCalculateScore_FrequencyMatters tests that frequency is the primary signal
func TestCalculateScore_FrequencyMatters(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-7 * 24 * time.Hour)

	highScore := CalculateScore(10000, createdAt, now, now, 0.5)
	lowScore := CalculateScore(100, createdAt, now, now, 0.5)

	assert.Greater(t, highScore, lowScore,
		"High frequency should have much higher score")
	assert.Greater(t, highScore/lowScore, 50.0,
		"Score ratio should roughly match frequency ratio")
}

// TestCalculateScore_RealWorldScenario tests realistic competing items
func TestCalculateScore_RealWorldScenario(t *testing.T) {
	now := time.Now()

	// Item A: Old but very popular, still active
	scoreA := CalculateScore(
		50000,                     // High frequency
		now.Add(-60*24*time.Hour), // 60 days old
		now.Add(-1*time.Hour),     // Recently accessed
		now,
		0.5,
	)

	// Item B: Recent but low frequency, dormant
	scoreB := CalculateScore(
		100,                      // Low frequency
		now.Add(-7*24*time.Hour), // 7 days old
		now.Add(-6*24*time.Hour), // Not accessed recently
		now,
		0.5,
	)

	assert.Greater(t, scoreA, scoreB,
		"High frequency active item (A=%.2f) should beat low frequency dormant item (B=%.2f)", scoreA, scoreB)
}

// TestCalculateScore_EvictionPriority tests realistic eviction ordering
func TestCalculateScore_EvictionPriority(t *testing.T) {
	now := time.Now()

	items := []struct {
		id           int
		frequency    float64
		createdAt    time.Time
		lastAccessAt time.Time
		description  string
	}{
		{1, 10000, now.Add(-30 * 24 * time.Hour), now, "high frequency, recent"},
		{2, 50, now.Add(-5 * 24 * time.Hour), now.Add(-4 * 24 * time.Hour), "low frequency, dormant"},
		{3, 5000, now.Add(-15 * 24 * time.Hour), now.Add(-1 * time.Hour), "medium frequency, active"},
		{4, 100, now.Add(-90 * 24 * time.Hour), now.Add(-89 * 24 * time.Hour), "old, low frequency, dormant"},
	}

	// Calculate scores
	scores := make(map[int]float64)
	for _, item := range items {
		scores[item.id] = CalculateScore(item.frequency, item.createdAt, item.lastAccessAt, now, 0.5)
		t.Logf("Item %d (%s): score=%.2f", item.id, item.description, scores[item.id])
	}

	// Verify expected ordering: 1 > 3 > 2 > 4
	assert.Greater(t, scores[1], scores[2], "Item 1 should score higher than 2")
	assert.Greater(t, scores[3], scores[2], "Item 3 should score higher than 2")
	assert.Greater(t, scores[2], scores[4], "Item 2 should score higher than 4")
	assert.Less(t, scores[4], scores[3], "Item 4 should be evicted before 3")
	assert.Less(t, scores[2], scores[1], "Item 2 should be evicted before 1")
}
