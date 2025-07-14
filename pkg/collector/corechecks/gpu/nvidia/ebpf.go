// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"
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
	return &SystemProbeCache{
		client: sysprobeclient.GetCheckClient(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
		stats:  nil, // Start with no data
	}
}

// Refresh fetches fresh GPU stats from system-probe.
// On failure, clears the cache to ensure collectors don't use stale data.
// Returns error if the refresh fails, nil on success.
func (c *SystemProbeCache) Refresh() error {
	if c.client == nil {
		return fmt.Errorf("system-probe client is nil")
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
	activeMetrics map[model.StatsKey]bool // activeMetrics tracks processes that are active for this device
}

// newEbpfCollector creates a new eBPF-based collector for the given device.
func newEbpfCollector(device ddnvml.Device, cache *SystemProbeCache) (*ebpfCollector, error) {
	if cache == nil {
		return nil, fmt.Errorf("system-probe cache cannot be nil")
	}

	return &ebpfCollector{
		device:        device,
		cache:         cache,
		activeMetrics: make(map[model.StatsKey]bool),
	}, nil
}

// Name returns the collector name.
func (c *ebpfCollector) Name() CollectorName {
	return systemProbe
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
		return []Metric{}, nil
	}

	// Get device info for filtering and limit metrics
	devInfo := c.device.GetDeviceInfo()
	deviceUUID := devInfo.UUID

	// Set all existing metrics to inactive for this device
	for key := range c.activeMetrics {
		c.activeMetrics[key] = false
	}

	var deviceMetrics []Metric
	var allPidTags []string

	// Process active metrics for this device
	for _, entry := range c.cache.GetStats().Metrics {
		if entry.Key.DeviceUUID != deviceUUID {
			continue // Skip metrics for other devices
		}

		key := entry.Key
		metrics := entry.UtilizationMetrics

		// Create PID tag for this process
		pidTag := []string{fmt.Sprintf("pid:%d", key.PID)}
		allPidTags = append(allPidTags, pidTag[0])

		// Add per-process usage metrics
		deviceMetrics = append(deviceMetrics,
			Metric{
				Name:  "core.usage",
				Value: metrics.UsedCores,
				Type:  ddmetrics.GaugeType,
				Tags:  pidTag,
			},
			Metric{
				Name:  "memory.usage",
				Value: float64(metrics.Memory.CurrentBytes),
				Type:  ddmetrics.GaugeType,
				Tags:  pidTag,
			},
		)

		// Mark this process as active
		c.activeMetrics[key] = true
	}

	// Handle inactive processes (emit zeros and collect their PIDs)
	for key, active := range c.activeMetrics {
		if !active {
			pidTag := []string{fmt.Sprintf("pid:%d", key.PID)}
			allPidTags = append(allPidTags, pidTag[0])

			// Emit zero metrics for inactive processes
			deviceMetrics = append(deviceMetrics,
				Metric{
					Name:     "core.usage",
					Value:    0,
					Type:     ddmetrics.GaugeType,
					Priority: 10,
					Tags:     pidTag,
				},
				Metric{
					Name:  "memory.usage",
					Value: 0,
					Type:  ddmetrics.GaugeType,
					Tags:  pidTag,
				},
			)

			// Remove inactive process from tracking
			delete(c.activeMetrics, key)
		}
	}

	// Emit limit metrics with aggregated PID tags
	deviceMetrics = append(deviceMetrics,
		Metric{
			Name:     "core.limit",
			Value:    float64(devInfo.CoreCount),
			Type:     ddmetrics.GaugeType,
			Priority: 10,
			Tags:     allPidTags,
		},
		Metric{
			Name:  "memory.limit",
			Value: float64(devInfo.Memory),
			Type:  ddmetrics.GaugeType,
			Tags:  allPidTags,
		},
	)

	return deviceMetrics, nil
}
