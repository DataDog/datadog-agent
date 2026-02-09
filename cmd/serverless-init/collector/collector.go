// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

/*
TODO:

only start for in process, not sidecar OR collect for sidecar and tag appropriately. Some sort of container_type or sidecar tag?

rename to enhanced metrics collector? Or otherwise organize file structure?
look into simplifying cpu limit and fallbacks, is it valid in Cloud Run?
Check in Cloud Run Functions, Cloud Run Jobs, Azure Web Apps

Refactor to move go routine to main.go?

Check for any other odd debug logs
*/

package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

// ServerlessCPUStats stores CPU stats for serverless environments
type ServerlessCPUStats struct {
	Total *float64 // Total CPU usage in nanoseconds
	Limit *float64 // CPU limit in nanocores
}

// ServerlessContainerStats wraps container metrics for serverless environments
// Similar to metrics.ContainerStats but simplified for serverless use cases
type ServerlessContainerStats struct {
	Timestamp time.Time
	CPU       *ServerlessCPUStats
}

// ServerlessRateMetrics holds previous values for rate calculation
// Similar to ContainerRateMetrics in the process agent
type ServerlessRateMetrics struct {
	StatsTimestamp time.Time
	TotalCPU       float64
}

// NullServerlessRateMetrics can be safely used when there are no previous rate values
var NullServerlessRateMetrics = ServerlessRateMetrics{
	TotalCPU: -1,
}

type Collector struct {
	metricAgent        *serverlessMetrics.ServerlessMetricAgent
	metricSource       metrics.MetricSource
	cgroupReader       *cgroups.Reader
	collectionInterval time.Duration
	cancelFunc         context.CancelFunc
	metricPrefix       string
	// Previous stats for rate calculation
	previousRateMetrics ServerlessRateMetrics
}

func NewCollector(metricAgent *serverlessMetrics.ServerlessMetricAgent, metricSource metrics.MetricSource, metricPrefix string) (*Collector, error) {
	if metricAgent == nil {
		return nil, fmt.Errorf("metricAgent cannot be nil")
	}

	cgroupReader, err := cgroups.NewSelfReader("/proc", true)
	if err != nil {
		return nil, err
	}

	return &Collector{
		metricAgent:  metricAgent,
		metricSource: metricSource,
		cgroupReader: cgroupReader,
		metricPrefix: metricPrefix + ".enhanced.test.",
	}, nil
}

func (c *Collector) Start(ctx context.Context) {
	collectorCtx, cancel := context.WithCancel(ctx)
	c.cancelFunc = cancel

	go c.collectLoop(collectorCtx)
	log.Info("Enhanced metrics collector started")
}

func (c *Collector) Stop() {
	if c.cancelFunc != nil {
		c.cancelFunc()
		log.Info("Enhanced metrics collector stopped")
	}
}

func (c *Collector) collectLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

func (c *Collector) collect() {
	if err := c.cgroupReader.RefreshCgroups(0); err != nil {
		log.Debugf("Failed to refresh cgroups: %v", err)
		return
	}

	cgroup := c.cgroupReader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if cgroup == nil {
		log.Debug("Failed to get self cgroup")
		return
	}

	stats := &cgroups.Stats{}
	allFailed, errs := cgroups.GetStats(cgroup, stats)
	if allFailed {
		log.Debugf("Failed to get cgroup stats: %v", errs)
		return
	} else if len(errs) > 0 {
		log.Debugf("Incomplete cgroup stats: %v", errs)
	}

	// Capture timestamp right after collecting stats to accurately reflect when data was collected
	collectionTime := time.Now()
	containerStats := c.convertToServerlessContainerStats(stats.CPU, collectionTime)
	c.processCPUStats(containerStats)
}

func (c *Collector) convertToServerlessContainerStats(cpuStats *cgroups.CPUStats, timestamp time.Time) *ServerlessContainerStats {
	if cpuStats == nil {
		return nil
	}

	serverlessStats := &ServerlessContainerStats{
		Timestamp: timestamp,
		CPU:       &ServerlessCPUStats{},
	}

	if cpuStats.Total != nil {
		serverlessStats.CPU.Total = pointer.Ptr(float64(*cpuStats.Total))
	}

	serverlessStats.CPU.Limit = computeCPULimit(cpuStats)

	return serverlessStats
}

func computeCPULimit(cgs *cgroups.CPUStats) *float64 {
	// Limit is computed using min(CPUSet, CFS CPU Quota)
	var limit *float64

	if cgs.CPUCount != nil && *cgs.CPUCount != uint64(systemutils.HostCPUCount()) {
		limit = pointer.Ptr(float64(*cgs.CPUCount))
		log.Debugf("CPU limit from CPUSet: %.0f cores", *limit)
	}

	if cgs.SchedulerQuota != nil && cgs.SchedulerPeriod != nil {
		quotaLimit := (float64(*cgs.SchedulerQuota) / float64(*cgs.SchedulerPeriod))
		log.Debugf("CPU limit from CFS quota: %.0f cores (quota=%d, period=%d)", quotaLimit, *cgs.SchedulerQuota, *cgs.SchedulerPeriod)
		if limit == nil || quotaLimit < *limit {
			limit = &quotaLimit
		}
	}

	if limit == nil {
		limit = pointer.Ptr(float64(systemutils.HostCPUCount()))
		log.Debugf("CPU limit from systemutils.HostCPUCount: %d cores", systemutils.HostCPUCount())
	}

	log.Debugf("CPU limit: %.0f cores", *limit)

	// Convert CPU limit from cores to nanocores
	limitNanos := *limit * 1e9
	return &limitNanos
}

func (c *Collector) processCPUStats(containerStats *ServerlessContainerStats) {
	if containerStats == nil || containerStats.CPU == nil {
		log.Debug("Container stats or CPU stats are nil, skipping")
		return
	}

	c.sendCPUMetrics(containerStats)
}

func (c *Collector) sendCPUMetrics(containerStats *ServerlessContainerStats) {
	if containerStats.CPU.Total != nil {
		currentTotal := *containerStats.CPU.Total

		// Calculate CPU rate (nanocores per second) similar to process agent
		cpuRate := c.calculateCPURate(currentTotal, containerStats.Timestamp, c.previousRateMetrics)

		if cpuRate >= 0 {
			// Submit as distribution metric
			c.metricAgent.AddMetric(c.metricPrefix+"cpu.usage", cpuRate, c.metricSource, metrics.DistributionType)
		}

		// Store current values for next calculation
		c.previousRateMetrics = ServerlessRateMetrics{
			StatsTimestamp: containerStats.Timestamp,
			TotalCPU:       currentTotal,
		}
	}

	if containerStats.CPU.Limit != nil {
		// CPU limit in nanocores - also submit as distribution
		c.metricAgent.AddMetric(c.metricPrefix+"cpu.limit", *containerStats.CPU.Limit, c.metricSource, metrics.DistributionType)
	}
}

// calculateCPURate calculates the CPU usage rate in nanocores per second
// Similar to cpuRateValue in the process agent
// Returns -1 if rate cannot be calculated (first run or invalid data)
func (c *Collector) calculateCPURate(currentTotal float64, currentTime time.Time, previous ServerlessRateMetrics) float64 {
	// First run - no previous value
	if previous.StatsTimestamp.IsZero() {
		return -1
	}

	// Check for invalid previous values (similar to process agent's -1 check)
	if previous.TotalCPU == -1 {
		return -1
	}

	timeDiff := currentTime.Sub(previous.StatsTimestamp).Seconds()
	if timeDiff <= 0 {
		return -1
	}

	valueDiff := currentTotal - previous.TotalCPU
	// Handle counter reset or negative diff
	if valueDiff < 0 {
		return -1
	}

	// Calculate rate: (current - previous) / time_diff
	// Result is in nanocores per second
	return valueDiff / timeDiff
}
