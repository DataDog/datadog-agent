// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// findStringRef is a helper function to find a string's reference (index) in the string table
func findStringRef(t *testing.T, stringTable []string, target string) uint32 {
	t.Helper()
	for i, s := range stringTable {
		if s == target {
			return uint32(i)
		}
	}
	t.Fatalf("String %q not found in string table", target)
	return 0
}

func TestMarshalAgentPayload_EmptyPayload(t *testing.T) {
	ap := &AgentPayload{}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.Empty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, ap.HostName, decoded.HostName)
	assert.Equal(t, ap.Env, decoded.Env)
}

func TestMarshalAgentPayload_NilPayload(t *testing.T) {
	data, err := MarshalAgentPayload(nil)
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestMarshalAgentPayload_BasicFields(t *testing.T) {
	ap := &AgentPayload{
		HostName:           "test-host",
		Env:                "production",
		AgentVersion:       "7.50.0",
		TargetTPS:          100.5,
		ErrorTPS:           10.25,
		RareSamplerEnabled: true,
	}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify all basic fields
	assert.Equal(t, ap.HostName, decoded.HostName)
	assert.Equal(t, ap.Env, decoded.Env)
	assert.Equal(t, ap.AgentVersion, decoded.AgentVersion)
	assert.Equal(t, ap.TargetTPS, decoded.TargetTPS)
	assert.Equal(t, ap.ErrorTPS, decoded.ErrorTPS)
	assert.Equal(t, ap.RareSamplerEnabled, decoded.RareSamplerEnabled)
}

func TestMarshalAgentPayload_WithTags(t *testing.T) {
	ap := &AgentPayload{
		HostName: "test-host",
		Tags: map[string]string{
			"region":      "us-east-1",
			"environment": "staging",
			"service":     "my-service",
		},
	}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify tags
	assert.Equal(t, ap.HostName, decoded.HostName)
	require.NotNil(t, decoded.Tags)
	assert.Equal(t, len(ap.Tags), len(decoded.Tags))
	for k, v := range ap.Tags {
		assert.Equal(t, v, decoded.Tags[k], "tag %q should match", k)
	}
}

func TestMarshalAgentPayload_WithIdxTracerPayloads(t *testing.T) {
	ap := &AgentPayload{
		HostName:     "test-host",
		Env:          "test",
		AgentVersion: "7.50.0",
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings:            []string{"", "container-1", "go", "1.21"},
				ContainerIDRef:     1,
				LanguageNameRef:    2,
				LanguageVersionRef: 3,
				EnvRef:             0,
				HostnameRef:        0,
			},
		},
	}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify basic fields
	assert.Equal(t, ap.HostName, decoded.HostName)
	assert.Equal(t, ap.Env, decoded.Env)
	assert.Equal(t, ap.AgentVersion, decoded.AgentVersion)

	// Verify IdxTracerPayloads
	require.Len(t, decoded.IdxTracerPayloads, 1)
	tp := decoded.IdxTracerPayloads[0]

	// Check resolved string values (compaction may reorder strings)
	assert.Equal(t, "container-1", tp.Strings[tp.ContainerIDRef])
	assert.Equal(t, "go", tp.Strings[tp.LanguageNameRef])
	assert.Equal(t, "1.21", tp.Strings[tp.LanguageVersionRef])
}

func TestMarshalAgentPayload_MultipleIdxTracerPayloads(t *testing.T) {
	ap := &AgentPayload{
		HostName: "multi-host",
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings:         []string{"", "container-1"},
				ContainerIDRef:  1,
				LanguageNameRef: 0,
			},
			{
				Strings:            []string{"", "container-2", "python", "3.11"},
				ContainerIDRef:     1,
				LanguageNameRef:    2,
				LanguageVersionRef: 3,
			},
			{
				Strings:          []string{"", "container-3", "java", "17", "1.2.3"},
				ContainerIDRef:   1,
				LanguageNameRef:  2,
				TracerVersionRef: 4,
			},
		},
	}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify IdxTracerPayloads count
	require.Len(t, decoded.IdxTracerPayloads, 3)

	// Verify each payload by checking resolved string values (compaction reorders strings)
	tp0 := decoded.IdxTracerPayloads[0]
	assert.Equal(t, "container-1", tp0.Strings[tp0.ContainerIDRef])

	tp1 := decoded.IdxTracerPayloads[1]
	assert.Equal(t, "container-2", tp1.Strings[tp1.ContainerIDRef])
	assert.Equal(t, "python", tp1.Strings[tp1.LanguageNameRef])
	assert.Equal(t, "3.11", tp1.Strings[tp1.LanguageVersionRef])

	tp2 := decoded.IdxTracerPayloads[2]
	assert.Equal(t, "container-3", tp2.Strings[tp2.ContainerIDRef])
	assert.Equal(t, "java", tp2.Strings[tp2.LanguageNameRef])
	assert.Equal(t, "1.2.3", tp2.Strings[tp2.TracerVersionRef])
}

func TestMarshalAgentPayload_WithTraceChunks(t *testing.T) {
	ap := &AgentPayload{
		HostName:     "chunk-host",
		AgentVersion: "7.50.0",
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings:        []string{"", "my-service", "span-name", "GET /api"},
				ContainerIDRef: 0,
				Chunks: []*idx.TraceChunk{
					{
						Priority:     1,
						TraceID:      []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
						DroppedTrace: false,
						Spans: []*idx.Span{
							{
								ServiceRef:  1,
								NameRef:     2,
								ResourceRef: 3,
								SpanID:      12345,
								ParentID:    0,
								Start:       1000000000,
								Duration:    500000,
							},
						},
					},
				},
			},
		},
	}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	require.Len(t, decoded.IdxTracerPayloads, 1)
	tp := decoded.IdxTracerPayloads[0]
	require.Len(t, tp.Chunks, 1)
	chunk := tp.Chunks[0]
	assert.Equal(t, int32(1), chunk.Priority)
	assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}, chunk.TraceID)
	assert.False(t, chunk.DroppedTrace)

	require.Len(t, chunk.Spans, 1)
	span := chunk.Spans[0]
	// Check resolved string values (compaction reorders strings)
	assert.Equal(t, "my-service", tp.Strings[span.ServiceRef])
	assert.Equal(t, "span-name", tp.Strings[span.NameRef])
	assert.Equal(t, "GET /api", tp.Strings[span.ResourceRef])
	assert.Equal(t, uint64(12345), span.SpanID)
	assert.Equal(t, uint64(0), span.ParentID)
	assert.Equal(t, uint64(1000000000), span.Start)
	assert.Equal(t, uint64(500000), span.Duration)
}

func TestMarshalAgentPayload_CompletePayload(t *testing.T) {
	ap := &AgentPayload{
		HostName:           "complete-host",
		Env:                "production",
		AgentVersion:       "7.50.0",
		TargetTPS:          100.0,
		ErrorTPS:           10.0,
		RareSamplerEnabled: true,
		Tags: map[string]string{
			"datacenter": "dc1",
			"cluster":    "prod-cluster",
		},
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings:            []string{"", "container-abc", "go", "1.21.0", "dd-trace-go/v1.60.0", "runtime-123"},
				ContainerIDRef:     1,
				LanguageNameRef:    2,
				LanguageVersionRef: 3,
				TracerVersionRef:   4,
				RuntimeIDRef:       5,
				Chunks: []*idx.TraceChunk{
					{
						Priority:          2,
						TraceID:           []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99},
						SamplingMechanism: 8,
					},
				},
			},
		},
	}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, ap.HostName, decoded.HostName)
	assert.Equal(t, ap.Env, decoded.Env)
	assert.Equal(t, ap.AgentVersion, decoded.AgentVersion)
	assert.Equal(t, ap.TargetTPS, decoded.TargetTPS)
	assert.Equal(t, ap.ErrorTPS, decoded.ErrorTPS)
	assert.Equal(t, ap.RareSamplerEnabled, decoded.RareSamplerEnabled)

	// Verify tags
	assert.Equal(t, len(ap.Tags), len(decoded.Tags))
	for k, v := range ap.Tags {
		assert.Equal(t, v, decoded.Tags[k])
	}

	// Verify IdxTracerPayloads
	require.Len(t, decoded.IdxTracerPayloads, 1)
	tp := decoded.IdxTracerPayloads[0]

	// Check resolved string values (compaction reorders strings)
	assert.Equal(t, "container-abc", tp.Strings[tp.ContainerIDRef])
	assert.Equal(t, "go", tp.Strings[tp.LanguageNameRef])
	assert.Equal(t, "1.21.0", tp.Strings[tp.LanguageVersionRef])
	assert.Equal(t, "dd-trace-go/v1.60.0", tp.Strings[tp.TracerVersionRef])
	assert.Equal(t, "runtime-123", tp.Strings[tp.RuntimeIDRef])

	require.Len(t, tp.Chunks, 1)
	assert.Equal(t, int32(2), tp.Chunks[0].Priority)
	assert.Equal(t, uint32(8), tp.Chunks[0].SamplingMechanism)
}

func TestMarshalAgentPayload_VTDecoder(t *testing.T) {
	// Test that our custom serializer output can be decoded by the VT decoder
	ap := &AgentPayload{
		HostName:           "vt-test-host",
		Env:                "test",
		AgentVersion:       "7.50.0",
		TargetTPS:          50.5,
		ErrorTPS:           5.5,
		RareSamplerEnabled: false,
		Tags: map[string]string{
			"key1": "value1",
		},
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings:        []string{"", "test-container"},
				ContainerIDRef: 1,
			},
		},
	}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)

	// Decode with VT decoder
	decoded := &AgentPayload{}
	err = decoded.UnmarshalVT(data)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, ap.HostName, decoded.HostName)
	assert.Equal(t, ap.Env, decoded.Env)
	assert.Equal(t, ap.AgentVersion, decoded.AgentVersion)
	assert.Equal(t, ap.TargetTPS, decoded.TargetTPS)
	assert.Equal(t, ap.ErrorTPS, decoded.ErrorTPS)
	assert.Equal(t, ap.RareSamplerEnabled, decoded.RareSamplerEnabled)
	assert.Equal(t, ap.Tags, decoded.Tags)
	require.Len(t, decoded.IdxTracerPayloads, 1)
	tp := decoded.IdxTracerPayloads[0]
	// Check resolved string value (compaction reorders strings)
	assert.Equal(t, "test-container", tp.Strings[tp.ContainerIDRef])
}

func TestMarshalAgentPayload_SizeCalculation(t *testing.T) {
	ap := &AgentPayload{
		HostName:           "size-test-host",
		Env:                "test",
		AgentVersion:       "7.50.0",
		TargetTPS:          100.0,
		ErrorTPS:           10.0,
		RareSamplerEnabled: true,
		Tags: map[string]string{
			"key": "value",
		},
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings:        []string{"", "container"},
				ContainerIDRef: 1,
			},
		},
	}

	// Calculate size
	expectedSize := SizeAgentPayload(ap)

	// Serialize and verify actual size matches
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.Equal(t, expectedSize, len(data), "calculated size should match actual serialized size")
}

func TestMarshalAgentPayload_ZeroValues(t *testing.T) {
	// Test that zero values for numeric fields are not serialized
	ap := &AgentPayload{
		HostName:           "zero-test",
		TargetTPS:          0,     // should not be serialized
		ErrorTPS:           0,     // should not be serialized
		RareSamplerEnabled: false, // should not be serialized
	}

	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)

	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.Equal(t, "zero-test", decoded.HostName)
	assert.Equal(t, float64(0), decoded.TargetTPS)
	assert.Equal(t, float64(0), decoded.ErrorTPS)
	assert.False(t, decoded.RareSamplerEnabled)
}

func TestMarshalAgentPayload_LargeVarintValues(t *testing.T) {
	// Test with large string reference values to verify varint encoding
	// Note: Compaction will remap indices, but the resolved values should be correct
	ap := &AgentPayload{
		HostName: "large-varint-test",
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings:            make([]string, 300), // Create a large strings array
				ContainerIDRef:     299,                 // Large value requiring 2 bytes for varint
				LanguageNameRef:    298,
				LanguageVersionRef: 297,
			},
		},
	}

	// Fill strings array with distinct values at the referenced indices
	for i := range ap.IdxTracerPayloads[0].Strings {
		ap.IdxTracerPayloads[0].Strings[i] = ""
	}
	ap.IdxTracerPayloads[0].Strings[299] = "container-299"
	ap.IdxTracerPayloads[0].Strings[298] = "language-298"
	ap.IdxTracerPayloads[0].Strings[297] = "version-297"

	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)

	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	require.Len(t, decoded.IdxTracerPayloads, 1)
	tp := decoded.IdxTracerPayloads[0]
	// Check resolved string values (compaction remaps indices)
	assert.Equal(t, "container-299", tp.Strings[tp.ContainerIDRef])
	assert.Equal(t, "language-298", tp.Strings[tp.LanguageNameRef])
	assert.Equal(t, "version-297", tp.Strings[tp.LanguageVersionRef])
}

func TestMarshalAgentPayload_TracerPayloadsIgnored(t *testing.T) {
	// Verify that the old TracerPayloads field is ignored during serialization
	ap := &AgentPayload{
		HostName: "ignore-test",
		TracerPayloads: []*TracerPayload{
			{
				ContainerID:     "should-be-ignored",
				LanguageName:    "go",
				LanguageVersion: "1.21",
			},
		},
	}

	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)

	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// The old TracerPayloads should not be in the decoded output
	assert.Equal(t, "ignore-test", decoded.HostName)
	assert.Empty(t, decoded.TracerPayloads)
}

func TestMarshalAgentPayload_SpecialCharactersInStrings(t *testing.T) {
	ap := &AgentPayload{
		HostName:     "special-chars-αβγ-日本語",
		Env:          "test-env-with-émojis-🎉",
		AgentVersion: "7.50.0-特殊",
		Tags: map[string]string{
			"tag-αβγ":   "value-日本語",
			"emoji-tag": "🎉🎊🎈",
			"newline\n": "tab\t",
			"quotes\"'": "backslash\\",
		},
	}

	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)

	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.Equal(t, ap.HostName, decoded.HostName)
	assert.Equal(t, ap.Env, decoded.Env)
	assert.Equal(t, ap.AgentVersion, decoded.AgentVersion)
	for k, v := range ap.Tags {
		assert.Equal(t, v, decoded.Tags[k])
	}
}

func TestMarshalAgentPayload_EmptyStringsAndMaps(t *testing.T) {
	ap := &AgentPayload{
		HostName:     "",  // empty string
		Env:          "",  // empty string
		AgentVersion: "",  // empty string
		Tags:         nil, // nil map
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings: []string{}, // empty strings slice
			},
		},
	}

	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)

	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.Equal(t, "", decoded.HostName)
	assert.Equal(t, "", decoded.Env)
	assert.Equal(t, "", decoded.AgentVersion)
}

func TestSizeAgentPayload_NilPayload(t *testing.T) {
	size := SizeAgentPayload(nil)
	assert.Equal(t, 0, size)
}

func TestAppendAgentPayload_NilPayload(t *testing.T) {
	buf := []byte{0x01, 0x02, 0x03}
	result, err := AppendAgentPayload(buf, nil)
	require.NoError(t, err)
	assert.Equal(t, buf, result)
}

func TestAppendAgentPayload_ToExistingBuffer(t *testing.T) {
	ap := &AgentPayload{
		HostName: "append-test",
	}

	// Start with a non-empty buffer
	existingData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	result, err := AppendAgentPayload(existingData, ap)
	require.NoError(t, err)

	// Verify the existing data is preserved at the beginning
	assert.Equal(t, existingData, result[:4])

	// Verify the appended data can be decoded
	decoded := &AgentPayload{}
	err = proto.Unmarshal(result[4:], decoded)
	require.NoError(t, err)
	assert.Equal(t, "append-test", decoded.HostName)
}

func TestMarshalAgentPayload_CompletePayload_FullTrace(t *testing.T) {
	ap := &AgentPayload{
		HostName:           "complete-host",
		Env:                "production",
		AgentVersion:       "7.50.0",
		TargetTPS:          100.0,
		ErrorTPS:           10.0,
		RareSamplerEnabled: true,
		Tags: map[string]string{
			"datacenter": "dc1",
			"cluster":    "prod-cluster",
		},
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings: []string{
					"",                    // 0
					"container-abc",       // 1
					"go",                  // 2
					"1.21.0",              // 3
					"dd-trace-go/v1.60.0", // 4
					"runtime-123",         // 5
					"lambda",              // 6
					"chunk-attr-key",      // 7
					"chunk-attr-value",    // 8
					"web-service",         // 9
					"http.request",        // 10
					"GET /api/users",      // 11
					"web",                 // 12
					"production",          // 13
					"1.0.0",               // 14
					"http-client",         // 15
					"string-attr-key",     // 16
					"string-attr-value",   // 17
					"bool-attr-key",       // 18
					"double-attr-key",     // 19
					"int-attr-key",        // 20
					"bytes-attr-key",      // 21
					"array-attr-key",      // 22
					"array-elem-1",        // 23
					"array-elem-2",        // 24
					"kvlist-attr-key",     // 25
					"nested-key-1",        // 26
					"nested-value-1",      // 27
					"nested-key-2",        // 28
					"db-service",          // 29
					"db.query",            // 30
					"SELECT * FROM users", // 31
					"sql",                 // 32
					"span-event-name",     // 33
					"event-attr-key",      // 34
					"event-attr-value",    // 35
					"link-tracestate",     // 36
					"link-attr-key",       // 37
					"link-attr-value",     // 38
				},
				ContainerIDRef:     1,
				LanguageNameRef:    2,
				LanguageVersionRef: 3,
				TracerVersionRef:   4,
				RuntimeIDRef:       5,
				Chunks: []*idx.TraceChunk{
					{
						// All chunk fields populated
						Priority:          2,
						OriginRef:         6, // "lambda"
						DroppedTrace:      false,
						TraceID:           []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99},
						SamplingMechanism: 8,
						// Chunk-level attributes
						Attributes: map[uint32]*idx.AnyValue{
							7: {Value: &idx.AnyValue_StringValueRef{StringValueRef: 8}}, // chunk-attr-key -> chunk-attr-value
						},
						// Two spans with various attribute types
						Spans: []*idx.Span{
							// Span 1: HTTP request span with all attribute types
							{
								ServiceRef:   9,  // "web-service"
								NameRef:      10, // "http.request"
								ResourceRef:  11, // "GET /api/users"
								SpanID:       0x123456789ABCDEF0,
								ParentID:     0,
								Start:        1700000000000000000, // nanoseconds
								Duration:     50000000,            // 50ms
								Error:        false,
								TypeRef:      12, // "web"
								EnvRef:       13, // "production"
								VersionRef:   14, // "1.0.0"
								ComponentRef: 15, // "http-client"
								Kind:         idx.SpanKind_SPAN_KIND_SERVER,
								// Attributes with one of each type
								Attributes: map[uint32]*idx.AnyValue{
									// StringValueRef
									16: {Value: &idx.AnyValue_StringValueRef{StringValueRef: 17}}, // string-attr-key -> string-attr-value
									// BoolValue
									18: {Value: &idx.AnyValue_BoolValue{BoolValue: true}}, // bool-attr-key -> true
									// DoubleValue
									19: {Value: &idx.AnyValue_DoubleValue{DoubleValue: 3.14159}}, // double-attr-key -> 3.14159
									// IntValue
									20: {Value: &idx.AnyValue_IntValue{IntValue: 42}}, // int-attr-key -> 42
									// BytesValue
									21: {Value: &idx.AnyValue_BytesValue{BytesValue: []byte{0xDE, 0xAD, 0xBE, 0xEF}}}, // bytes-attr-key -> 0xDEADBEEF
									// ArrayValue
									22: {Value: &idx.AnyValue_ArrayValue{ArrayValue: &idx.ArrayValue{
										Values: []*idx.AnyValue{
											{Value: &idx.AnyValue_StringValueRef{StringValueRef: 23}}, // array-elem-1
											{Value: &idx.AnyValue_StringValueRef{StringValueRef: 24}}, // array-elem-2
											{Value: &idx.AnyValue_IntValue{IntValue: 100}},
										},
									}}},
									// KeyValueList
									25: {Value: &idx.AnyValue_KeyValueList{KeyValueList: &idx.KeyValueList{
										KeyValues: []*idx.KeyValue{
											{Key: 26, Value: &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: 27}}}, // nested-key-1 -> nested-value-1
											{Key: 28, Value: &idx.AnyValue{Value: &idx.AnyValue_IntValue{IntValue: 999}}},            // nested-key-2 -> 999
										},
									}}},
								},
								// SpanLinks
								Links: []*idx.SpanLink{
									{
										TraceID:       []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00},
										SpanID:        0xFEDCBA9876543210,
										TracestateRef: 36, // "link-tracestate"
										Flags:         0x80000001,
										Attributes: map[uint32]*idx.AnyValue{
											37: {Value: &idx.AnyValue_StringValueRef{StringValueRef: 38}}, // link-attr-key -> link-attr-value
										},
									},
								},
								// SpanEvents
								Events: []*idx.SpanEvent{
									{
										Time:    1700000000025000000, // 25ms into the span
										NameRef: 33,                  // "span-event-name"
										Attributes: map[uint32]*idx.AnyValue{
											34: {Value: &idx.AnyValue_StringValueRef{StringValueRef: 35}}, // event-attr-key -> event-attr-value
										},
									},
								},
							},
							// Span 2: Database query span (child of Span 1)
							{
								ServiceRef:  29, // "db-service"
								NameRef:     30, // "db.query"
								ResourceRef: 31, // "SELECT * FROM users"
								SpanID:      0x0FEDCBA987654321,
								ParentID:    0x123456789ABCDEF0,  // Parent is Span 1
								Start:       1700000000010000000, // 10ms after parent start
								Duration:    30000000,            // 30ms
								Error:       true,                // This span has an error
								TypeRef:     32,                  // "sql"
								Kind:        idx.SpanKind_SPAN_KIND_CLIENT,
								// Different attribute types for variety
								Attributes: map[uint32]*idx.AnyValue{
									// BoolValue (error indicator)
									18: {Value: &idx.AnyValue_BoolValue{BoolValue: false}},
									// DoubleValue (query timing metric)
									19: {Value: &idx.AnyValue_DoubleValue{DoubleValue: 29.5}},
									// IntValue (rows returned)
									20: {Value: &idx.AnyValue_IntValue{IntValue: 150}},
									// BytesValue (query hash)
									21: {Value: &idx.AnyValue_BytesValue{BytesValue: []byte{0xCA, 0xFE, 0xBA, 0xBE}}},
								},
							},
						},
					},
				},
			},
		},
	}

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify all AgentPayload fields
	assert.Equal(t, ap.HostName, decoded.HostName)
	assert.Equal(t, ap.Env, decoded.Env)
	assert.Equal(t, ap.AgentVersion, decoded.AgentVersion)
	assert.Equal(t, ap.TargetTPS, decoded.TargetTPS)
	assert.Equal(t, ap.ErrorTPS, decoded.ErrorTPS)
	assert.Equal(t, ap.RareSamplerEnabled, decoded.RareSamplerEnabled)

	// Verify tags
	assert.Equal(t, len(ap.Tags), len(decoded.Tags))
	for k, v := range ap.Tags {
		assert.Equal(t, v, decoded.Tags[k])
	}

	// Verify IdxTracerPayloads
	require.Len(t, decoded.IdxTracerPayloads, 1)
	tp := decoded.IdxTracerPayloads[0]

	// Check resolved string values (compaction reorders strings)
	assert.Equal(t, "container-abc", tp.Strings[tp.ContainerIDRef])
	assert.Equal(t, "go", tp.Strings[tp.LanguageNameRef])
	assert.Equal(t, "1.21.0", tp.Strings[tp.LanguageVersionRef])
	assert.Equal(t, "dd-trace-go/v1.60.0", tp.Strings[tp.TracerVersionRef])
	assert.Equal(t, "runtime-123", tp.Strings[tp.RuntimeIDRef])

	// Verify chunk
	require.Len(t, tp.Chunks, 1)
	chunk := tp.Chunks[0]
	expectedChunk := ap.IdxTracerPayloads[0].Chunks[0]

	assert.Equal(t, expectedChunk.Priority, chunk.Priority)
	assert.Equal(t, "lambda", tp.Strings[chunk.OriginRef])
	assert.Equal(t, expectedChunk.DroppedTrace, chunk.DroppedTrace)
	assert.Equal(t, expectedChunk.TraceID, chunk.TraceID)
	assert.Equal(t, expectedChunk.SamplingMechanism, chunk.SamplingMechanism)

	// Verify chunk attributes - find by resolved key
	require.Len(t, chunk.Attributes, 1)
	chunkAttrKeyRef := findStringRef(t, tp.Strings, "chunk-attr-key")
	chunkAttr := chunk.Attributes[chunkAttrKeyRef]
	require.NotNil(t, chunkAttr)
	assert.Equal(t, "chunk-attr-value", tp.Strings[chunkAttr.GetStringValueRef()])

	// Verify spans
	require.Len(t, chunk.Spans, 2)

	// Span 1 verification - check resolved string values
	span1 := chunk.Spans[0]
	assert.Equal(t, "web-service", tp.Strings[span1.ServiceRef])
	assert.Equal(t, "http.request", tp.Strings[span1.NameRef])
	assert.Equal(t, "GET /api/users", tp.Strings[span1.ResourceRef])
	assert.Equal(t, uint64(0x123456789ABCDEF0), span1.SpanID)
	assert.Equal(t, uint64(0), span1.ParentID)
	assert.Equal(t, uint64(1700000000000000000), span1.Start)
	assert.Equal(t, uint64(50000000), span1.Duration)
	assert.False(t, span1.Error)
	assert.Equal(t, "web", tp.Strings[span1.TypeRef])
	assert.Equal(t, "production", tp.Strings[span1.EnvRef])
	assert.Equal(t, "1.0.0", tp.Strings[span1.VersionRef])
	assert.Equal(t, "http-client", tp.Strings[span1.ComponentRef])
	assert.Equal(t, idx.SpanKind_SPAN_KIND_SERVER, span1.Kind)

	// Verify Span 1 attributes (all types) - find by resolved key
	require.Len(t, span1.Attributes, 7)

	// StringValueRef
	stringAttrKey := findStringRef(t, tp.Strings, "string-attr-key")
	assert.Equal(t, "string-attr-value", tp.Strings[span1.Attributes[stringAttrKey].GetStringValueRef()])

	// BoolValue
	boolAttrKey := findStringRef(t, tp.Strings, "bool-attr-key")
	assert.Equal(t, true, span1.Attributes[boolAttrKey].GetBoolValue())

	// DoubleValue
	doubleAttrKey := findStringRef(t, tp.Strings, "double-attr-key")
	assert.Equal(t, 3.14159, span1.Attributes[doubleAttrKey].GetDoubleValue())

	// IntValue
	intAttrKey := findStringRef(t, tp.Strings, "int-attr-key")
	assert.Equal(t, int64(42), span1.Attributes[intAttrKey].GetIntValue())

	// BytesValue
	bytesAttrKey := findStringRef(t, tp.Strings, "bytes-attr-key")
	assert.Equal(t, []byte{0xDE, 0xAD, 0xBE, 0xEF}, span1.Attributes[bytesAttrKey].GetBytesValue())

	// ArrayValue
	arrayAttrKey := findStringRef(t, tp.Strings, "array-attr-key")
	arrayVal := span1.Attributes[arrayAttrKey].GetArrayValue()
	require.NotNil(t, arrayVal)
	require.Len(t, arrayVal.Values, 3)
	assert.Equal(t, "array-elem-1", tp.Strings[arrayVal.Values[0].GetStringValueRef()])
	assert.Equal(t, "array-elem-2", tp.Strings[arrayVal.Values[1].GetStringValueRef()])
	assert.Equal(t, int64(100), arrayVal.Values[2].GetIntValue())

	// KeyValueList
	kvlistAttrKey := findStringRef(t, tp.Strings, "kvlist-attr-key")
	kvList := span1.Attributes[kvlistAttrKey].GetKeyValueList()
	require.NotNil(t, kvList)
	require.Len(t, kvList.KeyValues, 2)
	assert.Equal(t, "nested-key-1", tp.Strings[kvList.KeyValues[0].Key])
	assert.Equal(t, "nested-value-1", tp.Strings[kvList.KeyValues[0].Value.GetStringValueRef()])
	assert.Equal(t, "nested-key-2", tp.Strings[kvList.KeyValues[1].Key])
	assert.Equal(t, int64(999), kvList.KeyValues[1].Value.GetIntValue())

	// Verify Span 1 links
	require.Len(t, span1.Links, 1)
	link := span1.Links[0]
	assert.Equal(t, []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00}, link.TraceID)
	assert.Equal(t, uint64(0xFEDCBA9876543210), link.SpanID)
	assert.Equal(t, "link-tracestate", tp.Strings[link.TracestateRef])
	assert.Equal(t, uint32(0x80000001), link.Flags)
	require.Len(t, link.Attributes, 1)
	linkAttrKey := findStringRef(t, tp.Strings, "link-attr-key")
	assert.Equal(t, "link-attr-value", tp.Strings[link.Attributes[linkAttrKey].GetStringValueRef()])

	// Verify Span 1 events
	require.Len(t, span1.Events, 1)
	event := span1.Events[0]
	assert.Equal(t, uint64(1700000000025000000), event.Time)
	assert.Equal(t, "span-event-name", tp.Strings[event.NameRef])
	require.Len(t, event.Attributes, 1)
	eventAttrKey := findStringRef(t, tp.Strings, "event-attr-key")
	assert.Equal(t, "event-attr-value", tp.Strings[event.Attributes[eventAttrKey].GetStringValueRef()])

	// Span 2 verification - check resolved string values
	span2 := chunk.Spans[1]
	assert.Equal(t, "db-service", tp.Strings[span2.ServiceRef])
	assert.Equal(t, "db.query", tp.Strings[span2.NameRef])
	assert.Equal(t, "SELECT * FROM users", tp.Strings[span2.ResourceRef])
	assert.Equal(t, uint64(0x0FEDCBA987654321), span2.SpanID)
	assert.Equal(t, uint64(0x123456789ABCDEF0), span2.ParentID)
	assert.Equal(t, uint64(1700000000010000000), span2.Start)
	assert.Equal(t, uint64(30000000), span2.Duration)
	assert.True(t, span2.Error)
	assert.Equal(t, "sql", tp.Strings[span2.TypeRef])
	assert.Equal(t, idx.SpanKind_SPAN_KIND_CLIENT, span2.Kind)

	// Verify Span 2 attributes - note that keys are remapped
	require.Len(t, span2.Attributes, 4)
	// The attribute keys reference the same strings as Span 1's attributes
	assert.Equal(t, false, span2.Attributes[boolAttrKey].GetBoolValue())
	assert.Equal(t, 29.5, span2.Attributes[doubleAttrKey].GetDoubleValue())
	assert.Equal(t, int64(150), span2.Attributes[intAttrKey].GetIntValue())
	assert.Equal(t, []byte{0xCA, 0xFE, 0xBA, 0xBE}, span2.Attributes[bytesAttrKey].GetBytesValue())
}

func TestMarshalAgentPayload_CompletePayload_UnusedStringsRemoved(t *testing.T) {
	// This test verifies that unused strings are removed from the string table
	// and all references are remapped to the new indices.
	//
	// Input string table has 33 strings, but many are unused:
	// - Unused: 10 ("http.request"), 11 ("GET /api/users"), 18-21 (attr keys), 29-32 (db strings)
	//
	// Expected output string table (23 strings) after removing unused:
	// 0: ""                    (was 0)
	// 1: "container-abc"       (was 1)
	// 2: "go"                  (was 2)
	// 3: "1.21.0"              (was 3)
	// 4: "dd-trace-go/v1.60.0" (was 4)
	// 5: "runtime-123"         (was 5)
	// 6: "lambda"              (was 6)
	// 7: "chunk-attr-key"      (was 7)
	// 8: "chunk-attr-value"    (was 8)
	// 9: "web-service"         (was 9)
	// 10: "web"                (was 12)
	// 11: "production"         (was 13)
	// 12: "1.0.0"              (was 14)
	// 13: "http-client"        (was 15)
	// 14: "string-attr-key"    (was 16)
	// 15: "string-attr-value"  (was 17)
	// 16: "array-attr-key"     (was 22)
	// 17: "array-elem-1"       (was 23)
	// 18: "array-elem-2"       (was 24)
	// 19: "kvlist-attr-key"    (was 25)
	// 20: "nested-key-1"       (was 26)
	// 21: "nested-value-1"     (was 27)
	// 22: "nested-key-2"       (was 28)

	ap := &AgentPayload{
		HostName:           "complete-host",
		Env:                "production",
		AgentVersion:       "7.50.0",
		TargetTPS:          100.0,
		ErrorTPS:           10.0,
		RareSamplerEnabled: true,
		Tags: map[string]string{
			"datacenter": "dc1",
			"cluster":    "prod-cluster",
		},
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings: []string{
					"",                    // 0 - used (empty)
					"container-abc",       // 1 - used by ContainerIDRef
					"go",                  // 2 - used by LanguageNameRef
					"1.21.0",              // 3 - used by LanguageVersionRef
					"dd-trace-go/v1.60.0", // 4 - used by TracerVersionRef
					"runtime-123",         // 5 - used by RuntimeIDRef
					"lambda",              // 6 - used by OriginRef
					"chunk-attr-key",      // 7 - used by chunk attributes key
					"chunk-attr-value",    // 8 - used by chunk attributes value
					"web-service",         // 9 - used by ServiceRef, NameRef, ResourceRef
					"http.request",        // 10 - NOT USED (should be removed)
					"GET /api/users",      // 11 - NOT USED (should be removed)
					"web",                 // 12 - used by TypeRef
					"production",          // 13 - used by EnvRef
					"1.0.0",               // 14 - used by VersionRef
					"http-client",         // 15 - used by ComponentRef
					"string-attr-key",     // 16 - used by span attr key
					"string-attr-value",   // 17 - used by span attr value
					"bool-attr-key",       // 18 - NOT USED (should be removed)
					"double-attr-key",     // 19 - NOT USED (should be removed)
					"int-attr-key",        // 20 - NOT USED (should be removed)
					"bytes-attr-key",      // 21 - NOT USED (should be removed)
					"array-attr-key",      // 22 - used by span attr key
					"array-elem-1",        // 23 - used in array value
					"array-elem-2",        // 24 - used in array value
					"kvlist-attr-key",     // 25 - used by span attr key
					"nested-key-1",        // 26 - used in kvlist key
					"nested-value-1",      // 27 - used in kvlist value
					"nested-key-2",        // 28 - used in kvlist key
					"db-service",          // 29 - NOT USED (should be removed)
					"db.query",            // 30 - NOT USED (should be removed)
					"SELECT * FROM users", // 31 - NOT USED (should be removed)
					"sql",                 // 32 - NOT USED (should be removed)
				},
				ContainerIDRef:     1,
				LanguageNameRef:    2,
				LanguageVersionRef: 3,
				TracerVersionRef:   4,
				RuntimeIDRef:       5,
				Chunks: []*idx.TraceChunk{
					{
						Priority:          2,
						OriginRef:         6, // "lambda"
						DroppedTrace:      false,
						TraceID:           []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99},
						SamplingMechanism: 8,
						Attributes: map[uint32]*idx.AnyValue{
							7: {Value: &idx.AnyValue_StringValueRef{StringValueRef: 8}}, // chunk-attr-key -> chunk-attr-value
						},
						Spans: []*idx.Span{
							{
								ServiceRef:   9, // "web-service"
								NameRef:      9, // "web-service"
								ResourceRef:  9, // "web-service"
								SpanID:       0x123456789ABCDEF0,
								ParentID:     0,
								Start:        1700000000000000000,
								Duration:     50000000,
								Error:        false,
								TypeRef:      12, // "web"
								EnvRef:       13, // "production"
								VersionRef:   14, // "1.0.0"
								ComponentRef: 15, // "http-client"
								Kind:         idx.SpanKind_SPAN_KIND_SERVER,
								Attributes: map[uint32]*idx.AnyValue{
									16: {Value: &idx.AnyValue_StringValueRef{StringValueRef: 17}}, // string-attr-key -> string-attr-value
									22: {Value: &idx.AnyValue_ArrayValue{ArrayValue: &idx.ArrayValue{
										Values: []*idx.AnyValue{
											{Value: &idx.AnyValue_StringValueRef{StringValueRef: 23}}, // array-elem-1
											{Value: &idx.AnyValue_StringValueRef{StringValueRef: 24}}, // array-elem-2
											{Value: &idx.AnyValue_IntValue{IntValue: 100}},
										},
									}}},
									25: {Value: &idx.AnyValue_KeyValueList{KeyValueList: &idx.KeyValueList{
										KeyValues: []*idx.KeyValue{
											{Key: 26, Value: &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: 27}}}, // nested-key-1 -> nested-value-1
											{Key: 28, Value: &idx.AnyValue{Value: &idx.AnyValue_IntValue{IntValue: 999}}},            // nested-key-2 -> 999
										},
									}}},
								},
							},
						},
					},
				},
			},
		},
	}

	// The custom marshaler performs string compaction during serialization,
	// so we don't need to pre-compact the payload.

	// Serialize with custom marshaler
	data, err := MarshalAgentPayload(ap)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Decode with standard protobuf decoder
	decoded := &AgentPayload{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify basic AgentPayload fields (unchanged)
	assert.Equal(t, ap.HostName, decoded.HostName)
	assert.Equal(t, ap.Env, decoded.Env)
	assert.Equal(t, ap.AgentVersion, decoded.AgentVersion)
	assert.Equal(t, ap.TargetTPS, decoded.TargetTPS)
	assert.Equal(t, ap.ErrorTPS, decoded.ErrorTPS)
	assert.Equal(t, ap.RareSamplerEnabled, decoded.RareSamplerEnabled)

	// Verify tags (unchanged)
	assert.Equal(t, len(ap.Tags), len(decoded.Tags))
	for k, v := range ap.Tags {
		assert.Equal(t, v, decoded.Tags[k])
	}

	// Verify IdxTracerPayloads
	require.Len(t, decoded.IdxTracerPayloads, 1)
	tp := decoded.IdxTracerPayloads[0]
	strings := tp.Strings

	// Helper to resolve a string reference to its actual string value
	getString := func(ref uint32) string {
		if int(ref) < len(strings) {
			return strings[ref]
		}
		return "<invalid ref>"
	}

	// Helper to find an attribute by its key's string value
	findAttrByKeyString := func(attrs map[uint32]*idx.AnyValue, keyStr string) *idx.AnyValue {
		for ref, val := range attrs {
			if getString(ref) == keyStr {
				return val
			}
		}
		return nil
	}

	// === KEY ASSERTION: String table should have unused strings removed ===
	// These strings should NOT be in the output (they were unused):
	unusedStrings := []string{
		"http.request",
		"GET /api/users",
		"bool-attr-key",
		"double-attr-key",
		"int-attr-key",
		"bytes-attr-key",
		"db-service",
		"db.query",
		"SELECT * FROM users",
		"sql",
	}
	for _, unused := range unusedStrings {
		assert.NotContains(t, strings, unused, "unused string %q should be removed from string table", unused)
	}

	// These strings SHOULD be in the output (they were used):
	usedStrings := []string{
		"",
		"container-abc",
		"go",
		"1.21.0",
		"dd-trace-go/v1.60.0",
		"runtime-123",
		"lambda",
		"chunk-attr-key",
		"chunk-attr-value",
		"web-service",
		"web",
		"production",
		"1.0.0",
		"http-client",
		"string-attr-key",
		"string-attr-value",
		"array-attr-key",
		"array-elem-1",
		"array-elem-2",
		"kvlist-attr-key",
		"nested-key-1",
		"nested-value-1",
		"nested-key-2",
	}
	for _, used := range usedStrings {
		assert.Contains(t, strings, used, "used string %q should be in string table", used)
	}
	assert.Len(t, strings, 23, "String table should have 23 strings (down from 33)")

	// Verify TracerPayload refs resolve to correct strings
	assert.Equal(t, "container-abc", getString(tp.ContainerIDRef))
	assert.Equal(t, "go", getString(tp.LanguageNameRef))
	assert.Equal(t, "1.21.0", getString(tp.LanguageVersionRef))
	assert.Equal(t, "dd-trace-go/v1.60.0", getString(tp.TracerVersionRef))
	assert.Equal(t, "runtime-123", getString(tp.RuntimeIDRef))

	// Verify chunk
	require.Len(t, tp.Chunks, 1)
	chunk := tp.Chunks[0]

	assert.Equal(t, int32(2), chunk.Priority)
	assert.Equal(t, "lambda", getString(chunk.OriginRef))
	assert.False(t, chunk.DroppedTrace)
	assert.Equal(t, []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99}, chunk.TraceID)
	assert.Equal(t, uint32(8), chunk.SamplingMechanism)

	// Verify chunk attributes by resolving string values
	require.Len(t, chunk.Attributes, 1)
	chunkAttr := findAttrByKeyString(chunk.Attributes, "chunk-attr-key")
	require.NotNil(t, chunkAttr, "chunk attribute with key 'chunk-attr-key' should exist")
	assert.Equal(t, "chunk-attr-value", getString(chunkAttr.GetStringValueRef()))

	// Verify span
	require.Len(t, chunk.Spans, 1)
	span := chunk.Spans[0]

	// Verify span refs resolve to correct strings
	assert.Equal(t, "web-service", getString(span.ServiceRef))
	assert.Equal(t, "web-service", getString(span.NameRef))
	assert.Equal(t, "web-service", getString(span.ResourceRef))
	assert.Equal(t, "web", getString(span.TypeRef))
	assert.Equal(t, "production", getString(span.EnvRef))
	assert.Equal(t, "1.0.0", getString(span.VersionRef))
	assert.Equal(t, "http-client", getString(span.ComponentRef))

	// Verify non-ref fields unchanged
	assert.Equal(t, uint64(0x123456789ABCDEF0), span.SpanID)
	assert.Equal(t, uint64(0), span.ParentID)
	assert.Equal(t, uint64(1700000000000000000), span.Start)
	assert.Equal(t, uint64(50000000), span.Duration)
	assert.False(t, span.Error)
	assert.Equal(t, idx.SpanKind_SPAN_KIND_SERVER, span.Kind)

	// Verify span attributes by resolving string values
	require.Len(t, span.Attributes, 3)

	// StringValueRef attribute
	stringAttr := findAttrByKeyString(span.Attributes, "string-attr-key")
	require.NotNil(t, stringAttr, "span attribute with key 'string-attr-key' should exist")
	assert.Equal(t, "string-attr-value", getString(stringAttr.GetStringValueRef()))

	// ArrayValue attribute
	arrayAttr := findAttrByKeyString(span.Attributes, "array-attr-key")
	require.NotNil(t, arrayAttr, "span attribute with key 'array-attr-key' should exist")
	arrayVal := arrayAttr.GetArrayValue()
	require.NotNil(t, arrayVal)
	require.Len(t, arrayVal.Values, 3)
	assert.Equal(t, "array-elem-1", getString(arrayVal.Values[0].GetStringValueRef()))
	assert.Equal(t, "array-elem-2", getString(arrayVal.Values[1].GetStringValueRef()))
	assert.Equal(t, int64(100), arrayVal.Values[2].GetIntValue())

	// KeyValueList attribute
	kvListAttr := findAttrByKeyString(span.Attributes, "kvlist-attr-key")
	require.NotNil(t, kvListAttr, "span attribute with key 'kvlist-attr-key' should exist")
	kvList := kvListAttr.GetKeyValueList()
	require.NotNil(t, kvList)
	require.Len(t, kvList.KeyValues, 2)

	// Find KV entries by their key strings (order may vary)
	var kv1, kv2 *idx.KeyValue
	for _, kv := range kvList.KeyValues {
		switch getString(kv.Key) {
		case "nested-key-1":
			kv1 = kv
		case "nested-key-2":
			kv2 = kv
		}
	}
	require.NotNil(t, kv1, "KeyValue with key 'nested-key-1' should exist")
	assert.Equal(t, "nested-value-1", getString(kv1.Value.GetStringValueRef()))
	require.NotNil(t, kv2, "KeyValue with key 'nested-key-2' should exist")
	assert.Equal(t, int64(999), kv2.Value.GetIntValue())
}

// =============================================================================
// Benchmarks
// =============================================================================

// createBenchmarkPayload creates a test payload with the specified number of spans
// and a realistic ratio of used vs unused strings in the string table.
func createBenchmarkPayload(numSpans int, unusedStringRatio float64) *AgentPayload {
	// Base strings that are always used
	baseStrings := []string{
		"",                    // 0
		"container-abc",       // 1
		"go",                  // 2
		"1.21.0",              // 3
		"dd-trace-go/v1.60.0", // 4
		"runtime-123",         // 5
		"lambda",              // 6
	}

	// Per-span strings (service, name, resource, type)
	spanStrings := []string{}
	for i := 0; i < numSpans; i++ {
		spanStrings = append(spanStrings,
			"service-"+string(rune('a'+i%26)),
			"operation-"+string(rune('a'+i%26)),
			"resource-"+string(rune('a'+i%26)),
			"type-"+string(rune('a'+i%26)),
		)
	}

	// Calculate unused strings to add
	usedCount := len(baseStrings) + len(spanStrings)
	unusedCount := int(float64(usedCount) * unusedStringRatio)

	// Add unused strings
	unusedStrings := make([]string, unusedCount)
	for i := 0; i < unusedCount; i++ {
		unusedStrings[i] = "unused-string-that-was-removed-from-trace-" + string(rune('0'+i%10))
	}

	// Build complete string table: base + span strings + unused
	allStrings := make([]string, 0, len(baseStrings)+len(spanStrings)+len(unusedStrings))
	allStrings = append(allStrings, baseStrings...)
	allStrings = append(allStrings, spanStrings...)
	allStrings = append(allStrings, unusedStrings...)

	// Create spans referencing the span strings
	spans := make([]*idx.Span, numSpans)
	for i := 0; i < numSpans; i++ {
		baseIdx := uint32(len(baseStrings) + i*4)
		spans[i] = &idx.Span{
			ServiceRef:  baseIdx,
			NameRef:     baseIdx + 1,
			ResourceRef: baseIdx + 2,
			TypeRef:     baseIdx + 3,
			SpanID:      uint64(i + 1),
			ParentID:    uint64(i),
			Start:       1700000000000000000,
			Duration:    50000000,
		}
	}

	return &AgentPayload{
		HostName:           "benchmark-host",
		Env:                "production",
		AgentVersion:       "7.50.0",
		TargetTPS:          100.0,
		ErrorTPS:           10.0,
		RareSamplerEnabled: true,
		Tags: map[string]string{
			"datacenter": "dc1",
			"cluster":    "prod-cluster",
		},
		IdxTracerPayloads: []*idx.TracerPayload{
			{
				Strings:            allStrings,
				ContainerIDRef:     1,
				LanguageNameRef:    2,
				LanguageVersionRef: 3,
				TracerVersionRef:   4,
				RuntimeIDRef:       5,
				Chunks: []*idx.TraceChunk{
					{
						Priority:          2,
						OriginRef:         6,
						TraceID:           []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99},
						SamplingMechanism: 8,
						Spans:             spans,
					},
				},
			},
		},
	}
}

// =============================================================================
// Custom marshaler benchmarks with inline string compaction
// =============================================================================

// BenchmarkMarshalAgentPayload_Custom benchmarks the custom marshaler
func BenchmarkMarshalAgentPayload_Custom_SmallPayload(b *testing.B) {
	template := createBenchmarkPayload(10, 0.5) // 10 spans, 50% unused strings
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		_, err := MarshalAgentPayload(ap)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalAgentPayload_Custom_LargePayload(b *testing.B) {
	template := createBenchmarkPayload(1000, 0.5) // 1000 spans, 50% unused strings
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		_, err := MarshalAgentPayload(ap)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalAgentPayload_Custom_HighUnused(b *testing.B) {
	template := createBenchmarkPayload(100, 2.0) // 200% unused = many unused strings
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		_, err := MarshalAgentPayload(ap)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Prepared payload benchmarks: Tests the optimized flow where compaction is
// computed once and reused for both size calculation and serialization.
// This is how TraceWriterV1 now works.
// =============================================================================

// BenchmarkPrepared measures the full flow with PreparedTracerPayload:
// 1. PrepareTracerPayload (computes compaction + size)
// 2. MarshalAgentPayloadPrepared (reuses compaction)
func BenchmarkPrepared_SmallPayload(b *testing.B) {
	template := createBenchmarkPayload(10, 0.5)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		// Prepare payloads (compute compaction once)
		prepared := make([]*PreparedTracerPayload, len(ap.IdxTracerPayloads))
		for j, tp := range ap.IdxTracerPayloads {
			prepared[j] = PrepareTracerPayload(tp)
		}
		// Marshal using prepared payloads (reuses compaction)
		_, err := MarshalAgentPayloadPrepared(ap, prepared)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrepared_MediumPayload(b *testing.B) {
	template := createBenchmarkPayload(100, 0.5)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		prepared := make([]*PreparedTracerPayload, len(ap.IdxTracerPayloads))
		for j, tp := range ap.IdxTracerPayloads {
			prepared[j] = PrepareTracerPayload(tp)
		}
		_, err := MarshalAgentPayloadPrepared(ap, prepared)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrepared_LargePayload(b *testing.B) {
	template := createBenchmarkPayload(1000, 0.5)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		prepared := make([]*PreparedTracerPayload, len(ap.IdxTracerPayloads))
		for j, tp := range ap.IdxTracerPayloads {
			prepared[j] = PrepareTracerPayload(tp)
		}
		_, err := MarshalAgentPayloadPrepared(ap, prepared)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrepared_HighUnused(b *testing.B) {
	template := createBenchmarkPayload(100, 2.0)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		prepared := make([]*PreparedTracerPayload, len(ap.IdxTracerPayloads))
		for j, tp := range ap.IdxTracerPayloads {
			prepared[j] = PrepareTracerPayload(tp)
		}
		_, err := MarshalAgentPayloadPrepared(ap, prepared)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPrepareOnly measures just the PrepareTracerPayload call
// (compaction + size calculation, no serialization)
func BenchmarkPrepareOnly_SmallPayload(b *testing.B) {
	template := createBenchmarkPayload(10, 0.5)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		for _, tp := range ap.IdxTracerPayloads {
			_ = PrepareTracerPayload(tp)
		}
	}
}

func BenchmarkPrepareOnly_MediumPayload(b *testing.B) {
	template := createBenchmarkPayload(100, 0.5)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		for _, tp := range ap.IdxTracerPayloads {
			_ = PrepareTracerPayload(tp)
		}
	}
}

func BenchmarkPrepareOnly_LargePayload(b *testing.B) {
	template := createBenchmarkPayload(1000, 0.5)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ap := cloneAgentPayload(template)
		for _, tp := range ap.IdxTracerPayloads {
			_ = PrepareTracerPayload(tp)
		}
	}
}

// cloneAgentPayload creates a deep copy of an AgentPayload for benchmarking
// This ensures each benchmark iteration works with a fresh, uncompacted payload
func cloneAgentPayload(src *AgentPayload) *AgentPayload {
	if src == nil {
		return nil
	}
	dst := &AgentPayload{
		HostName:           src.HostName,
		Env:                src.Env,
		AgentVersion:       src.AgentVersion,
		TargetTPS:          src.TargetTPS,
		ErrorTPS:           src.ErrorTPS,
		RareSamplerEnabled: src.RareSamplerEnabled,
	}

	if src.Tags != nil {
		dst.Tags = make(map[string]string, len(src.Tags))
		for k, v := range src.Tags {
			dst.Tags[k] = v
		}
	}

	if src.IdxTracerPayloads != nil {
		dst.IdxTracerPayloads = make([]*idx.TracerPayload, len(src.IdxTracerPayloads))
		for i, tp := range src.IdxTracerPayloads {
			dst.IdxTracerPayloads[i] = cloneTracerPayload(tp)
		}
	}

	return dst
}

// cloneTracerPayload creates a deep copy of a TracerPayload
func cloneTracerPayload(src *idx.TracerPayload) *idx.TracerPayload {
	if src == nil {
		return nil
	}
	dst := &idx.TracerPayload{
		ContainerIDRef:     src.ContainerIDRef,
		LanguageNameRef:    src.LanguageNameRef,
		LanguageVersionRef: src.LanguageVersionRef,
		TracerVersionRef:   src.TracerVersionRef,
		RuntimeIDRef:       src.RuntimeIDRef,
		EnvRef:             src.EnvRef,
		HostnameRef:        src.HostnameRef,
		AppVersionRef:      src.AppVersionRef,
	}

	// Clone strings slice
	if src.Strings != nil {
		dst.Strings = make([]string, len(src.Strings))
		copy(dst.Strings, src.Strings)
	}

	// Clone attributes map
	if src.Attributes != nil {
		dst.Attributes = make(map[uint32]*idx.AnyValue, len(src.Attributes))
		for k, v := range src.Attributes {
			dst.Attributes[k] = cloneAnyValue(v)
		}
	}

	// Clone chunks
	if src.Chunks != nil {
		dst.Chunks = make([]*idx.TraceChunk, len(src.Chunks))
		for i, chunk := range src.Chunks {
			dst.Chunks[i] = cloneTraceChunk(chunk)
		}
	}

	return dst
}

// cloneTraceChunk creates a deep copy of a TraceChunk
func cloneTraceChunk(src *idx.TraceChunk) *idx.TraceChunk {
	if src == nil {
		return nil
	}
	dst := &idx.TraceChunk{
		Priority:          src.Priority,
		OriginRef:         src.OriginRef,
		DroppedTrace:      src.DroppedTrace,
		SamplingMechanism: src.SamplingMechanism,
	}

	if src.TraceID != nil {
		dst.TraceID = make([]byte, len(src.TraceID))
		copy(dst.TraceID, src.TraceID)
	}

	if src.Attributes != nil {
		dst.Attributes = make(map[uint32]*idx.AnyValue, len(src.Attributes))
		for k, v := range src.Attributes {
			dst.Attributes[k] = cloneAnyValue(v)
		}
	}

	if src.Spans != nil {
		dst.Spans = make([]*idx.Span, len(src.Spans))
		for i, span := range src.Spans {
			dst.Spans[i] = cloneSpan(span)
		}
	}

	return dst
}

// cloneSpan creates a deep copy of a Span
func cloneSpan(src *idx.Span) *idx.Span {
	if src == nil {
		return nil
	}
	dst := &idx.Span{
		ServiceRef:   src.ServiceRef,
		NameRef:      src.NameRef,
		ResourceRef:  src.ResourceRef,
		SpanID:       src.SpanID,
		ParentID:     src.ParentID,
		Start:        src.Start,
		Duration:     src.Duration,
		Error:        src.Error,
		TypeRef:      src.TypeRef,
		EnvRef:       src.EnvRef,
		VersionRef:   src.VersionRef,
		ComponentRef: src.ComponentRef,
		Kind:         src.Kind,
	}

	if src.Attributes != nil {
		dst.Attributes = make(map[uint32]*idx.AnyValue, len(src.Attributes))
		for k, v := range src.Attributes {
			dst.Attributes[k] = cloneAnyValue(v)
		}
	}

	if src.Links != nil {
		dst.Links = make([]*idx.SpanLink, len(src.Links))
		for i, link := range src.Links {
			dst.Links[i] = cloneSpanLink(link)
		}
	}

	if src.Events != nil {
		dst.Events = make([]*idx.SpanEvent, len(src.Events))
		for i, event := range src.Events {
			dst.Events[i] = cloneSpanEvent(event)
		}
	}

	return dst
}

// cloneSpanLink creates a deep copy of a SpanLink
func cloneSpanLink(src *idx.SpanLink) *idx.SpanLink {
	if src == nil {
		return nil
	}
	dst := &idx.SpanLink{
		SpanID:        src.SpanID,
		TracestateRef: src.TracestateRef,
		Flags:         src.Flags,
	}

	if src.TraceID != nil {
		dst.TraceID = make([]byte, len(src.TraceID))
		copy(dst.TraceID, src.TraceID)
	}

	if src.Attributes != nil {
		dst.Attributes = make(map[uint32]*idx.AnyValue, len(src.Attributes))
		for k, v := range src.Attributes {
			dst.Attributes[k] = cloneAnyValue(v)
		}
	}

	return dst
}

// cloneSpanEvent creates a deep copy of a SpanEvent
func cloneSpanEvent(src *idx.SpanEvent) *idx.SpanEvent {
	if src == nil {
		return nil
	}
	dst := &idx.SpanEvent{
		Time:    src.Time,
		NameRef: src.NameRef,
	}

	if src.Attributes != nil {
		dst.Attributes = make(map[uint32]*idx.AnyValue, len(src.Attributes))
		for k, v := range src.Attributes {
			dst.Attributes[k] = cloneAnyValue(v)
		}
	}

	return dst
}

// cloneAnyValue creates a deep copy of an AnyValue
func cloneAnyValue(src *idx.AnyValue) *idx.AnyValue {
	if src == nil {
		return nil
	}

	switch v := src.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: v.StringValueRef}}
	case *idx.AnyValue_BoolValue:
		return &idx.AnyValue{Value: &idx.AnyValue_BoolValue{BoolValue: v.BoolValue}}
	case *idx.AnyValue_DoubleValue:
		return &idx.AnyValue{Value: &idx.AnyValue_DoubleValue{DoubleValue: v.DoubleValue}}
	case *idx.AnyValue_IntValue:
		return &idx.AnyValue{Value: &idx.AnyValue_IntValue{IntValue: v.IntValue}}
	case *idx.AnyValue_BytesValue:
		bytes := make([]byte, len(v.BytesValue))
		copy(bytes, v.BytesValue)
		return &idx.AnyValue{Value: &idx.AnyValue_BytesValue{BytesValue: bytes}}
	case *idx.AnyValue_ArrayValue:
		if v.ArrayValue == nil {
			return &idx.AnyValue{Value: &idx.AnyValue_ArrayValue{ArrayValue: nil}}
		}
		values := make([]*idx.AnyValue, len(v.ArrayValue.Values))
		for i, val := range v.ArrayValue.Values {
			values[i] = cloneAnyValue(val)
		}
		return &idx.AnyValue{Value: &idx.AnyValue_ArrayValue{ArrayValue: &idx.ArrayValue{Values: values}}}
	case *idx.AnyValue_KeyValueList:
		if v.KeyValueList == nil {
			return &idx.AnyValue{Value: &idx.AnyValue_KeyValueList{KeyValueList: nil}}
		}
		kvs := make([]*idx.KeyValue, len(v.KeyValueList.KeyValues))
		for i, kv := range v.KeyValueList.KeyValues {
			if kv != nil {
				kvs[i] = &idx.KeyValue{
					Key:   kv.Key,
					Value: cloneAnyValue(kv.Value),
				}
			}
		}
		return &idx.AnyValue{Value: &idx.AnyValue_KeyValueList{KeyValueList: &idx.KeyValueList{KeyValues: kvs}}}
	default:
		return nil
	}
}
