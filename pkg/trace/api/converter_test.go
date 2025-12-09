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
					"_dd.p.dm": "5",
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
					"_dd.p.dm": "5",
				},
				Spans: []*pb.Span{
					{
						Service:  "test-service",
						Name:     "test-span",
						Resource: "test-resource",
						TraceID:  123,
						SpanID:   456,
						Meta: map[string]string{
							"_dd.p.dm": "8",
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
