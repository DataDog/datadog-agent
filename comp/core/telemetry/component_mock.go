// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless && (test || functionaltests || stresstests)

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metric interface defines the retrieval functions to extract information from a metric
type Metric interface {
	// Tags returns the tags associated with the metric
	Tags() map[string]string
	// Value returns the metric value
	Value() float64
}

// Mock implements mock-specific methods.
type Mock interface {
	Component

	GetRegistry() *prometheus.Registry
	GetCountMetric(subsystem, name string) ([]Metric, error)
	GetGaugeMetric(subsystem, name string) ([]Metric, error)
	GetHistogramMetric(subsystem, name string) ([]Metric, error)
}
