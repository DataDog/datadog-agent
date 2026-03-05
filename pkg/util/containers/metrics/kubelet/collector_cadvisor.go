// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	cadvisorCachePrefix = "cadvisor-"
	cadvisorRefreshKey  = "cadvisor-refresh"
)

// getCadvisorContainerStats returns container stats enriched with cadvisor data.
// This supplements the stats/summary data with fields only available from cadvisor
// (CPU user/system, CFS throttling, memory cache/limit, IO read/write).
// The stats are merged into existing ContainerStats, not replacing them.
func (kc *kubeletCollector) getCadvisorContainerStats(containerID string, currentTime time.Time, cacheValidity time.Duration) *provider.ContainerStats {
	cached, found, _ := kc.statsCache.Get(currentTime, cadvisorCachePrefix+containerID, cacheValidity)
	if found && cached != nil {
		return cached.(*provider.ContainerStats)
	}
	return nil
}

// refreshCadvisorCache scrapes /metrics/cadvisor and populates the cache with
// container stats not available from /stats/summary.
func (kc *kubeletCollector) refreshCadvisorCache(currentTime time.Time, cacheValidity time.Duration) {
	_, found, _ := kc.statsCache.Get(currentTime, cadvisorRefreshKey, cacheValidity)
	if found {
		return
	}

	var metrics []prometheus.MetricFamily
	var err error

	textFilter := []string{"pod_name=\"\"", "pod=\"\""}

	if kc.dataSource != nil {
		// Use shared data source (avoids duplicate HTTP call with cadvisor provider)
		metrics, err = kc.dataSource.GetCadvisorMetrics(textFilter)
	} else {
		// Fallback: fetch directly
		var data []byte
		var statusCode int

		ctx, cancel := context.WithTimeout(context.Background(), kubeletCallTimeout)
		data, statusCode, err = kc.kubeletClient.QueryKubelet(ctx, "/metrics/cadvisor")
		cancel()

		if err != nil || statusCode != 200 {
			log.Debugf("Unable to collect cadvisor metrics from kubelet: status=%d, err=%v", statusCode, err)
			kc.statsCache.Store(currentTime, cadvisorRefreshKey, nil, err)
			return
		}

		metrics, err = prometheus.ParseMetricsWithFilter(data, textFilter)
	}

	if err != nil {
		log.Debugf("Unable to parse cadvisor metrics: %v", err)
		kc.statsCache.Store(currentTime, cadvisorRefreshKey, nil, err)
		return
	}

	// Build a map of container stats by container ID
	containerStats := make(map[string]*provider.ContainerStats)

	for i := range metrics {
		metricFam := &metrics[i]
		if len(metricFam.Samples) == 0 {
			continue
		}

		switch metricFam.Name {
		case "container_cpu_system_seconds_total":
			kc.processCadvisorCPUMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.CPU == nil {
					stats.CPU = &provider.ContainerCPUStats{}
				}
				stats.CPU.System = pointer.Ptr(value * 1e9) // seconds to nanoseconds
			})
		case "container_cpu_user_seconds_total":
			kc.processCadvisorCPUMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.CPU == nil {
					stats.CPU = &provider.ContainerCPUStats{}
				}
				stats.CPU.User = pointer.Ptr(value * 1e9) // seconds to nanoseconds
			})
		case "container_cpu_cfs_throttled_periods_total":
			kc.processCadvisorCPUMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.CPU == nil {
					stats.CPU = &provider.ContainerCPUStats{}
				}
				stats.CPU.ThrottledPeriods = pointer.Ptr(value)
			})
		case "container_cpu_cfs_throttled_seconds_total":
			kc.processCadvisorCPUMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.CPU == nil {
					stats.CPU = &provider.ContainerCPUStats{}
				}
				stats.CPU.ThrottledTime = pointer.Ptr(value * 1e9) // seconds to nanoseconds
			})
		case "container_memory_cache":
			kc.processCadvisorContainerMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.Memory == nil {
					stats.Memory = &provider.ContainerMemStats{}
				}
				stats.Memory.Cache = pointer.Ptr(value)
			})
		case "container_spec_memory_limit_bytes":
			kc.processCadvisorContainerMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.Memory == nil {
					stats.Memory = &provider.ContainerMemStats{}
				}
				stats.Memory.Limit = pointer.Ptr(value)
			})
		case "container_memory_swap":
			kc.processCadvisorContainerMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.Memory == nil {
					stats.Memory = &provider.ContainerMemStats{}
				}
				stats.Memory.Swap = pointer.Ptr(value)
			})
		case "container_memory_rss":
			kc.processCadvisorContainerMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				// Filter aberrant values (same as cadvisor provider)
				if value >= math.Pow(2, 63) {
					return
				}
				// Only set RSS if not already set by stats/summary
				if stats.Memory == nil {
					stats.Memory = &provider.ContainerMemStats{}
				}
			})
		case "container_fs_reads_bytes_total":
			kc.processCadvisorContainerMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.IO == nil {
					stats.IO = &provider.ContainerIOStats{}
				}
				stats.IO.ReadBytes = pointer.Ptr(value)
			})
		case "container_fs_writes_bytes_total":
			kc.processCadvisorContainerMetric(metricFam, containerStats, func(stats *provider.ContainerStats, value float64) {
				if stats.IO == nil {
					stats.IO = &provider.ContainerIOStats{}
				}
				stats.IO.WriteBytes = pointer.Ptr(value)
			})
		}
	}

	// Store in cache
	for cID, stats := range containerStats {
		kc.statsCache.Store(currentTime, cadvisorCachePrefix+cID, stats, nil)
	}
	kc.statsCache.Store(currentTime, cadvisorRefreshKey, nil, nil)
}

// processCadvisorContainerMetric extracts container-scoped metrics from cadvisor.
// It resolves the container ID from metric labels and applies the setter function.
func (kc *kubeletCollector) processCadvisorContainerMetric(
	metricFam *prometheus.MetricFamily,
	containerStats map[string]*provider.ContainerStats,
	setter func(*provider.ContainerStats, float64),
) {
	for i := range metricFam.Samples {
		sample := &metricFam.Samples[i]
		cID := kc.resolveContainerID(sample.Metric)
		if cID == "" {
			continue
		}

		if _, ok := containerStats[cID]; !ok {
			containerStats[cID] = &provider.ContainerStats{}
		}
		setter(containerStats[cID], sample.Value)
	}
}

// processCadvisorCPUMetric is like processCadvisorContainerMetric but takes the
// latest value when multiple samples exist for the same container (counter behavior).
func (kc *kubeletCollector) processCadvisorCPUMetric(
	metricFam *prometheus.MetricFamily,
	containerStats map[string]*provider.ContainerStats,
	setter func(*provider.ContainerStats, float64),
) {
	// For CPU counter metrics, just use the latest sample per container
	kc.processCadvisorContainerMetric(metricFam, containerStats, setter)
}

// resolveContainerID resolves a container ID from cadvisor prometheus metric labels.
// Returns empty string if the metric is not a container-scoped metric.
func (kc *kubeletCollector) resolveContainerID(labels prometheus.Metric) string {
	// Check that this is a container metric (not pod-level or node-level)
	containerName := labels["container"]
	if containerName == "" {
		containerName = labels["container_name"]
	}
	if containerName == "" || containerName == "POD" {
		return ""
	}

	// Resolve container ID from pod name/namespace
	namespace := labels["namespace"]
	podName := labels["pod"]
	if podName == "" {
		podName = labels["pod_name"]
	}
	if namespace == "" || podName == "" {
		return ""
	}

	pod, err := kc.metadataStore.GetKubernetesPodByName(podName, namespace)
	if err != nil || pod == nil {
		return ""
	}

	for _, c := range pod.GetAllContainers() {
		if c.Name == containerName {
			return c.ID
		}
	}

	return ""
}

// mergeContainerStats merges cadvisor stats into the primary stats.
// Fields already set in primary (from stats/summary) are NOT overwritten.
func mergeContainerStats(primary *provider.ContainerStats, cadvisor *provider.ContainerStats) {
	if cadvisor == nil {
		return
	}

	// Merge CPU stats
	if cadvisor.CPU != nil {
		if primary.CPU == nil {
			primary.CPU = &provider.ContainerCPUStats{}
		}
		if primary.CPU.System == nil {
			primary.CPU.System = cadvisor.CPU.System
		}
		if primary.CPU.User == nil {
			primary.CPU.User = cadvisor.CPU.User
		}
		if primary.CPU.ThrottledPeriods == nil {
			primary.CPU.ThrottledPeriods = cadvisor.CPU.ThrottledPeriods
		}
		if primary.CPU.ThrottledTime == nil {
			primary.CPU.ThrottledTime = cadvisor.CPU.ThrottledTime
		}
	}

	// Merge Memory stats
	if cadvisor.Memory != nil {
		if primary.Memory == nil {
			primary.Memory = &provider.ContainerMemStats{}
		}
		if primary.Memory.Cache == nil {
			primary.Memory.Cache = cadvisor.Memory.Cache
		}
		if primary.Memory.Limit == nil {
			primary.Memory.Limit = cadvisor.Memory.Limit
		}
		if primary.Memory.Swap == nil {
			primary.Memory.Swap = cadvisor.Memory.Swap
		}
	}

	// Merge IO stats
	if cadvisor.IO != nil {
		if primary.IO == nil {
			primary.IO = &provider.ContainerIOStats{}
		}
		if primary.IO.ReadBytes == nil {
			primary.IO.ReadBytes = cadvisor.IO.ReadBytes
		}
		if primary.IO.WriteBytes == nil {
			primary.IO.WriteBytes = cadvisor.IO.WriteBytes
		}
	}
}
