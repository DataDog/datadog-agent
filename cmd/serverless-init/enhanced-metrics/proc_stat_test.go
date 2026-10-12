// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package enhancedmetrics

import (
	"context"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCPUTimesToNanoseconds(t *testing.T) {
	total, err := cpuTimesToNanoseconds(cpu.TimesStat{
		User:    0.1,
		Nice:    0.2,
		System:  0.3,
		Iowait:  0.4,
		Irq:     0.5,
		Softirq: 0.6,
		Steal:   0.7,
		Guest:   0.8,
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(1.7e9), total)
}

func TestCPUTimesToNanosecondsRejectsInvalidInput(t *testing.T) {
	for _, stats := range []cpu.TimesStat{
		{User: math.NaN()},
		{User: math.Inf(1)},
		{User: -1},
		{User: 2e10},
	} {
		_, err := cpuTimesToNanoseconds(stats)
		assert.Error(t, err)
	}
}

func TestCPUTimesToNanosecondsRounds(t *testing.T) {
	total, err := cpuTimesToNanoseconds(cpu.TimesStat{User: 1.5e-9})

	require.NoError(t, err)
	assert.Equal(t, uint64(2), total)
}

func TestProcStatCPUStatsProviderRead(t *testing.T) {
	provider := &procStatCPUStatsProvider{
		procPath: "/tmp/proc",
		cpuCount: 2,
		getCPUTimes: func(context.Context, bool) ([]cpu.TimesStat, error) {
			return []cpu.TimesStat{{
				User:    0.1,
				Nice:    0.2,
				System:  0.3,
				Irq:     0.6,
				Softirq: 0.7,
			}}, nil
		},
	}

	stats, err := provider.read(time.Unix(100, 0))

	require.NoError(t, err)
	assert.Equal(t, uint64(1.9e9), *stats.CPU.Total)
	assert.Equal(t, 2e9, *stats.CPU.Limit)
}

func TestProcStatCPUStatsProviderReadsConfiguredProcPath(t *testing.T) {
	procPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(procPath, "stat"),
		[]byte("cpu 10 20 30 40 50 60 70 80\n"),
		0o644,
	))

	provider := newProcStatCPUStatsProvider(procPath, 0)
	stats, err := provider.read(time.Unix(100, 0))

	require.NoError(t, err)
	assert.Equal(t, uint64(math.Round(190/cpu.ClocksPerSec*float64(time.Second))), *stats.CPU.Total)
	assert.Nil(t, stats.CPU.Limit)
}

func TestProcStatCPUStatsProviderRejectsInvalidProcStat(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "short aggregate line", data: "cpu 1 2 3\n"},
		{name: "invalid counter", data: "cpu 1 x 3 4 5 6 7\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procPath := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(procPath, "stat"), []byte(tt.data), 0o644))

			provider := newProcStatCPUStatsProvider(procPath, 0)
			_, err := provider.read(time.Unix(100, 0))

			assert.Error(t, err)
		})
	}
}

func TestProcStatCPUStatsProviderReadUnavailable(t *testing.T) {
	provider := &procStatCPUStatsProvider{
		procPath: "/tmp/proc",
		getCPUTimes: func(context.Context, bool) ([]cpu.TimesStat, error) {
			return nil, errors.New("unavailable")
		},
	}

	_, err := provider.read(time.Unix(100, 0))

	assert.Error(t, err)
}

func TestProcStatCPUStatsProviderReadEmpty(t *testing.T) {
	provider := &procStatCPUStatsProvider{
		procPath:    "/tmp/proc",
		getCPUTimes: func(context.Context, bool) ([]cpu.TimesStat, error) { return nil, nil },
	}

	_, err := provider.read(time.Unix(100, 0))

	assert.Error(t, err)
}

func TestMicroVMNoCgroupSelectsProcStatProvider(t *testing.T) {
	cgroupErr := fmt.Errorf("reader startup: %w", cgroups.ErrNoCgroupMount)

	provider, err := newCPUStatsProvider(metrics.MetricSourceAWSMicroVMEnhanced, nil, cgroupErr)

	assert.NoError(t, err)
	assert.IsType(t, &procStatCPUStatsProvider{}, provider)
}
func TestCgroupReaderSelectsCgroupCPUStatsProvider(t *testing.T) {
	reader := &mockCgroupReader{version: 2}

	provider, err := newCPUStatsProvider(metrics.MetricSourceGoogleCloudRunEnhanced, reader, nil)

	assert.NoError(t, err)
	assert.IsType(t, &cgroupCPUStatsProvider{}, provider)
}

func TestNonMicroVMDoesNotFallbackWhenCgroupReaderFails(t *testing.T) {
	cgroupErr := fmt.Errorf("reader startup: %w", cgroups.ErrNoCgroupMount)

	provider, err := newCPUStatsProvider(metrics.MetricSourceGoogleCloudRunEnhanced, nil, cgroupErr)

	assert.Nil(t, provider)
	assert.ErrorIs(t, err, cgroupErr)
}
func TestMicroVMDoesNotFallbackForOtherCgroupErrors(t *testing.T) {
	cgroupErr := errors.New("cgroup reader failed")

	provider, err := newCPUStatsProvider(metrics.MetricSourceAWSMicroVMEnhanced, nil, cgroupErr)

	assert.Nil(t, provider)
	assert.ErrorIs(t, err, cgroupErr)
}

func TestProcStatCPUUsageAfterTwoSamples(t *testing.T) {
	t0 := time.Unix(100, 0)
	t1 := t0.Add(time.Second)
	provider := &procStatCPUStatsProvider{
		procPath: "/tmp/proc",
		cpuCount: 1,
		getCPUTimes: func(context.Context, bool) ([]cpu.TimesStat, error) {
			return []cpu.TimesStat{{
				User:    1.1,
				Nice:    0.2,
				System:  0.3,
				Irq:     0.6,
				Softirq: 0.7,
			}}, nil
		},
	}
	collector := &Collector{}
	first, err := provider.read(t0)
	assert.NoError(t, err)
	collector.computeEnhancedMetrics(first)

	provider.getCPUTimes = func(context.Context, bool) ([]cpu.TimesStat, error) {
		return []cpu.TimesStat{{
			User:    1.2,
			Nice:    0.3,
			System:  0.4,
			Irq:     0.7,
			Softirq: 0.7,
		}}, nil
	}
	second, err := provider.read(t1)
	assert.NoError(t, err)
	enhancedMetrics := collector.computeEnhancedMetrics(second)

	assert.Equal(t, 4e8, enhancedMetrics.CPUUsage)
}

type sequenceCPUStatsProvider struct {
	stats []*ServerlessContainerStats
}

func (p *sequenceCPUStatsProvider) read(time.Time) (*ServerlessContainerStats, error) {
	stats := p.stats[0]
	p.stats = p.stats[1:]
	return stats, nil
}

func TestProcStatCollectorEmitsUsageAfterTwoSamples(t *testing.T) {
	t0 := time.Unix(100, 0)
	metricAgent := new(mockEnhancedMetricSender)
	metricAgent.On("AddEnhancedMetric", "aws.lambda.microvm.enhanced.cpu.limit", 2e9, metrics.MetricSourceAWSMicroVMEnhanced, mock.Anything, []string(nil)).Return().Twice()
	metricAgent.On("AddEnhancedMetric", "aws.lambda.microvm.enhanced.cpu.usage", 1e8, metrics.MetricSourceAWSMicroVMEnhanced, mock.Anything, []string(nil)).Return().Once()
	collector := &Collector{
		metricAgent:  metricAgent,
		metricSource: metrics.MetricSourceAWSMicroVMEnhanced,
		cpuStatsProvider: &sequenceCPUStatsProvider{stats: []*ServerlessContainerStats{
			{CollectionTime: t0, CPU: &ServerlessCPUStats{Total: pointer.Ptr(uint64(1e9)), Limit: pointer.Ptr(2e9)}},
			{CollectionTime: t0.Add(time.Second), CPU: &ServerlessCPUStats{Total: pointer.Ptr(uint64(1.1e9)), Limit: pointer.Ptr(2e9)}},
		}},
		metricPrefix: "aws.lambda.microvm.enhanced.",
	}

	collector.collect()
	collector.collect()

	metricAgent.AssertExpectations(t)
}

func TestProcStatCollectorEmitsLimitOnFirstSample(t *testing.T) {
	metricAgent := new(mockEnhancedMetricSender)
	metricAgent.On("AddEnhancedMetric", "aws.lambda.microvm.enhanced.cpu.limit", 2e9, metrics.MetricSourceAWSMicroVMEnhanced, mock.Anything, []string(nil)).Return()
	provider := &procStatCPUStatsProvider{
		procPath: "/tmp/proc",
		cpuCount: 2,
		getCPUTimes: func(context.Context, bool) ([]cpu.TimesStat, error) {
			return []cpu.TimesStat{{
				User:    0.1,
				Nice:    0.2,
				System:  0.3,
				Irq:     0.6,
				Softirq: 0.7,
			}}, nil
		},
	}
	collector := &Collector{
		metricAgent:      metricAgent,
		metricSource:     metrics.MetricSourceAWSMicroVMEnhanced,
		cpuStatsProvider: provider,
		metricPrefix:     "aws.lambda.microvm.enhanced.",
	}

	collector.collect()

	metricAgent.AssertExpectations(t)
}

func TestProcStatSnapshotResumeSuppressesInvalidRate(t *testing.T) {
	t0 := time.Unix(100, 0)
	collector := &Collector{}
	collector.previousRateStats = ServerlessRateStats{
		TotalCPU:       pointer.Ptr(uint64(2e9)),
		CollectionTime: t0,
	}

	first := collector.computeEnhancedMetrics(&ServerlessContainerStats{
		CollectionTime: t0.Add(10 * time.Second),
		CPU:            &ServerlessCPUStats{Total: pointer.Ptr(uint64(1e9))},
	})
	second := collector.computeEnhancedMetrics(&ServerlessContainerStats{
		CollectionTime: t0.Add(11 * time.Second),
		CPU:            &ServerlessCPUStats{Total: pointer.Ptr(uint64(2e9))},
	})

	assert.True(t, isNaN(first.CPUUsage))
	assert.Equal(t, 1e9, second.CPUUsage)
}

func isNaN(value float64) bool {
	return value != value
}
