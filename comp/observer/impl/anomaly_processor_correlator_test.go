// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorrelator_SingleAnomalyNoReport(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Title:       "High retransmits",
		Description: "Network retransmits exceeded threshold",
	})

	reports := correlator.Flush()
	assert.Empty(t, reports, "single anomaly should not produce a report")
}

func TestCorrelator_TwoAnomaliesSameSignalNoReport(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Add two anomalies from the same signal
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Title:       "High retransmits 1",
		Description: "First occurrence",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Title:       "High retransmits 2",
		Description: "Second occurrence",
	})

	reports := correlator.Flush()
	assert.Empty(t, reports, "two anomalies from same signal should not produce a report")
}

func TestCorrelator_ThreeRequiredSignalsProduceReport(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Add anomalies from all three required signals for kernel bottleneck pattern
	// Note: Source names now include aggregation suffix (avg for value, count for frequency)
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Title:       "High retransmits",
		Description: "Network retransmits exceeded threshold",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "ebpf.lock_contention_ns:avg",
		Title:       "Lock contention",
		Description: "High lock contention detected",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "connection.errors:count",
		Title:       "Connection errors",
		Description: "Connection error rate elevated",
	})

	// Flush updates internal state
	reports := correlator.Flush()
	assert.Empty(t, reports, "Flush should return empty slice in stateful model")

	// Active correlations should include all matching patterns
	// With 3 signals present, all 3 patterns match (kernel_bottleneck, network_degradation, lock_contention_cascade)
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3, "three signals should match all three patterns")

	// Verify kernel_bottleneck pattern is present
	found := findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found, "kernel_bottleneck pattern should be active")
	assert.Equal(t, "Correlated: Kernel network bottleneck", found.Title)
}

// findCorrelation finds a correlation by pattern name in a slice.
func findCorrelation(correlations []observer.ActiveCorrelation, pattern string) *observer.ActiveCorrelation {
	for i := range correlations {
		if correlations[i].Pattern == pattern {
			return &correlations[i]
		}
	}
	return nil
}

func TestCorrelator_OldAnomaliesEvicted(t *testing.T) {
	config := CorrelatorConfig{
		WindowSeconds: 30,
	}
	correlator := NewCorrelator(config)

	// Add first anomaly at data time 1000
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Title:       "High retransmits",
		Description: "Network retransmits exceeded threshold",
		Timestamp:   1000,
	})

	// Add remaining anomalies at data time 1031 (31 seconds later, beyond window)
	correlator.Process(observer.AnomalyOutput{
		Source:      "ebpf.lock_contention_ns:avg",
		Title:       "Lock contention",
		Description: "High lock contention detected",
		Timestamp:   1031,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "connection.errors:count",
		Title:       "Connection errors",
		Description: "Connection error rate elevated",
		Timestamp:   1031,
	})

	// First anomaly should be evicted (1000 < 1031 - 30 = 1001), so pattern should not match
	reports := correlator.Flush()
	assert.Empty(t, reports, "pattern should not match when first anomaly is evicted")

	// Verify buffer only contains the two recent anomalies
	assert.Len(t, correlator.GetBuffer(), 2)
}

func TestCorrelator_ActiveCorrelationListsAllSignals(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors:count"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3, "all three patterns should match")

	// Find the kernel_bottleneck pattern
	found := findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found)

	// Signals should contain all three signals (sorted alphabetically)
	signals := found.Signals
	require.Len(t, signals, 3)
	assert.Contains(t, signals, "network.retransmits:avg")
	assert.Contains(t, signals, "ebpf.lock_contention_ns:avg")
	assert.Contains(t, signals, "connection.errors:count")
}

func TestCorrelator_ActiveCorrelationContainsPatternName(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors:count"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3, "all three patterns should match")

	// Verify all patterns are present
	assert.NotNil(t, findCorrelation(activeCorrs, "kernel_bottleneck"))
	assert.NotNil(t, findCorrelation(activeCorrs, "network_degradation"))
	assert.NotNil(t, findCorrelation(activeCorrs, "lock_contention_cascade"))
}

func TestCorrelator_BufferNotClearedAfterFlush(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors:count"})

	// First flush should create active correlations
	correlator.Flush()
	activeCorrs1 := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs1, 3)

	// Second flush should maintain active correlations (buffer not cleared)
	correlator.Flush()
	activeCorrs2 := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs2, 3)

	// Buffer should still have all entries
	assert.Len(t, correlator.GetBuffer(), 3)
}

func TestCorrelator_Name(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())
	assert.Equal(t, "cross_signal_correlator", correlator.Name())
}

func TestCorrelator_DefaultConfig(t *testing.T) {
	config := DefaultCorrelatorConfig()
	assert.Equal(t, int64(30), config.WindowSeconds)
}

func TestCorrelator_PartialPatternNoActiveCorrelation(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Only two of three required signals
	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns:avg"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	assert.Empty(t, activeCorrs, "partial pattern should not produce active correlation")
}

func TestCorrelator_ExtraSignalsStillMatch(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// All required signals plus an extra one
	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns:avg"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors:count"})
	correlator.Process(observer.AnomalyOutput{Source: "extra.signal:avg"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3, "all patterns should match even with extra signals")

	// Verify kernel_bottleneck is among the matches
	assert.NotNil(t, findCorrelation(activeCorrs, "kernel_bottleneck"))
}

func TestCorrelator_EmptyBufferNoActiveCorrelation(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	assert.Empty(t, activeCorrs, "empty buffer should have no active correlations")
}

func TestCorrelator_ActiveCorrelationTimestamps(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Add all required signals at data time 1000
	correlator.Process(observer.AnomalyOutput{
		Source:    "network.retransmits:avg",
		Timestamp: 1000,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:    "ebpf.lock_contention_ns:avg",
		Timestamp: 1000,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:    "connection.errors:count",
		Timestamp: 1000,
	})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3)

	// Check kernel_bottleneck timestamps
	found := findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found)

	// FirstSeen and LastUpdated should be data time 1000
	assert.Equal(t, int64(1000), found.FirstSeen)
	assert.Equal(t, int64(1000), found.LastUpdated)

	// Add new anomalies at data time 1010
	correlator.Process(observer.AnomalyOutput{
		Source:    "network.retransmits:avg",
		Timestamp: 1010,
	})

	correlator.Flush()
	activeCorrs = correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3)

	// Find it again
	found = findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found)

	// FirstSeen should stay the same, LastUpdated should update to latest data time
	assert.Equal(t, int64(1000), found.FirstSeen)
	assert.Equal(t, int64(1010), found.LastUpdated)
}

func TestCorrelator_ActiveCorrelationCleared(t *testing.T) {
	config := CorrelatorConfig{
		WindowSeconds: 30,
	}
	correlator := NewCorrelator(config)

	// Add all required signals at data time 1000
	correlator.Process(observer.AnomalyOutput{
		Source:    "network.retransmits:avg",
		Timestamp: 1000,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:    "ebpf.lock_contention_ns:avg",
		Timestamp: 1000,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:    "connection.errors:count",
		Timestamp: 1000,
	})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3, "all patterns should be active")

	// Add an anomaly at data time 1035 (35 seconds later), which advances currentDataTime
	// and causes all previous signals to expire (1000 < 1035 - 30 = 1005)
	correlator.Process(observer.AnomalyOutput{
		Source:    "some.other.signal:avg",
		Timestamp: 1035,
	})
	correlator.Flush()
	activeCorrs = correlator.ActiveCorrelations()
	assert.Empty(t, activeCorrs, "all patterns should be cleared when signals expire")
}

func TestCorrelator_ActiveCorrelationContainsAnomalies(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Add anomalies with descriptions for all required signals
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Title:       "High retransmits",
		Description: "network.retransmits:avg elevated: recent avg 100 vs baseline 10 (>3 stddev)",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "ebpf.lock_contention_ns:avg",
		Title:       "Lock contention",
		Description: "ebpf.lock_contention_ns:avg elevated: recent avg 500 vs baseline 50 (>3 stddev)",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "connection.errors:count",
		Title:       "Connection errors",
		Description: "connection.errors:count elevated: recent avg 25 vs baseline 2 (>3 stddev)",
	})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3)

	// Check kernel_bottleneck correlation
	found := findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found)

	// Anomalies field should contain all three anomalies
	anomalies := found.Anomalies
	require.Len(t, anomalies, 3, "should have 3 anomalies")

	// Verify each anomaly has the expected description
	descriptions := make(map[string]bool)
	for _, a := range anomalies {
		descriptions[a.Description] = true
	}
	assert.True(t, descriptions["network.retransmits:avg elevated: recent avg 100 vs baseline 10 (>3 stddev)"])
	assert.True(t, descriptions["ebpf.lock_contention_ns:avg elevated: recent avg 500 vs baseline 50 (>3 stddev)"])
	assert.True(t, descriptions["connection.errors:count elevated: recent avg 25 vs baseline 2 (>3 stddev)"])
}

func TestCorrelator_AnomaliesUpdatedOnFlush(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Add initial anomalies - all within 30 second window
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Description: "first retransmits anomaly",
		Timestamp:   1010,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "ebpf.lock_contention_ns:avg",
		Description: "first lock contention anomaly",
		Timestamp:   1010,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "connection.errors:count",
		Description: "first connection errors anomaly",
		Timestamp:   1010,
	})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3)

	found := findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found)
	require.Len(t, found.Anomalies, 3)

	// Add another anomaly for one of the signals with later timestamp (still within 30s window)
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Description: "second retransmits anomaly",
		Timestamp:   1020, // later but within window
	})

	correlator.Flush()
	activeCorrs = correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3)

	found = findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found)
	// Should still have 3 anomalies (deduped by source, keeping most recent)
	assert.Len(t, found.Anomalies, 3, "should dedupe anomalies by source")
}

func TestCorrelator_DedupesBySourceKeepingMostRecent(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Add multiple anomalies for the same source with different timestamps
	// All timestamps within the 30-second window
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Description: "oldest retransmits",
		Timestamp:   1010,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Description: "newest retransmits", // should be kept
		Timestamp:   1025, // latest End
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Description: "middle retransmits",
		Timestamp:   1015,
	})

	// Add other required signals - all within window
	correlator.Process(observer.AnomalyOutput{
		Source:      "ebpf.lock_contention_ns:avg",
		Description: "lock contention",
		Timestamp:   1010,
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "connection.errors:count",
		Description: "connection errors",
		Timestamp:   1010,
	})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3)

	found := findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found)

	// Should have exactly 3 anomalies (one per source)
	anomalies := found.Anomalies
	assert.Len(t, anomalies, 3, "should have one anomaly per source")

	// Find the network.retransmits anomaly and verify it's the newest one
	for _, a := range anomalies {
		if a.Source == "network.retransmits:avg" {
			assert.Equal(t, "newest retransmits", a.Description, "should keep anomaly with latest timestamp")
			assert.Equal(t, int64(1025), a.Timestamp, "should have latest timestamp")
		}
	}
}

func TestCorrelator_AnomaliesOnlyIncludesMatchingSignals(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Add all required signals plus an extra one
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits:avg",
		Description: "retransmits anomaly",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "ebpf.lock_contention_ns:avg",
		Description: "lock contention anomaly",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "connection.errors:count",
		Description: "connection errors anomaly",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "extra.signal:avg",
		Description: "extra signal anomaly",
	})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 3)

	// Check kernel_bottleneck pattern
	found := findCorrelation(activeCorrs, "kernel_bottleneck")
	require.NotNil(t, found)

	// Anomalies should only contain the 3 matching signals, not the extra one
	anomalies := found.Anomalies
	assert.Len(t, anomalies, 3, "should only include anomalies matching pattern's required sources")

	// Verify extra signal is not included
	for _, a := range anomalies {
		assert.NotEqual(t, "extra.signal:avg", a.Source, "extra signal should not be in anomalies")
	}
}
