// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package impl provides the prometheus-based implementation of the telemetry component.
package impl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// team: agent-runtimes

// Component extends telemetry.Component with prometheus-specific methods.
type Component interface {
	telemetry.Component

	// RegisterCollector registers a Collector with the prometheus registry
	RegisterCollector(c prometheus.Collector)
	// UnregisterCollector unregisters a Collector with the prometheus registry
	UnregisterCollector(c prometheus.Collector) bool
	// Gather exposes metrics from the general or default telemetry registry (see options.DefaultMetric)
	Gather(defaultGather bool) ([]*dto.MetricFamily, error)
}
