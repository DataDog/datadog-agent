// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

//go:embed fixtures/metric_bytes
var metricsData []byte

//go:embed fixtures/metric_json_bytes
var metricsJSONData []byte

func TestNewMetricPayloads(t *testing.T) {
	t.Run("parseMetricSeries empty JSON object should be ignored", func(t *testing.T) {
		metrics, err := ParseMetricSeries(api.Payload{
			Data:     []byte("{}"),
			Encoding: encodingJSON,
		})
		assert.NoError(t, err)
		assert.Empty(t, metrics)
	})

	t.Run("parseMetricSeries valid body should parse metrics", func(t *testing.T) {
		metrics, err := ParseMetricSeries(api.Payload{Data: metricsData, Encoding: encodingDeflate})
		assert.NoError(t, err)
		assert.Equal(t, 151, len(metrics))
		assert.Equal(t, "datadog.dogstatsd.client.aggregated_context_by_type", metrics[0].name())
		expectedTags := []string{"client:go", "client_version:5.1.1", "client_transport:udp", "metrics_type:distribution"}
		sort.Strings(expectedTags)
		gotTags := metrics[0].GetTags()
		sort.Strings(gotTags)
		assert.Equal(t, expectedTags, gotTags)
	})

	t.Run("parseMetricSeries application/json content type should parse metrics", func(t *testing.T) {
		metrics, err := ParseMetricSeries(api.Payload{Data: metricsJSONData, Encoding: encodingGzip, ContentType: "application/json"})
		assert.NoError(t, err)
		assert.Equal(t, 569, len(metrics))
		assert.Equal(t, "otelcol_datadog_trace_agent_stats_writer_stats_entries", metrics[0].name())
	})
}
