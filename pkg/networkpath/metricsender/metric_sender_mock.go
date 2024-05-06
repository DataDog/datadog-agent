// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package metricsender

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type MockReceivedMetric struct {
	MetricType metrics.MetricType
	Name       string
	Value      float64
	Tags       []string
}

type MockMetricSender struct {
	Metrics []MockReceivedMetric
}

func NewMetricSenderMock() MetricSender {
	return &MockMetricSender{}
}

func (s *MockMetricSender) Gauge(metricName string, value float64, tags []string) {
	s.Metrics = append(s.Metrics, MockReceivedMetric{
		MetricType: metrics.GaugeType,
		Name:       metricName,
		Value:      value,
		Tags:       tags,
	})
}
