// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/metric_bytes_v1
var metricsDatav1 []byte

func TestV1MetricPayloads(t *testing.T) {
	t.Run("ParseV1MetricSeries empty JSON object should be ignored", func(t *testing.T) {
		metrics, err := ParseV1MetricSeries(api.Payload{
			Data:     []byte("{}"),
			Encoding: encodingJSON,
		})
		assert.NoError(t, err)
		assert.Empty(t, metrics)
	})
	t.Run("ParseV1MetricSeries valid body should parse metrics", func(t *testing.T) {
		metrics, err := ParseV1MetricSeries(api.Payload{Data: metricsDatav1, Encoding: encodingDeflate})
		require.NoError(t, err)
		assert.Equal(t, metrics[0].Metric, "datadog.trace_agent.started")
		assert.Equal(t, metrics[0].Host, "COMP-WY4M717J6J")
		assert.Equal(t, metrics[0].Points[0][0].(float64), float64(1697177070))
	})
}

func TestParseMetricSeriesV1(t *testing.T) {
	t.Run("empty JSON object should be ignored", func(t *testing.T) {
		series, err := ParseMetricSeriesV1(api.Payload{
			Data:     []byte("{}"),
			Encoding: encodingJSON,
		})
		assert.NoError(t, err)
		assert.Empty(t, series)
	})

	t.Run("valid body should convert to the v2 series type", func(t *testing.T) {
		series, err := ParseMetricSeriesV1(api.Payload{Data: metricsDatav1, Encoding: encodingDeflate})
		require.NoError(t, err)
		require.NotEmpty(t, series)

		first := series[0]
		assert.Equal(t, "datadog.trace_agent.started", first.Metric)
		assert.Equal(t, metricspb.MetricPayload_RATE, first.Type)
		assert.Equal(t, []string{"version:7.46.0"}, first.Tags)
		assert.Equal(t, int64(10), first.Interval)
		require.Len(t, first.Points, 1)
		assert.Equal(t, int64(1697177070), first.Points[0].Timestamp)
		require.Len(t, first.Resources, 1)
		assert.Equal(t, "host", first.Resources[0].Type)
		assert.Equal(t, "COMP-WY4M717J6J", first.Resources[0].Name)
	})

	t.Run("host and device should become resources", func(t *testing.T) {
		series, err := ParseMetricSeriesV1(api.Payload{
			Data:     []byte(`{"series":[{"metric":"system.disk.free","points":[[1697177070,1]],"host":"my-host","device":"/dev/sda1","type":"gauge"}]}`),
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		require.Len(t, series, 1)

		assert.Equal(t, []*metricspb.MetricPayload_Resource{
			{Type: "host", Name: "my-host"},
			{Type: "device", Name: "/dev/sda1"},
		}, series[0].Resources)
	})

	t.Run("absent device should not produce a resource", func(t *testing.T) {
		series, err := ParseMetricSeriesV1(api.Payload{
			Data:     []byte(`{"series":[{"metric":"system.disk.free","points":[[1697177070,1]],"host":"my-host","type":"gauge"}]}`),
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		require.Len(t, series, 1)

		assert.Equal(t, []*metricspb.MetricPayload_Resource{
			{Type: "host", Name: "my-host"},
		}, series[0].Resources)
	})

	t.Run("origin metadata should be carried over", func(t *testing.T) {
		series, err := ParseMetricSeriesV1(api.Payload{
			Data:     []byte(`{"series":[{"metric":"a.b","points":[[1697177070,1]],"metadata":{"origin":{"product":10,"category":11,"service":12}}}]}`),
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		require.Len(t, series, 1)

		assert.Equal(t, &metricspb.Metadata{Origin: &metricspb.Origin{
			OriginProduct:  10,
			OriginCategory: 11,
			OriginService:  12,
		}}, series[0].Metadata)
	})

	t.Run("absent origin metadata should stay absent", func(t *testing.T) {
		series, err := ParseMetricSeriesV1(api.Payload{
			Data:     []byte(`{"series":[{"metric":"a.b","points":[[1697177070,1]]}]}`),
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		require.Len(t, series, 1)
		assert.Nil(t, series[0].Metadata)
	})

	t.Run("non-numeric point should error", func(t *testing.T) {
		_, err := ParseMetricSeriesV1(api.Payload{
			Data:     []byte(`{"series":[{"metric":"a.b","points":[["not-a-number",1]]}]}`),
			Encoding: encodingJSON,
		})
		assert.Error(t, err)
	})
}

func TestV1MetricType(t *testing.T) {
	assert.Equal(t, metricspb.MetricPayload_COUNT, v1MetricType(Count))
	assert.Equal(t, metricspb.MetricPayload_RATE, v1MetricType(Rate))
	assert.Equal(t, metricspb.MetricPayload_GAUGE, v1MetricType(Gauge))
	assert.Equal(t, metricspb.MetricPayload_UNSPECIFIED, v1MetricType("something-new"))
}
