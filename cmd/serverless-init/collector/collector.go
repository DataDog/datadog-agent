// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collector

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Collection interval for CPU metrics
	defaultCollectionInterval = 3 * time.Second
)

// Collector collects cgroup metrics (CPU, memory, IO, etc.) for the current process
type Collector struct {
	metricAgent        *serverlessMetrics.ServerlessMetricAgent
	metricSource       metrics.MetricSource
	cgroupReader       *cgroups.Reader
	collectionInterval time.Duration
	cancelFunc         context.CancelFunc
	metricPrefix       string
}

// NewCollector creates a new cgroup metrics collector
func NewCollector(metricAgent *serverlessMetrics.ServerlessMetricAgent, metricSource metrics.MetricSource) (*Collector, error) {
	// Create a self cgroup reader to read stats for the current process
	// This is the correct approach for self-monitoring within a container
	// inContainer=true because serverless-init runs inside a container
	cgroupReader, err := cgroups.NewSelfReader("/proc", true)
	if err != nil {
		return nil, err
	}

	// Determine metric prefix based on metric source
	var metricPrefix string
	switch metricSource {
	case metrics.MetricSourceAzureContainerAppEnhanced:
		metricPrefix = "azure.containerapp.enhanced.test.cpu."
	case metrics.MetricSourceGoogleCloudRunEnhanced:
		metricPrefix = "gcp.run.enhanced.test.cpu."
	default:
		metricPrefix = "serverless.enhanced.test.cpu."
	}

	return &Collector{
		metricAgent:        metricAgent,
		metricSource:       metricSource,
		cgroupReader:       cgroupReader,
		collectionInterval: defaultCollectionInterval,
		metricPrefix:       metricPrefix,
	}, nil
}

// convertToContainerCPUStats converts cgroups.CPUStats to provider.ContainerCPUStats
// This reuses the same logic as the system collector's buildCPUStats
func convertToContainerCPUStats(cpuStats *cgroups.CPUStats) *provider.ContainerCPUStats {
	containerCPUStats := &provider.ContainerCPUStats{}

	if cpuStats.Total != nil {
		containerCPUStats.Total = floatPtr(float64(*cpuStats.Total))
	}
	if cpuStats.User != nil {
		containerCPUStats.User = floatPtr(float64(*cpuStats.User))
	}
	if cpuStats.System != nil {
		containerCPUStats.System = floatPtr(float64(*cpuStats.System))
	}
	if cpuStats.ThrottledPeriods != nil {
		containerCPUStats.ThrottledPeriods = floatPtr(float64(*cpuStats.ThrottledPeriods))
	}
	if cpuStats.ThrottledTime != nil {
		containerCPUStats.ThrottledTime = floatPtr(float64(*cpuStats.ThrottledTime))
	}

	// PSI metrics - convert from microseconds to nanoseconds like system collector does
	if cpuStats.PSISome.Total != nil {
		containerCPUStats.PartialStallTime = floatPtr(float64(*cpuStats.PSISome.Total) * float64(time.Microsecond))
	}

	// CPU limit calculation - no parent cgroup check needed for serverless environments
	// (parent check is only needed for ECS which isn't used here)
	containerCPUStats.Limit = computeCPULimitNanos(cpuStats)

	return containerCPUStats
}

// computeCPULimitNanos computes the CPU limit in nanoseconds/second from cgroup stats
// Returns nanoseconds/second where 1 core = 1,000,000,000 ns/s (displayed as 1000 mcores in UI)
func computeCPULimitNanos(stats *cgroups.CPUStats) *float64 {
	var limitNanos *float64

	// Check CPUSet limit (cpuset.cpus.effective for v2, cpuset.cpus for v1)
	// 1 CPU = 1 billion nanoseconds/second
	if stats.CPUCount != nil && *stats.CPUCount > 0 {
		limitNanos = floatPtr(float64(*stats.CPUCount) * 1e9)
		log.Debugf("CPU limit from CPUSet: %.0f ns/s = %d cores", *limitNanos, *stats.CPUCount)
	}

	// Check CFS quota limit (cpu.max for v2, cpu.cfs_quota_us/cpu.cfs_period_us for v1)
	if stats.SchedulerQuota != nil && stats.SchedulerPeriod != nil && *stats.SchedulerPeriod > 0 {
		// Unlimited quota is represented as max uint64 or -1
		if *stats.SchedulerQuota != ^uint64(0) && int64(*stats.SchedulerQuota) != -1 {
			// Convert quota/period ratio to nanoseconds/second
			// Example: quota=50M, period=100M → 0.5 cores → 500M ns/s
			quotaLimitNanos := (float64(*stats.SchedulerQuota) / float64(*stats.SchedulerPeriod)) * 1e9
			log.Debugf("CPU limit from CFS quota: %.0f ns/s (quota=%d, period=%d)",
				quotaLimitNanos, *stats.SchedulerQuota, *stats.SchedulerPeriod)

			// Take minimum of CPUSet and quota limits
			if limitNanos == nil {
				limitNanos = &quotaLimitNanos
				log.Debugf("Setting CPU limit to CFS quota: %.0f ns/s", *limitNanos)
			} else if quotaLimitNanos < *limitNanos {
				log.Debugf("CFS quota (%.0f ns/s) is lower than CPUSet (%.0f ns/s), using CFS quota",
					quotaLimitNanos, *limitNanos)
				limitNanos = &quotaLimitNanos
			} else {
				log.Debugf("CPUSet limit (%.0f ns/s) is lower than CFS quota (%.0f ns/s), keeping CPUSet",
					*limitNanos, quotaLimitNanos)
			}
		} else {
			log.Debug("CFS quota is unlimited (max uint64 or -1)")
		}
	}

	if limitNanos != nil {
		log.Debugf("Final CPU limit: %.0f ns/s (%.2f cores)", *limitNanos, *limitNanos/1e9)
	} else {
		log.Debug("No CPU limit found in cgroup stats")
	}

	return limitNanos
}

func floatPtr(f float64) *float64 {
	return &f
}

// Start begins collecting CPU metrics periodically
func (c *Collector) Start(ctx context.Context) {
	collectorCtx, cancel := context.WithCancel(ctx)
	c.cancelFunc = cancel

	go c.collectLoop(collectorCtx)
	log.Info("Cgroup metrics collector started")
}

// Stop stops the cgroup metrics collector
func (c *Collector) Stop() {
	if c.cancelFunc != nil {
		c.cancelFunc()
		log.Info("Cgroup metrics collector stopped")
	}
}

// collectLoop runs the collection loop
func (c *Collector) collectLoop(ctx context.Context) {
	ticker := time.NewTicker(c.collectionInterval)
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

// collect retrieves and sends CPU metrics
// This follows the same pattern as the system collector's buildContainerMetrics
func (c *Collector) collect() {
	log.Debugf("Starting collect() with metricSource: %d (%s)", c.metricSource, c.metricSource.String())

	// Step 1: Refresh cgroup data
	if err := c.cgroupReader.RefreshCgroups(0); err != nil {
		log.Debugf("Failed to refresh cgroups: %v", err)
		return
	}

	// Step 2: Get self cgroup
	cgroup := c.cgroupReader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if cgroup == nil {
		log.Debug("Failed to get self cgroup")
		return
	}

	// Step 3: Get all stats at once (same as system collector)
	// This is more efficient than calling individual Get methods
	stats := &cgroups.Stats{}
	allFailed, errs := cgroups.GetStats(cgroup, stats)
	if allFailed {
		log.Debugf("Failed to get cgroup stats: %v", errs)
		return
	} else if len(errs) > 0 {
		log.Debugf("Incomplete cgroup stats: %v", errs)
	}

	// Step 4: Handle timestamp (same as system collector)
	timestamp := time.Now()
	log.Tracef("Collecting CPU stats at timestamp: %v", timestamp)

	// Step 5: Process CPU stats (same structure as system collector's buildContainerMetrics)
	c.processCPUStats(stats.CPU, timestamp)

	// TODO: Add memory metrics when ready (following same pattern)
	// if stats.Memory != nil {
	//     c.processMemoryStats(stats.Memory, timestamp)
	// }

	// TODO: Add IO metrics when ready (following same pattern)
	// if stats.IO != nil {
	//     c.processIOStats(stats.IO, timestamp)
	// }
}

// processCPUStats processes and sends CPU metrics
// This mirrors the system collector's buildCPUStats approach
func (c *Collector) processCPUStats(cpuStats *cgroups.CPUStats, timestamp time.Time) {
	// Validate input
	if cpuStats == nil {
		log.Debug("CPU stats are nil, skipping")
		return
	}

	// Convert to provider format (extracts values with null handling, same as buildCPUStats)
	containerCPUStats := convertToContainerCPUStats(cpuStats)

	// Send metrics (aggregator handles rate calculation automatically)
	// The container check uses the same pattern: send cumulative values with RateType
	c.sendCPUMetrics(containerCPUStats)
}

// sendCPUMetrics sends all CPU metrics from container CPU stats
// The aggregator automatically handles rate calculation for monotonic counters
// This follows the container check pattern: send cumulative values with RateType
func (c *Collector) sendCPUMetrics(cpuStats *provider.ContainerCPUStats) {
	if c.metricAgent.Demux == nil {
		return
	}

	// Send rate metrics - these are cumulative counters, aggregator calculates the rate
	// Using RateType to match the container check's sender.Rate behavior
	if cpuStats.Total != nil {
		c.sendMetric(c.metricPrefix+"usage", *cpuStats.Total, metrics.RateType)
	}

	if cpuStats.User != nil {
		c.sendMetric(c.metricPrefix+"user", *cpuStats.User, metrics.RateType)
	}

	if cpuStats.System != nil {
		c.sendMetric(c.metricPrefix+"system", *cpuStats.System, metrics.RateType)
	}

	if cpuStats.ThrottledTime != nil {
		c.sendMetric(c.metricPrefix+"throttled.time", *cpuStats.ThrottledTime, metrics.RateType)
	}

	if cpuStats.ThrottledPeriods != nil {
		c.sendMetric(c.metricPrefix+"throttled", *cpuStats.ThrottledPeriods, metrics.RateType)
	}

	// PSI (Pressure Stall Information) metrics for cgroupv2
	// Note: PartialStallTime is already in nanoseconds (converted by system collector)
	if cpuStats.PartialStallTime != nil {
		c.sendMetric(c.metricPrefix+"partial_stall", *cpuStats.PartialStallTime, metrics.RateType)
	}

	// Send gauge metric for CPU limit (already in nanoseconds/second)
	// Both usage and limit are in ns/s for easy percentage calculation: (usage / limit) * 100
	// Example: 500,000,000 ns/s = 0.5 cores (displayed as 500 mcores in UI)
	if cpuStats.Limit != nil {
		c.sendMetric(c.metricPrefix+"limit", *cpuStats.Limit, metrics.GaugeType)
		c.sendMetric(c.metricPrefix+"limit.additional", *cpuStats.Limit, metrics.GaugeType)
	}
}

// TODO: Add processMemoryStats following system collector's buildMemoryStats pattern
// TODO: Add processIOStats following system collector's buildIOStats pattern
// TODO: Add processPIDStats following system collector's buildPIDStats pattern

// sendMetric sends a metric to the metric agent
func (c *Collector) sendMetric(name string, value float64, metricType metrics.MetricType) {
	if c.metricAgent.Demux == nil {
		return
	}

	log.Debugf("sending metric with source: %d (%s)", c.metricSource, c.metricSource.String())

	metricTimestamp := float64(time.Now().UnixNano()) / float64(time.Second)
	c.metricAgent.Demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metricType,
		Tags:       c.metricAgent.GetExtraTags(),
		SampleRate: 1,
		Timestamp:  metricTimestamp,
		Source:     c.metricSource,
	})
}
