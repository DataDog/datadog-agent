// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"reflect"
	"strings"
	sync "sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInternalTracerPayload_RemoveUnusedStrings(t *testing.T) {
	payload := testPayload()
	payloadValue := reflect.ValueOf(payload).Elem()
	payloadType := payloadValue.Type()
	expectedUsedRefs := make(map[string]uint32)
	for i := 0; i < payloadType.NumField(); i++ {
		field := payloadType.Field(i)
		fieldValue := payloadValue.Field(i)

		// Look for fields ending with "Ref" that are uint32 (string references)
		if field.Type.Kind() == reflect.Uint32 && strings.HasSuffix(field.Name, "Ref") {
			refValue := fieldValue.Uint()
			if refValue != 0 {
				expectedUsedRefs[field.Name] = uint32(refValue)
			} else {
				assert.Fail(t, "testPayload must provide a non-empty string value for all fields", "missing field: %s", field.Name)
			}
		}
	}

	payload.RemoveUnusedStrings()

	// Use reflection to verify all expected string references are still present
	for fieldName, expectedRef := range expectedUsedRefs {
		field := payloadValue.FieldByName(fieldName)
		if field.IsValid() {
			actualRef := field.Uint()
			// Get the string value to verify it's still there
			stringValue := payload.Strings.Get(uint32(actualRef))
			assert.NotEmpty(t, stringValue, "String reference field %s should not be empty", fieldName)
			assert.Equal(t, expectedRef, uint32(actualRef), "String reference field %s should not have changed", fieldName)
		}
	}
	// Check InternalTraceChunks within the payload
	for chunkIndex, chunk := range payload.Chunks {
		chunkValue := reflect.ValueOf(chunk).Elem()
		chunkType := chunkValue.Type()
		for i := 0; i < chunkType.NumField(); i++ {
			field := chunkType.Field(i)
			fieldValue := chunkValue.Field(i)
			if field.Type.Kind() == reflect.Uint32 && strings.HasSuffix(field.Name, "Ref") {
				refValue := fieldValue.Uint()
				if refValue != 0 {
					stringValue := payload.Strings.Get(uint32(refValue))
					assert.NotEmpty(t, stringValue, "Chunk %d field %s should not be empty", chunkIndex, field.Name)
				}
			}
		}
		// Check spans within the chunk
		for spanIndex, span := range chunk.Spans {
			spanValue := reflect.ValueOf(span.span).Elem()
			spanType := spanValue.Type()
			for i := 0; i < spanType.NumField(); i++ {
				field := spanType.Field(i)
				fieldValue := spanValue.Field(i)
				if field.Type.Kind() == reflect.Uint32 && strings.HasSuffix(field.Name, "Ref") {
					refValue := fieldValue.Uint()
					if refValue != 0 {
						stringValue := payload.Strings.Get(uint32(refValue))
						assert.NotEmpty(t, stringValue, "Chunk %d span %d field %s should not be empty", chunkIndex, spanIndex, field.Name)
					}
				}
			}
			// Check span links
			for linkIndex, link := range span.span.Links {
				linkValue := reflect.ValueOf(link).Elem()
				linkType := linkValue.Type()
				for i := 0; i < linkType.NumField(); i++ {
					field := linkType.Field(i)
					fieldValue := linkValue.Field(i)
					if field.Type.Kind() == reflect.Uint32 && strings.HasSuffix(field.Name, "Ref") {
						refValue := fieldValue.Uint()
						if refValue != 0 {
							stringValue := payload.Strings.Get(uint32(refValue))
							assert.NotEmpty(t, stringValue, "Chunk %d span %d link %d field %s should not be empty", chunkIndex, spanIndex, linkIndex, field.Name)
						}
					}
				}
			}
			// Check span events
			for eventIndex, event := range span.span.Events {
				eventValue := reflect.ValueOf(event).Elem()
				eventType := eventValue.Type()
				for i := 0; i < eventType.NumField(); i++ {
					field := eventType.Field(i)
					fieldValue := eventValue.Field(i)
					if field.Type.Kind() == reflect.Uint32 && strings.HasSuffix(field.Name, "Ref") {
						refValue := fieldValue.Uint()
						if refValue != 0 {
							stringValue := payload.Strings.Get(uint32(refValue))
							assert.NotEmpty(t, stringValue, "Chunk %d span %d event %d field %s should not be empty", chunkIndex, spanIndex, eventIndex, field.Name)
						}
					}
				}
			}
		}
	}
}

func TestInternalSpan_GetStringAttributeAs_UnknownKey(t *testing.T) {
	strings := NewStringTable()
	span := &InternalSpan{
		Strings: strings,
		span: &Span{
			Attributes: make(map[uint32]*AnyValue),
		},
	}
	span.SetStringAttribute("", "old-value")
	_, found := span.GetAttributeAsString("unknown-key")
	assert.False(t, found)

	span.SetFloat64Attribute("", 1.0)
	_, found = span.GetAttributeAsFloat64("unknown-key")
	assert.False(t, found)
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
	value, found = span.GetAttributeAsString("key2")
	assert.True(t, found)
	assert.Equal(t, "old-value", value)
}

func TestInternalTracerPayload_CutConcurrentSafe(t *testing.T) {
	payload := testPayload()

	halfPayload := payload.Cut(1)

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		payload.Chunks[0].SetStringAttribute("key1", "value1")
		str, found := payload.Chunks[0].GetAttributeAsString("key1")
		assert.True(t, found)
		assert.Equal(t, "value1", str)
		_, found = payload.Chunks[0].GetAttributeAsString("key2")
		assert.False(t, found)
	}()
	go func() {
		defer wg.Done()
		halfPayload.Chunks[0].SetStringAttribute("key2", "value2")
		str, found := halfPayload.Chunks[0].GetAttributeAsString("key2")
		assert.True(t, found)
		assert.Equal(t, "value2", str)
		_, found = halfPayload.Chunks[0].GetAttributeAsString("key1")
		assert.False(t, found)
	}()
	wg.Wait()
}

func testPayload() *InternalTracerPayload {
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
		Spans:             []*InternalSpan{NewInternalSpan(strings, span)},
		DroppedTrace:      false,
		TraceID:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		samplingMechanism: 4,
	}

	// Build a second chunk with all fields
	chunk2 := &InternalTraceChunk{
		Strings:   strings,
		Priority:  1,
		originRef: strings.Add("chunk-origin2"),
		Attributes: map[uint32]*AnyValue{
			strings.Add("attr-key2"): {Value: &AnyValue_StringValueRef{StringValueRef: strings.Add("attr-value2")}},
		},
		Spans:             []*InternalSpan{NewInternalSpan(strings, span)},
		DroppedTrace:      false,
		TraceID:           []byte{6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21},
		samplingMechanism: 4,
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
		Chunks:             []*InternalTraceChunk{chunk, chunk2},
	}

	return payload
}
