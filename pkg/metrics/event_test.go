// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/serializer/jsonstream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestMarshal(t *testing.T) {
	events := Events{{
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

	payload, err := events.Marshal()
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

func TestMarshalJSON(t *testing.T) {
	events := Events{{
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

	mockConfig := config.Mock()
	oldName := mockConfig.GetString("hostname")
	defer mockConfig.Set("hostname", oldName)
	mockConfig.Set("hostname", "test-hostname")

	payload, err := events.MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("{\"apiKey\":\"\",\"events\":{\"custom_source_type\":[{\"msg_title\":\"An event occurred\",\"msg_text\":\"event description\",\"timestamp\":12345,\"priority\":\"normal\",\"host\":\"my-hostname\",\"tags\":[\"tag1\",\"tag2:yes\"],\"alert_type\":\"error\",\"aggregation_key\":\"my_agg_key\",\"source_type_name\":\"custom_source_type\"}]},\"internalHostname\":\"test-hostname\"}\n"))
}

func TestMarshalJSONOmittedFields(t *testing.T) {
	events := Events{{
		// Don't populate optional fields
		Title: "An event occurred",
		Text:  "event description",
		Ts:    12345,
		Host:  "my-hostname",
	}}

	mockConfig := config.Mock()
	oldName := mockConfig.GetString("hostname")
	defer mockConfig.Set("hostname", oldName)
	mockConfig.Set("hostname", "test-hostname")

	payload, err := events.MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	// These optional fields are not present in the serialized payload, and a default source type name is used
	assert.Equal(t, payload, []byte("{\"apiKey\":\"\",\"events\":{\"api\":[{\"msg_title\":\"An event occurred\",\"msg_text\":\"event description\",\"timestamp\":12345,\"host\":\"my-hostname\"}]},\"internalHostname\":\"test-hostname\"}\n"))
}

func TestSplitEvents(t *testing.T) {
	var events = Events{}
	for i := 0; i < 2; i++ {
		e := Event{
			Title:          "An event occurred",
			Text:           "event description",
			Ts:             12345,
			Priority:       EventPriorityNormal,
			Host:           "my-hostname",
			Tags:           []string{"tag1", "tag2:yes"},
			AlertType:      EventAlertTypeError,
			AggregationKey: "my_agg_key",
			SourceTypeName: "custom_source_type",
		}
		events = append(events, &e)
	}

	newEvents, err := events.SplitPayload(2)
	require.Nil(t, err)
	require.Len(t, newEvents, 2)

	newEvents, err = events.SplitPayload(3)
	require.Nil(t, err)
	require.Len(t, newEvents, 2)
}

func buildPayload(t *testing.T, m marshaler.StreamJSONMarshaler) [][]byte {
	builder := jsonstream.NewPayloadBuilder()
	payloads, err := builder.Build(m)
	assert.NoError(t, err)
	var uncompressedPayloads [][]byte

	for _, compressedPayload := range payloads {
		payload, err := decompressPayload(*compressedPayload)
		assert.NoError(t, err)

		uncompressedPayloads = append(uncompressedPayloads, payload)
	}
	return uncompressedPayloads
}

func assertEqualToMarshalJSON(t *testing.T, m marshaler.StreamJSONMarshaler) {
	payloads := buildPayload(t, m)
	json, err := m.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(payloads))
	assert.Equal(t, strings.TrimSpace(string(json)), string(payloads[0]))
}

func createEvent(sourceTypeName string) *Event {
	return &Event{
		Title:          "1",
		Text:           "2",
		Ts:             3,
		Priority:       EventPriorityNormal,
		Host:           "5",
		Tags:           []string{"6", "7"},
		AlertType:      EventAlertTypeError,
		AggregationKey: "9",
		SourceTypeName: sourceTypeName,
		EventType:      "10"}
}

func TestEventsDescribeItem(t *testing.T) {
	events := Events{createEvent("sourceTypeName")}
	assert.Equal(t, `Title:"1", Text:"2"`, events.DescribeItem(0))
}

func TestPayloadsNoEvent(t *testing.T) {
	assertEqualToMarshalJSON(t, Events{})
}

func TestPayloadsSingleEvent(t *testing.T) {
	events := Events{createEvent("sourceTypeName")}
	assertEqualToMarshalJSON(t, events)
}

func TestPayloadsEmptyEvent(t *testing.T) {
	assertEqualToMarshalJSON(t, Events{&Event{}})
}

func TestPayloadsEvents(t *testing.T) {
	events := Events{
		createEvent("3"),
		createEvent("1"),
		createEvent("2"),
		createEvent("2"),
		createEvent("1"),
		createEvent("3")}

	assertEqualToMarshalJSON(t, events)
}

func TestPayloadsEventsSeveralPayloads(t *testing.T) {
	maxPayloadSize := config.Datadog.GetInt("serializer_max_payload_size")
	config.Datadog.SetDefault("serializer_max_payload_size", 400)
	defer config.Datadog.SetDefault("serializer_max_payload_size", maxPayloadSize)

	eventsCollection := []Events{
		{createEvent("1"), createEvent("1"), createEvent("2")},
		{createEvent("2"), createEvent("3"), createEvent("3")},
		{createEvent("4"), createEvent("4")}}
	var allEvents Events
	for _, event := range eventsCollection {
		allEvents = append(allEvents, event...)
	}

	payloads := buildPayload(t, allEvents)
	assert.Equal(t, 3, len(payloads))
	
	for index, events := range eventsCollection {
		json, err := events.MarshalJSON()
		assert.NoError(t, err)

		assert.Equal(t, strings.TrimSpace(string(json)), string(payloads[index]))
	}
}
