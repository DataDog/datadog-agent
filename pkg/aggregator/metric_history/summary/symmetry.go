// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"math"
	"time"
)

// DetectSymmetry analyzes events in a cluster to find inverse or proportional
// relationships between metrics.
//
// For inverse detection:
// - Two metrics with opposite Direction (increase vs decrease)
// - Similar Magnitude (within tolerance, e.g., 20%)
// - Events occurring at similar times
//
// For proportional detection:
// - Two metrics with same Direction
// - Correlated Magnitude changes
//
// Returns nil if no significant pattern detected.
func DetectSymmetry(events []AnomalyEvent) *SymmetryPattern {
	// 1. Edge cases
	if len(events) < 2 {
		return nil
	}

	// 2. Get unique metrics
	metrics := uniqueMetrics(events)
	if len(metrics) < 2 {
		return nil // need at least 2 different metrics
	}

	// 3. Group events by timestamp (within 5 seconds)
	groups := groupByTimestamp(events, 5*time.Second)

	// 4. Track evidence for each metric pair
	type pairKey struct {
		m1 string
		m2 string
	}
	type directionMapping struct {
		m1Dir string
		m2Dir string
	}
	inverseEvidence := make(map[pairKey]int)
	proportionalEvidence := make(map[pairKey]int)
	inverseMappings := make(map[pairKey]map[directionMapping]int)
	proportionalMappings := make(map[pairKey]map[directionMapping]int)
	totalPairs := make(map[pairKey]int)

	// For each timestamp group, look for metric pairs
	for _, group := range groups {
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				a, b := group[i], group[j]
				if a.Metric == b.Metric {
					continue // same metric, skip
				}

				// Ensure consistent ordering
				m1, m2 := a.Metric, b.Metric
				e1, e2 := a, b
				if m1 > m2 {
					m1, m2 = m2, m1
					e1, e2 = b, a
				}

				key := pairKey{m1, m2}
				totalPairs[key]++
				mapping := directionMapping{e1.Direction, e2.Direction}

				// Check for inverse pattern
				if isInverse(e1, e2) {
					inverseEvidence[key]++
					if inverseMappings[key] == nil {
						inverseMappings[key] = make(map[directionMapping]int)
					}
					inverseMappings[key][mapping]++
				}

				// Check for proportional pattern
				if isProportional(e1, e2) {
					proportionalEvidence[key]++
					if proportionalMappings[key] == nil {
						proportionalMappings[key] = make(map[directionMapping]int)
					}
					proportionalMappings[key][mapping]++
				}
			}
		}
	}

	// 5. Find the strongest pattern
	var bestPair pairKey
	var bestType SymmetryType
	var bestConfidence float64

	for pair, total := range totalPairs {
		inverseCount := inverseEvidence[pair]
		proportionalCount := proportionalEvidence[pair]

		// Calculate confidence for inverse
		if inverseCount > 0 {
			// Check direction consistency
			mappings := inverseMappings[pair]
			maxMappingCount := 0
			for _, count := range mappings {
				if count > maxMappingCount {
					maxMappingCount = count
				}
			}

			// Direction consistency: ratio of most common mapping to total inverse observations
			directionConsistency := float64(maxMappingCount) / float64(inverseCount)

			// Overall consistency: ratio of inverse observations to total pairs
			consistency := float64(inverseCount) / float64(total)

			// Base confidence starts at 0.6 for single observation
			// Approaches 0.9+ for multiple consistent observations
			confidence := 0.6 + (consistency * 0.3)

			// Apply direction consistency penalty
			confidence *= directionConsistency

			// Boost confidence based on number of observations
			if inverseCount >= 3 && directionConsistency >= 0.8 {
				confidence = math.Min(0.95, confidence+0.1)
			} else if inverseCount >= 2 && directionConsistency >= 0.8 {
				confidence = math.Min(0.9, confidence+0.05)
			}

			if confidence > bestConfidence {
				bestPair = pair
				bestType = Inverse
				bestConfidence = confidence
			}
		}

		// Calculate confidence for proportional
		if proportionalCount > 0 {
			// Check direction consistency
			mappings := proportionalMappings[pair]
			maxMappingCount := 0
			for _, count := range mappings {
				if count > maxMappingCount {
					maxMappingCount = count
				}
			}

			// Direction consistency
			directionConsistency := float64(maxMappingCount) / float64(proportionalCount)

			consistency := float64(proportionalCount) / float64(total)
			// Proportional has slightly lower base confidence
			confidence := 0.55 + (consistency * 0.25)

			// Apply direction consistency penalty
			confidence *= directionConsistency

			// Boost confidence based on number of observations
			if proportionalCount >= 3 && directionConsistency >= 0.8 {
				confidence = math.Min(0.85, confidence+0.08)
			} else if proportionalCount >= 2 && directionConsistency >= 0.8 {
				confidence = math.Min(0.8, confidence+0.04)
			}

			// Only consider if better than current best and no inverse found
			if confidence > bestConfidence && bestType != Inverse {
				bestPair = pair
				bestType = Proportional
				bestConfidence = confidence
			}
		}
	}

	// 6. Return strongest pattern if confidence > 0.5
	if bestConfidence > 0.5 {
		return &SymmetryPattern{
			Type:       bestType,
			Metrics:    [2]string{bestPair.m1, bestPair.m2},
			Confidence: bestConfidence,
		}
	}

	return nil
}

// uniqueMetrics returns the set of unique metric names
func uniqueMetrics(events []AnomalyEvent) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, e := range events {
		if !seen[e.Metric] {
			seen[e.Metric] = true
			result = append(result, e.Metric)
		}
	}
	return result
}

// groupByTimestamp groups events that occur within tolerance of each other
func groupByTimestamp(events []AnomalyEvent, tolerance time.Duration) [][]AnomalyEvent {
	if len(events) == 0 {
		return nil
	}

	// Sort events by timestamp (simple bubble sort for small data)
	sorted := make([]AnomalyEvent, len(events))
	copy(sorted, events)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Timestamp.Before(sorted[i].Timestamp) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	groups := [][]AnomalyEvent{}
	currentGroup := []AnomalyEvent{sorted[0]}
	groupStart := sorted[0].Timestamp

	for i := 1; i < len(sorted); i++ {
		if sorted[i].Timestamp.Sub(groupStart) <= tolerance {
			// Within tolerance, add to current group
			currentGroup = append(currentGroup, sorted[i])
		} else {
			// Start new group
			groups = append(groups, currentGroup)
			currentGroup = []AnomalyEvent{sorted[i]}
			groupStart = sorted[i].Timestamp
		}
	}

	// Add final group
	groups = append(groups, currentGroup)
	return groups
}

// isInverse checks if two events show inverse relationship
func isInverse(a, b AnomalyEvent) bool {
	if a.Metric == b.Metric {
		return false
	}
	if a.Direction == b.Direction {
		return false
	}
	return magnitudeWithinTolerance(a.Magnitude, b.Magnitude, 0.20)
}

// isProportional checks if two events show proportional relationship
func isProportional(a, b AnomalyEvent) bool {
	if a.Metric == b.Metric {
		return false
	}
	if a.Direction != b.Direction {
		return false
	}
	// For proportional, we're more lenient - same direction is the main signal
	return magnitudeWithinTolerance(a.Magnitude, b.Magnitude, 0.50)
}

// magnitudeWithinTolerance checks if two magnitudes are within tolerance
func magnitudeWithinTolerance(a, b, tolerance float64) bool {
	if a == 0 && b == 0 {
		return true
	}
	max := math.Max(a, b)
	if max == 0 {
		return true
	}
	diff := math.Abs(a - b)
	return diff/max <= tolerance
}
