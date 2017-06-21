package aggregator

import (
	"testing"

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

func TestPopulateDeviceField(t *testing.T) {
	series := []*Serie{
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
	series := []*Serie{{
		contextKey: "test_context",
		Points: []Point{
			{int64(12345), float64(21.21)},
			{int64(67890), float64(12.12)},
		},
		MType:          APIGaugeType,
		Name:           "test.metrics",
		Host:           "localHost",
		Tags:           []string{"tag1", "tag2:yes", "device:/dev/sda1"},
		SourceTypeName: "System",
	}}

	payload, err := MarshalJSONSeries(series)
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("{\"series\":[{\"metric\":\"test.metrics\",\"points\":[[12345,21.21],[67890,12.12]],\"tags\":[\"tag1\",\"tag2:yes\"],\"host\":\"localHost\",\"device\":\"/dev/sda1\",\"type\":\"gauge\",\"interval\":0,\"source_type_name\":\"System\"}]}\n"))
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
