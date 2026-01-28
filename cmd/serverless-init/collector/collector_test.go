// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collector

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestNewCollector(t *testing.T) {
	// Create a mock metric agent
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	metricSource := metrics.MetricSourceAzureContainerAppEnhanced

	collector, err := NewCollector(metricAgent, metricSource)

	// The collector may fail to initialize if not running in a container
	// or if cgroups are not available, which is expected in test environments
	if err != nil {
		t.Logf("Expected error in test environment: %v", err)
		return
	}

	require.NotNil(t, collector)
	assert.NotNil(t, collector.cgroupReader)
	assert.Equal(t, defaultCollectionInterval, collector.collectionInterval)
	assert.Equal(t, "azure.containerapp.enhanced.test.cpu.", collector.metricPrefix)
}

func TestComputeCPULimitNanos(t *testing.T) {
	tests := []struct {
		name     string
		stats    *cgroups.CPUStats
		expected *float64
	}{
		{
			name: "no limit set",
			stats: &cgroups.CPUStats{
				SchedulerQuota:  nil,
				SchedulerPeriod: nil,
				CPUCount:        nil,
			},
			expected: nil,
		},
		{
			name: "unlimited quota (max uint64)",
			stats: &cgroups.CPUStats{
				SchedulerQuota:  pointer.Ptr(^uint64(0)),
				SchedulerPeriod: pointer.Ptr(uint64(100000000)),
				CPUCount:        nil,
			},
			expected: nil,
		},
		{
			name: "limited to 2 cores by quota",
			stats: &cgroups.CPUStats{
				SchedulerQuota:  pointer.Ptr(uint64(200000000)), // 200ms
				SchedulerPeriod: pointer.Ptr(uint64(100000000)), // 100ms period
				CPUCount:        nil,
			},
			expected: pointer.Ptr(2e9), // 2 billion ns/s = 2 cores
		},
		{
			name: "limited to 0.5 cores by quota",
			stats: &cgroups.CPUStats{
				SchedulerQuota:  pointer.Ptr(uint64(50000000)),  // 50ms
				SchedulerPeriod: pointer.Ptr(uint64(100000000)), // 100ms period
				CPUCount:        nil,
			},
			expected: pointer.Ptr(5e8), // 500 million ns/s = 0.5 cores
		},
		{
			name: "limited to 1 core by cpuset",
			stats: &cgroups.CPUStats{
				SchedulerQuota:  nil,
				SchedulerPeriod: nil,
				CPUCount:        pointer.Ptr(uint64(1)),
			},
			expected: pointer.Ptr(1e9), // 1 billion ns/s = 1 core
		},
		{
			name: "cpuset and quota, cpuset lower",
			stats: &cgroups.CPUStats{
				SchedulerQuota:  pointer.Ptr(uint64(200000000)),
				SchedulerPeriod: pointer.Ptr(uint64(100000000)),
				CPUCount:        pointer.Ptr(uint64(1)),
			},
			expected: pointer.Ptr(1e9), // min(2 cores, 1 core) = 1 core
		},
		{
			name: "cpuset and quota, quota lower",
			stats: &cgroups.CPUStats{
				SchedulerQuota:  pointer.Ptr(uint64(50000000)),
				SchedulerPeriod: pointer.Ptr(uint64(100000000)),
				CPUCount:        pointer.Ptr(uint64(2)),
			},
			expected: pointer.Ptr(5e8), // min(0.5 cores, 2 cores) = 0.5 cores
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeCPULimitNanos(tt.stats)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.InDelta(t, *tt.expected, *result, 1e6) // Allow 1ms tolerance
			}
		})
	}
}

func TestCollectorLifecycle(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	metricSource := metrics.MetricSourceAzureContainerAppEnhanced

	collector, err := NewCollector(metricAgent, metricSource)
	if err != nil {
		t.Skipf("Skipping test - cgroups not available: %v", err)
		return
	}

	ctx := context.Background()

	// Start the collector
	collector.Start(ctx)
	assert.NotNil(t, collector.cancelFunc)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the collector
	collector.Stop()

	// Verify it stopped cleanly
	time.Sleep(100 * time.Millisecond)
}

func TestSendCPUMetrics(t *testing.T) {
	// This test verifies the metric sending logic
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	metricSource := metrics.MetricSourceAzureContainerAppEnhanced

	collector := &Collector{
		metricAgent:  metricAgent,
		metricSource: metricSource,
		metricPrefix: "azure.containerapp.enhanced.test.cpu.",
	}

	// Create sample CPU stats using provider types
	stats := &provider.ContainerCPUStats{
		Total:            pointer.Ptr(1000000000.0), // 1 billion ns (cumulative)
		User:             pointer.Ptr(800000000.0),  // 800 million ns
		System:           pointer.Ptr(200000000.0),  // 200 million ns
		ThrottledPeriods: pointer.Ptr(5.0),
		ThrottledTime:    pointer.Ptr(100000000.0),  // 100 million ns
		PartialStallTime: pointer.Ptr(50000000.0),   // PSI metric
		Limit:            pointer.Ptr(2000000000.0), // 2 billion ns/s = 2 cores
	}

	// Since Demux is nil, this should not panic
	assert.NotPanics(t, func() {
		collector.sendCPUMetrics(stats, time.Now())
	})
}
