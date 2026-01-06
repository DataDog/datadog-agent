// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"
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

	reports := correlator.Flush()
	require.Len(t, reports, 1, "three matching signals should produce one report")
	assert.Equal(t, "Correlated: Kernel network bottleneck", reports[0].Title)
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

func TestCorrelator_ReportBodyListsAllSignals(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})

	reports := correlator.Flush()
	require.Len(t, reports, 1)

	// Body should contain all three signals (sorted alphabetically)
	body := reports[0].Body
	assert.True(t, strings.HasPrefix(body, "Correlated signals: "))
	assert.Contains(t, body, "network.retransmits")
	assert.Contains(t, body, "ebpf.lock_contention_ns")
	assert.Contains(t, body, "connection.errors")
}

func TestCorrelator_ReportMetadataContainsPatternAndCount(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})

	reports := correlator.Flush()
	require.Len(t, reports, 1)

	metadata := reports[0].Metadata
	assert.Equal(t, "kernel_bottleneck", metadata["pattern"])
	assert.Equal(t, "3", metadata["signal_count"])
}

func TestCorrelator_BufferNotClearedAfterFlush(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})

	// First flush should produce report
	reports1 := correlator.Flush()
	require.Len(t, reports1, 1)

	// Second flush should also produce report (buffer not cleared)
	reports2 := correlator.Flush()
	require.Len(t, reports2, 1)

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

func TestCorrelator_PartialPatternNoReport(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// Only two of three required signals
	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})

	reports := correlator.Flush()
	assert.Empty(t, reports, "partial pattern should not produce report")
}

func TestCorrelator_ExtraSignalsStillMatch(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	// All required signals plus an extra one
	correlator.Process(observer.AnomalyOutput{Source: "network.retransmits"})
	correlator.Process(observer.AnomalyOutput{Source: "ebpf.lock_contention_ns"})
	correlator.Process(observer.AnomalyOutput{Source: "connection.errors"})
	correlator.Process(observer.AnomalyOutput{Source: "extra.signal"})

	reports := correlator.Flush()
	require.Len(t, reports, 1, "pattern should match even with extra signals")
}

func TestCorrelator_EmptyBufferNoReport(t *testing.T) {
	correlator := NewCorrelator(DefaultCorrelatorConfig())

	reports := correlator.Flush()
	assert.Nil(t, reports, "empty buffer should return nil")
}
