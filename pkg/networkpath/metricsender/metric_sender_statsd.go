// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metricsender

import "github.com/DataDog/datadog-agent/pkg/process/statsd"

type metricSenderStatsd struct{}

// NewMetricSenderStatsd constructor
func NewMetricSenderStatsd() MetricSender {
	return &metricSenderStatsd{}
}

// Compile-time check to ensure that metricSenderStatsd conforms to the MetricSender interface
var _ MetricSender = (*metricSenderStatsd)(nil)

// Gauge metric sender
func (s metricSenderStatsd) Gauge(metricName string, value float64, tags []string) {
	statsd.Client.Gauge(metricName, value, tags, 1) //nolint:errcheck
}
