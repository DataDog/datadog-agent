// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

// MetricType represents the type of metric (Gauge or Count)
type MetricType int

const (
	// MetricTypeGauge represents a gauge metric (can go up or down)
	MetricTypeGauge MetricType = iota
	// MetricTypeCount represents a count metric (incremental)
	MetricTypeCount
)

// StructuredMetric represents a single metric with its value and tags
type StructuredMetric struct {
	Name  string
	Type  MetricType
	Value float64
	Tags  []string
}

// StructuredMetrics is a collection of metrics for one object
type StructuredMetrics []StructuredMetric
