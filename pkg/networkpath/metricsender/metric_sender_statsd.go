// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metricsender

import (
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

type statsdMetricSender struct {
	statsdClient ddgostatsd.ClientInterface
}

// NewMetricSenderStatsd constructor
func NewMetricSenderStatsd(statsdClient ddgostatsd.ClientInterface) MetricSender {
	return &statsdMetricSender{
		statsdClient: statsdClient,
	}
}

// Compile-time check to ensure that statsdMetricSender conforms to the MetricSender interface
var _ MetricSender = (*statsdMetricSender)(nil)

// Gauge metric sender
func (s *statsdMetricSender) Gauge(metricName string, value float64, tags []string) {
	s.statsdClient.Gauge(metricName, value, tags, 1) //nolint:errcheck
}
