package serializer

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestMarshalSeries(t *testing.T) {
	series := []*metrics.Serie{{
		Points: []metrics.Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType: metrics.APIGaugeType,
		Name:  "test.metrics",
		Host:  "localHost",
		Tags:  []string{"tag1", "tag2:yes"},
	}}

	payload, contentType, err := MarshalSeries(series)
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, contentType, "application/x-protobuf")

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
	series := []*metrics.Serie{
		&metrics.Serie{},
		&metrics.Serie{
			Tags: []string{"some:tag", "device:/dev/sda1"},
		},
		&metrics.Serie{
			Tags: []string{"some:tag", "device:/dev/sda2", "some_other:tag"},
		},
		&metrics.Serie{
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
	series := []*metrics.Serie{{
		Points: []metrics.Point{
			{Ts: 12345.0, Value: float64(21.21)},
			{Ts: 67890.0, Value: float64(12.12)},
		},
		MType:          metrics.APIGaugeType,
		Name:           "test.metrics",
		Host:           "localHost",
		Tags:           []string{"tag1", "tag2:yes", "device:/dev/sda1"},
		SourceTypeName: "System",
	}}

	payload, contentType, err := MarshalJSONSeries(series)
	assert.Nil(t, err)
	assert.Equal(t, contentType, "application/json")
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("{\"series\":[{\"metric\":\"test.metrics\",\"points\":[[12345,21.21],[67890,12.12]],\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"device\":\"/dev/sda1\",\"type\":\"gauge\",\"interval\":0,\"source_type_name\":\"System\"}]}\n"))
}
