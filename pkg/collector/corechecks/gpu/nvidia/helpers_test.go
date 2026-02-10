// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveDuplicateMetrics(t *testing.T) {
	t.Run("ComprehensiveScenario", func(t *testing.T) {
		// Test the exact scenario from function comment plus additional edge cases including zero priority
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1001"}},
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1002"}},
				{Name: "core.temp", Priority: Low}, // Zero priority (default)
			},
			stateless: {
				{Name: "memory.usage", Priority: Low, Tags: []string{"pid:1003"}},
				{Name: "fan.speed", Priority: Low}, // Zero priority (default)
				{Name: "power.draw", Priority: Low},
				{Name: "disk.usage", Priority: Low}, // Zero priority, unique metric
			},
			ebpf: {
				{Name: "core.temp", Priority: Medium}, // Conflicts with CollectorA, higher priority beats zero
				{Name: "voltage", Priority: Low},
				{Name: "fan.speed", Priority: Low}, // Zero priority tie with CollectorB
			},
			field: {}, // Empty collector
		}

		result := RemoveDuplicateMetrics(allMetrics)

		require.Len(t, result, 7) // 6 + 1 for fan.speed winner

		// Check all the deterministic results
		var memoryUsageCount, coreTempCount, powerDrawCount, voltageCount, diskUsageCount, fanSpeedCount int
		for _, metric := range result {
			switch metric.Name {
			case "memory.usage":
				require.Equal(t, Medium, metric.Priority)
				require.NotContains(t, metric.Tags, "pid:1003")
				memoryUsageCount++
			case "core.temp":
				require.Equal(t, Medium, metric.Priority)
				coreTempCount++
			case "power.draw":
				require.Equal(t, Low, metric.Priority)
				powerDrawCount++
			case "voltage":
				require.Equal(t, Low, metric.Priority)
				voltageCount++
			case "disk.usage":
				require.Equal(t, Low, metric.Priority)
				diskUsageCount++
			case "fan.speed":
				require.Equal(t, Low, metric.Priority) // Zero priority tie winner
				fanSpeedCount++
			}
		}

		require.Equal(t, 2, memoryUsageCount) // Both from CollectorA
		require.Equal(t, 1, coreTempCount)    // CollectorC wins
		require.Equal(t, 1, powerDrawCount)   // CollectorB unique
		require.Equal(t, 1, voltageCount)     // CollectorC unique
		require.Equal(t, 1, diskUsageCount)   // CollectorB unique (zero priority)
		require.Equal(t, 1, fanSpeedCount)    // One collector wins the zero priority tie
	})

	t.Run("SingleCollectorMultipleSameName", func(t *testing.T) {
		// Ensure intra-collector preservation - no deduplication within same collector
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1001"}},
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1002"}},
				{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1003"}},
				{Name: "cpu.usage", Priority: Low},
			},
		}

		result := RemoveDuplicateMetrics(allMetrics)

		expected := []Metric{
			{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1001"}},
			{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1002"}},
			{Name: "memory.usage", Priority: Medium, Tags: []string{"pid:1003"}},
			{Name: "cpu.usage", Priority: Low},
		}

		require.Len(t, result, 4)
		require.ElementsMatch(t, result, expected)
	})

	t.Run("PriorityTie", func(t *testing.T) {
		// Edge case: same metric name with same priority across collectors
		// First collector (in iteration order) should win
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "metric1", Priority: Low, Tags: []string{"tagA"}},
			},
			stateless: {
				{Name: "metric1", Priority: Low, Tags: []string{"tagB"}},
			},
		}

		result := RemoveDuplicateMetrics(allMetrics)

		// Should have exactly 1 metric (one collector wins the tie)
		require.Len(t, result, 1)
		require.Equal(t, Low, result[0].Priority)
		// Don't assert which specific tag wins since map iteration order is not guaranteed
	})

	t.Run("EmptyInputs", func(t *testing.T) {
		// Edge case: empty inputs
		t.Run("EmptyMap", func(t *testing.T) {
			result := RemoveDuplicateMetrics(map[CollectorName][]Metric{})
			require.Len(t, result, 0)
		})

		t.Run("EmptyCollectors", func(t *testing.T) {
			allMetrics := map[CollectorName][]Metric{
				sampling: {},
				ebpf:     {},
			}
			result := RemoveDuplicateMetrics(allMetrics)
			require.Len(t, result, 0)
		})

		t.Run("MixedEmptyAndNonEmpty", func(t *testing.T) {
			allMetrics := map[CollectorName][]Metric{
				sampling: {},
				stateless: {
					{Name: "metric1", Priority: Low},
				},
			}
			result := RemoveDuplicateMetrics(allMetrics)
			require.Len(t, result, 1)
			require.Equal(t, "metric1", result[0].Name)
		})
	})

	t.Run("PreservedTags", func(t *testing.T) {
		tags := []string{"pid:1001", "pid:1002"}
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "memory.limit", Priority: Medium, Tags: tags},
			},
			ebpf: {
				{Name: "memory.limit", Priority: Low, Tags: nil},
			},
		}
		result := RemoveDuplicateMetrics(allMetrics)
		require.Len(t, result, 1)
		require.ElementsMatch(t, result[0].Tags, tags)
	})

	t.Run("DifferentPrioritySameCollector", func(t *testing.T) {
		allMetrics := map[CollectorName][]Metric{
			sampling: {
				{Name: "memory.limit", Priority: Medium, Tags: []string{"pid:1001"}},
				{Name: "memory.limit", Priority: Low, Tags: []string{""}},
			},
		}
		result := RemoveDuplicateMetrics(allMetrics)
		require.Len(t, result, 1)
		require.ElementsMatch(t, result[0].Tags, []string{"pid:1001"})
	})
}
