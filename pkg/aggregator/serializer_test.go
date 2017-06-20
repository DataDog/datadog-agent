package aggregator

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/percentile"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
)

func TestMarshalSeries(t *testing.T) {
	series := []*Serie{{
		contextKey: "test_context",
		Points: []Point{
			{int64(12345), float64(21.21)},
			{int64(67890), float64(12.12)},
		},
		MType: APIGaugeType,
		Name:  "test.metrics",
		Host:  "localHost",
		Tags:  []string{"tag1", "tag2:yes"},
	}}

	payload, err := MarshalSeries(series)
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

func TestMarshalServiceChecks(t *testing.T) {
	serviceChecks := []*ServiceCheck{{
		CheckName: "test.check",
		Host:      "test.localhost",
		Ts:        1000,
		Status:    ServiceCheckOK,
		Message:   "this is fine",
		Tags:      []string{"tag1", "tag2:yes"},
	}}

	payload, err := MarshalServiceChecks(serviceChecks)
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	newPayload := &agentpayload.ServiceChecksPayload{}
	err = proto.Unmarshal(payload, newPayload)
	assert.Nil(t, err)

	require.Len(t, newPayload.ServiceChecks, 1)
	assert.Equal(t, newPayload.ServiceChecks[0].Name, "test.check")
	assert.Equal(t, newPayload.ServiceChecks[0].Host, "test.localhost")
	assert.Equal(t, newPayload.ServiceChecks[0].Ts, int64(1000))
	assert.Equal(t, newPayload.ServiceChecks[0].Status, int32(ServiceCheckOK))
	assert.Equal(t, newPayload.ServiceChecks[0].Message, "this is fine")
	require.Len(t, newPayload.ServiceChecks[0].Tags, 2)
	assert.Equal(t, newPayload.ServiceChecks[0].Tags[0], "tag1")
	assert.Equal(t, newPayload.ServiceChecks[0].Tags[1], "tag2:yes")
}

func TestMarshalEvents(t *testing.T) {
	events := []*Event{{
		Title:          "test title",
		Text:           "test text",
		Ts:             12345,
		Priority:       EventPriorityNormal,
		Host:           "test.localhost",
		Tags:           []string{"tag1", "tag2:yes"},
		AlertType:      EventAlertTypeError,
		AggregationKey: "test aggregation",
		SourceTypeName: "test source",
	}}

	payload, err := MarshalEvents(events)
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	newPayload := &agentpayload.EventsPayload{}
	err = proto.Unmarshal(payload, newPayload)
	assert.Nil(t, err)

	require.Len(t, newPayload.Events, 1)
	assert.Equal(t, newPayload.Events[0].Title, "test title")
	assert.Equal(t, newPayload.Events[0].Text, "test text")
	assert.Equal(t, newPayload.Events[0].Ts, int64(12345))
	assert.Equal(t, newPayload.Events[0].Priority, string(EventPriorityNormal))
	assert.Equal(t, newPayload.Events[0].Host, "test.localhost")
	require.Len(t, newPayload.Events[0].Tags, 2)
	assert.Equal(t, newPayload.Events[0].Tags[0], "tag1")
	assert.Equal(t, newPayload.Events[0].Tags[1], "tag2:yes")
	assert.Equal(t, newPayload.Events[0].AlertType, string(EventAlertTypeError))
	assert.Equal(t, newPayload.Events[0].AggregationKey, "test aggregation")
	assert.Equal(t, newPayload.Events[0].SourceTypeName, "test source")
}

func TestMarshalJSONSeries(t *testing.T) {
	series := []*Serie{{
		contextKey: "test_context",
		Points: []Point{
			{int64(12345), float64(21.21)},
			{int64(67890), float64(12.12)},
		},
		MType: APIGaugeType,
		Name:  "test.metrics",
		Host:  "localHost",
		Tags:  []string{"tag1", "tag2:yes"},
	}}

	payload, err := MarshalJSONSeries(series)
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("{\"series\":[{\"metric\":\"test.metrics\",\"points\":[[12345,21.21],[67890,12.12]],\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"type\":\"gauge\",\"interval\":0}]}\n"))
}

func TestMarshalJSONServiceChecks(t *testing.T) {
	serviceChecks := []ServiceCheck{{
		CheckName: "my_service.can_connect",
		Host:      "my-hostname",
		Ts:        int64(12345),
		Status:    ServiceCheckOK,
		Message:   "my_service is up",
		Tags:      []string{"tag1", "tag2:yes"},
	}}

	payload, err := MarshalJSONServiceChecks(serviceChecks)
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("[{\"check\":\"my_service.can_connect\",\"host_name\":\"my-hostname\",\"timestamp\":12345,\"status\":0,\"message\":\"my_service is up\",\"tags\":[\"tag1\",\"tag2:yes\"]}]\n"))
}

func TestMarshalJSONEvents(t *testing.T) {
	events := []Event{{
		Title:          "An event occurred",
		Text:           "event description",
		Ts:             12345,
		Priority:       EventPriorityNormal,
		Host:           "my-hostname",
		Tags:           []string{"tag1", "tag2:yes"},
		AlertType:      EventAlertTypeError,
		AggregationKey: "my_agg_key",
		SourceTypeName: "custom_source_type",
	}}

	payload, err := MarshalJSONEvents(events, "testapikey", "test-hostname")
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("{\"apiKey\":\"testapikey\",\"events\":{\"custom_source_type\":[{\"msg_title\":\"An event occurred\",\"msg_text\":\"event description\",\"timestamp\":12345,\"priority\":\"normal\",\"host\":\"my-hostname\",\"tags\":[\"tag1\",\"tag2:yes\"],\"alert_type\":\"error\",\"aggregation_key\":\"my_agg_key\",\"source_type_name\":\"custom_source_type\"}]},\"internalHostname\":\"test-hostname\"}\n"))
}

func TestMarshalJSONEventsOmittedFields(t *testing.T) {
	events := []Event{{
		// Don't populate optional fields
		Title: "An event occurred",
		Text:  "event description",
		Ts:    12345,
		Host:  "my-hostname",
	}}

	payload, err := MarshalJSONEvents(events, "testapikey", "test-hostname")
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	// These optional fields are not present in the serialized payload, and a default source type name is used
	assert.Equal(t, payload, []byte("{\"apiKey\":\"testapikey\",\"events\":{\"api\":[{\"msg_title\":\"An event occurred\",\"msg_text\":\"event description\",\"timestamp\":12345,\"host\":\"my-hostname\"}]},\"internalHostname\":\"test-hostname\"}\n"))
}

func TestMarshalJSONSketchSeries(t *testing.T) {
	sketch1 := QSketch{
		percentile.GKArray{
			Entries: []percentile.Entry{{1, 1, 0}},
			Min:     1, ValCount: 1}}
	sketch2 := QSketch{
		percentile.GKArray{
			Entries: []percentile.Entry{{10, 1, 0}, {14, 3, 0}, {21, 2, 0}},
			Min:     10, ValCount: 6}}
	series := []*SketchSerie{{
		contextKey: "test_context",
		Sketches:   []Sketch{{int64(12345), sketch1}, {int64(67890), sketch2}},
		Name:       "test.metrics",
		Host:       "localHost",
		Tags:       []string{"tag1", "tag2:yes"},
	}}

	payload, err := MarshalJSONSketchSeries(series)
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	expectedPayload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"min\":1,\"n\":1}},{\"timestamp\":67890,\"qsketch\":{\"entries\":[[10,1,0],[14,3,0],[21,2,0]],\"min\":10,\"n\":6}}]}]}\n")
	assert.Equal(t, payload, []byte(expectedPayload))
}

func TestUnmarshalJSONSketchSeries(t *testing.T) {

	payload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"min\":1,\"n\":1}},{\"timestamp\":67890,\"qsketch\":{\"entries\":[[10,1,0],[14,3,0],[21,2,0]],\"min\":10,\"n\":6}}]}]}\n")

	sketch1 := QSketch{
		percentile.GKArray{
			Entries: []percentile.Entry{{1, 1, 0}},
			Min:     1, ValCount: 1}}
	sketch2 := QSketch{
		percentile.GKArray{
			Entries: []percentile.Entry{{10, 1, 0}, {14, 3, 0}, {21, 2, 0}},
			Min:     10, ValCount: 6}}
	expectedSeries := SketchSerie{
		Sketches: []Sketch{{int64(12345), sketch1}, {int64(67890), sketch2}},
		Name:     "test.metrics",
		Host:     "localHost",
		Tags:     []string{"tag1", "tag2:yes"},
	}

	data, err := UnmarshalJSONSketchSeries(payload)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(data))
	AssertSketchSerieEqual(t, &expectedSeries, data[0])
}
