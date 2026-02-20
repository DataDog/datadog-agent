// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package nvidia holds the logic to collect metrics from the NVIDIA Management Library (NVML).
package nvidia

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// MetricPriority represents the priority level of a metric
type MetricPriority int

const (
	// Low priority is the default priority level (0)
	Low MetricPriority = 0
	// MediumLow priority level (5)
	MediumLow MetricPriority = 5
	// Medium priority level (10)
	Medium MetricPriority = 10
	// High priority level (20)
	High MetricPriority = 20
)

// CollectorName is the name of the nvml sub-collectors
type CollectorName string

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name                string                  // Name holds the name of the metric.
	Value               float64                 // Value holds the value of the metric.
	Type                metrics.MetricType      // Type holds the type of the metric.
	Priority            MetricPriority          // Priority is the priority of the metric, indicating which metric to keep in case of duplicates. Low (default) is the lowest priority.
	Tags                []string                // Tags holds optional metric-specific tags (e.g., "error type").
	AssociatedWorkloads []workloadmeta.EntityID // AssociatedWorkloads represents specific workloads that are associated with the metric, e.g. a process associated with a process-level metric. Used for tagging.
}

// Collector defines a collector that gets metric from a specific NVML subsystem and device
type Collector interface {
	// Collect collects metrics from the given NVML device. This method should not fill the tags
	// unless they're metric-specific (i.e., all device-specific tags will be added by the Collector itself)
	Collect() ([]Metric, error)

	// Name returns the name of the subsystem
	Name() CollectorName

	// DeviceUUID returns the UUID of the device this collector is collecting metrics from. Returns an empty string if there's no UUID
	DeviceUUID() string
}
