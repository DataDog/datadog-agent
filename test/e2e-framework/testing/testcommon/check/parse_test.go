// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJSONOutput(t *testing.T) {
	input := []byte(`[{
		"aggregator": {
			"events": [
				{
					"msg_title": "hello.event",
					"msg_text": "hello.text",
					"timestamp": 1234567890,
					"priority": "normal",
					"host": "host",
					"alert_type": "info"
				}
			],
			"metrics": [
				{
					"host": "host",
					"interval": 0,
					"metric": "hello.gauge",
					"points": [[1234567890, 1]],
					"source_type_name": "System",
					"tags": [],
					"type": "gauge"
				}
			],
			"service_checks": [
				{
					"check": "hello.service_check",
					"host_name": "host",
					"timestamp": 1234567890,
					"status": 0,
					"message": "",
					"tags": []
				}
			]
		},
		"runner": {
			"TotalRuns": 1,
			"TotalErrors": 0,
			"TotalWarnings": 0
		}
	}]`)

	result := ParseJSONOutput(t, input)
	require.Len(t, result, 1)

	// metric
	require.Len(t, result[0].Aggregator.Metrics, 1)
	metric := result[0].Aggregator.Metrics[0]
	assert.Equal(t, "host", metric.Host)
	assert.Equal(t, 0, metric.Interval)
	assert.Equal(t, "hello.gauge", metric.Metric)
	assert.Equal(t, [][]float64{{1234567890, 1}}, metric.Points)
	assert.Equal(t, "System", metric.SourceTypeName)
	assert.Empty(t, metric.Tags)
	assert.Equal(t, "gauge", metric.Type)

	// service check
	require.Len(t, result[0].Aggregator.ServiceChecks, 1)
	serviceCheck := result[0].Aggregator.ServiceChecks[0]
	assert.Equal(t, "hello.service_check", serviceCheck.Name)
	assert.Equal(t, "host", serviceCheck.Host)
	assert.Equal(t, 0, serviceCheck.Status)
	assert.Equal(t, int64(1234567890), serviceCheck.Timestamp)
	assert.Empty(t, serviceCheck.Message)
	assert.Empty(t, serviceCheck.Tags)

	// event
	require.Len(t, result[0].Aggregator.Events, 1)
	event := result[0].Aggregator.Events[0]
	assert.Equal(t, "hello.event", event.Title)
	assert.Equal(t, "hello.text", event.Text)
	assert.Equal(t, "host", event.Host)
	assert.Equal(t, int64(1234567890), event.Timestamp)
	assert.Equal(t, "normal", event.Priority)
	assert.Equal(t, "info", event.AlertType)

	// runner
	assert.Equal(t, 1, result[0].Runner.TotalRuns)
	assert.Equal(t, 0, result[0].Runner.TotalErrors)
	assert.Equal(t, 0, result[0].Runner.TotalWarnings)
}
