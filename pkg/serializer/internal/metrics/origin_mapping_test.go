// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestMetricSourceToOriginProduct(t *testing.T) {
	const agentType = 10
	const serverlessType = 1
	const datadogExporterType = 19
	const gpuType = 38

	tests := []struct {
		name     string
		source   metrics.MetricSource
		expected int32
	}{
		{"agent type - dogstatsd", metrics.MetricSourceDogstatsd, agentType},
		{"agent type - internal", metrics.MetricSourceInternal, agentType},
		{"agent type - docker", metrics.MetricSourceDocker, agentType},
		{"agent type - cpu", metrics.MetricSourceCPU, agentType},
		{"serverless", metrics.MetricSourceServerless, serverlessType},
		{"gpu", metrics.MetricSourceGPU, gpuType},
		{"otel collector", metrics.MetricSourceOpenTelemetryCollectorUnknown, datadogExporterType},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := metricSourceToOriginProduct(tc.source)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMetricSourceToOriginCategory(t *testing.T) {
	tests := []struct {
		name     string
		source   metrics.MetricSource
		expected int32
	}{
		{"unknown", metrics.MetricSourceUnknown, 0},
		{"dogstatsd", metrics.MetricSourceDogstatsd, 10},
		// Most sources (internal, docker, cpu, etc.) map to 11 (integrationMetrics)
		{"internal", metrics.MetricSourceInternal, 11},
		{"docker", metrics.MetricSourceDocker, 11},
		{"kafka", metrics.MetricSourceKafka, 11},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := metricSourceToOriginCategory(tc.source)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMetricSourceToOriginService(t *testing.T) {
	tests := []struct {
		name     string
		source   metrics.MetricSource
		expected int32
	}{
		{"unknown", metrics.MetricSourceUnknown, 0},
		{"dogstatsd", metrics.MetricSourceDogstatsd, 0},
		{"jmx_custom", metrics.MetricSourceJmxCustom, 9},
		{"activemq", metrics.MetricSourceActivemq, 12},
		{"cassandra", metrics.MetricSourceCassandra, 28},
		{"disk", metrics.MetricSourceDisk, 48},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := metricSourceToOriginService(tc.source)
			assert.Equal(t, tc.expected, result)
		})
	}
}
