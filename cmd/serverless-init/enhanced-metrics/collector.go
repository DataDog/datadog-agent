// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package enhancedmetrics provides enhanced metrics collection
package enhancedmetrics

import (
	"context"
	"errors"
	"math"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

// CgroupReader reads cgroup stats. Satisfied by *cgroups.Reader
type CgroupReader interface {
	RefreshCgroups(cacheValidity time.Duration) error
	GetCgroup(id string) cgroups.Cgroup
	CgroupVersion() int
}

// EnhancedMetricSender sends enhanced metrics. Satisfied by *serverlessMetrics.ServerlessMetricAgent
type EnhancedMetricSender interface {
	AddEnhancedMetric(name string, value float64, metricSource metrics.MetricSource, timestamp float64, extraTags ...string)
	AddEnhancedUsageMetric(name string, value float64, metricSource metrics.MetricSource, timestamp float64, extraTags ...string)
}

// ServerlessCPUStats stores CPU stats for serverless environments
type ServerlessCPUStats struct {
	Total *uint64  // Total CPU usage in nanoseconds
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
	CPUUsage  float64 // Total CPU usage in nanocores (nanoseconds per second)
	Timestamp float64 // Unix timestamp in seconds
}

// ServerlessRateStats stores previous stat values for rate calculation
type ServerlessRateStats struct {
	CollectionTime time.Time
	TotalCPU       *uint64
}

// NullServerlessRateStats can be safely used when there are no previous rate values
var NullServerlessRateStats = ServerlessRateStats{
	TotalCPU: nil,
}

// Collector stores the cgroup reader used for data collection, the collection interval,
// the metric prefix, the metric metadata, and the metrics agent where metrics are sent
type Collector struct {
	metricAgent        EnhancedMetricSender
	metricSource       metrics.MetricSource
	cgroupReader       CgroupReader
	collectionInterval time.Duration
	cancelFunc         context.CancelFunc
	done               chan struct{}
	metricPrefix       string
	usageMetricSuffix  string
	// Previous stats for rate calculation
	previousRateStats ServerlessRateStats
}

// NewCollector creates a new Collector
func NewCollector(metricAgent EnhancedMetricSender, metricSource metrics.MetricSource, metricPrefix string, usageMetricSuffix string, collectionInterval time.Duration) (*Collector, error) {
	if metricAgent == nil || reflect.ValueOf(metricAgent).IsNil() {
		return nil, errors.New("metricAgent cannot be nil")
	}

	cgroupReader, err := cgroups.NewSelfReader("/proc", true)
	if err != nil {
		return nil, err
	}

	return &Collector{
		metricAgent:        metricAgent,
		metricSource:       metricSource,
		cgroupReader:       cgroupReader,
		collectionInterval: collectionInterval,
		metricPrefix:       metricPrefix + "enhanced.",
		usageMetricSuffix:  usageMetricSuffix,
		previousRateStats:  NullServerlessRateStats,
	}, nil
}

// Start starts the Collector
func (c *Collector) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelFunc = cancel
	c.done = make(chan struct{})

	log.Info("Enhanced metrics collector started")
	log.Debugf("Using cgroup version %d", c.cgroupReader.CgroupVersion())
	c.collectLoop(ctx)
	close(c.done)
}

// Stop stops the Collector
func (c *Collector) Stop() {
	if c.cancelFunc != nil {
		c.cancelFunc()
		// Wait for collectLoop to exit, including one final collect on ctx.Done.
		<-c.done
		log.Info("Enhanced metrics collector stopped")
	}
}

// collectLoop starts the collection loop with the specified collection interval
func (c *Collector) collectLoop(ctx context.Context) {
	ticker := time.NewTicker(c.collectionInterval)
	defer ticker.Stop()

	// Do an initial collect before starting to collect on a ticker
	c.collect()

	for {
		select {
		case <-ctx.Done():
			// Final collect for a partial interval before shutdown.
			c.collect()
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect collects the enhanced metrics from the cgroup and sends them to the metric agent
func (c *Collector) collect() {
	collectionTime := time.Now()
	timestamp := float64(collectionTime.UnixNano()) / float64(time.Second)

	// Always send the usage metric, regardless of cgroup collection success.
	if c.usageMetricSuffix != "" {
		c.metricAgent.AddEnhancedUsageMetric(c.metricPrefix+c.usageMetricSuffix, 1, c.metricSource, timestamp)
	}

	if err := c.cgroupReader.RefreshCgroups(0); err != nil {
		log.Warnf("Failed to refresh cgroups: %v", err)
		return
	}

	cgroup := c.cgroupReader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if cgroup == nil {
		log.Warn("Failed to get self cgroup")
		return
	}

	stats := &cgroups.Stats{}
	allFailed, errs := cgroups.GetStats(cgroup, stats)
	if allFailed {
		log.Warnf("Failed to get cgroup stats: %v", errs)
		return
	} else if len(errs) > 0 {
		log.Debugf("Incomplete cgroup stats: %v", errs)
	}

	containerStats := c.convertToServerlessContainerStats(stats, collectionTime)
	enhancedMetrics := c.computeEnhancedMetrics(containerStats)
	c.sendMetrics(enhancedMetrics)
}

// convertToServerlessContainerStats converts the cgroup stats to the ServerlessContainerStats struct
func (c *Collector) convertToServerlessContainerStats(stats *cgroups.Stats, collectionTime time.Time) *ServerlessContainerStats {
	serverlessStats := &ServerlessContainerStats{
		CollectionTime: collectionTime,
	}

	if stats == nil || stats.CPU == nil {
		return serverlessStats
	}

	serverlessStats.CPU = &ServerlessCPUStats{}

	// only set when cgroup reports CPU.Total.
	var total *uint64
	if stats.CPU.Total != nil {
		total = pointer.Ptr(*stats.CPU.Total)
	}

	serverlessStats.CPU.Total = total
	serverlessStats.CPU.Limit = computeCPULimit(stats.CPU, systemutils.HostCPUCount())

	return serverlessStats
}

// computeEnhancedMetrics computes the enhanced metrics from the ServerlessContainerStats struct
func (c *Collector) computeEnhancedMetrics(inStats *ServerlessContainerStats) ServerlessEnhancedMetrics {
	enhancedMetrics := ServerlessEnhancedMetrics{}

	if inStats == nil {
		return enhancedMetrics
	}

	enhancedMetrics.Timestamp = float64(inStats.CollectionTime.UnixNano()) / float64(time.Second)

	if inStats.CPU != nil {
		currentTotal := inStats.CPU.Total
		enhancedMetrics.CPUUsage = calculateCPUUsage(currentTotal, c.previousRateStats.TotalCPU, inStats.CollectionTime, c.previousRateStats.CollectionTime)

		// Store current cpu total and collection time for next rate calculation
		c.previousRateStats.TotalCPU = currentTotal
		c.previousRateStats.CollectionTime = inStats.CollectionTime

		enhancedMetrics.CPULimit = statValue(inStats.CPU.Limit, math.NaN())
	}

	return enhancedMetrics
}

// computeCPULimit computes the CPU limit from the cgroup stats
func computeCPULimit(stats *cgroups.CPUStats, hostCPUCount int) *float64 {
	if stats == nil {
		return nil
	}

	// Limit is computed using min(CPUSet, CFS CPU Quota)
	// Default to host CPU count if no other limit is available
	var limit *float64

	log.Debugf("CPU limit from host: %d cores", hostCPUCount)

	if stats.CPUCount != nil {
		log.Debugf("CPU limit from CPUSet: %d cores", *stats.CPUCount)
	} else {
		log.Debugf("CPU limit from CPUSet: nil")
	}

	if stats.CPUCount != nil && *stats.CPUCount != uint64(hostCPUCount) {
		limit = pointer.Ptr(float64(*stats.CPUCount))
	}

	if stats.SchedulerQuota != nil && stats.SchedulerPeriod != nil {
		quotaLimit := (float64(*stats.SchedulerQuota) / float64(*stats.SchedulerPeriod))
		log.Debugf("CPU limit from CFS quota: %.3f cores (quota=%d, period=%d)", quotaLimit, *stats.SchedulerQuota, *stats.SchedulerPeriod)
		if limit == nil || quotaLimit < *limit {
			limit = &quotaLimit
		}
	}

	if limit == nil {
		limit = pointer.Ptr(float64(hostCPUCount))
	}

	log.Debugf("CPU limit: %.3f cores", *limit)

	// Convert CPU limit from cores to nanocores
	limitNanos := *limit * 1e9

	return &limitNanos
}

// calculateCPUUsage calculates the CPU usage rate in nanoseconds per second (nanocores)
// Returns NaN if first run or invalid data
func calculateCPUUsage(currentTotal *uint64, previousTotal *uint64, currentTime time.Time, previousTime time.Time) float64 {
	log.Debugf("currentTime=%v, previousTime=%v", currentTime, previousTime)

	if currentTotal == nil || previousTotal == nil {
		log.Debugf("currentTotal or previousTotal is nil")
		return math.NaN()
	}

	cur := *currentTotal
	prev := *previousTotal
	log.Debugf("currentTotal=%d, previousTotal=%d", cur, prev)

	if previousTime.IsZero() {
		log.Debugf("previousTime is zero")
		return math.NaN()
	}

	timeDiff := currentTime.Sub(previousTime).Seconds()
	if timeDiff <= 0 {
		log.Debugf("current time is less than or equal to previous time")
		return math.NaN()
	}

	// compare before subtracting as uint64 underflows if cur < prev.
	if cur <= prev {
		log.Debugf("current total less than or equal to previous total")
		return math.NaN()
	}

	usage := float64(cur-prev) / timeDiff
	log.Debugf("CPU usage: %.3f cores", usage/1e9)

	return usage
}

// sendMetrics sends the enhanced metrics to the metric agent
func (c *Collector) sendMetrics(enhancedMetrics ServerlessEnhancedMetrics) {
	// CPU usage in nanocores
	// Skip when value is NaN since this value is used on the first collect before the rate can be computed
	if !math.IsNaN(enhancedMetrics.CPUUsage) {
		c.metricAgent.AddEnhancedMetric(c.metricPrefix+"cpu.usage", enhancedMetrics.CPUUsage, c.metricSource, enhancedMetrics.Timestamp)
	}

	// CPU limit in nanocores
	// Skip when value is NaN as no limit is available
	if !math.IsNaN(enhancedMetrics.CPULimit) {
		c.metricAgent.AddEnhancedMetric(c.metricPrefix+"cpu.limit", enhancedMetrics.CPULimit, c.metricSource, enhancedMetrics.Timestamp)
	}
}

// statValue returns the value of a float64 pointer, or a default value if the pointer is nil
func statValue(val *float64, def float64) float64 {
	if val != nil {
		return *val
	}
	return def
}
