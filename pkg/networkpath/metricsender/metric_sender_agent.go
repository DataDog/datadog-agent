// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metricsender

import "github.com/DataDog/datadog-agent/pkg/aggregator/sender"

// agentMetricSender sends metrics using Agent sender.Sender
type agentMetricSender struct {
	sender sender.Sender
}

// Compile-time check to ensure that agentMetricSender conforms to the MetricSender interface
var _ MetricSender = (*agentMetricSender)(nil)

// NewMetricSenderAgent creates a new agentMetricSender
func NewMetricSenderAgent(sender sender.Sender) MetricSender {
	return &agentMetricSender{sender: sender}
}

// Gauge metric sender
func (s *agentMetricSender) Gauge(metricName string, value float64, tags []string) {
	s.sender.Gauge(metricName, value, "", tags)
}
