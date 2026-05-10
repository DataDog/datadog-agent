// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package enhancedmetrics provides enhanced metrics collection
package enhancedmetrics

import (
	"errors"
	"math"
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
		usageMetricSuffix:  "instance",
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
	collectionTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
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

	serverlessContainerStats := c.convertToServerlessContainerStats(stats, collectionTime)

	assert.Equal(t, uint64(5e8), *serverlessContainerStats.CPU.Total)
	assert.Equal(t, 1e9, *serverlessContainerStats.CPU.Limit)
}

func TestCollectorConvertToServerlessContainerStatsNilCPU(t *testing.T) {
	collectionTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mockReader := &mockCgroupReader{cgroup: &cgroups.MockCgroup{}, version: 1}
	c := &Collector{cgroupReader: mockReader}

	// Simulate partial cgroup failure: CPU controller failed, Memory succeeded
	stats := &cgroups.Stats{
		CPU:    nil,
		Memory: &cgroups.MemoryStats{},
	}

	serverlessContainerStats := c.convertToServerlessContainerStats(stats, collectionTime)

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
			TotalCPU:       pointer.Ptr(uint64(5e8)),
			CollectionTime: previousTime,
		},
	}

	serverlessEnhancedMetrics := c.computeEnhancedMetrics(&ServerlessContainerStats{
		CollectionTime: currentTime,
		CPU: &ServerlessCPUStats{
			Total: pointer.Ptr(uint64(6e8)),
			Limit: pointer.Ptr(1e9),
		},
	})

	assert.Equal(t, 1e8, serverlessEnhancedMetrics.CPUUsage)
	assert.Equal(t, 1e9, serverlessEnhancedMetrics.CPULimit)
}

func TestCollectorComputeEnhancedMetricsRecoversFromInvalidCPUTotal(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)

	c := &Collector{
		previousRateStats: ServerlessRateStats{
			TotalCPU:       pointer.Ptr(uint64(1e9)),
			CollectionTime: t0,
		},
	}

	// First sample is invalid due to CPU Total decreasing, usage is NaN
	metrics1 := c.computeEnhancedMetrics(&ServerlessContainerStats{
		CollectionTime: t1,
		CPU: &ServerlessCPUStats{
			Total: pointer.Ptr(uint64(1e8)),
			Limit: pointer.Ptr(1e9),
		},
	})
	assert.True(t, math.IsNaN(metrics1.CPUUsage))
	assert.Equal(t, uint64(1e8), *c.previousRateStats.TotalCPU)
	assert.Equal(t, t1, c.previousRateStats.CollectionTime)

	// Second sample is valid, usage is computed based on the previous total from t1
	metrics2 := c.computeEnhancedMetrics(&ServerlessContainerStats{
		CollectionTime: t2,
		CPU: &ServerlessCPUStats{
			Total: pointer.Ptr(uint64(2e8)),
			Limit: pointer.Ptr(1e9),
		},
	})
	assert.Equal(t, float64(1e8), metrics2.CPUUsage)
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

	CPUUsage := calculateCPUUsage(pointer.Ptr(uint64(5e8)), nil, currentTime, previousTime)

	assert.True(t, math.IsNaN(CPUUsage))
}

func TestCalculateCPUUsageCurrentTotalNegativeOne(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := previousTime.Add(1 * time.Second)

	CPUUsage := calculateCPUUsage(nil, pointer.Ptr(uint64(6e8)), currentTime, previousTime)

	assert.True(t, math.IsNaN(CPUUsage))
}

func TestCalculateCPUUsagePreviousTimeZero(t *testing.T) {
	previousTime := time.Time{}
	currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	CPUUsage := calculateCPUUsage(nil, pointer.Ptr(uint64(6e8)), currentTime, previousTime)

	assert.True(t, math.IsNaN(CPUUsage))
}

func TestCalculateCPUUsageCurrentTimeZero(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := time.Time{}

	CPUUsage := calculateCPUUsage(nil, pointer.Ptr(uint64(6e8)), currentTime, previousTime)

	assert.True(t, math.IsNaN(CPUUsage))
}

func TestCalculateCPUUsageValueDiffNegative(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := previousTime.Add(1 * time.Second)

	CPUUsage := calculateCPUUsage(pointer.Ptr(uint64(5e8)), pointer.Ptr(uint64(6e8)), currentTime, previousTime)

	assert.True(t, math.IsNaN(CPUUsage))
}

func TestCalculateCPUUsageValueDiffPositive(t *testing.T) {
	previousTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	currentTime := previousTime.Add(1 * time.Second)

	CPUUsage := calculateCPUUsage(pointer.Ptr(uint64(6e8)), pointer.Ptr(uint64(5e8)), currentTime, previousTime)

	assert.Equal(t, float64(1e8), CPUUsage)
}

func TestCollectorSendsUsageMetricOnCgroupFailure(t *testing.T) {
	mockAgent := new(mockEnhancedMetricSender)
	mockAgent.On("AddEnhancedUsageMetric", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	mockReader := &mockCgroupReader{refreshErr: errors.New("cgroup failure"), version: 1}

	c := &Collector{
		metricAgent:       mockAgent,
		metricSource:      metrics.MetricSourceGoogleCloudRunEnhanced,
		cgroupReader:      mockReader,
		metricPrefix:      "gcp.run.container.enhanced.",
		usageMetricSuffix: "instance",
		previousRateStats: NullServerlessRateStats,
	}

	c.collect()

	mockAgent.AssertCalled(t, "AddEnhancedUsageMetric",
		"gcp.run.container.enhanced.instance", float64(1),
		metrics.MetricSourceGoogleCloudRunEnhanced, mock.Anything, mock.Anything)
	mockAgent.AssertNotCalled(t, "AddEnhancedMetric", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
