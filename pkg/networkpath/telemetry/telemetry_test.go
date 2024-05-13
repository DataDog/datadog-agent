// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package telemetry

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/networkpath/metricsender"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/stretchr/testify/assert"
)

func TestSubmitNetworkPathTelemetry(t *testing.T) {
	metricTags := []string{"foo:bar", "tag2:val2"}
	expectedTags := []string{
		"collector:network_path_integration",
		"destination_hostname:abc",
		"destination_port:unspecified",
		"foo:bar",
		"protocol:udp",
		"tag2:val2",
	}
	tests := []struct {
		name            string
		path            payload.NetworkPath
		checkDuration   time.Duration
		checkInterval   time.Duration
		tags            []string
		expectedMetrics []metricsender.MockReceivedMetric
	}{
		{
			name: "with hops and interval",
			path: payload.NetworkPath{
				Destination: payload.NetworkPathDestination{Hostname: "abc"},
				Hops: []payload.NetworkPathHop{
					{Hostname: "hop_1", IPAddress: "1.1.1.1"},
					{Hostname: "hop_2", IPAddress: "1.1.1.2"},
				},
			},
			checkDuration: 10 * time.Second,
			checkInterval: 20 * time.Second,
			tags:          metricTags,
			expectedMetrics: []metricsender.MockReceivedMetric{
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.check_duration",
					Value:      float64(10),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.check_interval",
					Value:      float64(20),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.monitored",
					Value:      float64(1),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.reachable",
					Value:      float64(0),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.unreachable",
					Value:      float64(1),
					Tags:       expectedTags,
				},
			},
		},
		{
			name: "with last hop successful",
			path: payload.NetworkPath{
				Destination: payload.NetworkPathDestination{Hostname: "abc"},
				Hops: []payload.NetworkPathHop{
					{Hostname: "hop_1", IPAddress: "1.1.1.1"},
					{Hostname: "hop_2", IPAddress: "1.1.1.2", Success: true},
				},
			},
			checkDuration: 10 * time.Second,
			checkInterval: 0,
			tags:          metricTags,
			expectedMetrics: []metricsender.MockReceivedMetric{
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.check_duration",
					Value:      float64(10),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.monitored",
					Value:      float64(1),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.hops",
					Value:      float64(2),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.reachable",
					Value:      float64(1),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.unreachable",
					Value:      float64(0),
					Tags:       expectedTags,
				},
			},
		},
		{
			name: "no hops and no interval",
			path: payload.NetworkPath{
				Destination: payload.NetworkPathDestination{Hostname: "abc"},
				Hops:        []payload.NetworkPathHop{},
			},
			checkDuration: 10 * time.Second,
			checkInterval: 0,
			tags:          metricTags,
			expectedMetrics: []metricsender.MockReceivedMetric{
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.check_duration",
					Value:      float64(10),
					Tags:       expectedTags,
				},
				{
					MetricType: metrics.GaugeType,
					Name:       "datadog.network_path.path.monitored",
					Value:      float64(1),
					Tags:       expectedTags,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &metricsender.MockMetricSender{}
			SubmitNetworkPathTelemetry(sender, tt.path, CollectorTypeNetworkPathIntegration, tt.checkDuration, tt.checkInterval, tt.tags)
			assert.Equal(t, tt.expectedMetrics, sender.Metrics)
		})
	}
}
