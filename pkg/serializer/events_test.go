package serializer

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestMarshalEvents(t *testing.T) {
	events := []*metrics.Event{{
		Title:          "test title",
		Text:           "test text",
		Ts:             12345,
		Priority:       metrics.EventPriorityNormal,
		Host:           "test.localhost",
		Tags:           []string{"tag1", "tag2:yes"},
		AlertType:      metrics.EventAlertTypeError,
		AggregationKey: "test aggregation",
		SourceTypeName: "test source",
	}}

	payload, contentType, err := MarshalEvents(events)
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, contentType, "application/x-protobuf")

	newPayload := &agentpayload.EventsPayload{}
	err = proto.Unmarshal(payload, newPayload)
	assert.Nil(t, err)

	require.Len(t, newPayload.Events, 1)
	assert.Equal(t, newPayload.Events[0].Title, "test title")
	assert.Equal(t, newPayload.Events[0].Text, "test text")
	assert.Equal(t, newPayload.Events[0].Ts, int64(12345))
	assert.Equal(t, newPayload.Events[0].Priority, string(metrics.EventPriorityNormal))
	assert.Equal(t, newPayload.Events[0].Host, "test.localhost")
	require.Len(t, newPayload.Events[0].Tags, 2)
	assert.Equal(t, newPayload.Events[0].Tags[0], "tag1")
	assert.Equal(t, newPayload.Events[0].Tags[1], "tag2:yes")
	assert.Equal(t, newPayload.Events[0].AlertType, string(metrics.EventAlertTypeError))
	assert.Equal(t, newPayload.Events[0].AggregationKey, "test aggregation")
	assert.Equal(t, newPayload.Events[0].SourceTypeName, "test source")
}

func TestMarshalJSONEvents(t *testing.T) {
	events := []metrics.Event{{
		Title:          "An event occurred",
		Text:           "event description",
		Ts:             12345,
		Priority:       metrics.EventPriorityNormal,
		Host:           "my-hostname",
		Tags:           []string{"tag1", "tag2:yes"},
		AlertType:      metrics.EventAlertTypeError,
		AggregationKey: "my_agg_key",
		SourceTypeName: "custom_source_type",
	}}

	payload, contentType, err := MarshalJSONEvents(events, "testapikey", "test-hostname")
	assert.Nil(t, err)
	assert.Equal(t, contentType, "application/json")
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("{\"apiKey\":\"testapikey\",\"events\":{\"custom_source_type\":[{\"msg_title\":\"An event occurred\",\"msg_text\":\"event description\",\"timestamp\":12345,\"priority\":\"normal\",\"host\":\"my-hostname\",\"tags\":[\"tag1\",\"tag2:yes\"],\"alert_type\":\"error\",\"aggregation_key\":\"my_agg_key\",\"source_type_name\":\"custom_source_type\"}]},\"internalHostname\":\"test-hostname\"}\n"))
}

func TestMarshalJSONEventsOmittedFields(t *testing.T) {
	events := []metrics.Event{{
		// Don't populate optional fields
		Title: "An event occurred",
		Text:  "event description",
		Ts:    12345,
		Host:  "my-hostname",
	}}

	payload, contentType, err := MarshalJSONEvents(events, "testapikey", "test-hostname")
	assert.Nil(t, err)
	assert.Equal(t, contentType, "application/json")
	assert.NotNil(t, payload)
	// These optional fields are not present in the serialized payload, and a default source type name is used
	assert.Equal(t, payload, []byte("{\"apiKey\":\"testapikey\",\"events\":{\"api\":[{\"msg_title\":\"An event occurred\",\"msg_text\":\"event description\",\"timestamp\":12345,\"host\":\"my-hostname\"}]},\"internalHostname\":\"test-hostname\"}\n"))
}
