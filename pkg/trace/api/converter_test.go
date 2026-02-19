// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test that error field is converted from int32 to bool
func TestConvertToIdx_ErrorFieldIsBool(t *testing.T) {
	tests := []struct {
		name         string
		errorValue   int32
		expectedBool bool
	}{
		{
			name:         "error is zero",
			errorValue:   0,
			expectedBool: false,
		},
		{
			name:         "error is one",
			errorValue:   1,
			expectedBool: true,
		},
		{
			name:         "error is greater than one",
			errorValue:   5,
			expectedBool: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Service:  "test-service",
								Name:     "test-span",
								Resource: "test-resource",
								TraceID:  123,
								SpanID:   456,
								Error:    tt.errorValue,
							},
						},
					},
				},
			}

			result := ConvertToIdx(payload, "")

			require.Len(t, result.Chunks, 1)
			require.Len(t, result.Chunks[0].Spans, 1)
			assert.Equal(t, tt.expectedBool, result.Chunks[0].Spans[0].Error())
		})
	}
}

// Test that kind field is correctly converted to OTEL spec values
func TestConvertToIdx_KindFieldMatchesOTELSpec(t *testing.T) {
	tests := []struct {
		name         string
		kindMeta     string
		expectedKind idx.SpanKind
	}{
		{
			name:         "server kind",
			kindMeta:     "server",
			expectedKind: idx.SpanKind_SPAN_KIND_SERVER,
		},
		{
			name:         "client kind",
			kindMeta:     "client",
			expectedKind: idx.SpanKind_SPAN_KIND_CLIENT,
		},
		{
			name:         "producer kind",
			kindMeta:     "producer",
			expectedKind: idx.SpanKind_SPAN_KIND_PRODUCER,
		},
		{
			name:         "consumer kind",
			kindMeta:     "consumer",
			expectedKind: idx.SpanKind_SPAN_KIND_CONSUMER,
		},
		{
			name:         "internal kind",
			kindMeta:     "internal",
			expectedKind: idx.SpanKind_SPAN_KIND_INTERNAL,
		},
		{
			name:         "empty kind defaults to internal",
			kindMeta:     "",
			expectedKind: idx.SpanKind_SPAN_KIND_INTERNAL,
		},
		{
			name:         "unknown kind defaults to internal",
			kindMeta:     "unknown",
			expectedKind: idx.SpanKind_SPAN_KIND_INTERNAL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := map[string]string{}
			if tt.kindMeta != "" {
				meta["span.kind"] = tt.kindMeta
			}

			payload := &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Service:  "test-service",
								Name:     "test-span",
								Resource: "test-resource",
								TraceID:  123,
								SpanID:   456,
								Meta:     meta,
							},
						},
					},
				},
			}

			result := ConvertToIdx(payload, "")

			require.Len(t, result.Chunks, 1)
			require.Len(t, result.Chunks[0].Spans, 1)
			assert.Equal(t, tt.expectedKind, result.Chunks[0].Spans[0].Kind())
		})
	}
}

// Test that promoted fields (env, version, component) are moved to their fields instead of attributes
func TestConvertToIdx_PromotedFields(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"env":       "production",
							"version":   "1.2.3",
							"component": "http-client",
							"kind":      "client",
							"other":     "should-remain",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	// Verify promoted fields are set on the span
	assert.Equal(t, "production", span.Env())
	assert.Equal(t, "1.2.3", span.Version())
	assert.Equal(t, "http-client", span.Component())
}

// Test that env field is correctly promoted at span level
func TestConvertToIdx_EnvFieldPromoted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"env": "staging",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	assert.Equal(t, "staging", span.Env())
	assert.Equal(t, "staging", result.Env())
}

// Test that env field is correctly promoted at span level
func TestConvertToIdx_HostnameFieldPromoted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"_dd.hostname": "test-hostname",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	assert.Equal(t, "test-hostname", result.Hostname())
}

// Test that version field is correctly promoted at span level
func TestConvertToIdx_VersionFieldPromoted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"version": "2.0.1",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	assert.Equal(t, "2.0.1", result.AppVersion())
}

// Test that component field is correctly promoted at span level
func TestConvertToIdx_ComponentFieldPromoted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"component": "redis",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	assert.Equal(t, "redis", span.Component())
}

// Test that _dd.p.dm field is correctly converted to sampling mechanism
func TestConvertToIdx_DecisionMakerFieldSetsSamplingMechanism(t *testing.T) {
	tests := []struct {
		name                      string
		decisionMaker             string
		expectedSamplingMechanism uint32
	}{
		{
			name:                      "decision maker with value 8",
			decisionMaker:             "8",
			expectedSamplingMechanism: 8,
		},
		{
			name:                      "decision maker with negative prefix",
			decisionMaker:             "-4",
			expectedSamplingMechanism: 4,
		},
		{
			name:                      "decision maker with value 0",
			decisionMaker:             "0",
			expectedSamplingMechanism: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := &pb.TracerPayload{
				Chunks: []*pb.TraceChunk{
					{
						Spans: []*pb.Span{
							{
								Service:  "test-service",
								Name:     "test-span",
								Resource: "test-resource",
								TraceID:  123,
								SpanID:   456,
								Meta: map[string]string{
									"_dd.p.dm": tt.decisionMaker,
								},
							},
						},
					},
				},
			}

			result := ConvertToIdx(payload, "")

			require.Len(t, result.Chunks, 1)
			assert.Equal(t, tt.expectedSamplingMechanism, result.Chunks[0].SamplingMechanism())
		})
	}
}

// Test that nil chunks are skipped
func TestConvertToIdx_NilChunksSkipped(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			nil,
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 2)
	// First chunk should be nil/empty, second should have data
	assert.Nil(t, result.Chunks[0])
	assert.NotNil(t, result.Chunks[1])
	assert.Len(t, result.Chunks[1].Spans, 1)
}

// Test that chunks with empty spans are skipped
func TestConvertToIdx_EmptySpansChunksSkipped(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{},
			},
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 2)
	assert.Nil(t, result.Chunks[0])
	assert.NotNil(t, result.Chunks[1])
}

// Test that span metrics are converted correctly
func TestConvertToIdx_SpanMetricsConverted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Metrics: map[string]float64{
							"custom.metric":   1.23,
							"another.metric":  4.56,
							"zero.metric":     0.0,
							"negative.metric": -1.5,
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	attrs := span.Attributes()
	assert.Greater(t, len(attrs), 0)
}

// Test that _sampling_priority_v1 metric sets chunk priority
func TestConvertToIdx_SamplingPriorityMetricSetsChunkPriority(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Metrics: map[string]float64{
							"_sampling_priority_v1": 2.0,
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	assert.Equal(t, int32(2), result.Chunks[0].Priority)
}

// Test that chunk priority is used when span has no priority
func TestConvertToIdx_ChunkPriorityUsedWhenSpanHasNone(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Priority: 1,
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	assert.Equal(t, int32(1), result.Chunks[0].Priority)
}

// Test that span priority takes precedence over chunk priority
func TestConvertToIdx_SpanPriorityTakesPrecedence(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Priority: 1,
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Metrics: map[string]float64{
							"_sampling_priority_v1": 2.0,
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	assert.Equal(t, int32(2), result.Chunks[0].Priority)
}

// Test that MetaStruct fields are converted correctly
func TestConvertToIdx_MetaStructConverted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						MetaStruct: map[string][]byte{
							"binary.data": {0x01, 0x02, 0x03},
							"more.data":   {0xFF, 0xFE},
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	attrs := span.Attributes()
	assert.Greater(t, len(attrs), 0)
}

// Test that SpanLinks are converted correctly
func TestConvertToIdx_SpanLinksConverted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						SpanLinks: []*pb.SpanLink{
							{
								TraceID:     789,
								TraceIDHigh: 1011,
								SpanID:      1213,
								Tracestate:  "test=state",
								Flags:       1,
								Attributes: map[string]string{
									"link.attr": "link.value",
								},
							},
							{
								TraceID:     1415,
								TraceIDHigh: 1617,
								SpanID:      1819,
							},
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	links := span.Links()
	require.Len(t, links, 2)
	assert.Equal(t, uint64(1213), links[0].SpanID())
	assert.Equal(t, uint32(1), links[0].Flags())
	assert.NotEmpty(t, links[0].TraceID())
}

// Test that SpanEvents are converted correctly
func TestConvertToIdx_SpanEventsConverted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						SpanEvents: []*pb.SpanEvent{
							{
								Name:         "event1",
								TimeUnixNano: 1234567890,
								Attributes: map[string]*pb.AttributeAnyValue{
									"event.attr": {
										Type:        pb.AttributeAnyValue_STRING_VALUE,
										StringValue: "event.value",
									},
								},
							},
							{
								Name:         "event2",
								TimeUnixNano: 9876543210,
							},
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	events := span.Events()
	require.Len(t, events, 2)
	assert.Equal(t, uint64(1234567890), events[0].Time())
	assert.Equal(t, uint64(9876543210), events[1].Time())
}

// Test convertSpanEventAttributes with different attribute types
func TestConvertToIdx_SpanEventAttributeTypes(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						SpanEvents: []*pb.SpanEvent{
							{
								Name:         "comprehensive_event",
								TimeUnixNano: 1234567890,
								Attributes: map[string]*pb.AttributeAnyValue{
									"string.attr": {
										Type:        pb.AttributeAnyValue_STRING_VALUE,
										StringValue: "test string",
									},
									"bool.attr": {
										Type:      pb.AttributeAnyValue_BOOL_VALUE,
										BoolValue: true,
									},
									"int.attr": {
										Type:     pb.AttributeAnyValue_INT_VALUE,
										IntValue: 42,
									},
									"double.attr": {
										Type:        pb.AttributeAnyValue_DOUBLE_VALUE,
										DoubleValue: 3.14,
									},
									"array.attr": {
										Type: pb.AttributeAnyValue_ARRAY_VALUE,
										ArrayValue: &pb.AttributeArray{
											Values: []*pb.AttributeArrayValue{
												{
													Type:        pb.AttributeArrayValue_STRING_VALUE,
													StringValue: "item1",
												},
												{
													Type:     pb.AttributeArrayValue_INT_VALUE,
													IntValue: 100,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	events := span.Events()
	require.Len(t, events, 1)
	assert.Equal(t, uint64(1234567890), events[0].Time())
	assert.NotEmpty(t, events[0].Attributes())
}

// Test convertAttributeArrayValue with all types
func TestConvertToIdx_ArrayValueAllTypes(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						SpanEvents: []*pb.SpanEvent{
							{
								Name:         "array_test",
								TimeUnixNano: 1234567890,
								Attributes: map[string]*pb.AttributeAnyValue{
									"string.array": {
										Type: pb.AttributeAnyValue_ARRAY_VALUE,
										ArrayValue: &pb.AttributeArray{
											Values: []*pb.AttributeArrayValue{
												{
													Type:        pb.AttributeArrayValue_STRING_VALUE,
													StringValue: "str1",
												},
												{
													Type:        pb.AttributeArrayValue_STRING_VALUE,
													StringValue: "str2",
												},
											},
										},
									},
									"bool.array": {
										Type: pb.AttributeAnyValue_ARRAY_VALUE,
										ArrayValue: &pb.AttributeArray{
											Values: []*pb.AttributeArrayValue{
												{
													Type:      pb.AttributeArrayValue_BOOL_VALUE,
													BoolValue: true,
												},
												{
													Type:      pb.AttributeArrayValue_BOOL_VALUE,
													BoolValue: false,
												},
											},
										},
									},
									"int.array": {
										Type: pb.AttributeAnyValue_ARRAY_VALUE,
										ArrayValue: &pb.AttributeArray{
											Values: []*pb.AttributeArrayValue{
												{
													Type:     pb.AttributeArrayValue_INT_VALUE,
													IntValue: 1,
												},
												{
													Type:     pb.AttributeArrayValue_INT_VALUE,
													IntValue: 2,
												},
											},
										},
									},
									"double.array": {
										Type: pb.AttributeAnyValue_ARRAY_VALUE,
										ArrayValue: &pb.AttributeArray{
											Values: []*pb.AttributeArrayValue{
												{
													Type:        pb.AttributeArrayValue_DOUBLE_VALUE,
													DoubleValue: 1.1,
												},
												{
													Type:        pb.AttributeArrayValue_DOUBLE_VALUE,
													DoubleValue: 2.2,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	events := span.Events()
	require.Len(t, events, 1)
	assert.NotEmpty(t, events[0].Attributes())
}

// Test that git commit sha is extracted and promoted
func TestConvertToIdx_GitCommitShaExtracted(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"_dd.git.commit.sha": "abc123def456",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	// Git commit sha should be set as a string attribute on the payload
	attrs := result.Attributes
	assert.NotEmpty(t, attrs)
}

// Test that decision maker from chunk tags is used
func TestConvertToIdx_DecisionMakerFromChunkTags(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Tags: map[string]string{
					"_dd.p.dm": "-5",
				},
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	assert.Equal(t, uint32(5), result.Chunks[0].SamplingMechanism())
}

// Test that span decision maker takes precedence over chunk tags
func TestConvertToIdx_SpanDecisionMakerTakesPrecedence(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Tags: map[string]string{
					"_dd.p.dm": "-5",
				},
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"_dd.p.dm": "-8",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	assert.Equal(t, uint32(8), result.Chunks[0].SamplingMechanism())
}

// Test that invalid sampling mechanism is ignored
func TestConvertToIdx_InvalidSamplingMechanismIgnored(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"_dd.p.dm": "invalid",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	// Should default to 0 when invalid
	assert.Equal(t, uint32(0), result.Chunks[0].SamplingMechanism())
}

// Test that payload-level fields override span-level fields
func TestConvertToIdx_PayloadFieldsOverrideSpanFields(t *testing.T) {
	payload := &pb.TracerPayload{
		Hostname:   "payload-hostname",
		Env:        "payload-env",
		AppVersion: "payload-version",
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"_dd.hostname": "span-hostname",
							"env":          "span-env",
							"version":      "span-version",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	assert.Equal(t, "payload-hostname", result.Hostname())
	assert.Equal(t, "payload-env", result.Env())
	assert.Equal(t, "payload-version", result.AppVersion())
}

// Test that payload-level metadata fields are set
func TestConvertToIdx_PayloadMetadataFieldsSet(t *testing.T) {
	payload := &pb.TracerPayload{
		ContainerID:     "container-123",
		LanguageName:    "go",
		LanguageVersion: "1.20",
		TracerVersion:   "v1.2.3",
		RuntimeID:       "runtime-456",
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	assert.Equal(t, "container-123", result.ContainerID())
	assert.Equal(t, "go", result.LanguageName())
	assert.Equal(t, "1.20", result.LanguageVersion())
	assert.Equal(t, "v1.2.3", result.TracerVersion())
	assert.Equal(t, "runtime-456", result.RuntimeID())
}

// Test that chunk attributes and tags are converted
func TestConvertToIdx_ChunkAttributesConverted(t *testing.T) {
	payload := &pb.TracerPayload{
		Tags: map[string]string{
			"payload.tag1": "payload.value1",
			"payload.tag2": "payload.value2",
		},
		Chunks: []*pb.TraceChunk{
			{
				Tags: map[string]string{
					"chunk.tag1": "chunk.value1",
					"chunk.tag2": "chunk.value2",
				},
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	assert.NotEmpty(t, result.Attributes)
	assert.NotEmpty(t, result.Chunks[0].Attributes)
}

// Test that chunk origin and dropped trace are preserved
func TestConvertToIdx_ChunkOriginAndDroppedTracePreserved(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Origin:       "lambda",
				DroppedTrace: true,
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	assert.Equal(t, "lambda", result.Chunks[0].Origin())
	assert.True(t, result.Chunks[0].DroppedTrace)
}

// Test that promoted tags are not included in attributes
func TestConvertToIdx_PromotedTagsNotInAttributes(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"env":       "production",
							"version":   "1.2.3",
							"component": "http-client",
							"span.kind": "client",
							"normal":    "attribute",
						},
						Metrics: map[string]float64{
							"env":           999.0, // Should be ignored as promoted
							"normal.metric": 123.0,
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	// Promoted fields should be set
	assert.Equal(t, "production", span.Env())
	assert.Equal(t, "1.2.3", span.Version())
	assert.Equal(t, "http-client", span.Component())
	assert.Equal(t, idx.SpanKind_SPAN_KIND_CLIENT, span.Kind())

	// Only non-promoted attributes should be in attributes map
	attrs := span.Attributes()
	// We can't easily check the string table references, but we can verify
	// that the map doesn't have too many entries
	assert.LessOrEqual(t, len(attrs), 3) // "normal", "normal.metric", and "_dd.convertedv1"
}

// Test isPromotedTag function behavior
func TestIsPromotedTag(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{"env is promoted", "env", true},
		{"version is promoted", "version", true},
		{"component is promoted", "component", true},
		{"span.kind is promoted", "span.kind", true},
		{"other is not promoted", "other", false},
		{"env-like is not promoted", "environment", false},
		{"empty is not promoted", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPromotedTag(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test that first env from multiple spans is used at payload level
func TestConvertToIdx_FirstEnvUsedForPayload(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"env": "first-env",
						},
					},
					{
						Service:  "test-service2",
						Name:     "test-span2",
						Resource: "test-resource2",
						TraceID:  123,
						SpanID:   789,
						ParentID: 456,
						Meta: map[string]string{
							"env": "second-env",
						},
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	assert.Equal(t, "first-env", result.Env())
}

// Test multiple chunks with different data
func TestConvertToIdx_MultipleChunks(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Priority: 1,
				Origin:   "synthetics",
				Spans: []*pb.Span{
					{
						Service:  "service1",
						Name:     "span1",
						Resource: "resource1",
						TraceID:  100,
						SpanID:   200,
					},
				},
			},
			{
				Priority: 2,
				Origin:   "lambda",
				Spans: []*pb.Span{
					{
						Service:  "service2",
						Name:     "span2",
						Resource: "resource2",
						TraceID:  300,
						SpanID:   400,
					},
				},
			},
		},
	}

	result := ConvertToIdx(payload, "")

	require.Len(t, result.Chunks, 2)
	assert.Equal(t, int32(1), result.Chunks[0].Priority)
	assert.Equal(t, "synthetics", result.Chunks[0].Origin())
	assert.Equal(t, int32(2), result.Chunks[1].Priority)
	assert.Equal(t, "lambda", result.Chunks[1].Origin())
}

// Test that _dd.convertedv1 attribute is properly set on spans
func TestConvertToIdx_ConvertedV1AttributeSet(t *testing.T) {
	payload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
					},
					{
						Service:  "test-service",
						Name:     "test-span-2",
						Resource: "test-resource-2",
						TraceID:  123,
						SpanID:   789,
						ParentID: 456,
					},
				},
			},
		},
	}

	t.Run("attribute set when originPayloadVersion is provided", func(t *testing.T) {
		result := ConvertToIdx(payload, "v0.5")

		require.Len(t, result.Chunks, 1)
		require.Len(t, result.Chunks[0].Spans, 2)

		// Both spans should have the _dd.convertedv1 attribute set
		for i, span := range result.Chunks[0].Spans {
			val, ok := span.GetAttributeAsString("_dd.convertedv1")
			assert.True(t, ok, "span %d should have _dd.convertedv1 attribute", i)
			assert.Equal(t, "v0.5", val, "span %d should have correct version value", i)
		}
	})

	t.Run("attribute not set when originPayloadVersion is empty", func(t *testing.T) {
		result := ConvertToIdx(payload, "")

		require.Len(t, result.Chunks, 1)
		require.Len(t, result.Chunks[0].Spans, 2)

		// Neither span should have the _dd.convertedv1 attribute set
		for i, span := range result.Chunks[0].Spans {
			_, ok := span.GetAttributeAsString("_dd.convertedv1")
			assert.False(t, ok, "span %d should not have _dd.convertedv1 attribute when originPayloadVersion is empty", i)
		}
	})
}

func TestConverterParity(t *testing.T) {
	// Test that ConvertToIdx and UnmarshalMsgConverted produce functionally equivalent results
	// Note: The string table indices may differ, but the actual string values should be the same

	tests := []struct {
		name   string
		traces pb.Traces
	}{
		{
			name: "basic span",
			traces: pb.Traces{
				{
					{
						Service:  "my-service",
						Name:     "span-name",
						Resource: "GET /res",
						SpanID:   12345678,
						ParentID: 1111,
						Duration: 234,
						Start:    171615,
						TraceID:  556677,
						Type:     "web",
					},
				},
			},
		},
		{
			name: "span with meta and metrics",
			traces: pb.Traces{
				{
					{
						Service:  "my-service",
						Name:     "span-name",
						Resource: "GET /res",
						SpanID:   12345678,
						ParentID: 1111,
						Duration: 234,
						Start:    171615,
						TraceID:  556677,
						Meta: map[string]string{
							"someStr":            "bar",
							"_dd.p.tid":          "BABA",
							"env":                "production",
							"version":            "1.2.3",
							"component":          "http-client",
							"span.kind":          "client",
							"_dd.git.commit.sha": "abc123def456",
							"_dd.p.dm":           "-1",
							"_dd.hostname":       "my-hostname",
						},
						Metrics: map[string]float64{
							"someNum":               1.0,
							"_sampling_priority_v1": 2.0,
						},
					},
				},
			},
		},
		{
			name: "span with error",
			traces: pb.Traces{
				{
					{
						Service:  "test-service",
						Name:     "error-span",
						Resource: "GET /error",
						SpanID:   789,
						ParentID: 0,
						Duration: 100,
						Start:    1000,
						TraceID:  111,
						Error:    1,
					},
				},
			},
		},
		{
			name: "span with links",
			traces: pb.Traces{
				{
					{
						Service:  "test-service",
						Name:     "span-with-links",
						Resource: "GET /linked",
						SpanID:   456,
						ParentID: 0,
						Duration: 200,
						Start:    2000,
						TraceID:  222,
						SpanLinks: []*pb.SpanLink{
							{
								TraceID:     0xAA_BB,
								TraceIDHigh: 0x12_34,
								SpanID:      1111,
								Tracestate:  "test=state",
								Flags:       1,
								Attributes: map[string]string{
									"link.attr": "link.value",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "span with events",
			traces: pb.Traces{
				{
					{
						Service:  "test-service",
						Name:     "span-with-events",
						Resource: "GET /events",
						SpanID:   789,
						ParentID: 0,
						Duration: 300,
						Start:    3000,
						TraceID:  333,
						SpanEvents: []*pb.SpanEvent{
							{
								TimeUnixNano: 171615,
								Name:         "event-name",
								Attributes: map[string]*pb.AttributeAnyValue{
									"event.attr": {
										Type:        pb.AttributeAnyValue_STRING_VALUE,
										StringValue: "event.value",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple spans in trace",
			traces: pb.Traces{
				{
					{
						Service:  "service-1",
						Name:     "parent-span",
						Resource: "GET /parent",
						SpanID:   100,
						ParentID: 0,
						Duration: 500,
						Start:    1000,
						TraceID:  444,
						Meta: map[string]string{
							"env": "staging",
						},
					},
					{
						Service:  "service-1",
						Name:     "child-span",
						Resource: "GET /child",
						SpanID:   101,
						ParentID: 100,
						Duration: 200,
						Start:    1100,
						TraceID:  444,
					},
				},
			},
		},
		{
			name: "multiple traces",
			traces: pb.Traces{
				{
					{
						Service:  "service-a",
						Name:     "span-a",
						Resource: "GET /a",
						SpanID:   200,
						ParentID: 0,
						Duration: 100,
						Start:    5000,
						TraceID:  555,
					},
				},
				{
					{
						Service:  "service-b",
						Name:     "span-b",
						Resource: "GET /b",
						SpanID:   300,
						ParentID: 0,
						Duration: 150,
						Start:    6000,
						TraceID:  666,
					},
				},
			},
		},
		{
			name: "span with metastruct",
			traces: pb.Traces{
				{
					{
						Service:  "test-service",
						Name:     "span-with-metastruct",
						Resource: "GET /meta",
						SpanID:   400,
						ParentID: 0,
						Duration: 50,
						Start:    7000,
						TraceID:  777,
						MetaStruct: map[string][]byte{
							"binary.data": {0x01, 0x02, 0x03},
						},
					},
				},
			},
		},
		{
			name: "span with origin",
			traces: pb.Traces{
				{
					{
						Service:  "test-service",
						Name:     "span-with-origin",
						Resource: "GET /origin",
						SpanID:   500,
						ParentID: 0,
						Duration: 75,
						Start:    8000,
						TraceID:  888,
						Meta: map[string]string{
							"_dd.origin": "lambda",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Method 1: Use UnmarshalMsgConverted
			tracesBytes, err := tt.traces.MarshalMsg(nil)
			require.NoError(t, err)

			tp1 := &idx.InternalTracerPayload{}
			_, err = tp1.UnmarshalMsgConverted(tracesBytes)
			require.NoError(t, err)

			// Method 2: Use ConvertToIdx
			// First convert traces to TracerPayload structure
			tracerPayload := &pb.TracerPayload{
				Chunks: traceChunksFromTraces(tt.traces),
			}
			tp2 := ConvertToIdx(tracerPayload, "v04") // Use "v04" to match what UnmarshalMsgConverted sets

			// Compare the two results
			require.Equal(t, len(tp1.Chunks), len(tp2.Chunks), "number of chunks should match")

			for chunkIdx := range tp1.Chunks {
				chunk1 := tp1.Chunks[chunkIdx]
				chunk2 := tp2.Chunks[chunkIdx]

				// Compare chunk-level fields
				assert.Equal(t, chunk1.Priority, chunk2.Priority, "chunk %d: priority should match", chunkIdx)
				assert.Equal(t, chunk1.Origin(), chunk2.Origin(), "chunk %d: origin should match", chunkIdx)
				assert.Equal(t, chunk1.DroppedTrace, chunk2.DroppedTrace, "chunk %d: dropped trace should match", chunkIdx)
				assert.Equal(t, chunk1.TraceID, chunk2.TraceID, "chunk %d: trace ID should match", chunkIdx)
				assert.Equal(t, chunk1.SamplingMechanism(), chunk2.SamplingMechanism(), "chunk %d: sampling mechanism should match", chunkIdx)

				require.Equal(t, len(chunk1.Spans), len(chunk2.Spans), "chunk %d: number of spans should match", chunkIdx)

				for spanIdx := range chunk1.Spans {
					span1 := chunk1.Spans[spanIdx]
					span2 := chunk2.Spans[spanIdx]

					// Compare span-level fields
					assert.Equal(t, span1.Service(), span2.Service(), "chunk %d span %d: service should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Name(), span2.Name(), "chunk %d span %d: name should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Resource(), span2.Resource(), "chunk %d span %d: resource should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.SpanID(), span2.SpanID(), "chunk %d span %d: span ID should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.ParentID(), span2.ParentID(), "chunk %d span %d: parent ID should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Start(), span2.Start(), "chunk %d span %d: start should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Duration(), span2.Duration(), "chunk %d span %d: duration should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Error(), span2.Error(), "chunk %d span %d: error should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Type(), span2.Type(), "chunk %d span %d: type should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Kind(), span2.Kind(), "chunk %d span %d: kind should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Env(), span2.Env(), "chunk %d span %d: env should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Version(), span2.Version(), "chunk %d span %d: version should match", chunkIdx, spanIdx)
					assert.Equal(t, span1.Component(), span2.Component(), "chunk %d span %d: component should match", chunkIdx, spanIdx)

					// Compare attributes by iterating through each one
					// We use GetAttributeAsString and GetAttributeAsFloat64 which handle promoted fields
					compareSpanAttributes(t, span1, span2, chunkIdx, spanIdx)

					// Compare links
					links1 := span1.Links()
					links2 := span2.Links()
					require.Equal(t, len(links1), len(links2), "chunk %d span %d: number of links should match", chunkIdx, spanIdx)
					for linkIdx := range links1 {
						assert.Equal(t, links1[linkIdx].TraceID(), links2[linkIdx].TraceID(), "chunk %d span %d link %d: trace ID should match", chunkIdx, spanIdx, linkIdx)
						assert.Equal(t, links1[linkIdx].SpanID(), links2[linkIdx].SpanID(), "chunk %d span %d link %d: span ID should match", chunkIdx, spanIdx, linkIdx)
						assert.Equal(t, links1[linkIdx].Tracestate(), links2[linkIdx].Tracestate(), "chunk %d span %d link %d: tracestate should match", chunkIdx, spanIdx, linkIdx)
						assert.Equal(t, links1[linkIdx].Flags(), links2[linkIdx].Flags(), "chunk %d span %d link %d: flags should match", chunkIdx, spanIdx, linkIdx)
						compareLinkAttributes(t, links1[linkIdx], links2[linkIdx], span1.Strings, span2.Strings, chunkIdx, spanIdx, linkIdx)
					}

					// Compare events
					events1 := span1.Events()
					events2 := span2.Events()
					require.Equal(t, len(events1), len(events2), "chunk %d span %d: number of events should match", chunkIdx, spanIdx)
					for eventIdx := range events1 {
						assert.Equal(t, events1[eventIdx].Time(), events2[eventIdx].Time(), "chunk %d span %d event %d: time should match", chunkIdx, spanIdx, eventIdx)
						assert.Equal(t, events1[eventIdx].Name(), events2[eventIdx].Name(), "chunk %d span %d event %d: name should match", chunkIdx, spanIdx, eventIdx)
						compareEventAttributes(t, events1[eventIdx], events2[eventIdx], span1.Strings, span2.Strings, chunkIdx, spanIdx, eventIdx)
					}
				}
			}

			// Compare payload-level fields (these come from span metadata)
			assert.Equal(t, tp1.Env(), tp2.Env(), "payload env should match")
			assert.Equal(t, tp1.Hostname(), tp2.Hostname(), "payload hostname should match")
			assert.Equal(t, tp1.AppVersion(), tp2.AppVersion(), "payload app version should match")
		})
	}
}

// compareSpanAttributes compares attributes between two spans, handling the fact that
// string table indices may be different
func compareSpanAttributes(t *testing.T, span1, span2 *idx.InternalSpan, chunkIdx, spanIdx int) {
	// Get all attribute keys from both spans
	keys1 := make(map[string]bool)
	keys2 := make(map[string]bool)

	for keyRef := range span1.Attributes() {
		key := span1.Strings.Get(keyRef)
		// Skip promoted fields as they are stored differently
		if key == "env" || key == "version" || key == "component" || key == "span.kind" {
			continue
		}
		keys1[key] = true
	}
	for keyRef := range span2.Attributes() {
		key := span2.Strings.Get(keyRef)
		// Skip promoted fields as they are stored differently
		if key == "env" || key == "version" || key == "component" || key == "span.kind" {
			continue
		}
		keys2[key] = true
	}

	// Check all keys from span1 exist in span2 with same values
	for key := range keys1 {
		_, ok := keys2[key]
		assert.True(t, ok, "chunk %d span %d: attribute key %q should exist in both spans", chunkIdx, spanIdx, key)

		// Try to get as string first
		val1, ok1 := span1.GetAttributeAsString(key)
		val2, ok2 := span2.GetAttributeAsString(key)
		if ok1 && ok2 {
			assert.Equal(t, val1, val2, "chunk %d span %d: string attribute %q should match", chunkIdx, spanIdx, key)
			continue
		}

		// Try to get as float64
		fval1, fok1 := span1.GetAttributeAsFloat64(key)
		fval2, fok2 := span2.GetAttributeAsFloat64(key)
		if fok1 && fok2 {
			assert.Equal(t, fval1, fval2, "chunk %d span %d: float64 attribute %q should match", chunkIdx, spanIdx, key)
			continue
		}

		// Fall back to raw attribute comparison
		attr1, _ := span1.GetAttribute(key)
		attr2, _ := span2.GetAttribute(key)
		assert.Equal(t, attr1.AsString(span1.Strings), attr2.AsString(span2.Strings), "chunk %d span %d: attribute %q should match", chunkIdx, spanIdx, key)
	}

	// Check all keys from span2 exist in span1
	for key := range keys2 {
		_, ok := keys1[key]
		assert.True(t, ok, "chunk %d span %d: attribute key %q should exist in both spans", chunkIdx, spanIdx, key)
	}
}

// compareLinkAttributes compares attributes between two span links
func compareLinkAttributes(t *testing.T, link1, link2 *idx.InternalSpanLink, strings1, strings2 *idx.StringTable, chunkIdx, spanIdx, linkIdx int) {
	attrs1 := link1.Attributes()
	attrs2 := link2.Attributes()

	keys1 := make(map[string]string)
	keys2 := make(map[string]string)

	for keyRef, val := range attrs1 {
		key := strings1.Get(keyRef)
		keys1[key] = val.AsString(strings1)
	}
	for keyRef, val := range attrs2 {
		key := strings2.Get(keyRef)
		keys2[key] = val.AsString(strings2)
	}

	assert.Equal(t, keys1, keys2, "chunk %d span %d link %d: attributes should match", chunkIdx, spanIdx, linkIdx)
}

// compareEventAttributes compares attributes between two span events
func compareEventAttributes(t *testing.T, event1, event2 *idx.InternalSpanEvent, strings1, strings2 *idx.StringTable, chunkIdx, spanIdx, eventIdx int) {
	attrs1 := event1.Attributes()
	attrs2 := event2.Attributes()

	keys1 := make(map[string]string)
	keys2 := make(map[string]string)

	for keyRef, val := range attrs1 {
		key := strings1.Get(keyRef)
		keys1[key] = val.AsString(strings1)
	}
	for keyRef, val := range attrs2 {
		key := strings2.Get(keyRef)
		keys2[key] = val.AsString(strings2)
	}

	assert.Equal(t, keys1, keys2, "chunk %d span %d event %d: attributes should match", chunkIdx, spanIdx, eventIdx)
}
