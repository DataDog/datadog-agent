// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && test

package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/v5/gogen"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
)

func TestMarshal(t *testing.T) {
	events := Events{{
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
	assert.Equal(t, newPayload.Events[0].Priority, string(metrics.EventPriorityNormal))
	assert.Equal(t, newPayload.Events[0].Host, "test.localhost")
	require.Len(t, newPayload.Events[0].Tags, 2)
	assert.Equal(t, newPayload.Events[0].Tags[0], "tag1")
	assert.Equal(t, newPayload.Events[0].Tags[1], "tag2:yes")
	assert.Equal(t, newPayload.Events[0].AlertType, string(metrics.EventAlertTypeError))
	assert.Equal(t, newPayload.Events[0].AggregationKey, "test aggregation")
	assert.Equal(t, newPayload.Events[0].SourceTypeName, "test source")
}

func TestMarshalJSON(t *testing.T) {
	events := Events{{
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

	mockConfig := config.Mock(t)
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

	mockConfig := config.Mock(t)
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
		e := metrics.Event{
			Title:          "An event occurred",
			Text:           "event description",
			Ts:             12345,
			Priority:       metrics.EventPriorityNormal,
			Host:           "my-hostname",
			Tags:           []string{"tag1", "tag2:yes"},
			AlertType:      metrics.EventAlertTypeError,
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

// Test StreamJSONMarshaler
func TestPayloadDescribeItem(t *testing.T) {
	events := Events{createEvent("sourceTypeName")}
	assert.Equal(t, `Source type: sourceTypeName, events count: 1`,
		events.CreateSingleMarshaler().DescribeItem(0))
	assert.Equal(t, `Title: 1, Text: 2, Source Type: sourceTypeName`,
		events.CreateMarshalersBySourceType()[0].DescribeItem(0))
}

func TestPayloadsNoEvent(t *testing.T) {
	assertEqualEventsToMarshalJSON(t, Events{})
}

func TestPayloadsSingleEvent(t *testing.T) {
	events := createEvents("sourceTypeName")
	assertEqualEventsToMarshalJSON(t, events)
}

func TestPayloadsEmptyEvent(t *testing.T) {
	assertEqualEventsToMarshalJSON(t, Events{&metrics.Event{}})
}

func TestPayloadsEvents(t *testing.T) {
	events := createEvents("1", "2", "3", "2", "1", "3")
	assertEqualEventsToMarshalJSON(t, events)
}

func TestEventsSeveralPayloadsCreateSingleMarshaler(t *testing.T) {
	events := createEvents("3", "3", "2", "2", "1", "1")

	config.Datadog.Set("serializer_max_payload_size", 500)
	defer config.Datadog.Set("serializer_max_payload_size", nil)

	expectedPayloads, err := events.MarshalJSON()
	assert.NoError(t, err)

	payloadsBySourceType := buildPayload(t, events.CreateSingleMarshaler())
	assert.Equal(t, 3, len(payloadsBySourceType))
	assertEqualEventsPayloads(t, expectedPayloads, payloadsBySourceType)
}

func TestEventsSeveralPayloadsCreateMarshalersBySourceType(t *testing.T) {
	events := createEvents("3", "3", "2", "2", "1", "1")

	config.Datadog.Set("serializer_max_payload_size", 300)
	defer config.Datadog.Set("serializer_max_payload_size", nil)

	expectedPayloads, err := events.MarshalJSON()
	assert.NoError(t, err)

	marshalers := events.CreateMarshalersBySourceType()
	assert.Equal(t, 3, len(marshalers))
	var payloadForEachSourceType []payloadsType
	for _, marshaler := range marshalers {
		payloads := buildPayload(t, marshaler)
		assert.Equal(t, 2, len(payloads))
		payloadForEachSourceType = append(payloadForEachSourceType, payloads...)
	}

	assertEqualEventsPayloads(t, expectedPayloads, payloadForEachSourceType)
}

// Helpers
type payloadsType = []byte

func createEvent(sourceTypeName string) *metrics.Event {
	return &metrics.Event{
		Title:          "1",
		Text:           "2",
		Ts:             3,
		Priority:       metrics.EventPriorityNormal,
		Host:           "5",
		Tags:           []string{"6", "7"},
		AlertType:      metrics.EventAlertTypeError,
		AggregationKey: "9",
		SourceTypeName: sourceTypeName,
		EventType:      "10"}
}

func createEvents(sourceTypeNames ...string) Events {
	var events []*metrics.Event
	for _, s := range sourceTypeNames {
		events = append(events, createEvent(s))
	}
	return events
}

// Check JSONPayloadBuilder for CreateSingleMarshaler and CreateMarshalersBySourceType
// return the same results as for MarshalJSON.
func assertEqualEventsToMarshalJSON(t *testing.T, events Events) {
	json, err := events.MarshalJSON()
	assert.NoError(t, err)

	payloadsBySourceType := buildPayload(t, events.CreateSingleMarshaler())
	assertEqualEventsPayloads(t, json, payloadsBySourceType)

	var payloads []payloadsType
	for _, e := range events.CreateMarshalersBySourceType() {
		payloads = append(payloads, buildPayload(t, e)...)
	}
	assertEqualEventsPayloads(t, json, payloads)
}

func assertEqualEventsPayloads(t *testing.T, expected payloadsType, actual []payloadsType) {
	// The payload order returned by Events is not deterministic because we use a map inside
	// getEventsBySourceType().
	expectedBySourceTypes, err := buildEventsJSON([]payloadsType{expected})
	assert.NoError(t, err)

	actualBySourceTypes, err := buildEventsJSON(actual)
	assert.NoError(t, err)

	assert.Truef(t,
		reflect.DeepEqual(expectedBySourceTypes, actualBySourceTypes),
		"\n%+p\nVS\n%+v", expectedBySourceTypes, actualBySourceTypes)
}

func buildEventsJSON(payloads []payloadsType) (*eventsJSON, error) {
	var allEventsJSON *eventsJSON

	for _, p := range payloads {
		events := eventsJSON{}
		err := json.Unmarshal(p, &events)

		if err != nil {
			return nil, err
		}

		if allEventsJSON == nil {
			allEventsJSON = &events
		} else {
			switch {
			case allEventsJSON.APIKey != events.APIKey:
				return nil, errors.New("APIKey missmatch")
			case allEventsJSON.InternalHostname != events.InternalHostname:
				return nil, errors.New("InternalHostname missmatch")
			default:
				for k, v := range events.Events {
					allEventsJSON.Events[k] = append(allEventsJSON.Events[k], v...)
				}
			}
		}
	}
	if allEventsJSON == nil {
		allEventsJSON = &eventsJSON{}
	}
	return allEventsJSON, nil
}

type eventsJSON struct {
	APIKey           string
	Events           map[string][]metrics.Event
	InternalHostname string
}

func createBenchmarkEvents(numberOfItem int) Events {
	var events Events

	maxValue := int(math.Sqrt(float64(numberOfItem)))
	for i := 0; i < numberOfItem; i++ {
		events = append(events, createEvent(strconv.Itoa(i%maxValue)))
	}
	return events
}

func runBenchmark(b *testing.B, bench func(*testing.B, int)) {
	for i := 1; i <= 1000*1000; i *= 10 {
		numberOfItem := i // To avoid linter waring
		b.Run(fmt.Sprintf("%d", i), func(b *testing.B) {
			bench(b, numberOfItem)
		})
	}
}

func BenchmarkCreateSingleMarshaler(b *testing.B) {
	benchmarkCreateSingleMarshaler(b, createBenchmarkEvents)
}

func BenchmarkCreateSingleMarshalerOneEventBySource(b *testing.B) {
	benchmarkCreateSingleMarshaler(b, func(numberOfItem int) Events {
		var events Events

		for i := 0; i < numberOfItem; i++ {
			events = append(events, createEvent(strconv.Itoa(i)))
		}
		return events
	})
}

func benchmarkCreateSingleMarshaler(b *testing.B, createEvents func(numberOfItem int) Events) {
	runBenchmark(b, func(b *testing.B, numberOfItem int) {
		payloadBuilder := stream.NewJSONPayloadBuilder(true)
		events := createEvents(numberOfItem)

		b.ResetTimer()

		for n := 0; n < b.N; n++ {
			stream.BuildJSONPayload(payloadBuilder, events.CreateSingleMarshaler())
		}
	})
}

func BenchmarkCreateMarshalersBySourceType(b *testing.B) {
	runBenchmark(b, func(b *testing.B, numberOfItem int) {
		payloadBuilder := stream.NewJSONPayloadBuilder(true)
		events := createBenchmarkEvents(numberOfItem)

		b.ResetTimer()

		for n := 0; n < b.N; n++ {
			for _, m := range events.CreateMarshalersBySourceType() {
				stream.BuildJSONPayload(payloadBuilder, m)
			}
		}
	})
}

func BenchmarkCreateMarshalersSeveralSourceTypes(b *testing.B) {
	runBenchmark(b, func(b *testing.B, numberOfItem int) {
		payloadBuilder := stream.NewJSONPayloadBuilder(true)

		var events Events
		// Half of events have the same source type
		for i := 0; i < numberOfItem/2; i++ {
			events = append(events, createEvent("sourceType"))
		}
		// Half of events have their own source type
		for i := 0; i < numberOfItem/2; i++ {
			events = append(events, createEvent(strconv.Itoa(i)))
		}

		b.ResetTimer()

		for n := 0; n < b.N; n++ {
			// As CreateMarshalersBySourceType is called only after CreateSingleMarshaler,
			// we also call CreateSingleMarshaler in this benchmark.
			stream.BuildJSONPayload(payloadBuilder, events.CreateSingleMarshaler())
			for _, m := range events.CreateMarshalersBySourceType() {
				stream.BuildJSONPayload(payloadBuilder, m)
			}
		}
	})
}
