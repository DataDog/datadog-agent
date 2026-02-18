// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"strconv"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SystemProbeCache manages the system-probe GPU stats cache with fail-fast semantics.
// When refresh fails, stats is set to nil to prevent collectors from using stale data.
type SystemProbeCache struct {
	client *sysprobeclient.CheckClient
	stats  *model.GPUStats // nil indicates no valid data available
}

// NewSystemProbeCache creates a new stats cache that connects to system-probe using sysprobeclient.
func NewSystemProbeCache() *SystemProbeCache {
	timeout := pkgconfigsetup.Datadog().GetDuration("gpu.sp_process_metrics_request_timeout")
	client := sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
		sysprobeclient.WithCheckTimeout(timeout),
		sysprobeclient.WithStartupCheckTimeout(timeout),
	)

	return &SystemProbeCache{
		client: client,
		stats:  nil, // Start with no data
	}
}

// Refresh fetches fresh GPU stats from system-probe.
// On failure, clears the cache to ensure collectors don't use stale data.
// Returns error if the refresh fails, nil on success.
func (c *SystemProbeCache) Refresh() error {
	if c.client == nil {
		return errors.New("system-probe client is nil")
	}

	stats, err := sysprobeclient.GetCheck[model.GPUStats](c.client, sysconfig.GPUMonitoringModule)
	if err != nil {
		// Clear cache on failure to ensure fail-fast behavior
		c.stats = nil

		// Check if this is a startup error that should be ignored
		if sysprobeclient.IgnoreStartupError(err) == nil {
			log.Debugf("System-probe GPU module not ready yet")
			return nil
		}

		return fmt.Errorf("failed to get stats from system-probe: %w", err)
	}

	// Success - cache the new stats
	c.stats = &stats
	return nil
}

// GetStats returns the cached GPU stats if available.
// Returns nil if no valid data is available (e.g., refresh failed or never called).
func (c *SystemProbeCache) GetStats() *model.GPUStats {
	return c.stats
}

// IsValid returns true if the cache contains valid data.
func (c *SystemProbeCache) IsValid() bool {
	return c.stats != nil
}

// ebpfCollector is a virtual collector that provides system-probe process metrics
// for a specific GPU device. It filters the global system-probe stats by device UUID.
type ebpfCollector struct {
	device        ddnvml.Device
	cache         *SystemProbeCache
	activeMetrics map[model.ProcessStatsKey]bool // activeMetrics tracks processes that are active for this device
}

// newEbpfCollector creates a new eBPF-based collector for the given device.
func newEbpfCollector(device ddnvml.Device, cache *SystemProbeCache) (*ebpfCollector, error) {
	if cache == nil {
		return nil, errors.New("system-probe cache cannot be nil")
	}

	return &ebpfCollector{
		device:        device,
		cache:         cache,
		activeMetrics: make(map[model.ProcessStatsKey]bool),
	}, nil
}

// Name returns the collector name.
func (c *ebpfCollector) Name() CollectorName {
	return ebpf
}

// DeviceUUID returns the UUID of the device this collector monitors.
func (c *ebpfCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

// Collect returns system-probe process metrics for this device with high priority.
// Returns empty slice if cache is invalid or no metrics found for this device.
// core.usage and core.limit metrics get higher priority from eBPF collector than from the process collector,
func (c *ebpfCollector) Collect() ([]Metric, error) {
	// Check cache validity
	if !c.cache.IsValid() {
		log.Debugf("ebpf collector: cache not valid")
		return []Metric{}, nil
	}

	// Get device info for filtering and limit metrics
	devInfo := c.device.GetDeviceInfo()
	deviceUUID := devInfo.UUID

	// Set all existing metrics to inactive for this device
	log.Debugf("ebpf collector: %d active metrics in the cache from previous iteration", len(c.activeMetrics))
	for key := range c.activeMetrics {
		c.activeMetrics[key] = false
	}

	var deviceMetrics []Metric
	var allWorkloadIDs []workloadmeta.EntityID

	stats := c.cache.GetStats()
	log.Debugf("ebpf collector: received %d metrics from SP", len(stats.ProcessMetrics))
	// Process active metrics for this device
	for _, entry := range stats.ProcessMetrics {
		if entry.Key.DeviceUUID != deviceUUID {
			continue // Skip metrics for other devices
		}

		key := entry.Key
		metrics := entry.UtilizationMetrics

		workloads := []workloadmeta.EntityID{{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(key.PID)),
		}}

		allWorkloadIDs = append(allWorkloadIDs, workloads...)

		// Add per-process usage metrics
		deviceMetrics = append(deviceMetrics,
			Metric{
				Name:                "process.core.usage",
				Value:               metrics.UsedCores,
				Type:                ddmetrics.GaugeType,
				AssociatedWorkloads: workloads,
			},
			Metric{
				Name:                "process.memory.usage",
				Value:               float64(metrics.Memory.CurrentBytes),
				Type:                ddmetrics.GaugeType,
				AssociatedWorkloads: workloads,
			},
			Metric{
				Name:                "process.sm_active",
				Value:               metrics.ActiveTimePct,
				Type:                ddmetrics.GaugeType,
				Priority:            Low,
				AssociatedWorkloads: workloads,
			},
		)

		// Mark this process as active
		c.activeMetrics[key] = true
	}

	// Handle inactive processes (emit zeros and collect their PIDs)
	log.Debugf("ebpf collector: %d active metrics in the cache after processing", len(c.activeMetrics))
	for key, active := range c.activeMetrics {
		if !active {
			workloads := []workloadmeta.EntityID{{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(int(key.PID)),
			}}
			allWorkloadIDs = append(allWorkloadIDs, workloads...)

			// Emit zero metrics for inactive processes
			deviceMetrics = append(deviceMetrics,
				Metric{
					Name:                "process.core.usage",
					Value:               0,
					Type:                ddmetrics.GaugeType,
					AssociatedWorkloads: workloads,
				},
				Metric{
					Name:                "process.memory.usage",
					Value:               0,
					Type:                ddmetrics.GaugeType,
					AssociatedWorkloads: workloads,
				},
				Metric{
					Name:                "process.sm_active",
					Value:               0,
					Type:                ddmetrics.GaugeType,
					Priority:            Low,
					AssociatedWorkloads: workloads,
				},
			)

			// Remove inactive process from tracking
			delete(c.activeMetrics, key)
		}
	}

	// Emit limit metrics with aggregated PID tags
	deviceMetrics = append(deviceMetrics,
		Metric{
			Name:                "core.limit",
			Value:               float64(devInfo.CoreCount),
			Type:                ddmetrics.GaugeType,
			Priority:            Medium,
			AssociatedWorkloads: allWorkloadIDs,
		},
		Metric{
			Name:                "memory.limit",
			Value:               float64(devInfo.Memory),
			Type:                ddmetrics.GaugeType,
			AssociatedWorkloads: allWorkloadIDs,
		},
	)

	// Emit device-level sm_active metric
	for _, deviceMetric := range stats.DeviceMetrics {
		if deviceMetric.DeviceUUID == deviceUUID {
			deviceMetrics = append(deviceMetrics, Metric{
				Name:     "sm_active",
				Value:    deviceMetric.Metrics.ActiveTimePct,
				Type:     ddmetrics.GaugeType,
				Priority: Low,
				// No AssociatedWorkloads - device-wide metric
			})
			break
		}
	}

	return deviceMetrics, nil
}
