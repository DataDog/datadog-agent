// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/DataDog/gopsutil/process"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-go/statsd"
)

// RuntimeMonitor is used to export runtime.MemStats metrics for debugging purposes
type RuntimeMonitor struct {
	client *statsd.Client
}

// SendStats sends the metric of the runtime monitor
func (m *RuntimeMonitor) SendStats() error {
	if err := m.sendGoRuntimeMetrics(); err != nil {
		return err
	}
	if err := m.sendProcMetrics(); err != nil {
		return err
	}
	return m.sendCgroupMetrics()
}

func (m *RuntimeMonitor) sendCgroupMetrics() error {
	pid := uint32(utils.Getpid())

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
		return errors.Wrapf(err, "couldn't find memory controller in: %v", cgroups)
	}

	usageInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.usage_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryUsageInBytes, float64(usageInBytes), []string{}, 1.0); err != nil {
			return errors.Wrap(err, "failed to send MetricRuntimeCgroupMemoryUsageInBytes metric")
		}
	}

	limitInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.limit_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryLimitInBytes, float64(limitInBytes), []string{}, 1.0); err != nil {
			return errors.Wrap(err, "failed to send MetricRuntimeCgroupMemoryLimitInBytes metric")
		}
	}

	memswUsageInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.memsw.usage_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryMemSWUsageInBytes, float64(memswUsageInBytes), []string{}, 1.0); err != nil {
			return errors.Wrap(err, "failed to send MetricRuntimeCgroupMemoryMemSWUsageInBytes metric")
		}
	}

	memswLimitInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.memsw.limit_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryMemSWLimitInBytes, float64(memswLimitInBytes), []string{}, 1.0); err != nil {
			return errors.Wrap(err, "failed to send MetricRuntimeCgroupMemoryMemSWLimitInBytes metric")
		}
	}

	kmemUsageInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.kmem.usage_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryKmemUsageInBytes, float64(kmemUsageInBytes), []string{}, 1.0); err != nil {
			return errors.Wrap(err, "failed to send MetricRuntimeCgroupMemoryKmemUsageInBytes metric")
		}
	}

	kmemLimitInBytes, err := utils.ParseCgroupFileValue("memory", memoryCgroup.Path, "memory.kmem.limit_in_bytes")
	if err == nil {
		if err = m.client.Gauge(metrics.MetricRuntimeCgroupMemoryKmemLimitInBytes, float64(kmemLimitInBytes), []string{}, 1.0); err != nil {
			return errors.Wrap(err, "failed to send MetricRuntimeCgroupMemoryKmemLimitInBytes metric")
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
			return errors.Wrapf(err, "failed to send %s metric", metrics.MetricRuntimeCgroupMemoryStatPrefix+lineSplit[0])
		}
	}

	return nil
}

func (m *RuntimeMonitor) sendProcMetrics() error {
	p, err := process.NewProcess(utils.Getpid())
	if err != nil {
		return err
	}

	memoryExt, err := p.MemoryInfoEx()
	if err != nil {
		return err
	}

	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcRSS, float64(memoryExt.RSS), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorProcRSS metric")
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcVMS, float64(memoryExt.VMS), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorProcVMS metric")
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcShared, float64(memoryExt.Shared), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorProcShared metric")
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcText, float64(memoryExt.Text), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorProcText metric")
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcLib, float64(memoryExt.Lib), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorProcLib metric")
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcData, float64(memoryExt.Data), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorProcData metric")
	}
	if err = m.client.Gauge(metrics.MetricRuntimeMonitorProcDirty, float64(memoryExt.Dirty), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorProcDirty metric")
	}

	return nil
}

func (m *RuntimeMonitor) sendGoRuntimeMetrics() error {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoAlloc, float64(stats.Alloc), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoAlloc metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoTotalAlloc, float64(stats.TotalAlloc), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoTotalAlloc metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoSys, float64(stats.Sys), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoSys metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoLookups, float64(stats.Lookups), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoLookups metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMallocs, float64(stats.Mallocs), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoMallocs metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoFrees, float64(stats.Frees), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoFrees metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapAlloc, float64(stats.HeapAlloc), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoHeapAlloc metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapSys, float64(stats.HeapSys), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoHeapSys metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapIdle, float64(stats.HeapIdle), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoHeapIdle metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapInuse, float64(stats.HeapInuse), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoHeapInuse metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapReleased, float64(stats.HeapReleased), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoHeapReleased metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoHeapObjects, float64(stats.HeapObjects), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoHeapObjects metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoStackInuse, float64(stats.StackInuse), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoStackInuse metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoStackSys, float64(stats.StackSys), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoStackSys metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMSpanInuse, float64(stats.MSpanInuse), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoMSpanInuse metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMSpanSys, float64(stats.MSpanSys), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoMSpanSys metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMCacheInuse, float64(stats.MCacheInuse), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoMCacheInuse metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoMCacheSys, float64(stats.MCacheSys), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoMCacheSys metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoBuckHashSys, float64(stats.BuckHashSys), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoBuckHashSys metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoGCSys, float64(stats.GCSys), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoGCSys metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoOtherSys, float64(stats.OtherSys), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoOtherSys metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoNextGC, float64(stats.NextGC), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoNextGC metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoNumGC, float64(stats.NumGC), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoNumGC metric")
	}
	if err := m.client.Gauge(metrics.MetricRuntimeMonitorGoNumForcedGC, float64(stats.NumForcedGC), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send MetricRuntimeMonitorGoNumForcedGC metric")
	}
	return nil
}

// NewRuntimeMonitor returns a new instance of RuntimeMonitor
func NewRuntimeMonitor(client *statsd.Client) *RuntimeMonitor {
	return &RuntimeMonitor{
		client: client,
	}
}
