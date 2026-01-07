// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorrelator_SingleAnomalyNoReport(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits",
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
		Source:      "network.retransmits",
		Title:       "High retransmits 1",
		Description: "First occurrence",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits",
		Title:       "High retransmits 2",
		Description: "Second occurrence",
	})

	reports := correlator.Flush()
	assert.Empty(t, reports, "two anomalies from same signal should not produce a report")
}

func TestCorrelator_ThreeRequiredSignalsProduceReport(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Add anomalies from all three required signals for kernel bottleneck pattern
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits",
		Title:       "High retransmits",
		Description: "Network retransmits exceeded threshold",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "ebpf.lock_contention_ns",
		Title:       "Lock contention",
		Description: "High lock contention detected",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "connection.errors",
		Title:       "Connection errors",
		Description: "Connection error rate elevated",
	})

	// Flush updates internal state
	reports := correlator.Flush()
	assert.Empty(t, reports, "Flush should return empty slice in stateful model")

	// Active correlations should reflect the matched pattern
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 1, "three matching signals should produce one active correlation")
	assert.Equal(t, "Correlated: Kernel network bottleneck", activeCorrs[0].Title)
	assert.Equal(t, "kernel_bottleneck", activeCorrs[0].Pattern)
}

func TestCorrelator_OldAnomaliesEvicted(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	currentTime := baseTime

	config := CorrelatorConfig{
		WindowDuration: 30 * time.Second,
		Now:            func() time.Time { return currentTime },
	}
	correlator := NewCorrelator(config)

	// Add first anomaly at base time
	correlator.Process(observer.AnomalyOutput{
		Source:      "network.retransmits",
		Title:       "High retransmits",
		Description: "Network retransmits exceeded threshold",
	})

	// Advance time by 31 seconds (beyond window)
	currentTime = baseTime.Add(31 * time.Second)

	// Add remaining anomalies
	correlator.Process(observer.AnomalyOutput{
		Source:      "ebpf.lock_contention_ns",
		Title:       "Lock contention",
		Description: "High lock contention detected",
	})
	correlator.Process(observer.AnomalyOutput{
		Source:      "connection.errors",
		Title:       "Connection errors",
		Description: "Connection error rate elevated",
	})

	// First anomaly should be evicted, so pattern should not match
	reports := correlator.Flush()
	assert.Empty(t, reports, "pattern should not match when first anomaly is evicted")

	// Verify buffer only contains the two recent anomalies
	assert.Len(t, correlator.GetBuffer(), 2)
}

func TestCorrelator_ActiveCorrelationListsAllSignals(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 1)

	// Signals should contain all three signals (sorted alphabetically)
	signals := activeCorrs[0].Signals
	require.Len(t, signals, 3)
	assert.Contains(t, signals, "network.retransmits")
	assert.Contains(t, signals, "ebpf.lock_contention_ns")
	assert.Contains(t, signals, "connection.errors")
}

func TestCorrelator_ActiveCorrelationContainsPatternName(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 1)

	assert.Equal(t, "kernel_bottleneck", activeCorrs[0].Pattern)
}

func TestCorrelator_BufferNotClearedAfterFlush(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})

	// First flush should create active correlation
	correlator.Flush()
	activeCorrs1 := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs1, 1)

	// Second flush should maintain active correlation (buffer not cleared)
	correlator.Flush()
	activeCorrs2 := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs2, 1)

	// Buffer should still have all entries
	assert.Len(t, correlator.GetBuffer(), 3)
}

func TestCorrelator_Name(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())
	assert.Equal(t, "cross_signal_correlator", correlator.Name())
}

func TestCorrelator_DefaultConfig(t *testing.T) {
	config := DefaultCorrelatorConfig()
	assert.Equal(t, 30*time.Second, config.WindowDuration)
	assert.NotNil(t, config.Now)
}

func TestCorrelator_PartialPatternNoActiveCorrelation(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Only two of three required signals
	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	assert.Empty(t, activeCorrs, "partial pattern should not produce active correlation")
}

func TestCorrelator_ExtraSignalsStillMatch(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// All required signals plus an extra one
	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})
	correlator.Process(observer.AnomalyOutput{Source: "extra.signal"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 1, "pattern should match even with extra signals")
}

func TestCorrelator_EmptyBufferNoActiveCorrelation(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	assert.Empty(t, activeCorrs, "empty buffer should have no active correlations")
}

func TestCorrelator_ActiveCorrelationTimestamps(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	currentTime := baseTime

	config := CorrelatorConfig{
		WindowDuration: 30 * time.Second,
		Now:            func() time.Time { return currentTime },
	}
	correlator := NewCorrelator(config)

	// Add all required signals at base time
	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 1)

	// FirstSeen and LastUpdated should be base time
	assert.Equal(t, baseTime, activeCorrs[0].FirstSeen)
	assert.Equal(t, baseTime, activeCorrs[0].LastUpdated)

	// Advance time and flush again
	currentTime = baseTime.Add(10 * time.Second)
	correlator.Flush()
	activeCorrs = correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 1)

	// FirstSeen should stay the same, LastUpdated should update
	assert.Equal(t, baseTime, activeCorrs[0].FirstSeen)
	assert.Equal(t, currentTime, activeCorrs[0].LastUpdated)
}

func TestCorrelator_ActiveCorrelationCleared(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	currentTime := baseTime

	config := CorrelatorConfig{
		WindowDuration: 30 * time.Second,
		Now:            func() time.Time { return currentTime },
	}
	correlator := NewCorrelator(config)

	// Add all required signals
	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})

	correlator.Flush()
	activeCorrs := correlator.ActiveCorrelations()
	require.Len(t, activeCorrs, 1, "pattern should be active")

	// Advance time beyond window so all signals expire
	currentTime = baseTime.Add(35 * time.Second)
	correlator.Flush()
	activeCorrs = correlator.ActiveCorrelations()
	assert.Empty(t, activeCorrs, "pattern should be cleared when signals expire")
}
