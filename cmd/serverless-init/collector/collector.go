// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package collector provides enhanced metrics collection
package collector

import (
	"context"
	"errors"
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
	CPUUsage  float64 // Total CPU usage in nanocores (nanoseconds per second)
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
	usageMetricName    string
	// Previous stats for rate calculation
	previousRateStats ServerlessRateStats
}

// NewCollector creates a new Collector
func NewCollector(metricAgent EnhancedMetricSender, metricSource metrics.MetricSource, metricPrefix string, usageMetricName string, collectionInterval time.Duration) (*Collector, error) {
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
		metricPrefix:       metricPrefix + ".enhanced.",
		usageMetricName:    usageMetricName,
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
		// Wait for the previous collect to finish before starting the final collect
		<-c.done
		// One final collect before shutdown to collect a partial interval of enhanced metrics
		c.collect()
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
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect collects the enhanced metrics from the cgroup and sends them to the metric agent
func (c *Collector) collect() {
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

	containerStats := c.convertToServerlessContainerStats(stats)
	enhancedMetrics := c.computeEnhancedMetrics(containerStats)
	c.sendMetrics(enhancedMetrics)
}

// convertToServerlessContainerStats converts the cgroup stats to the ServerlessContainerStats struct
func (c *Collector) convertToServerlessContainerStats(stats *cgroups.Stats) *ServerlessContainerStats {
	serverlessStats := &ServerlessContainerStats{
		CollectionTime: time.Now(),
	}

	if stats == nil || stats.CPU == nil {
		return serverlessStats
	}

	serverlessStats.CPU = &ServerlessCPUStats{}
	if stats.CPU.Total != nil {
		serverlessStats.CPU.Total = pointer.Ptr(float64(*stats.CPU.Total))
	}

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
		currentTotal := statValue(inStats.CPU.Total, -1)
		enhancedMetrics.CPUUsage = calculateCPUUsage(currentTotal, c.previousRateStats.TotalCPU, inStats.CollectionTime, c.previousRateStats.CollectionTime)

		// Store current cpu total and collection time for next rate calculation
		c.previousRateStats.TotalCPU = currentTotal
		c.previousRateStats.CollectionTime = inStats.CollectionTime

		enhancedMetrics.CPULimit = statValue(inStats.CPU.Limit, 0)
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
// Returns -1 if first run or invalid data
func calculateCPUUsage(currentTotal float64, previousTotal float64, currentTime time.Time, previousTime time.Time) float64 {
	log.Debugf("calculateCPUUsage: currentTotal=%.0f, previousTotal=%.0f, currentTime=%v, previousTime=%v",
		currentTotal, previousTotal, currentTime, previousTime)

	if currentTotal == -1 || previousTotal == -1 {
		return -1
	}

	if previousTime.IsZero() {
		return -1
	}

	timeDiff := currentTime.Sub(previousTime).Seconds()
	if timeDiff <= 0 {
		return -1
	}

	valueDiff := currentTotal - previousTotal
	if valueDiff <= 0 {
		return -1
	}

	usage := valueDiff / timeDiff
	log.Debugf("CPU usage: %.3f cores", usage/1e9)

	return usage
}

// sendMetrics sends the enhanced metrics to the metric agent
func (c *Collector) sendMetrics(enhancedMetrics ServerlessEnhancedMetrics) {
	if c.usageMetricName != "" {
		c.metricAgent.AddEnhancedUsageMetric(c.metricPrefix+c.usageMetricName, 1, c.metricSource, enhancedMetrics.Timestamp)
	}

	// CPU usage in nanocores
	// Skip when value is -1 since this value is used on the first collect before the rate can be computed
	if enhancedMetrics.CPUUsage != -1 {
		c.metricAgent.AddEnhancedMetric(c.metricPrefix+"cpu.usage", enhancedMetrics.CPUUsage, c.metricSource, enhancedMetrics.Timestamp)
	}

	// CPU limit in nanocores
	c.metricAgent.AddEnhancedMetric(c.metricPrefix+"cpu.limit", enhancedMetrics.CPULimit, c.metricSource, enhancedMetrics.Timestamp)
}

// statValue returns the value of a float64 pointer, or a default value if the pointer is nil
func statValue(val *float64, def float64) float64 {
	if val != nil {
		return *val
	}
	return def
}
