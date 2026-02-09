// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

/*
TODO:

2/9
Add back containerapp tests
add collector tests
update serverlessMetricAgent import in main to have consistent naming
review rate calculation (distribution vs. rate), 1 instance, multiple instances
only start for in process, not sidecar OR collect for sidecar and tag appropriately. Some sort of container_type or sidecar tag?
rename to enhanced metrics collector? Or otherwise organize file structure?
Check in Cloud Run Functions, Cloud Run Jobs, Azure Web Apps
Refactor to move go routine to main.go?
Remove/add debug logs as needed
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

// ServerlessContainerStats stores raw container metrics for serverless environments
type ServerlessContainerStats struct {
	CollectionTime time.Time
	CPU            *ServerlessCPUStats
}

// ServerlessEnhancedMetrics stores computed metrics ready to be sent
type ServerlessEnhancedMetrics struct {
	CPULimit  float64 // CPU limit in nanocores
	CPUUsage  float64 // Total CPU usage in nanocores( nanoseconds per second)
	Timestamp float64 // Unix timestamp in seconds
}

// ServerlessRateStats stores previous stat values for rate calculation
type ServerlessRateStats struct {
	CollectionTime time.Time
	TotalCPU       float64
}

// NullServerlessRateStats can be safely used when there are no previous rate values
var NullServerlessRateStats = ServerlessRateStats{
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
	previousRateStats ServerlessRateStats
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
		metricAgent:       metricAgent,
		metricSource:      metricSource,
		cgroupReader:      cgroupReader,
		metricPrefix:      metricPrefix + ".enhanced.test.",
		previousRateStats: NullServerlessRateStats,
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

	containerStats := c.convertToServerlessContainerStats(stats)
	enhancedMetrics := c.computeContainerMetrics(containerStats)
	c.sendMetrics(containerStats, enhancedMetrics)
}

func (c *Collector) convertToServerlessContainerStats(stats *cgroups.Stats) *ServerlessContainerStats {
	serverlessStats := &ServerlessContainerStats{
		CollectionTime: time.Now(),
		CPU:            &ServerlessCPUStats{},
	}

	if stats.CPU.Total != nil {
		serverlessStats.CPU.Total = pointer.Ptr(float64(*stats.CPU.Total))
	}

	serverlessStats.CPU.Limit = computeCPULimit(stats.CPU)

	return serverlessStats
}

func (c *Collector) computeContainerMetrics(inStats *ServerlessContainerStats) ServerlessEnhancedMetrics {
	enhancedMetrics := ServerlessEnhancedMetrics{}

	if inStats == nil {
		return enhancedMetrics
	}

	enhancedMetrics.Timestamp = float64(inStats.CollectionTime.UnixNano()) / float64(time.Second)

	if inStats.CPU != nil {
		currentTotal := statValue(inStats.CPU.Total, -1)
		enhancedMetrics.CPUUsage = c.calculateCPUUsage(currentTotal, c.previousRateStats.TotalCPU, inStats.CollectionTime, c.previousRateStats.CollectionTime)

		// Store current cpu total for next calculation
		c.previousRateStats.TotalCPU = currentTotal

		enhancedMetrics.CPULimit = statValue(inStats.CPU.Limit, 0)
	}

	// Store current collection time for next calculation
	c.previousRateStats.CollectionTime = inStats.CollectionTime

	return enhancedMetrics
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

// calculateCPUUsage calculates the CPU usage rate in nanoseconds per second (nanocores)
// Returns -1 if invalid data or first run, returns 0 if unable to calculate using values and times provided
func (c *Collector) calculateCPUUsage(currentTotal float64, previousTotal float64, currentTime time.Time, previousTime time.Time) float64 {
	log.Debugf("calculateCPUUsage: currentTotal=%.0f, previousTotal=%.0f, currentTime=%v, previousTime=%v",
		currentTotal, previousTotal, currentTime, previousTime)

	if currentTotal == -1 || previousTotal == -1 {
		return -1
	}

	if previousTime.IsZero() {
		return 0
	}

	timeDiff := currentTime.Sub(previousTime).Seconds()
	if timeDiff <= 0 {
		return 0
	}

	valueDiff := currentTotal - previousTotal
	if valueDiff <= 0 {
		return 0
	}

	return valueDiff / timeDiff
}

func (c *Collector) sendMetrics(inStats *ServerlessContainerStats, enhancedMetrics ServerlessEnhancedMetrics) {
	// CPU usage in nanocores
	c.metricAgent.AddMetricWithTimestamp(c.metricPrefix+"cpu.usage", enhancedMetrics.CPUUsage, c.metricSource, metrics.DistributionType, enhancedMetrics.Timestamp)

	// CPU usage in nanocores
	c.metricAgent.AddMetricWithTimestamp(c.metricPrefix+"cpu.usage.rate", statValue(inStats.CPU.Total, -1), c.metricSource, metrics.RateType, enhancedMetrics.Timestamp)

	// CPU limit in nanocores
	c.metricAgent.AddMetricWithTimestamp(c.metricPrefix+"cpu.limit", enhancedMetrics.CPULimit, c.metricSource, metrics.DistributionType, enhancedMetrics.Timestamp)

}

func statValue(val *float64, def float64) float64 {
	if val != nil {
		return *val
	}
	return def
}
