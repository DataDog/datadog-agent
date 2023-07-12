// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && test

package metrics

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestPopulateDeviceField(t *testing.T) {
	for _, tc := range []struct {
		Tags           []string
		ExpectedTags   []string
		ExpectedDevice string
	}{
		{
			[]string{"some:tag", "device:/dev/sda1"},
			[]string{"some:tag"},
			"/dev/sda1",
		},
		{
			[]string{"some:tag", "device:/dev/sda2", "some_other:tag"},
			[]string{"some:tag", "some_other:tag"},
			"/dev/sda2",
		},
		{
			[]string{"yet_another:value", "one_last:tag_value", "long:array", "very_long:array", "many:tags", "such:wow"},
			[]string{"yet_another:value", "one_last:tag_value", "long:array", "very_long:array", "many:tags", "such:wow"},
			"",
		},
	} {
		t.Run(fmt.Sprintf(""), func(t *testing.T) {
			s := &metrics.Serie{Tags: tagset.CompositeTagsFromSlice(tc.Tags)}

			// Run a few times to ensure stability
			for i := 0; i < 4; i++ {
				s.PopulateDeviceField()
				assert.Equal(t, strings.Join(tc.ExpectedTags, ","), s.Tags.Join(","))
				assert.Equal(t, tc.ExpectedDevice, s.Device)
			}

		})
	}
}

func TestPopulateResources(t *testing.T) {
	for _, tc := range []struct {
		Tags              []string
		ExpectedTags      []string
		ExpectedResources []metrics.Resource
	}{
		{
			[]string{"some:tag", "dd.internal.resource:aws_rds_instance:some_instance_endpoint"},
			[]string{"some:tag"},
			[]metrics.Resource{{
				Type: "aws_rds_instance",
				Name: "some_instance_endpoint",
			}},
		},
		{
			[]string{"some:tag", "dd.internal.resource:database_instance:some_db_host", "dd.internal.resource:aws_rds_instance:some_instance_endpoint", "some_other:tag"},
			[]string{"some:tag", "some_other:tag"},
			[]metrics.Resource{
				{
					Type: "database_instance",
					Name: "some_db_host",
				},
				{
					Type: "aws_rds_instance",
					Name: "some_instance_endpoint",
				}},
		},
		{
			[]string{"some:tag", "dd.internal.resource:database_instance:some_db_host", "resource:some_resource_value", "some_other:tag"},
			[]string{"some:tag", "resource:some_resource_value", "some_other:tag"},
			[]metrics.Resource{
				{
					Type: "database_instance",
					Name: "some_db_host",
				},
			},
		},
		{
			[]string{"some:tag", "dd.internal.resource:wrong_resource_format", "some_other:tag"},
			[]string{"some:tag", "some_other:tag"},
			nil,
		},
		{
			[]string{"some:tag", "dd.internal.resource:type_without_value:", "some_other:tag"},
			[]string{"some:tag", "some_other:tag"},
			nil,
		},
		{
			[]string{"yet_another:value", "one_last:tag_value", "long:array", "very_long:array", "many:tags", "such:wow"},
			[]string{"yet_another:value", "one_last:tag_value", "long:array", "very_long:array", "many:tags", "such:wow"},
			nil,
		},
	} {
		t.Run(fmt.Sprintf(""), func(t *testing.T) {
			s := &metrics.Serie{Tags: tagset.CompositeTagsFromSlice(tc.Tags)}

			// Run a few times to ensure stability
			for i := 0; i < 4; i++ {
				s.PopulateResources()
				assert.Equal(t, strings.Join(tc.ExpectedTags, ","), s.Tags.Join(","))
				assert.Equal(t, tc.ExpectedResources, s.Resources)
			}

		})
	}
}

func TestMarshalJSONSeries(t *testing.T) {
	series := Series{{
		Points: []metrics.Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:          metrics.APIGaugeType,
		Name:           "test.metrics",
		Host:           "localHost",
		Tags:           tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes", "device:/dev/sda1"}),
		SourceTypeName: "System",
	}}

	payload, err := series.MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("{\"series\":[{\"metric\":\"test.metrics\",\"points\":[[12345,21.21],[67890,12.12]],\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"device\":\"/dev/sda1\",\"type\":\"gauge\",\"interval\":0,\"source_type_name\":\"System\"}]}\n"))
}

func TestSplitSerieasOneMetric(t *testing.T) {
	s := Series{
		{Points: []metrics.Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
			MType: metrics.APIGaugeType,
			Name:  "test.metrics",
			Host:  "localHost",
			Tags:  tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
		},
		{Points: []metrics.Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
			MType: metrics.APIGaugeType,
			Name:  "test.metrics",
			Host:  "localHost",
			Tags:  tagset.CompositeTagsFromSlice([]string{"tag3"}),
		},
	}

	// One metric should not be splitable
	res, err := s.SplitPayload(2)
	assert.Nil(t, res)
	assert.NotNil(t, err)
}

func TestSplitSeriesByName(t *testing.T) {
	var series = Series{}
	for _, name := range []string{"name1", "name2", "name3"} {
		s1 := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: metrics.APIGaugeType,
			Name:  name,
			Host:  "localHost",
			Tags:  tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
		}
		series = append(series, &s1)
		s2 := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: metrics.APIGaugeType,
			Name:  name,
			Host:  "localHost",
			Tags:  tagset.CompositeTagsFromSlice([]string{"tag3"}),
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
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: metrics.APIGaugeType,
			Name:  "test.test1",
			Host:  "localHost",
			Tags:  tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
		},
	}
	for _, tag := range []string{"tag1", "tag2", "tag3"} {
		series = append(series, &metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType: metrics.APIGaugeType,
			Name:  "test.test2",
			Host:  "localHost",
			Tags:  tagset.CompositeTagsFromSlice([]string{tag}),
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
		Points: []metrics.Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:    metrics.APIGaugeType,
		Name:     "test.metrics",
		Interval: 1,
		Host:     "localHost",
		Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
	}, {
		Points: []metrics.Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:    metrics.APIRateType,
		Name:     "test.metrics",
		Interval: 1,
		Host:     "localHost",
		Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
	}, {
		Points: []metrics.Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:    metrics.APICountType,
		Name:     "test.metrics",
		Interval: 1,
		Host:     "localHost",
		Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
	}}

	seriesJSON, err := series.MarshalJSON()
	require.Nil(t, err)
	var newSeries map[string]Series
	err = json.Unmarshal(seriesJSON, &newSeries)
	require.Nil(t, err)

	badPointJSON := []byte(`[12345,21.21,1]`)
	var badPoint metrics.Point
	err = json.Unmarshal(badPointJSON, &badPoint)
	require.NotNil(t, err)
}

func TestStreamJSONMarshalerWithDevice(t *testing.T) {
	series := metrics.Series{
		{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType:    metrics.APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes", "device:/dev/sda1"}),
		},
	}

	stream := jsoniter.NewStream(jsoniter.ConfigDefault, nil, 0)
	serializer := CreateIterableSeries(CreateSerieSource(series))
	serializer.MoveNext()
	err := serializer.WriteCurrentItem(stream)
	assert.NoError(t, err)

	// Make sure the output is valid and fields are as expected
	item := &metrics.Serie{}
	err = json.Unmarshal(stream.Buffer(), item)
	assert.NoError(t, err)
	assert.Equal(t, item.Device, "/dev/sda1")
	assert.Equal(t, item.Tags.Join(","), "tag1,tag2:yes")
}

func TestDescribeItem(t *testing.T) {
	series := metrics.Series{
		{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType:    metrics.APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes", "device:/dev/sda1"}),
		},
	}

	serializer := CreateIterableSeries(CreateSerieSource(series))
	serializer.MoveNext()
	desc1 := serializer.DescribeCurrentItem()
	assert.Equal(t, "name \"test.metrics\", 2 points", desc1)
}

func makeSeries(numItems, numPoints int) *IterableSeries {
	series := make([]*metrics.Serie, 0, numItems)
	for i := 0; i < numItems; i++ {
		series = append(series, &metrics.Serie{
			Points: func() []metrics.Point {
				ps := make([]metrics.Point, numPoints)
				for p := 0; p < numPoints; p++ {
					ps[p] = metrics.Point{Ts: float64(p * i), Value: float64(p + i)}
				}
				return ps
			}(),
			MType:    metrics.APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes", "device:SomeDevice", "dd.internal.resource:device:some_other_device", "dd.internal.resource:database_instance:some_instance", "dd.internal.resource:aws_rds_instance:some_endpoint"}),
		})
	}
	return CreateIterableSeries(CreateSerieSource(series))
}

func TestMarshalSplitCompress(t *testing.T) {
	series := makeSeries(10000, 50)

	payloads, err := series.MarshalSplitCompress(marshaler.NewBufferContext())
	require.NoError(t, err)
	// check that we got multiple payloads, so splitting occurred
	require.Greater(t, len(payloads), 1)
	for _, compressedPayload := range payloads {
		payload, err := decompressPayload(compressedPayload.GetContent())
		require.NoError(t, err)

		pl := new(gogen.MetricPayload)
		err = pl.Unmarshal(payload)
		for _, s := range pl.Series {
			assert.Equal(t, []*gogen.MetricPayload_Resource{{Type: "host", Name: "localHost"}, {Type: "device", Name: "SomeDevice"}, {Type: "device", Name: "some_other_device"}, {Type: "database_instance", Name: "some_instance"}, {Type: "aws_rds_instance", Name: "some_endpoint"}}, s.Resources)
			assert.Equal(t, []string{"tag1", "tag2:yes"}, s.Tags)
		}
		require.NoError(t, err)
	}
}

func TestMarshalSplitCompressPointsLimit(t *testing.T) {
	mockConfig := config.Mock(t)
	oldMax := mockConfig.GetInt("serializer_max_series_points_per_payload")
	defer mockConfig.Set("serializer_max_series_points_per_payload", oldMax)
	mockConfig.Set("serializer_max_series_points_per_payload", 100)

	// ten series, each with 50 points, so two should fit in each payload
	series := makeSeries(10, 50)

	payloads, err := series.MarshalSplitCompress(marshaler.NewBufferContext())
	require.NoError(t, err)
	require.Equal(t, 5, len(payloads))
}

func TestMarshalSplitCompressPointsLimitTooBig(t *testing.T) {
	mockConfig := config.Mock(t)
	oldMax := mockConfig.GetInt("serializer_max_series_points_per_payload")
	defer mockConfig.Set("serializer_max_series_points_per_payload", oldMax)
	mockConfig.Set("serializer_max_series_points_per_payload", 1)

	series := makeSeries(1, 2)
	payloads, err := series.MarshalSplitCompress(marshaler.NewBufferContext())
	require.NoError(t, err)
	require.Len(t, payloads, 0)
}

// test taken from the spliter
func TestPayloadsSeries(t *testing.T) {
	testSeries := metrics.Series{}
	for i := 0; i < 30000; i++ {
		point := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
				{Ts: 2222.0, Value: float64(22.12)},
				{Ts: 333.0, Value: float64(32.12)},
				{Ts: 444444.0, Value: float64(42.12)},
				{Ts: 882787.0, Value: float64(52.12)},
				{Ts: 99990.0, Value: float64(62.12)},
				{Ts: 121212.0, Value: float64(72.12)},
				{Ts: 222227.0, Value: float64(82.12)},
				{Ts: 808080.0, Value: float64(92.12)},
				{Ts: 9090.0, Value: float64(13.12)},
			},
			MType:    metrics.APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
		}
		testSeries = append(testSeries, &point)
	}

	originalLength := len(testSeries)
	builder := stream.NewJSONPayloadBuilder(true)
	iterableSeries := CreateIterableSeries(CreateSerieSource(testSeries))
	payloads, err := builder.BuildWithOnErrItemTooBigPolicy(iterableSeries, stream.DropItemOnErrItemTooBig)
	require.Nil(t, err)
	var splitSeries = []Series{}
	for _, compressedPayload := range payloads {
		payload, err := decompressPayload(compressedPayload.GetContent())
		require.NoError(t, err)

		var s = map[string]Series{}
		err = json.Unmarshal(payload, &s)
		require.NoError(t, err)
		splitSeries = append(splitSeries, s["series"])
	}

	unrolledSeries := Series{}
	for _, series := range splitSeries {
		for _, s := range series {
			unrolledSeries = append(unrolledSeries, s)
		}
	}

	newLength := len(unrolledSeries)
	require.Equal(t, originalLength, newLength)
}

var result transaction.BytesPayloads

func BenchmarkPayloadsSeries(b *testing.B) {
	testSeries := metrics.Series{}
	for i := 0; i < 400000; i++ {
		point := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: 1.2 * float64(i)},
			},
			MType:    metrics.APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
		}
		testSeries = append(testSeries, &point)
	}

	var r transaction.BytesPayloads
	builder := stream.NewJSONPayloadBuilder(true)
	for n := 0; n < b.N; n++ {
		// always record the result of Payloads to prevent
		// the compiler eliminating the function call.
		iterableSeries := CreateIterableSeries(CreateSerieSource(testSeries))
		r, _ = builder.BuildWithOnErrItemTooBigPolicy(iterableSeries, stream.DropItemOnErrItemTooBig)
	}
	// ensure we actually had to split
	if len(r) != 13 {
		panic(fmt.Sprintf("expecting two payloads, got %d", len(r)))
	}
	// test the compressed size
	var compressedSize int
	for _, p := range r {
		if p == nil {
			continue
		}
		compressedSize += len(p.GetContent())
	}
	if compressedSize > 3000000 {
		panic(fmt.Sprintf("expecting no more than 3 MB, got %d", compressedSize))
	}
	// always store the result to a package level variable
	// so the compiler cannot eliminate the Benchmark itself.
	result = r
}
