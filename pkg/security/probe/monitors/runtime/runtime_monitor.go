// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build linux

// Package runtime holds runtime related files
package runtime

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Monitor is used to export runtime.MemStats metrics for debugging purposes
type Monitor struct {
	client statsd.ClientInterface
}

// SendStats sends the metric of the runtime monitor
func (m *Monitor) SendStats() error {
	if err := m.sendGoRuntimeMetrics(); err != nil {
		return err
	}
	if err := m.sendProcMetrics(); err != nil {
		return err
	}
	return m.sendCgroupMetrics()
}

func (m *Monitor) sendCgroupMetrics() error {
	pid := utils.Getpid()

	// get cgroups
	cgroups, err := utils.GetProcControlGroups(pid, pid)
	if err != nil {
		return err
	}

	// find memory controller
	var memoryCgroup utils.ControlGroup
	for _, cgroup := range cgroups {
		for _, controller := range cgroup.Controllers {
			if controller == "memory" {
				memoryCgroup = cgroup
				break
			}
		}
	}
	if len(memoryCgroup.Path) == 0 {
		return fmt.Errorf("couldn't find memory controller in: %v", cgroups)
	}

	usageInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.usage_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryUsageInBytes, float64(usageInBytes), []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send MetricRuntimeCgroupMemoryUsageInBytes metric: %w", err)
		}
	}

	limitInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.limit_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryLimitInBytes, float64(limitInBytes), []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send MetricRuntimeCgroupMemoryLimitInBytes metric: %w", err)
		}
	}

	memswUsageInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.memsw.usage_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryMemSWUsageInBytes, float64(memswUsageInBytes), []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send MetricRuntimeCgroupMemoryMemSWUsageInBytes metric: %w", err)
		}
	}

	memswLimitInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.memsw.limit_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryMemSWLimitInBytes, float64(memswLimitInBytes), []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send MetricRuntimeCgroupMemoryMemSWLimitInBytes metric: %w", err)
		}
	}

	kmemUsageInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.kmem.usage_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryKmemUsageInBytes, float64(kmemUsageInBytes), []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send MetricRuntimeCgroupMemoryKmemUsageInBytes metric: %w", err)
		}
	}

	kmemLimitInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.kmem.limit_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryKmemLimitInBytes, float64(kmemLimitInBytes), []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send MetricRuntimeCgroupMemoryKmemLimitInBytes metric: %w", err)
		}
	}

	data, _, err := utils.ReadCgroupFile("memory", memoryCgroup.Path, "memory.stat")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		lineSplit := strings.Split(line, " ")
		if len(lineSplit) < 2 {
			continue
		}

		value, err := strconv.Atoi(lineSplit[1])
		if err != nil {
			continue
		}

		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryStatPrefix+lineSplit[0], float64(value), []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send %s metric: %w", metrics.MetricRuntimeCgroupMemoryStatPrefix+lineSplit[0], err)
		}
	}

	return nil
}

func (m *Monitor) sendProcMetrics() error {
	p, err := process.NewProcess(int32(utils.Getpid()))
	if err != nil {
		return err
	}

	memoryExt, err := p.MemoryInfoEx()
	if err != nil {
		return err
	}

	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcRSS, float64(memoryExt.RSS), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorProcRSS metric: %w", err)
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcVMS, float64(memoryExt.VMS), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorProcVMS metric: %w", err)
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcShared, float64(memoryExt.Shared), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorProcShared metric: %w", err)
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcText, float64(memoryExt.Text), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorProcText metric: %w", err)
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcLib, float64(memoryExt.Lib), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorProcLib metric: %w", err)
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcData, float64(memoryExt.Data), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorProcData metric: %w", err)
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcDirty, float64(memoryExt.Dirty), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorProcDirty metric: %w", err)
	}

	return nil
}

func (m *Monitor) sendGoRuntimeMetrics() error {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoAlloc, float64(stats.Alloc), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoAlloc metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoTotalAlloc, float64(stats.TotalAlloc), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoTotalAlloc metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoSys, float64(stats.Sys), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoSys metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoLookups, float64(stats.Lookups), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoLookups metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMallocs, float64(stats.Mallocs), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoMallocs metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoFrees, float64(stats.Frees), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoFrees metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapAlloc, float64(stats.HeapAlloc), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoHeapAlloc metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapSys, float64(stats.HeapSys), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoHeapSys metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapIdle, float64(stats.HeapIdle), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoHeapIdle metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapInuse, float64(stats.HeapInuse), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoHeapInuse metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapReleased, float64(stats.HeapReleased), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoHeapReleased metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapObjects, float64(stats.HeapObjects), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoHeapObjects metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoStackInuse, float64(stats.StackInuse), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoStackInuse metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoStackSys, float64(stats.StackSys), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoStackSys metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMSpanInuse, float64(stats.MSpanInuse), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoMSpanInuse metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMSpanSys, float64(stats.MSpanSys), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoMSpanSys metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMCacheInuse, float64(stats.MCacheInuse), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoMCacheInuse metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMCacheSys, float64(stats.MCacheSys), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoMCacheSys metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoBuckHashSys, float64(stats.BuckHashSys), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoBuckHashSys metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoGCSys, float64(stats.GCSys), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoGCSys metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoOtherSys, float64(stats.OtherSys), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoOtherSys metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoNextGC, float64(stats.NextGC), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoNextGC metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoNumGC, float64(stats.NumGC), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoNumGC metric: %w", err)
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoNumForcedGC, float64(stats.NumForcedGC), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send MetricRuntimeMonitorGoNumForcedGC metric: %w", err)
	}
	return nil
}

// NewRuntimeMonitor returns a new instance of Monitor
func NewRuntimeMonitor(client statsd.ClientInterface) *Monitor {
	return &Monitor{
		client: client,
	}
}
