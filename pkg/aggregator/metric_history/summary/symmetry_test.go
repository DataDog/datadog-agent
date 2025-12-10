// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDetectSymmetry_Inverse(t *testing.T) {
	// Test basic inverse relationship: one metric increases, another decreases
	// by the same amount at the same time
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 12.0,
		},
		{
			Timestamp: now,
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "decrease",
			Magnitude: 12.0,
		},
	}

	pattern := DetectSymmetry(events)

	assert.NotNil(t, pattern)
	assert.Equal(t, Inverse, pattern.Type)
	assert.Contains(t, pattern.Metrics[:], "system.disk.free")
	assert.Contains(t, pattern.Metrics[:], "system.disk.used")
	assert.Greater(t, pattern.Confidence, 0.8, "Should have high confidence for perfect inverse match")
}

func TestDetectSymmetry_MagnitudeTolerance(t *testing.T) {
	// Test that inverse detection tolerates magnitude differences within 20%
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "disk.free",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 12.0,
		},
		{
			Timestamp: now,
			Metric:    "disk.used",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "decrease",
			Magnitude: 11.0, // 8.3% difference, within 20% tolerance
		},
	}

	pattern := DetectSymmetry(events)

	assert.NotNil(t, pattern)
	assert.Equal(t, Inverse, pattern.Type)
	assert.Contains(t, pattern.Metrics[:], "disk.free")
	assert.Contains(t, pattern.Metrics[:], "disk.used")
}

func TestDetectSymmetry_NoFalsePositives(t *testing.T) {
	// Test that unrelated metrics with same direction don't trigger false positives
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "disk.free",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 12.0,
		},
		{
			Timestamp: now,
			Metric:    "cpu.user",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 5.0,
		},
	}

	pattern := DetectSymmetry(events)

	// Should not detect a meaningful pattern between unrelated metrics
	// (or if it does detect proportional, confidence should be low)
	if pattern != nil && pattern.Type == Inverse {
		t.Errorf("Should not detect inverse pattern for same-direction unrelated metrics")
	}
}

func TestDetectSymmetry_Proportional(t *testing.T) {
	// Test proportional relationship: both metrics increase together
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "io.read",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 100.0,
		},
		{
			Timestamp: now,
			Metric:    "io.write",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 95.0,
		},
	}

	pattern := DetectSymmetry(events)

	assert.NotNil(t, pattern)
	assert.Equal(t, Proportional, pattern.Type)
	assert.Contains(t, pattern.Metrics[:], "io.read")
	assert.Contains(t, pattern.Metrics[:], "io.write")
	assert.Greater(t, pattern.Confidence, 0.0, "Should have some confidence in proportional relationship")
}

func TestDetectSymmetry_MultiEvent(t *testing.T) {
	// Test that consistent patterns across multiple timestamp groups
	// yield higher confidence
	now := time.Now()
	events := []AnomalyEvent{
		// First pair at T0
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 12.0,
		},
		{
			Timestamp: now,
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "decrease",
			Magnitude: 12.0,
		},
		// Second pair at T0+2s (within 5 second grouping)
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.85,
			Direction: "increase",
			Magnitude: 11.5,
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.85,
			Direction: "decrease",
			Magnitude: 11.5,
		},
		// Third pair at T0+10s (new time group)
		{
			Timestamp: now.Add(10 * time.Second),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server3"},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 13.0,
		},
		{
			Timestamp: now.Add(10 * time.Second),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "server3"},
			Severity:  0.8,
			Direction: "decrease",
			Magnitude: 13.0,
		},
	}

	pattern := DetectSymmetry(events)

	assert.NotNil(t, pattern)
	assert.Equal(t, Inverse, pattern.Type)
	assert.Contains(t, pattern.Metrics[:], "system.disk.free")
	assert.Contains(t, pattern.Metrics[:], "system.disk.used")
	// Multiple consistent observations should yield higher confidence
	assert.Greater(t, pattern.Confidence, 0.85, "Multiple consistent patterns should have higher confidence")
}

func TestDetectSymmetry_MixedDirections(t *testing.T) {
	// Test that inconsistent patterns yield NoSymmetry
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 12.0,
		},
		{
			Timestamp: now,
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "decrease",
			Magnitude: 12.0,
		},
		// Second event has same metric but opposite direction - inconsistent
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.85,
			Direction: "decrease", // opposite direction from first
			Magnitude: 11.0,
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.85,
			Direction: "increase", // opposite direction from first
			Magnitude: 11.0,
		},
	}

	pattern := DetectSymmetry(events)

	// Should not detect a strong pattern with inconsistent directions
	if pattern != nil {
		assert.Less(t, pattern.Confidence, 0.5, "Mixed directions should result in low confidence")
	}
}

func TestDetectSymmetry_SingleMetric(t *testing.T) {
	// Test that events with only one metric type return NoSymmetry
	// (need at least 2 different metrics to compare)
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 12.0,
		},
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server2"},
			Severity:  0.85,
			Direction: "increase",
			Magnitude: 11.0,
		},
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "server3"},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 10.0,
		},
	}

	pattern := DetectSymmetry(events)

	// Cannot detect symmetry with only one metric
	assert.Nil(t, pattern, "Should return nil when only one metric type is present")
}

func TestDetectSymmetry_RealDiskScenario(t *testing.T) {
	// Test the real-world scenario from the demo:
	// 6 events with 3x system.disk.free (increase) and 3x system.disk.used (decrease)
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "web-1", "device": "/dev/sda1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 12.3,
		},
		{
			Timestamp: now,
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "web-1", "device": "/dev/sda1"},
			Severity:  0.9,
			Direction: "decrease",
			Magnitude: 12.3,
		},
		{
			Timestamp: now.Add(1 * time.Second),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "web-2", "device": "/dev/sdb1"},
			Severity:  0.85,
			Direction: "increase",
			Magnitude: 11.8,
		},
		{
			Timestamp: now.Add(1 * time.Second),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "web-2", "device": "/dev/sdb1"},
			Severity:  0.85,
			Direction: "decrease",
			Magnitude: 11.8,
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.disk.free",
			Tags:      map[string]string{"host": "web-3", "device": "/dev/sdc1"},
			Severity:  0.8,
			Direction: "increase",
			Magnitude: 12.1,
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Metric:    "system.disk.used",
			Tags:      map[string]string{"host": "web-3", "device": "/dev/sdc1"},
			Severity:  0.8,
			Direction: "decrease",
			Magnitude: 12.1,
		},
	}

	pattern := DetectSymmetry(events)

	assert.NotNil(t, pattern, "Should detect pattern in real disk scenario")
	assert.Equal(t, Inverse, pattern.Type, "Disk free/used should show inverse relationship")
	assert.Contains(t, pattern.Metrics[:], "system.disk.free")
	assert.Contains(t, pattern.Metrics[:], "system.disk.used")
	assert.Greater(t, pattern.Confidence, 0.8, "Consistent pattern across 3 pairs should have high confidence")
}

func TestDetectSymmetry_EmptyEvents(t *testing.T) {
	// Test edge case of empty event slice
	events := []AnomalyEvent{}

	pattern := DetectSymmetry(events)

	assert.Nil(t, pattern, "Should return nil for empty events")
}

func TestDetectSymmetry_OutsideTolerance(t *testing.T) {
	// Test that magnitude differences outside 20% tolerance don't trigger inverse detection
	now := time.Now()
	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "disk.free",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
			Magnitude: 100.0,
		},
		{
			Timestamp: now,
			Metric:    "disk.used",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "decrease",
			Magnitude: 50.0, // 50% difference, outside 20% tolerance
		},
	}

	pattern := DetectSymmetry(events)

	// Should not detect inverse pattern when magnitudes differ too much
	if pattern != nil && pattern.Type == Inverse {
		assert.Less(t, pattern.Confidence, 0.5, "Large magnitude difference should result in low confidence")
	}
}
