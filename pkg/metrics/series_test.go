// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestPointMarshalJSON(t *testing.T) {
	p := Point{Ts: 1234567890.0, Value: 42.5}
	data, err := p.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, "[1234567890, 42.5]", string(data))
}

func TestPointMarshalJSONInteger(t *testing.T) {
	p := Point{Ts: 1234567890.0, Value: 100.0}
	data, err := p.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, "[1234567890, 100]", string(data))
}

func TestPointUnmarshalJSON(t *testing.T) {
	var p Point
	err := json.Unmarshal([]byte("[1234567890, 42.5]"), &p)
	require.NoError(t, err)
	assert.Equal(t, 1234567890.0, p.Ts)
	assert.Equal(t, 42.5, p.Value)
}

func TestPointUnmarshalJSONInvalid(t *testing.T) {
	var p Point
	err := json.Unmarshal([]byte("[1234567890]"), &p)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wrong number of fields")
}

func TestPointUnmarshalJSONMalformed(t *testing.T) {
	var p Point
	err := json.Unmarshal([]byte("not json"), &p)
	assert.Error(t, err)
}

func TestAPIMetricTypeSeriesAPIV2Enum(t *testing.T) {
	tests := []struct {
		name     string
		apiType  APIMetricType
		expected int32
	}{
		{"count", APICountType, 1},
		{"rate", APIRateType, 2},
		{"gauge", APIGaugeType, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.apiType.SeriesAPIV2Enum())
		})
	}
}

func TestAPIMetricTypeSeriesAPIV2EnumPanic(t *testing.T) {
	invalidType := APIMetricType(999)
	assert.Panics(t, func() {
		invalidType.SeriesAPIV2Enum()
	})
}

func TestSerieGetName(t *testing.T) {
	serie := &Serie{Name: "test.metric"}
	assert.Equal(t, "test.metric", serie.GetName())
}

func TestSerieString(t *testing.T) {
	serie := Serie{
		Name:   "test.metric",
		Host:   "test-host",
		MType:  APIGaugeType,
		Points: []Point{{Ts: 1234567890.0, Value: 42.0}},
	}
	str := serie.String()
	assert.Contains(t, str, "test.metric")
	assert.Contains(t, str, "test-host")
	assert.Contains(t, str, "gauge")
}

func TestSeriePopulateDeviceField(t *testing.T) {
	tests := []struct {
		name           string
		tags           []string
		expectedDevice string
		expectedTags   int
	}{
		{
			name:           "with device tag",
			tags:           []string{"env:prod", "device:sda", "service:web"},
			expectedDevice: "sda",
			expectedTags:   2,
		},
		{
			name:           "without device tag",
			tags:           []string{"env:prod", "service:web"},
			expectedDevice: "",
			expectedTags:   2,
		},
		{
			name:           "empty tags",
			tags:           []string{},
			expectedDevice: "",
			expectedTags:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serie := &Serie{
				Tags: tagset.CompositeTagsFromSlice(tc.tags),
			}
			serie.PopulateDeviceField()
			assert.Equal(t, tc.expectedDevice, serie.Device)
			assert.Equal(t, tc.expectedTags, serie.Tags.Len())
		})
	}
}

func TestSeriePopulateResources(t *testing.T) {
	tests := []struct {
		name              string
		tags              []string
		expectedResources []Resource
		expectedTags      int
	}{
		{
			name:              "with resource tag",
			tags:              []string{"env:prod", "dd.internal.resource:container:abc123", "service:web"},
			expectedResources: []Resource{{Type: "container", Name: "abc123"}},
			expectedTags:      2,
		},
		{
			name:              "with multiple resource tags",
			tags:              []string{"dd.internal.resource:host:myhost", "dd.internal.resource:container:abc123"},
			expectedResources: []Resource{{Type: "host", Name: "myhost"}, {Type: "container", Name: "abc123"}},
			expectedTags:      0,
		},
		{
			name:              "without resource tag",
			tags:              []string{"env:prod", "service:web"},
			expectedResources: nil,
			expectedTags:      2,
		},
		{
			name:              "invalid resource tag format - no colon",
			tags:              []string{"dd.internal.resource:invalidformat"},
			expectedResources: nil,
			expectedTags:      0,
		},
		{
			name:              "invalid resource tag format - trailing colon",
			tags:              []string{"dd.internal.resource:type:"},
			expectedResources: nil,
			expectedTags:      0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serie := &Serie{
				Tags: tagset.CompositeTagsFromSlice(tc.tags),
			}
			serie.PopulateResources()
			assert.Equal(t, tc.expectedResources, serie.Resources)
			assert.Equal(t, tc.expectedTags, serie.Tags.Len())
		})
	}
}

func TestSeriesAppend(t *testing.T) {
	var series Series
	serie1 := &Serie{Name: "metric1"}
	serie2 := &Serie{Name: "metric2"}

	series.Append(serie1)
	series.Append(serie2)

	assert.Len(t, series, 2)
	assert.Equal(t, "metric1", series[0].Name)
	assert.Equal(t, "metric2", series[1].Name)
}

func TestSeriesMarshalStrings(t *testing.T) {
	series := Series{
		&Serie{
			Name:   "metric.b",
			MType:  APIGaugeType,
			Points: []Point{{Ts: 1234567890.0, Value: 42.0}},
			Tags:   tagset.CompositeTagsFromSlice([]string{"env:prod"}),
		},
		&Serie{
			Name:   "metric.a",
			MType:  APICountType,
			Points: []Point{{Ts: 1234567891.0, Value: 10.0}},
			Tags:   tagset.CompositeTagsFromSlice([]string{"env:dev"}),
		},
	}

	headers, payload := series.MarshalStrings()

	assert.Equal(t, []string{"Metric", "Type", "Timestamp", "Value", "Tags"}, headers)
	assert.Len(t, payload, 2)
	// Should be sorted by metric name
	assert.Equal(t, "metric.a", payload[0][0])
	assert.Equal(t, "metric.b", payload[1][0])
}

func TestSeriesMarshalStringsSameMetricDifferentTimestamp(t *testing.T) {
	series := Series{
		&Serie{
			Name:   "metric.a",
			MType:  APIGaugeType,
			Points: []Point{{Ts: 1234567892.0, Value: 42.0}},
			Tags:   tagset.CompositeTagsFromSlice([]string{"env:prod"}),
		},
		&Serie{
			Name:   "metric.a",
			MType:  APIGaugeType,
			Points: []Point{{Ts: 1234567890.0, Value: 10.0}},
			Tags:   tagset.CompositeTagsFromSlice([]string{"env:dev"}),
		},
	}

	_, payload := series.MarshalStrings()

	// Should be sorted by timestamp when metric names are equal
	assert.Equal(t, "1234567890", payload[0][2])
	assert.Equal(t, "1234567892", payload[1][2])
}

func TestSeriesMarshalStringsEmpty(t *testing.T) {
	series := Series{}
	headers, payload := series.MarshalStrings()

	assert.Equal(t, []string{"Metric", "Type", "Timestamp", "Value", "Tags"}, headers)
	assert.Empty(t, payload)
}
