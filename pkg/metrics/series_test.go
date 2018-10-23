// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
	tags := [][]string{
		{},
		{"some:tag", "device:/dev/sda1"},
		{"some:tag", "device:/dev/sda2", "some_other:tag"},
		{"yet_another:value", "one_last:tag_value"},
	}
	series := Series{
		&Serie{},
		&Serie{
			Tags: tags[1],
		},
		&Serie{
			Tags: tags[2],
		},
		&Serie{
			Tags: tags[3],
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

	series = Series{
		&Serie{},
		&Serie{
			Tags: tags[1],
		},
		&Serie{
			Tags: tags[2],
		},
		&Serie{
			Tags: tags[3],
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
	assert.JSONEq(t, string(payload), "{\"series\":[{\"metric\":\"test.metrics\",\"points\":[[12345,21.21],[67890,12.12]],\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"device\":\"/dev/sda1\",\"type\":\"gauge\",\"interval\":0,\"source_type_name\":\"System\"}]}\n")
}

func TestSplitSerieasOneMetric(t *testing.T) {
	s := Series{
		{Points: []Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
			MType: APIGaugeType,
			Name:  "test.metrics",
			Host:  "localHost",
			Tags:  []string{"tag1", "tag2:yes"},
		},
		{Points: []Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
			MType: APIGaugeType,
			Name:  "test.metrics",
			Host:  "localHost",
			Tags:  []string{"tag3"},
		},
	}

	// One metric should not be splitable
	res, err := s.SplitPayload(2)
	assert.Nil(t, res)
	assert.NotNil(t, err)
}

func TestSplitSerieasByName(t *testing.T) {
	var series = Series{}
	for _, name := range []string{"name1", "name2", "name3"} {
		s1 := Serie{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: APIGaugeType,
			Name:  name,
			Host:  "localHost",
			Tags:  []string{"tag1", "tag2:yes"},
		}
		series = append(series, &s1)
		s2 := Serie{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: APIGaugeType,
			Name:  name,
			Host:  "localHost",
			Tags:  []string{"tag3"},
		}
		series = append(series, &s2)
	}

	// splitting 3 group of 2 series in two should not be possible. We
	// should endup we 3 groups
	res, err := series.SplitPayload(2)
	assert.Nil(t, err)
	require.Len(t, res, 3)
	// Test grouping by name works
	assert.Equal(t, res[0].(Series)[0].Name, res[0].(Series)[1].Name)
	assert.Equal(t, res[1].(Series)[0].Name, res[1].(Series)[1].Name)
	assert.Equal(t, res[2].(Series)[0].Name, res[2].(Series)[1].Name)
}

func TestSplitOversizedMetric(t *testing.T) {
	var series = Series{
		{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: APIGaugeType,
			Name:  "test.test1",
			Host:  "localHost",
			Tags:  []string{"tag1", "tag2:yes"},
		},
	}
	for _, tag := range []string{"tag1", "tag2", "tag3"} {
		series = append(series, &Serie{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: APIGaugeType,
			Name:  "test.test2",
			Host:  "localHost",
			Tags:  []string{tag},
		})
	}

	// splitting 3 group of 2 series in two should not be possible. We
	// should endup we 3 groups
	res, err := series.SplitPayload(2)
	assert.Nil(t, err)
	require.Len(t, res, 2)
	// Test grouping by name works
	if !((len(res[0].(Series)) == 1 && len(res[1].(Series)) == 3) ||
		(len(res[1].(Series)) == 1 && len(res[0].(Series)) == 3)) {
		assert.Fail(t, "Oversized metric was split among multiple payload")
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
