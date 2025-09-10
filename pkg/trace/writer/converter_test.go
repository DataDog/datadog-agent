// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/stretchr/testify/assert"
)

func TestConvertToIdx(t *testing.T) {
	payload := &pb.TracerPayload{
		ContainerID:     "container_id",
		LanguageName:    "language_name",
		LanguageVersion: "language_version",
		TracerVersion:   "tracer_version",
		RuntimeID:       "runtime_id",
		Env:             "env",
		Hostname:        "hostname",
		AppVersion:      "app_version",
		Tags: map[string]string{
			"shared_key": "value",
		},
		Chunks: []*pb.TraceChunk{
			{
				Origin: "origin",
				Tags: map[string]string{
					"chunk_key": "chunk_value",
					"_dd.p.dm":  "-2",
				},
				Spans: []*pb.Span{
					{
						Service:  "service",
						Name:     "name",
						Resource: "resource",
						Type:     "type",
						SpanID:   123,
						TraceID:  456,
						ParentID: 0,
						Start:    5_000,
						Duration: 6_000,
						Error:    1,
						Meta: map[string]string{
							"shared_key": "span_value",
							"env":        "env",
							"version":    "app_version",
							"component":  "component",
							"kind":       "client",
							"_dd.p.tid":  "123FE",
						},
						Metrics: map[string]float64{
							"metric_key": 1.0,
						},
						MetaStruct: map[string][]byte{
							"meta_key": []byte("meta_value"),
						},
						SpanLinks: []*pb.SpanLink{
							{
								TraceID:     456,
								TraceIDHigh: 0x555FE,
								SpanID:      789,
								Attributes: map[string]string{
									"link_key": "link_value",
								},
								Tracestate: "tracestate",
								Flags:      0x1,
							},
						},
						SpanEvents: []*pb.SpanEvent{
							{
								TimeUnixNano: 123,
								Name:         "event_name",
								Attributes: map[string]*pb.AttributeAnyValue{
									"event_key": {
										Type:        pb.AttributeAnyValue_STRING_VALUE,
										StringValue: "event_value",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	idxPayload := convertToIdx(payload)
	assert.Len(t, idxPayload.Strings, 33) // Should match the number of unique strings in the payload (_dd.p.dm is not indexed)
	assert.Equal(t, "container_id", idxPayload.Strings[idxPayload.ContainerIDRef])
	assert.Equal(t, "language_name", idxPayload.Strings[idxPayload.LanguageNameRef])
	assert.Equal(t, "language_version", idxPayload.Strings[idxPayload.LanguageVersionRef])
	assert.Equal(t, "tracer_version", idxPayload.Strings[idxPayload.TracerVersionRef])
	assert.Equal(t, "runtime_id", idxPayload.Strings[idxPayload.RuntimeIDRef])
	assert.Equal(t, "env", idxPayload.Strings[idxPayload.EnvRef])
	assert.Equal(t, "hostname", idxPayload.Strings[idxPayload.HostnameRef])
	assert.Equal(t, "app_version", idxPayload.Strings[idxPayload.VersionRef])
	assert.Len(t, idxPayload.Attributes, 1)
	for kRef, attrValue := range idxPayload.Attributes { // Use a loop to get the first value as we don't know for sure what the string index will be
		assert.Equal(t, "shared_key", idxPayload.Strings[kRef])
		assert.Equal(t, "value", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
	}
	assert.Len(t, idxPayload.Chunks, 1)
	assert.Equal(t, uint32(2), idxPayload.Chunks[0].SamplingMechanism)
	assert.Equal(t, "origin", idxPayload.Strings[idxPayload.Chunks[0].OriginRef])
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x23, 0xfe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0xc8}, idxPayload.Chunks[0].TraceID)
	assert.Len(t, idxPayload.Chunks[0].Attributes, 1) // _dd.p.dm is not indexed anymore
	for kRef, attrValue := range idxPayload.Chunks[0].Attributes {
		switch idxPayload.Strings[kRef] {
		case "chunk_key":
			assert.Equal(t, "chunk_value", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		default:
			t.Fatalf("unexpected attribute key: %s", idxPayload.Strings[kRef])
		}
	}
	assert.Len(t, idxPayload.Chunks[0].Spans, 1)
	idxSpan := idxPayload.Chunks[0].Spans[0]
	assert.Len(t, idxSpan.Attributes, 8)
	for kRef, attrValue := range idxSpan.Attributes {
		switch idxPayload.Strings[kRef] {
		case "shared_key":
			assert.Equal(t, "span_value", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		case "metric_key":
			assert.Equal(t, 1.0, attrValue.Value.(*idx.AnyValue_DoubleValue).DoubleValue)
		case "meta_key":
			assert.Equal(t, "meta_value", string(attrValue.Value.(*idx.AnyValue_BytesValue).BytesValue))
		case "env":
			assert.Equal(t, "env", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		case "version":
			assert.Equal(t, "app_version", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		case "component":
			assert.Equal(t, "component", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		case "kind":
			assert.Equal(t, "client", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		case "_dd.p.tid":
			assert.Equal(t, "123FE", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		default:
			t.Fatalf("unexpected attribute key: %s", idxPayload.Strings[kRef])
		}
	}
	assert.Equal(t, "service", idxPayload.Strings[idxSpan.ServiceRef])
	assert.Equal(t, "name", idxPayload.Strings[idxSpan.NameRef])
	assert.Equal(t, "resource", idxPayload.Strings[idxSpan.ResourceRef])
	assert.Equal(t, "type", idxPayload.Strings[idxSpan.TypeRef])
	assert.Equal(t, uint64(123), idxSpan.SpanID)
	assert.Equal(t, uint64(0), idxSpan.ParentID)
	assert.Equal(t, uint64(5_000), idxSpan.Start)
	assert.Equal(t, uint64(6_000), idxSpan.Duration)
	assert.Equal(t, true, idxSpan.Error)
	assert.Equal(t, idx.SpanKind_SPAN_KIND_CLIENT, idxSpan.Kind)
	assert.Equal(t, "env", idxPayload.Strings[idxSpan.EnvRef])
	assert.Equal(t, "app_version", idxPayload.Strings[idxSpan.VersionRef])
	assert.Equal(t, "component", idxPayload.Strings[idxSpan.ComponentRef])

	assert.Len(t, idxSpan.SpanLinks, 1)
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x55, 0xfe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0xc8}, idxSpan.SpanLinks[0].TraceID)
	assert.Equal(t, uint64(789), idxSpan.SpanLinks[0].SpanID)
	assert.Len(t, idxSpan.SpanLinks[0].Attributes, 1)
	for kRef, attrValue := range idxSpan.SpanLinks[0].Attributes {
		switch idxPayload.Strings[kRef] {
		case "link_key":
			assert.Equal(t, "link_value", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		default:
			t.Fatalf("unexpected attribute key: %s", idxPayload.Strings[kRef])
		}
	}
	assert.Equal(t, "tracestate", idxPayload.Strings[idxSpan.SpanLinks[0].TracestateRef])
	assert.Equal(t, uint32(0x1), idxSpan.SpanLinks[0].FlagsRef)

	assert.Len(t, idxSpan.SpanEvents, 1)
	assert.Equal(t, uint64(123), idxSpan.SpanEvents[0].Time)
	assert.Equal(t, "event_name", idxPayload.Strings[idxSpan.SpanEvents[0].NameRef])
	assert.Len(t, idxSpan.SpanEvents[0].Attributes, 1)
	for kRef, attrValue := range idxSpan.SpanEvents[0].Attributes {
		switch idxPayload.Strings[kRef] {
		case "event_key":
			assert.Equal(t, "event_value", idxPayload.Strings[attrValue.Value.(*idx.AnyValue_StringValueRef).StringValueRef])
		default:
			t.Fatalf("unexpected attribute key: %s", idxPayload.Strings[kRef])
		}
	}
}

func TestConvertToIdx_SamplingMechanism(t *testing.T) {
	testCases := []struct {
		name                      string
		ddPDMValue                string
		expectedSamplingMechanism uint32
	}{
		{
			name:                      "positive number",
			ddPDMValue:                "5",
			expectedSamplingMechanism: 5,
		},
		{
			name:                      "large positive number",
			ddPDMValue:                "1234567890",
			expectedSamplingMechanism: 1234567890,
		},
		{
			name:                      "negative number",
			ddPDMValue:                "-3",
			expectedSamplingMechanism: 3,
		},
		{
			name:                      "negative large number",
			ddPDMValue:                "-999",
			expectedSamplingMechanism: 999,
		},
		{
			name:                      "zero",
			ddPDMValue:                "0",
			expectedSamplingMechanism: 0,
		},
		{
			name:                      "negative zero",
			ddPDMValue:                "-0",
			expectedSamplingMechanism: 0,
		},
		{
			name:                      "invalid string",
			ddPDMValue:                "abc",
			expectedSamplingMechanism: 0,
		},
		{
			name:                      "empty string",
			ddPDMValue:                "",
			expectedSamplingMechanism: 0,
		},
		{
			name:                      "mixed alphanumeric",
			ddPDMValue:                "123abc",
			expectedSamplingMechanism: 0,
		},
		{
			name:                      "float number",
			ddPDMValue:                "42.5",
			expectedSamplingMechanism: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Tags: map[string]string{
							"_dd.p.dm": tc.ddPDMValue,
						},
						Spans: []*pb.Span{
							{
								Service:  "test-service",
								Name:     "test-span",
								Resource: "test-resource",
								Type:     "test-type",
								SpanID:   123,
								TraceID:  456,
								Start:    1000,
								Duration: 2000,
							},
						},
					},
				},
			}

			idxPayload := convertToIdx(payload)
			assert.Equal(t, tc.expectedSamplingMechanism, idxPayload.Chunks[0].SamplingMechanism,
				"SamplingMechanism should be %d for _dd.p.dm value '%s'", tc.expectedSamplingMechanism, tc.ddPDMValue)
		})
	}
}

func TestConvertToIdx_NoSamplingMechanism(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Tags: map[string]string{
					"other_tag": "value",
				},
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						Type:     "test-type",
						SpanID:   123,
						TraceID:  456,
						Start:    1000,
						Duration: 2000,
					},
				},
			},
		},
	}

	idxPayload := convertToIdx(payload)
	assert.Equal(t, uint32(0), idxPayload.Chunks[0].SamplingMechanism,
		"SamplingMechanism should be 0 when _dd.p.dm tag is not present")
}
