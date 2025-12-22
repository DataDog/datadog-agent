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

func TestInternalSpan_MapStringAttributes_BasicValueTransformation(t *testing.T) {
	stringTable := NewStringTable()
	span := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("foo.bar"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("baz")}},
				stringTable.Add("qux"):     {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("quux")}},
			},
		},
	}

	span.MapStringAttributes(func(k, v string) (string, string, bool) {
		return k, strings.ToUpper(v), true
	})

	fooBar, found := span.GetAttributeAsString("foo.bar")
	assert.True(t, found)
	assert.Equal(t, "BAZ", fooBar)

	qux, found := span.GetAttributeAsString("qux")
	assert.True(t, found)
	assert.Equal(t, "QUUX", qux)
}

func TestInternalSpan_MapStringAttributes_KeyTransformation(t *testing.T) {
	stringTable := NewStringTable()
	span := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("foo.bar"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("baz")}},
				stringTable.Add("qux"):     {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("quux")}},
			},
		},
	}

	span.MapStringAttributes(func(k, v string) (string, string, bool) {
		return "dd." + k, v, true
	})

	fooBar, found := span.GetAttributeAsString("dd.foo.bar")
	assert.True(t, found)
	assert.Equal(t, "baz", fooBar)

	qux, found := span.GetAttributeAsString("dd.qux")
	assert.True(t, found)
	assert.Equal(t, "quux", qux)

	_, found = span.GetAttributeAsString("foo.bar")
	assert.False(t, found)
	_, found = span.GetAttributeAsString("qux")
	assert.False(t, found)
}

func TestInternalSpan_MapStringAttributes_NoReplace(t *testing.T) {
	stringTable := NewStringTable()
	originalFoo := "baz"
	originalQux := "quux"

	span := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("foo.bar"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add(originalFoo)}},
				stringTable.Add("qux"):     {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add(originalQux)}},
			},
		},
	}

	span.MapStringAttributes(func(k, v string) (string, string, bool) {
		return "transformed-" + k, "transformed-" + v, false
	})

	fooBar, found := span.GetAttributeAsString("foo.bar")
	assert.True(t, found)
	assert.Equal(t, originalFoo, fooBar)

	qux, found := span.GetAttributeAsString("qux")
	assert.True(t, found)
	assert.Equal(t, originalQux, qux)
}

func TestInternalSpan_MapStringAttributes_NonStringAttributesIgnored(t *testing.T) {
	stringTable := NewStringTable()
	span := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("string.attr"):  {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("string-value")}},
				stringTable.Add("number.attr"):  {Value: &AnyValue_DoubleValue{DoubleValue: 42.0}},
				stringTable.Add("boolean.attr"): {Value: &AnyValue_BoolValue{BoolValue: true}},
			},
		},
	}

	span.MapStringAttributes(func(k, v string) (string, string, bool) {
		return "transformed-" + k, "transformed-" + v, true
	})

	stringAttr, found := span.GetAttributeAsString("transformed-string.attr")
	assert.True(t, found)
	assert.Equal(t, "transformed-string-value", stringAttr)

	numberAttr, found := span.GetAttributeAsFloat64("number.attr")
	assert.True(t, found)
	assert.Equal(t, 42.0, numberAttr)

	boolAttrStr, found := span.GetAttributeAsString("boolean.attr")
	assert.True(t, found)
	assert.Equal(t, "true", boolAttrStr)
}

func TestInternalSpan_MapStringAttributes_EmptyAttributes(t *testing.T) {
	stringTable := NewStringTable()
	span := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			Attributes: make(map[uint32]*AnyValue),
		},
	}

	span.MapStringAttributes(func(k, v string) (string, string, bool) {
		return k, v, true
	})

	assert.Empty(t, span.span.Attributes)
}

func TestInternalSpan_MapStringAttributes_MixedAttributes(t *testing.T) {
	stringTable := NewStringTable()
	span := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("string1"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("value1")}},
				stringTable.Add("number"):  {Value: &AnyValue_DoubleValue{DoubleValue: 123.0}},
				stringTable.Add("string2"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("value2")}},
				stringTable.Add("bool"):    {Value: &AnyValue_BoolValue{BoolValue: false}},
			},
		},
	}
	span.MapStringAttributes(func(k, v string) (string, string, bool) {
		return "prefix." + k, "transformed." + v, true
	})

	string1, found := span.GetAttributeAsString("prefix.string1")
	assert.True(t, found)
	assert.Equal(t, "transformed.value1", string1)
	string2, found := span.GetAttributeAsString("prefix.string2")
	assert.True(t, found)
	assert.Equal(t, "transformed.value2", string2)
	number, found := span.GetAttributeAsFloat64("number")
	assert.True(t, found)
	assert.Equal(t, 123.0, number)
	boolAttrStr, found := span.GetAttributeAsString("bool")
	assert.True(t, found)
	assert.Equal(t, "false", boolAttrStr)
	_, found = span.GetAttributeAsString("string1")
	assert.False(t, found)
	_, found = span.GetAttributeAsString("string2")
	assert.False(t, found)
}

func TestInternalSpan_MapStringAttributes_MultipleStringAttributes(t *testing.T) {
	stringTable := NewStringTable()
	span := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("foo"):         {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("bar")}},
				stringTable.Add("potato"):      {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("soup")}},
				stringTable.Add("banana"):      {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("split")}},
				stringTable.Add("pizza"):       {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("slice")}},
				stringTable.Add("http.status"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("200")}},
			},
		},
	}
	span.MapStringAttributes(func(k, v string) (string, string, bool) {
		if strings.HasPrefix(k, "http") {
			return "dd." + k, "status_" + v, true
		}
		return k, v, false
	})

	httpStatus, found := span.GetAttributeAsString("dd.http.status")
	assert.True(t, found)
	assert.Equal(t, "status_200", httpStatus)
	foo, found := span.GetAttributeAsString("foo")
	assert.True(t, found)
	assert.Equal(t, "bar", foo)
	potato, found := span.GetAttributeAsString("potato")
	assert.True(t, found)
	assert.Equal(t, "soup", potato)
	banana, found := span.GetAttributeAsString("banana")
	assert.True(t, found)
	assert.Equal(t, "split", banana)
	pizza, found := span.GetAttributeAsString("pizza")
	assert.True(t, found)
	assert.Equal(t, "slice", pizza)
	_, found = span.GetAttributeAsString("http.status")
	assert.False(t, found)
}

func TestInternalSpan_MapStringAttributes_KeyAndValueTransformation(t *testing.T) {
	stringTable := NewStringTable()
	span := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("user.id"):    {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("12345")}},
				stringTable.Add("request.id"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("req-abc")}},
			},
		},
	}

	span.MapStringAttributes(func(k, v string) (string, string, bool) {
		newKey := "custom." + strings.ToUpper(k)
		newValue := "processed_" + strings.ToUpper(v)
		return newKey, newValue, true
	})

	userID, found := span.GetAttributeAsString("custom.USER.ID")
	assert.True(t, found)
	assert.Equal(t, "processed_12345", userID)

	requestID, found := span.GetAttributeAsString("custom.REQUEST.ID")
	assert.True(t, found)
	assert.Equal(t, "processed_REQ-ABC", requestID)

	_, found = span.GetAttributeAsString("user.id")
	assert.False(t, found)
	_, found = span.GetAttributeAsString("request.id")
	assert.False(t, found)
}

func TestInternalSpan_Clone(t *testing.T) {
	// Create an original span with all fields populated
	stringTable := NewStringTable()
	originalSpan := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			ServiceRef:  stringTable.Add("test-service"),
			NameRef:     stringTable.Add("test-operation"),
			ResourceRef: stringTable.Add("test-resource"),
			SpanID:      12345,
			ParentID:    67890,
			Start:       1000000,
			Duration:    5000,
			Error:       true,
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("http.method"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("GET")}},
				stringTable.Add("http.status"): {Value: &AnyValue_IntValue{IntValue: 200}},
			},
			TypeRef: stringTable.Add("web"),
			Links: []*SpanLink{
				{
					TraceID:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					SpanID:        111,
					TracestateRef: stringTable.Add("tracestate"),
					Attributes: map[uint32]*AnyValue{
						stringTable.Add("link.attr"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("link-value")}},
					},
				},
			},
			Events: []*SpanEvent{
				{
					NameRef: stringTable.Add("event-name"),
					Time:    2000000,
					Attributes: map[uint32]*AnyValue{
						stringTable.Add("event.attr"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("event-value")}},
					},
				},
			},
			EnvRef:       stringTable.Add("production"),
			VersionRef:   stringTable.Add("1.0.0"),
			ComponentRef: stringTable.Add("http-client"),
			Kind:         SpanKind_SPAN_KIND_CLIENT,
		},
	}

	// Clone the span
	clonedSpan := originalSpan.Clone()

	// Verify all fields are copied correctly
	assert.Equal(t, originalSpan.Service(), clonedSpan.Service())
	assert.Equal(t, originalSpan.Name(), clonedSpan.Name())
	assert.Equal(t, originalSpan.Resource(), clonedSpan.Resource())
	assert.Equal(t, originalSpan.SpanID(), clonedSpan.SpanID())
	assert.Equal(t, originalSpan.ParentID(), clonedSpan.ParentID())
	assert.Equal(t, originalSpan.Start(), clonedSpan.Start())
	assert.Equal(t, originalSpan.Duration(), clonedSpan.Duration())
	assert.Equal(t, originalSpan.Error(), clonedSpan.Error())
	assert.Equal(t, originalSpan.Type(), clonedSpan.Type())
	assert.Equal(t, originalSpan.Env(), clonedSpan.Env())
	assert.Equal(t, originalSpan.Version(), clonedSpan.Version())
	assert.Equal(t, originalSpan.Component(), clonedSpan.Component())
	assert.Equal(t, originalSpan.Kind(), clonedSpan.Kind())

	// Verify attributes are copied
	httpMethod, found := clonedSpan.GetAttributeAsString("http.method")
	assert.True(t, found)
	assert.Equal(t, "GET", httpMethod)
	httpStatus, found := clonedSpan.GetAttributeAsFloat64("http.status")
	assert.True(t, found)
	assert.Equal(t, float64(200), httpStatus)

	// Verify the Attributes map is independent (deep copy)
	// Modify the cloned span's attributes and verify original is unaffected
	clonedSpan.SetStringAttribute("new.attribute", "new-value")
	_, found = originalSpan.GetAttributeAsString("new.attribute")
	assert.False(t, found, "Original span should not have the new attribute")

	// Verify the string tables are independent
	assert.NotSame(t, originalSpan.Strings, clonedSpan.Strings)

	// Verify Links slice is copied (length preserved)
	assert.Equal(t, len(originalSpan.span.Links), len(clonedSpan.span.Links))

	// Verify Events slice is copied (length preserved)
	assert.Equal(t, len(originalSpan.span.Events), len(clonedSpan.span.Events))
}

func TestInternalSpan_CloneConcurrentSafe(t *testing.T) {
	// Create an original span
	stringTable := NewStringTable()
	originalSpan := &InternalSpan{
		Strings: stringTable,
		span: &Span{
			ServiceRef:  stringTable.Add("test-service"),
			NameRef:     stringTable.Add("test-operation"),
			ResourceRef: stringTable.Add("test-resource"),
			SpanID:      12345,
			Attributes: map[uint32]*AnyValue{
				stringTable.Add("attr1"): {Value: &AnyValue_StringValueRef{StringValueRef: stringTable.Add("value1")}},
			},
		},
	}

	// Clone the span
	clonedSpan := originalSpan.Clone()

	// Concurrently modify both spans
	wg := &sync.WaitGroup{}
	wg.Add(2)

	// Goroutine 1: Modify original span
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			originalSpan.SetStringAttribute("original.attr", "original-value")
			_, found := originalSpan.GetAttributeAsString("original.attr")
			assert.True(t, found)
		}
	}()

	// Goroutine 2: Modify cloned span
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			clonedSpan.SetStringAttribute("cloned.attr", "cloned-value")
			_, found := clonedSpan.GetAttributeAsString("cloned.attr")
			assert.True(t, found)
		}
	}()

	wg.Wait()

	// Verify that the spans don't have each other's attributes
	_, found := originalSpan.GetAttributeAsString("cloned.attr")
	assert.False(t, found, "Original span should not have cloned span's attribute")

	_, found = clonedSpan.GetAttributeAsString("original.attr")
	assert.False(t, found, "Cloned span should not have original span's attribute")
}

func TestSetStringAttribute_NilAttributesMap(t *testing.T) {
	t.Run("InternalTracerPayload", func(t *testing.T) {
		// Create a payload with nil Attributes (simulating deserialized payload without attributes)
		tp := &InternalTracerPayload{
			Strings:    NewStringTable(),
			Attributes: nil,
		}

		// Should not panic and should properly set the attribute
		tp.SetStringAttribute("test.key", "test.value")

		// Verify the attribute was set
		val, found := tp.GetAttributeAsString("test.key")
		assert.True(t, found, "Attribute should be found after SetStringAttribute")
		assert.Equal(t, "test.value", val)
	})

	t.Run("InternalTraceChunk", func(t *testing.T) {
		strings := NewStringTable()
		// Create a chunk with nil Attributes
		chunk := &InternalTraceChunk{
			Strings:    strings,
			Attributes: nil,
		}

		// Should not panic and should properly set the attribute
		chunk.SetStringAttribute("chunk.key", "chunk.value")

		// Verify the attribute was set
		val, found := chunk.GetAttributeAsString("chunk.key")
		assert.True(t, found, "Attribute should be found after SetStringAttribute")
		assert.Equal(t, "chunk.value", val)
	})
}
