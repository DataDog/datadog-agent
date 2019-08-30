// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"encoding/json"
	"reflect"
	"testing"

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

func TestPayloadDescribeItem(t *testing.T) {
	events := Events{createEvent("sourceTypeName")}
	assert.Equal(t, `Source type: sourceTypeName, events count: 1`, events.CreateMarshalerBySourceType().DescribeItem(0))
}

func TestPayloadsNoEvent(t *testing.T) {
	assertEqualEventsToMarshalJSON(t, Events{})
}

func TestPayloadsSingleEvent(t *testing.T) {
	events := Events{createEvent("sourceTypeName")}
	assertEqualEventsToMarshalJSON(t, events)
}

func TestPayloadsEmptyEvent(t *testing.T) {
	assertEqualEventsToMarshalJSON(t, Events{&Event{}})
}

func TestPayloadsEvents(t *testing.T) {
	events := Events{
		createEvent("1"),
		createEvent("2"),
		createEvent("3"),
		createEvent("2"),
		createEvent("1"),
		createEvent("3")}

	assertEqualEventsToMarshalJSON(t, events)
}

func TestPayloadsEventsSeveralPayloads(t *testing.T) {
	config.Datadog.Set("serializer_max_payload_size", 500)
	defer config.Datadog.Set("serializer_max_payload_size", nil)

	eventsCollection := []Events{
		{createEvent("3"), createEvent("3")},
		{createEvent("2"), createEvent("2")},
		{createEvent("1"), createEvent("1")}}
	var allEvents Events
	var expectedPayloads [][]byte

	for _, events := range eventsCollection {
		allEvents = append(allEvents, events...)
		json, err := events.MarshalJSON()
		assert.NoError(t, err)
		expectedPayloads = append(expectedPayloads, json)
	}

	payloads := buildPayload(t, allEvents.CreateMarshalerBySourceType())

	assertEqualEventsPayloads(t, expectedPayloads, payloads)
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

func assertEqualEventsToMarshalJSON(t *testing.T, events Events) {
	payloads := buildPayload(t, events.CreateMarshalerBySourceType())
	json, err := events.MarshalJSON()
	assert.NoError(t, err)
	assertEqualEventsPayloads(t, [][]byte{json}, payloads)
}

func assertEqualEventsPayloads(t *testing.T, expected [][]byte, actual [][]byte) {
	// The payload order returned by Events is not deterministic because we use a map inside
	// getEventsBySourceType().
	expectedBySourceTypes, err := createEventsJSONCollection(expected)
	assert.NoError(t, err)

	actualBySourceTypes, err := createEventsJSONCollection(actual)
	assert.NoError(t, err)

	assert.Equal(t, len(expectedBySourceTypes), len(actualBySourceTypes))
	assert.True(t, reflect.DeepEqual(expectedBySourceTypes, actualBySourceTypes))
}

func createEventsJSONCollection(payloads [][]byte) (map[string][]*eventsJSON, error) {
	eventsJSONBySourceType := make(map[string][]*eventsJSON)

	for _, p := range payloads {
		events := eventsJSON{}
		err := json.Unmarshal(p, &events)

		if err != nil {
			return nil, err
		}
		for sourceTypeName := range events.Events {
			eventsJSONBySourceType[sourceTypeName] = append(eventsJSONBySourceType[sourceTypeName], &events)
		}
	}
	return eventsJSONBySourceType, nil
}

type eventsJSON struct {
	APIKey           string
	Events           map[string][]Event
	InternalHostname string
}
