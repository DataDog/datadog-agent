// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package telemetry implements a component for all agent telemetry.
package telemetry

import (
	deftelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	dto "github.com/prometheus/client_model/go"
)

// team: agent-runtimes

// Component is an alias to the def telemetry Component for backwards compatibility
// It extends the def Component with prometheus-specific methods
type Component interface {
	deftelemetry.Component
	// RegisterCollector Registers a Collector with the prometheus registry
	RegisterCollector(c Collector)
	// UnregisterCollector unregisters a Collector with the prometheus registry
	UnregisterCollector(c Collector) bool
	// Gather exposes metrics from the general or default telemetry registry (see options.DefaultMetric)
	Gather(defaultGather bool) ([]*dto.MetricFamily, error)
}
