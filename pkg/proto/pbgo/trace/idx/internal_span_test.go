// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInternalSpan_SetService_RemovesOldStringFromTable(t *testing.T) {
	strings := NewStringTable()
	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			ServiceRef: strings.Add("old-service"),
		},
	}
	assert.Equal(t, "old-service", strings.Get(span.span.ServiceRef))
	assert.Equal(t, uint32(1), strings.refs[span.span.ServiceRef]) // 1 from Add

	span.SetService("new-service")

	assert.Equal(t, "new-service", span.Service())
	assert.Equal(t, uint32(0), strings.Lookup("old-service"))
	for _, str := range strings.strings {
		// Assert the old service is no longer in the string table
		assert.NotEqual(t, "old-service", str)
	}
}

func TestInternalSpan_SetStringAttribute_RemovesOldStringFromTable(t *testing.T) {
	strings := NewStringTable()
	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			Attributes: make(map[uint32]*AnyValue),
		},
	}

	span.SetStringAttribute("old-key", "old-value")
	value, found := span.GetAttributeAsString("old-key")
	assert.True(t, found)
	assert.Equal(t, "old-value", value)

	span.SetStringAttribute("old-key", "new-value")

	value, found = span.GetAttributeAsString("old-key")
	assert.True(t, found)
	assert.Equal(t, "new-value", value)
	assert.Equal(t, uint32(0), strings.Lookup("old-value"))
	for _, str := range strings.strings {
		// Assert the old value is no longer in the string table
		assert.NotEqual(t, "old-value", str)
	}
}

func TestInternalSpan_MultipleRefsKept(t *testing.T) {
	strings := NewStringTable()
	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			Attributes: make(map[uint32]*AnyValue),
		},
	}

	span.SetStringAttribute("key1", "old-value")
	span.SetStringAttribute("key2", "old-value")
	span.SetStringAttribute("key1", "new-value")

	value, found := span.GetAttributeAsString("key1")
	assert.True(t, found)
	assert.Equal(t, "new-value", value)
	oldValIdx := strings.Lookup("old-value")
	assert.NotZero(t, oldValIdx)
	assert.Equal(t, uint32(1), strings.refs[oldValIdx])
	value, found = span.GetAttributeAsString("key2")
	assert.True(t, found)
	assert.Equal(t, "old-value", value)
}

func TestInternalSpanToProto_CountUsedStrings(t *testing.T) {
	strings := NewStringTable()
	secretIdx := strings.Add("SOME_SECRET")
	keyIdx := strings.Add("some-key")
	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			ServiceRef: strings.Add("some-service"),
			Attributes: map[uint32]*AnyValue{
				keyIdx: {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("some-attribute")}},
			},
		},
	}
	usedStrings := make([]bool, strings.Len())
	span.ToProto(usedStrings)
	assert.True(t, usedStrings[span.span.ServiceRef])
	assert.True(t, usedStrings[span.span.Attributes[1].GetStringValueRef()])
	assert.True(t, usedStrings[keyIdx])
	assert.False(t, usedStrings[secretIdx])
}

func TestInternalTracerPayload_ToProto_UnusedStringsRemoved(t *testing.T) {
	strings := NewStringTable()
	strings.Add("unused-string")

	// Build a span with all fields
	span := &Span{
		ServiceRef:  strings.Add("svc"),
		NameRef:     strings.Add("op"),
		ResourceRef: strings.Add("res"),
		SpanID:      123,
		ParentID:    0,
		Start:       1000,
		Duration:    200,
		Error:       true,
		Attributes: map[uint32]*AnyValue{
			strings.Add("attr-key"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("attr-value")}},
		},
		TypeRef: strings.Add("web"),
		Links: []*SpanLink{
			{
				TracestateRef: strings.Add("ts"),
				Attributes: map[uint32]*AnyValue{
					strings.Add("attr-key"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("attr-value")}},
				},
			},
		},
		Events: []*SpanEvent{
			{
				NameRef: strings.Add("event"),
				Attributes: map[uint32]*AnyValue{
					strings.Add("attr-key"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("attr-value")}},
				},
			},
		},
		EnvRef:       strings.Add("prod"),
		VersionRef:   strings.Add("1.0.1"),
		ComponentRef: strings.Add("http"),
		Kind:         2, // e.g. SPAN_KIND_SERVER
	}

	// Build a chunk with all fields
	chunk := &InternalTraceChunk{
		Strings:   strings,
		Priority:  1,
		originRef: strings.Add("chunk-origin"),
		Attributes: map[uint32]*AnyValue{
			strings.Add("attr-key"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("attr-value")}},
		},
		Spans:            []*InternalSpan{NewInternalSpan(strings, span)},
		DroppedTrace:     false,
		TraceID:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		decisionMakerRef: strings.Add("dm"),
	}

	// Build attributes for the tracer payload
	payloadAttributes := map[uint32]*AnyValue{
		strings.Add("attr-key"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("attr-value")}},
	}

	payload := &InternalTracerPayload{
		Strings:            strings,
		containerIDRef:     strings.Add("container-123"),
		languageNameRef:    strings.Add("go"),
		languageVersionRef: strings.Add("1.21"),
		tracerVersionRef:   strings.Add("v2.0.0"),
		runtimeIDRef:       strings.Add("runtime-uuid"),
		envRef:             strings.Add("prod"),
		hostnameRef:        strings.Add("host-abc"),
		appVersionRef:      strings.Add("1.0.1"),
		Attributes:         payloadAttributes,
		Chunks:             []*InternalTraceChunk{chunk},
	}

	protoPayload := payload.ToProto()

	assert.NotContains(t, protoPayload.Strings, "unused-string")
	assert.Equal(t, "container-123", protoPayload.Strings[protoPayload.ContainerIDRef])
	assert.Equal(t, "go", protoPayload.Strings[protoPayload.LanguageNameRef])
	assert.Equal(t, "1.21", protoPayload.Strings[protoPayload.LanguageVersionRef])
	assert.Equal(t, "v2.0.0", protoPayload.Strings[protoPayload.TracerVersionRef])
	assert.Equal(t, "runtime-uuid", protoPayload.Strings[protoPayload.RuntimeIDRef])
	assert.Equal(t, "prod", protoPayload.Strings[protoPayload.EnvRef])
	assert.Equal(t, "host-abc", protoPayload.Strings[protoPayload.HostnameRef])
	assert.Equal(t, "1.0.1", protoPayload.Strings[protoPayload.AppVersionRef])

	// Check chunk strings
	assert.Len(t, protoPayload.Chunks, 1)
	protoChunk := protoPayload.Chunks[0]
	assert.Equal(t, "chunk-origin", protoPayload.Strings[protoChunk.OriginRef])
	assert.Equal(t, "dm", protoPayload.Strings[protoChunk.DecisionMakerRef])

	// Check span strings
	assert.Len(t, protoChunk.Spans, 1)
	protoSpan := protoChunk.Spans[0]
	assert.Equal(t, "svc", protoPayload.Strings[protoSpan.ServiceRef])
	assert.Equal(t, "op", protoPayload.Strings[protoSpan.NameRef])
	assert.Equal(t, "res", protoPayload.Strings[protoSpan.ResourceRef])
	assert.Equal(t, "web", protoPayload.Strings[protoSpan.TypeRef])
	assert.Equal(t, "prod", protoPayload.Strings[protoSpan.EnvRef])
	assert.Equal(t, "1.0.1", protoPayload.Strings[protoSpan.VersionRef])
	assert.Equal(t, "http", protoPayload.Strings[protoSpan.ComponentRef])

	// Check span links
	assert.Len(t, protoSpan.Links, 1)
	protoLink := protoSpan.Links[0]
	assert.Equal(t, "ts", protoPayload.Strings[protoLink.TracestateRef])

	// Check span events
	assert.Len(t, protoSpan.Events, 1)
	protoEvent := protoSpan.Events[0]
	assert.Equal(t, "event", protoPayload.Strings[protoEvent.NameRef])

	// Check attributes (these should be present in the string table)
	assert.Contains(t, protoPayload.Strings, "attr-key")
	assert.Contains(t, protoPayload.Strings, "attr-value")
}
