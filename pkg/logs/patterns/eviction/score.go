// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eviction provides shared eviction scoring algorithms for patterns and tags.
package eviction

import (
	"math"
	"time"
)

// CalculateScore calculates an eviction score using frequency and temporal metadata.
// Lower scores indicate higher priority for eviction.
//
// The score combines:
// - Frequency (usage count): More frequent items get higher scores
// - Age decay: Older items gradually lose priority
// - Recency boost: Recently accessed items get bonus points
// Formula: score = (frequency / (1 + age)^decayFactor) * (1 + recencyBoost)
//
// This algorithm uses power-law decay to balance frequency with age, ensuring that
// old but still-used items aren't evicted before truly unused ones.
func CalculateScore(frequency float64, createdAt, lastAccessAt, now time.Time, decayFactor float64) float64 {
	// Age-based decay (from CreatedAt)
	ageDays := now.Sub(createdAt).Hours() / 24.0

	// Clamp to reasonable range [0, 365] to handle clock skew
	if ageDays < 0 {
		ageDays = 0 // Clock moved backward
	} else if ageDays > 365 {
		ageDays = 365 // Clock moved forward or genuinely old item
	}

	// Apply power-law decay: score = frequency / (1 + age)^decayFactor
	ageDecay := 1.0 / math.Pow(1.0+ageDays, decayFactor)
	baseScore := frequency * ageDecay

	// Recency boost (from LastAccessAt)
	hoursSinceAccess := now.Sub(lastAccessAt).Hours()
	if hoursSinceAccess < 0 {
		hoursSinceAccess = 0 // Handle clock skew
	}

	// Items accessed recently get a bonus (hyperbolic decay)
	// recencyBoost ranges from ~1.0 (just accessed) to ~0.0 (very old access)
	recencyBoost := 1.0 / (1.0 + hoursSinceAccess/24.0)

	// Combine base score with recency boost
	// Frequency is primary signal, recency is secondary
	finalScore := baseScore * (1.0 + recencyBoost)

	return finalScore
}
