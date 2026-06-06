// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && test && zstd

package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Helpers
type payloadsType = []byte

func createEvent(sourceTypeName string) *event.Event {
	return &event.Event{
		Title:          "1",
		Text:           "2",
		Ts:             3,
		Priority:       event.PriorityNormal,
		Host:           "5",
		Tags:           []string{"6", "7"},
		AlertType:      event.AlertTypeError,
		AggregationKey: "9",
		SourceTypeName: sourceTypeName,
		EventType:      "10",
		OriginInfo:     taggertypes.OriginInfo{},
	}
}

func createEvents(sourceTypeNames ...string) []*event.Event {
	var events []*event.Event
	for _, s := range sourceTypeNames {
		events = append(events, createEvent(s))
	}
	return events
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
		"\n%+#v\nVS\n%+#v", expectedBySourceTypes, actualBySourceTypes)
}

func buildEventsJSON(payloads []payloadsType) (*eventsJSON, error) {
	var allEventsJSON *eventsJSON

	for _, p := range payloads {
		events := eventsJSON{}
		err := json.Unmarshal(p, &events)
		if err != nil {
			return nil, fmt.Errorf("failed to decode %q: %v", string(p), err)
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
	Events           map[string][]event.Event
	InternalHostname string
}

func createBenchmarkEvents(numberOfItem int) []*event.Event {
	events := make([]*event.Event, 0, numberOfItem)

	maxValue := int(math.Sqrt(float64(numberOfItem)))
	for i := 0; i < numberOfItem; i++ {
		events = append(events, createEvent(strconv.Itoa(i%maxValue)))
	}
	return events
}

func runBenchmark(b *testing.B, bench func(*testing.B, int)) {
	for i := 1; i <= 1000*1000; i *= 10 {
		numberOfItem := i // To avoid linter waring
		b.Run(strconv.Itoa(i), func(b *testing.B) {
			bench(b, numberOfItem)
		})
	}
}

func TestEventsMarshaler2(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("serializer_max_payload_size", 500)
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)
			events := createEvents("3", "3", "2", "2", "1", "1")

			bytePayloads, err := MarshalEvents(
				events,
				"",
				mockConfig,
				logmock.New(t),
				metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp,
			)
			assert.NoError(t, err)
			payloads := decodePayload(t, mockConfig, bytePayloads)
			assert.Equal(t, 1, len(payloads))

			expectedPayloads := payloadsType(`{"apiKey":"","events":{"1":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"1","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"1","event_type":"10"}],"2":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"2","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"2","event_type":"10"}],"3":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"3","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"3","event_type":"10"}]},"internalHostname":""}`)
			assertEqualEventsPayloads(t, expectedPayloads, payloads)
		})
	}
}

func TestEventsMarshaler2Split(t *testing.T) {
	tests := map[string]struct {
		kind      string
		npayloads int
	}{
		"zlib": {kind: compression.ZlibKind, npayloads: 2},
		"zstd": {kind: compression.ZstdKind, npayloads: 6},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("serializer_max_payload_size", 400)
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)
			events := createEvents("3", "3", "2", "2", "1", "1")

			bytePayloads, err := MarshalEvents(
				events,
				"",
				mockConfig,
				logmock.New(t),
				metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp,
			)
			assert.NoError(t, err)
			payloads := decodePayload(t, mockConfig, bytePayloads)
			assert.Equal(t, tc.npayloads, len(payloads))

			expectedPayloads := payloadsType(`{"apiKey":"","events":{"1":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"1","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"1","event_type":"10"}],"2":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"2","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"2","event_type":"10"}],"3":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"3","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"3","event_type":"10"}]},"internalHostname":""}`)
			assertEqualEventsPayloads(t, expectedPayloads, payloads)
		})
	}
}

func TestEventsMarshaler2Drop(t *testing.T) {
	tests := map[string]struct {
		kind      string
		npayloads int
	}{
		"zlib": {kind: compression.ZlibKind, npayloads: 2},
		"zstd": {kind: compression.ZstdKind, npayloads: 6},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.New(t)

			largeText := strings.Repeat("1", 500)

			mockConfig.SetInTest("serializer_max_payload_size", 400)
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)
			events := createEvents("3", "3", "2", "2", "2", "1", "1", largeText)
			events[3].Text = largeText

			marshaler := createMarshaler2(
				events,
				"",
				mockConfig,
				logmock.New(t),
				metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp,
			)

			// assert positions of big events in the sorted array to make sure we're testing the expected states
			assert.Equal(t, marshaler.events[2].SourceTypeName, largeText)
			assert.Equal(t, marshaler.events[4].Text, largeText)

			bytePayloads, err := marshaler.marshal()
			assert.NoError(t, err)

			payloads := decodePayload(t, mockConfig, bytePayloads)
			assert.Equal(t, tc.npayloads, len(payloads))

			expectedPayloads := payloadsType(`{"apiKey":"","events":{"1":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"1","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"1","event_type":"10"}],"2":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"2","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"2","event_type":"10"}],"3":[{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"3","event_type":"10"},{"msg_title":"1","msg_text":"2","timestamp":3,"priority":"normal","host":"5","tags":["6","7"],"alert_type":"error","aggregation_key":"9","source_type_name":"3","event_type":"10"}]},"internalHostname":""}`)
			assertEqualEventsPayloads(t, expectedPayloads, payloads)
		})
	}
}

func BenchmarkMarshaler2(b *testing.B) {
	runBenchmark(b, func(b *testing.B, numberOfItem int) {
		cfg := configmock.New(b)
		logger := logmock.New(b)
		compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: cfg}).Comp
		events := createBenchmarkEvents(numberOfItem)

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			_, _ = MarshalEvents(events, "", cfg, logger, compressor)
		}
	})
}
