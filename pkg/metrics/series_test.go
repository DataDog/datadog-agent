// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package metrics

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
)

func TestMarshalSeries(t *testing.T) {
	series := Series{{
		Points: []Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType: APIGaugeType,
		Name:  "test.metrics",
		Host:  "localHost",
		Tags:  []string{"tag1", "tag2:yes"},
	}}

	payload, err := series.Marshal()
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	newPayload := &agentpayload.MetricsPayload{}
	err = proto.Unmarshal(payload, newPayload)
	assert.Nil(t, err)

	require.Len(t, newPayload.Samples, 1)
	assert.Equal(t, newPayload.Samples[0].Metric, "test.metrics")
	assert.Equal(t, newPayload.Samples[0].Type, "gauge")
	assert.Equal(t, newPayload.Samples[0].Host, "localHost")
	require.Len(t, newPayload.Samples[0].Tags, 2)
	assert.Equal(t, newPayload.Samples[0].Tags[0], "tag1")
	assert.Equal(t, newPayload.Samples[0].Tags[1], "tag2:yes")
	require.Len(t, newPayload.Samples[0].Points, 2)
	assert.Equal(t, newPayload.Samples[0].Points[0].Ts, int64(12345))
	assert.Equal(t, newPayload.Samples[0].Points[0].Value, float64(21.21))
	assert.Equal(t, newPayload.Samples[0].Points[1].Ts, int64(67890))
	assert.Equal(t, newPayload.Samples[0].Points[1].Value, float64(12.12))
}

func TestPopulateDeviceField(t *testing.T) {
	series := Series{
		&Serie{},
		&Serie{
			Tags: []string{"some:tag", "device:/dev/sda1"},
		},
		&Serie{
			Tags: []string{"some:tag", "device:/dev/sda2", "some_other:tag"},
		},
		&Serie{
			Tags: []string{"yet_another:value", "one_last:tag_value"},
		}}

	populateDeviceField(series)

	require.Len(t, series, 4)
	assert.Empty(t, series[0].Tags)
	assert.Empty(t, series[0].Device)
	assert.Equal(t, series[1].Tags, []string{"some:tag"})
	assert.Equal(t, series[1].Device, "/dev/sda1")
	assert.Equal(t, series[2].Tags, []string{"some:tag", "some_other:tag"})
	assert.Equal(t, series[2].Device, "/dev/sda2")
	assert.Equal(t, series[3].Tags, []string{"yet_another:value", "one_last:tag_value"})
	assert.Empty(t, series[3].Device)
}

func TestMarshalJSONSeries(t *testing.T) {
	series := Series{{
		Points: []Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:          APIGaugeType,
		Name:           "test.metrics",
		Host:           "localHost",
		Tags:           []string{"tag1", "tag2:yes", "device:/dev/sda1"},
		SourceTypeName: "System",
	}}

	payload, err := series.MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("{\"series\":[{\"metric\":\"test.metrics\",\"points\":[[12345,21.21],[67890,12.12]],\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"device\":\"/dev/sda1\",\"type\":\"gauge\",\"interval\":0,\"source_type_name\":\"System\"}]}\n"))
}

func TestSplitSeries(t *testing.T) {
	var series = Series{}
	for i := 0; i < 2; i++ {
		s := Serie{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: APIGaugeType,
			Name:  "test.metrics",
			Host:  "localHost",
			Tags:  []string{"tag1", "tag2:yes"},
		}
		series = append(series, &s)
	}

	newSeries, err := series.SplitPayload(2)
	require.Nil(t, err)
	require.Len(t, newSeries, 2)
	newSeries, err = series.SplitPayload(3)
	require.Nil(t, err)
	require.Len(t, newSeries, 2)

	series = Series{{
		Points: []Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType: APIGaugeType,
		Name:  "test.metrics",
		Host:  "localHost",
		Tags:  []string{"tag1", "tag2:yes"},
	}}
	newSeries, err = series.SplitPayload(2)
	require.Nil(t, err)
	require.Len(t, newSeries, 2)
	for _, s := range newSeries {
		ser := s.(Series)
		require.Len(t, ser[0].Points, 1)
	}
	newSeries, err = series.SplitPayload(3)
	require.Nil(t, err)
	require.Len(t, newSeries, 2)
	for _, s := range newSeries {
		ser := s.(Series)
		require.Len(t, ser[0].Points, 1)
	}
}

func TestUnmarshalSeriesJSON(t *testing.T) {
	// Test one for each value of the API Type
	series := Series{{
		Points: []Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:    APIGaugeType,
		Name:     "test.metrics",
		Interval: 1,
		Host:     "localHost",
		Tags:     []string{"tag1", "tag2:yes"},
	}, {
		Points: []Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:    APIRateType,
		Name:     "test.metrics",
		Interval: 1,
		Host:     "localHost",
		Tags:     []string{"tag1", "tag2:yes"},
	}, {
		Points: []Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:    APICountType,
		Name:     "test.metrics",
		Interval: 1,
		Host:     "localHost",
		Tags:     []string{"tag1", "tag2:yes"},
	}}

	seriesJSON, err := series.MarshalJSON()
	require.Nil(t, err)
	var newSeries map[string]Series
	err = json.Unmarshal(seriesJSON, &newSeries)
	require.Nil(t, err)

	badPointJSON := []byte(`[12345,21.21,1]`)
	var badPoint Point
	err = json.Unmarshal(badPointJSON, &badPoint)
	require.NotNil(t, err)
}
