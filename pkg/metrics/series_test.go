// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//+build zlib

package metrics

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	jsoniter "github.com/json-iterator/go"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/jsonstream"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			[]string{"some:tag", "device_name:/dev/sda1"},
			[]string{"some:tag", "device_name:/dev/sda1"},
			"/dev/sda1",
		},
		{
			[]string{"some:tag", "device:/dev/sda1", "device_name:sda1"},
			[]string{"some:tag", "device_name:sda1"},
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
			s := &Serie{Tags: []string{}}
			for _, t := range tc.Tags {
				s.Tags = append(s.Tags, t)
			}

			// Run a few times to ensure stability
			for i := 0; i < 4; i++ {
				populateDeviceField(s)
				assert.Equal(t, tc.ExpectedTags, s.Tags)
				assert.Equal(t, tc.ExpectedDevice, s.Device)
			}

		})
	}
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

func TestStreamJSONMarshaler(t *testing.T) {
	series := Series{
		{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType:    APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		},
		{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType:    APIRateType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		},
		{
			Points:   []Point{},
			MType:    APICountType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags:     []string{},
		},
	}

	stream := jsoniter.NewStream(jsoniter.ConfigDefault, nil, 0)

	assert.Equal(t, 3, series.Len())

	series.WriteHeader(stream)
	assert.Equal(t, []byte(`{"series":[`), stream.Buffer())
	stream.Reset(nil)

	series.WriteFooter(stream)
	assert.Equal(t, []byte(`]}`), stream.Buffer())
	stream.Reset(nil)

	// Access an out-of-bounds item
	err := series.WriteItem(stream, 10)
	assert.EqualError(t, err, "out of range")
	err = series.WriteItem(stream, -10)
	assert.EqualError(t, err, "out of range")

	// Test each item type
	for i := range series {
		stream.Reset(nil)
		err = series.WriteItem(stream, i)
		assert.NoError(t, err)

		// Make sure the output is valid and matches the original item
		item := &Serie{}
		err = json.Unmarshal(stream.Buffer(), item)
		assert.NoError(t, err)
		assert.EqualValues(t, series[i], item)
	}
}

func TestStreamJSONMarshalerWithDevice(t *testing.T) {
	series := Series{
		{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType:    APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes", "device:/dev/sda1"},
		},
	}

	stream := jsoniter.NewStream(jsoniter.ConfigDefault, nil, 0)

	err := series.WriteItem(stream, 0)
	assert.NoError(t, err)

	// Make sure the output is valid and fields are as expected
	item := &Serie{}
	err = json.Unmarshal(stream.Buffer(), item)
	assert.NoError(t, err)
	assert.Equal(t, item.Device, "/dev/sda1")
	assert.Equal(t, item.Tags, []string{"tag1", "tag2:yes"})
}

func TestDescribeItem(t *testing.T) {
	series := Series{
		{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType:    APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes", "device:/dev/sda1"},
		},
	}

	desc1 := series.DescribeItem(0)
	assert.Equal(t, "name \"test.metrics\", 2 points", desc1)

	// Out of range
	desc2 := series.DescribeItem(2)
	assert.Equal(t, "out of range", desc2)
}

// test taken from the spliter
func TestPayloadsSeries(t *testing.T) {
	testSeries := Series{}
	for i := 0; i < 30000; i++ {
		point := Serie{
			Points: []Point{
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
			MType:    APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		}
		testSeries = append(testSeries, &point)
	}

	originalLength := len(testSeries)
	builder := jsonstream.NewPayloadBuilder()
	payloads, err := builder.Build(testSeries)
	require.Nil(t, err)
	var splitSeries = []Series{}
	for _, compressedPayload := range payloads {
		payload, err := decompressPayload(*compressedPayload)
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

var result forwarder.Payloads

func BenchmarkPayloadsSeries(b *testing.B) {
	testSeries := Series{}
	for i := 0; i < 400000; i++ {
		point := Serie{
			Points: []Point{
				{Ts: 12345.0, Value: 1.2 * float64(i)},
			},
			MType:    APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		}
		testSeries = append(testSeries, &point)
	}

	var r forwarder.Payloads
	builder := jsonstream.NewPayloadBuilder()
	for n := 0; n < b.N; n++ {
		// always record the result of Payloads to prevent
		// the compiler eliminating the function call.
		r, _ = builder.Build(testSeries)
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
		compressedSize += len(*p)
	}
	if compressedSize > 3000000 {
		panic(fmt.Sprintf("expecting no more than 3 MB, got %d", compressedSize))
	}
	// always store the result to a package level variable
	// so the compiler cannot eliminate the Benchmark itself.
	result = r
}

func decompressPayload(payload []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	dst, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return dst, nil
}
