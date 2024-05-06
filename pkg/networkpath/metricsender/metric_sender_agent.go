// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metricsender

import "github.com/DataDog/datadog-agent/pkg/aggregator/sender"

// AgentMetricSender sends metrics using Agent sender.Sender
type AgentMetricSender struct {
	sender sender.Sender
}

// NewMetricSenderAgent creates a new AgentMetricSender
func NewMetricSenderAgent(sender sender.Sender) MetricSender {
	return &AgentMetricSender{sender: sender}
}

// Gauge metric sender
func (s *AgentMetricSender) Gauge(metricName string, value float64, tags []string) {
	s.sender.Gauge(metricName, value, "", tags)
}
