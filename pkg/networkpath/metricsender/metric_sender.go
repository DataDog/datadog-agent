// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package metricsender holds the interface used to send Metrics with Agent Sender and Statsd sender
package metricsender

// MetricSender is an interface used to send Metrics with Agent Sender and Statsd sender
type MetricSender interface {
	Gauge(metricName string, value float64, tags []string)
}
