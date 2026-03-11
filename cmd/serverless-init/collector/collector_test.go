// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package collector provides enhanced metrics collection
package collector

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockCgroupReader struct {
	refreshErr error
	cgroup     cgroups.Cgroup
	version    int
}

func (m *mockCgroupReader) RefreshCgroups(time.Duration) error { return m.refreshErr }
func (m *mockCgroupReader) GetCgroup(string) cgroups.Cgroup    { return m.cgroup }
func (m *mockCgroupReader) CgroupVersion() int                 { return m.version }

type mockEnhancedMetricSender struct {
	mock.Mock
}

func (m *mockEnhancedMetricSender) AddEnhancedMetric(name string, value float64, source metrics.MetricSource, ts float64, tags ...string) {
	m.Called(name, value, source, ts, tags)
}

func (m *mockEnhancedMetricSender) AddEnhancedUsageMetric(name string, value float64, source metrics.MetricSource, ts float64, tags ...string) {
	m.Called(name, value, source, ts, tags)
}

func TestCollectorSendsMetricsOnStartIntervalAndStop(t *testing.T) {
	mockAgent := new(mockEnhancedMetricSender)
	mockAgent.On("AddEnhancedMetric", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockAgent.On("AddEnhancedUsageMetric", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	mockReader := &mockCgroupReader{cgroup: &cgroups.MockCgroup{}, version: 1}

	c := &Collector{
		metricAgent:        mockAgent,
		cgroupReader:       mockReader,
		usageMetricName:    "instance",
		collectionInterval: 100 * time.Millisecond,
		cancelFunc:         func() {},
	}

	go c.Start()

	// collect on start
	time.Sleep(50 * time.Millisecond)
	mockAgent.AssertNumberOfCalls(t, "AddEnhancedUsageMetric", 1)

	// collect on interval
	time.Sleep(100 * time.Millisecond)
	mockAgent.AssertNumberOfCalls(t, "AddEnhancedUsageMetric", 2)

	// collect on stop
	c.Stop()
	mockAgent.AssertNumberOfCalls(t, "AddEnhancedUsageMetric", 3)
}

func TestCollectorConvertToServerlessContainerStats(t *testing.T) {
	mockReader := &mockCgroupReader{cgroup: &cgroups.MockCgroup{}, version: 1}
	c := &Collector{
		cgroupReader: mockReader,
	}

	stats := &cgroups.Stats{
		CPU: &cgroups.CPUStats{
			Total:    pointer.Ptr(uint64(5e8)),
			CPUCount: pointer.Ptr(uint64(1)),
		},
	}

	serverlessContainerStats := c.convertToServerlessContainerStats(stats)

	assert.Equal(t, 5e8, *serverlessContainerStats.CPU.Total)
	assert.Equal(t, 1e9, *serverlessContainerStats.CPU.Limit)
}

func TestCollectorConvertToServerlessContainerStatsNilCPU(t *testing.T) {
	mockReader := &mockCgroupReader{cgroup: &cgroups.MockCgroup{}, version: 1}
	c := &Collector{cgroupReader: mockReader}

	// Simulate partial cgroup failure: CPU controller failed, Memory succeeded
	stats := &cgroups.Stats{
		CPU:    nil,
		Memory: &cgroups.MemoryStats{},
	}

	serverlessContainerStats := c.convertToServerlessContainerStats(stats)

	assert.NotNil(t, serverlessContainerStats)
	assert.Nil(t, serverlessContainerStats.CPU)
}

func TestComputeCPULimitNilStats(t *testing.T) {
	limit := computeCPULimit(nil, 4)
	assert.Nil(t, limit)
}

func TestCollectorComputeEnhancedMetricsNilStats(t *testing.T) {
	c := &Collector{}

	serverlessEnhancedMetrics := c.computeEnhancedMetrics(nil)

	assert.Equal(t, &ServerlessEnhancedMetrics{}, &serverlessEnhancedMetrics)
}

func TestCollectorComputeEnhancedMetricsNilCPUStats(t *testing.T) {
	currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c := &Collector{}

	serverlessEnhancedMetrics := c.computeEnhancedMetrics(&ServerlessContainerStats{
		CollectionTime: currentTime,
	})

	assert.Equal(t, &ServerlessEnhancedMetrics{Timestamp: float64(currentTime.Unix())}, &serverlessEnhancedMetrics)
}

func TestCollectorComputeEnhancedMetrics(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := previousTime.Add(1 * time.Second)

	c := &Collector{
		previousRateStats: ServerlessRateStats{
			TotalCPU:       5e8,
			CollectionTime: previousTime,
		},
	}

	serverlessEnhancedMetrics := c.computeEnhancedMetrics(&ServerlessContainerStats{
		CollectionTime: currentTime,
		CPU: &ServerlessCPUStats{
			Total: pointer.Ptr(6e8),
			Limit: pointer.Ptr(1e9),
		},
	})

	assert.Equal(t, 1e8, serverlessEnhancedMetrics.CPUUsage)
	assert.Equal(t, 1e9, serverlessEnhancedMetrics.CPULimit)
}

func TestCPULimitSchedulerQuotaAndPeriod(t *testing.T) {
	stats := &cgroups.Stats{
		CPU: &cgroups.CPUStats{
			SchedulerQuota:  pointer.Ptr(uint64(50000)),
			SchedulerPeriod: pointer.Ptr(uint64(100000)),
			CPUCount:        pointer.Ptr(uint64(1)),
		},
	}

	limit := computeCPULimit(stats.CPU, 4)

	assert.Equal(t, 5e8, *limit)
}

func TestCPULimitCPUSet(t *testing.T) {
	stats := &cgroups.Stats{
		CPU: &cgroups.CPUStats{
			SchedulerQuota:  pointer.Ptr(uint64(200000)),
			SchedulerPeriod: pointer.Ptr(uint64(100000)),
			CPUCount:        pointer.Ptr(uint64(1)),
		},
	}

	limit := computeCPULimit(stats.CPU, 4)

	assert.Equal(t, 1e9, *limit)
}

func TestCPULimitHost(t *testing.T) {
	stats := &cgroups.Stats{
		CPU: &cgroups.CPUStats{},
	}

	limit := computeCPULimit(stats.CPU, 4)

	assert.Equal(t, 4e9, *limit)
}

func TestCalculateCPUUsagePreviousTotalNegativeOne(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := previousTime.Add(1 * time.Second)

	CPUUsage := calculateCPUUsage(5e8, -1, currentTime, previousTime)

	assert.Equal(t, float64(-1), CPUUsage)
}

func TestCalculateCPUUsageCurrentTotalNegativeOne(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := previousTime.Add(1 * time.Second)

	CPUUsage := calculateCPUUsage(-1, 6e8, currentTime, previousTime)

	assert.Equal(t, float64(-1), CPUUsage)
}

func TestCalculateCPUUsagePreviousTimeZero(t *testing.T) {
	previousTime := time.Time{}
	currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	CPUUsage := calculateCPUUsage(-1, 6e8, currentTime, previousTime)

	assert.Equal(t, float64(-1), CPUUsage)
}

func TestCalculateCPUUsageCurrentTimeZero(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := time.Time{}

	CPUUsage := calculateCPUUsage(-1, 6e8, currentTime, previousTime)

	assert.Equal(t, float64(-1), CPUUsage)
}

func TestCalculateCPUUsageValueDiffNegative(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := previousTime.Add(1 * time.Second)

	CPUUsage := calculateCPUUsage(5e8, 6e8, currentTime, previousTime)

	assert.Equal(t, float64(-1), CPUUsage)
}

func TestCalculateCPUUsageValueDiffPositive(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := previousTime.Add(1 * time.Second)

	CPUUsage := calculateCPUUsage(6e8, 5e8, currentTime, previousTime)

	assert.Equal(t, float64(1e8), CPUUsage)
}
