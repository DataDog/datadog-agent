// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metricsender

import "github.com/DataDog/datadog-agent/pkg/aggregator/sender"

type metricSenderAgent struct {
	sender sender.Sender
}

func NewMetricSenderAgent(sender sender.Sender) MetricSender {
	return &metricSenderAgent{sender: sender}
}

func (s *metricSenderAgent) Gauge(metricName string, value float64, tags []string) {
	s.sender.Gauge(metricName, value, "", tags)
}
