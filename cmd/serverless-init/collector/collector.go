// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

/*
TODO:

rename to enhanced metrics collector? Or otherwise organize file structure?
look into simplifying cpu limit and fallbacks, is it valid in Cloud Run?
Check in Cloud Run Functions, Cloud Run Jobs, Azure Web Apps

Refactor to move go routine to main.go?
Clean up sendCPUMetrics to look more like what's used in the container collector
What about updating some of the helper methods for conversion?

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
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

type Collector struct {
	metricAgent        *serverlessMetrics.ServerlessMetricAgent
	metricSource       metrics.MetricSource
	cgroupReader       *cgroups.Reader
	collectionInterval time.Duration
	cancelFunc         context.CancelFunc
	metricPrefix       string
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
	log.Debugf("Starting collect() with metricSource: %d (%s)", c.metricSource, c.metricSource.String())

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

	timestamp := time.Now()
	log.Debugf("Collecting CPU stats at timestamp: %v", timestamp)

	c.processCPUStats(stats.CPU, timestamp)
}

func convertToContainerCPUStats(cpuStats *cgroups.CPUStats) *provider.ContainerCPUStats {
	containerCPUStats := &provider.ContainerCPUStats{}

	if cpuStats.Total != nil {
		containerCPUStats.Total = pointer.Ptr(float64(*cpuStats.Total))
	}

	containerCPUStats.Limit = computeCPULimit(cpuStats)

	return containerCPUStats
}

func computeCPULimit(cgs *cgroups.CPUStats) *float64 {
	// Limit is computed using min(CPUSet, CFS CPU Quota)
	var limit *float64

	if cgs.CPUCount != nil && *cgs.CPUCount != uint64(systemutils.HostCPUCount()) {
		limit = pointer.Ptr(float64(*cgs.CPUCount))
		log.Debugf("CPU limit from CPUSet: %.0f ns/s = %d cores", *limit, *cgs.CPUCount)
	}

	if cgs.SchedulerQuota != nil && cgs.SchedulerPeriod != nil {
		quotaLimit := (float64(*cgs.SchedulerQuota) / float64(*cgs.SchedulerPeriod))
		log.Debugf("CPU limit from CFS quota: %.0f ns/s (quota=%d, period=%d)", quotaLimit, *cgs.SchedulerQuota, *cgs.SchedulerPeriod)
		if limit == nil || quotaLimit < *limit {
			limit = &quotaLimit
		}
	}

	if limit == nil {
		limit = pointer.Ptr(float64(systemutils.HostCPUCount()))
		log.Debugf("CPU limit from systemutils.HostCPUCount: %d cores", systemutils.HostCPUCount())
	}

	log.Debugf("Final CPU limit: %.0f ns/s (%.2f cores)", *limit, *limit/float64(time.Second))

	// Convert cpu limit to nanoseconds
	limitNanos := *limit * float64(time.Second)
	return &limitNanos
}

func (c *Collector) processCPUStats(cpuStats *cgroups.CPUStats, timestamp time.Time) {
	if cpuStats == nil {
		log.Debug("CPU stats are nil, skipping")
		return
	}

	containerCPUStats := convertToContainerCPUStats(cpuStats)

	c.sendCPUMetrics(containerCPUStats)
}

func (c *Collector) sendCPUMetrics(cpuStats *provider.ContainerCPUStats) {

	if cpuStats.Total != nil {
		// CPU usage in nanoseconds/second
		c.metricAgent.AddMetric(c.metricPrefix+"cpu.usage", *cpuStats.Total, c.metricSource, metrics.RateType)
	}

	if cpuStats.Limit != nil {
		// CPU limit in nanoseconds
		c.metricAgent.AddMetric(c.metricPrefix+"cpu.limit", *cpuStats.Limit, c.metricSource, metrics.GaugeType)
	}
}
