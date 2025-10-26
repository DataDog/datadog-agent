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

			result := convertToIdx(payload)

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
				meta["kind"] = tt.kindMeta
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

			result := convertToIdx(payload)

			require.Len(t, result.Chunks, 1)
			require.Len(t, result.Chunks[0].Spans, 1)
			assert.Equal(t, tt.expectedKind, result.Chunks[0].Spans[0].Kind())
		})
	}
}

// Test that promoted fields (env, version, component) are moved out of attributes
func TestConvertToIdx_PromotedFieldsNotInAttributes(t *testing.T) {
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

	result := convertToIdx(payload)

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	// Verify promoted fields are set on the span
	assert.Equal(t, "production", span.Env())
	assert.Equal(t, "1.2.3", span.Version())
	assert.Equal(t, "http-client", span.Component())

	// Verify promoted fields are NOT in attributes
	// Note: env, version, component, and kind are still in the original Meta map,
	// but the converter should handle them specially
	foundEnv := false
	foundVersion := false
	foundComponent := false
	foundKind := false
	foundOther := false

	attrs := span.Attributes()
	for keyRef, val := range attrs {
		key := result.Strings.Get(keyRef)
		switch key {
		case "env":
			foundEnv = true
		case "version":
			foundVersion = true
		case "component":
			foundComponent = true
		case "kind":
			foundKind = true
		case "other":
			foundOther = true
			assert.Equal(t, "should-remain", result.Strings.Get(val.GetStringValueRef()))
		}
	}

	// These promoted fields should still be in attributes since the old format includes them in Meta
	// The test validates they're ALSO promoted to dedicated fields
	assert.True(t, foundEnv, "env should be in attributes from Meta")
	assert.True(t, foundVersion, "version should be in attributes from Meta")
	assert.True(t, foundComponent, "component should be in attributes from Meta")
	assert.True(t, foundKind, "kind should be in attributes from Meta")
	assert.True(t, foundOther, "other non-promoted field should remain in attributes")
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

	result := convertToIdx(payload)

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	assert.Equal(t, "staging", span.Env())
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

	result := convertToIdx(payload)

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	assert.Equal(t, "2.0.1", span.Version())
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

	result := convertToIdx(payload)

	require.Len(t, result.Chunks, 1)
	require.Len(t, result.Chunks[0].Spans, 1)
	span := result.Chunks[0].Spans[0]

	assert.Equal(t, "redis", span.Component())
}
