// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"fmt"
	"strings"
	sync "sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestMapFilterAttributes(t *testing.T) {
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
	span.MapFilteredAttributes(func(k string) bool {
		return k == "foo.bar"
	}, func(_k, _v string) string {
		return "new value!!"
	})

	fooBar, found := span.GetAttributeAsString("foo.bar")
	assert.True(t, found)
	assert.Equal(t, "new value!!", fooBar)

	qux, found := span.GetAttributeAsString("qux")
	assert.True(t, found)
	assert.Equal(t, "quux", qux)
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

func TestCompactStrings(t *testing.T) {
	payload := testPayload()
	pbPayload := payload.ToProto()

	// Record original string values before compaction
	originalValues := map[string]string{
		"ContainerID":     pbPayload.Strings[pbPayload.ContainerIDRef],
		"LanguageName":    pbPayload.Strings[pbPayload.LanguageNameRef],
		"LanguageVersion": pbPayload.Strings[pbPayload.LanguageVersionRef],
		"TracerVersion":   pbPayload.Strings[pbPayload.TracerVersionRef],
		"RuntimeID":       pbPayload.Strings[pbPayload.RuntimeIDRef],
		"Env":             pbPayload.Strings[pbPayload.EnvRef],
		"Hostname":        pbPayload.Strings[pbPayload.HostnameRef],
		"AppVersion":      pbPayload.Strings[pbPayload.AppVersionRef],
	}

	originalStringsLen := len(pbPayload.Strings)

	// Perform compaction
	pbPayload.CompactStrings()

	// Verify string table is smaller (unused strings removed)
	assert.Less(t, len(pbPayload.Strings), originalStringsLen,
		"Compacted string table should be smaller (was %d, now %d)", originalStringsLen, len(pbPayload.Strings))

	// Verify all TracerPayload level refs still resolve to correct values
	assert.Equal(t, originalValues["ContainerID"], pbPayload.Strings[pbPayload.ContainerIDRef])
	assert.Equal(t, originalValues["LanguageName"], pbPayload.Strings[pbPayload.LanguageNameRef])
	assert.Equal(t, originalValues["LanguageVersion"], pbPayload.Strings[pbPayload.LanguageVersionRef])
	assert.Equal(t, originalValues["TracerVersion"], pbPayload.Strings[pbPayload.TracerVersionRef])
	assert.Equal(t, originalValues["RuntimeID"], pbPayload.Strings[pbPayload.RuntimeIDRef])
	assert.Equal(t, originalValues["Env"], pbPayload.Strings[pbPayload.EnvRef])
	assert.Equal(t, originalValues["Hostname"], pbPayload.Strings[pbPayload.HostnameRef])
	assert.Equal(t, originalValues["AppVersion"], pbPayload.Strings[pbPayload.AppVersionRef])

	// Verify no empty strings in the compacted table (except index 0 if present)
	for i, s := range pbPayload.Strings {
		if i > 0 {
			assert.NotEmpty(t, s, "Compacted string table should not have empty strings at index %d", i)
		}
	}
}

// TestRemapAttributes_KeyOverlap demonstrates a bug where remapAttributes can lose
// attributes when the new key of one attribute equals the old key of another attribute.
func TestRemapAttributes_KeyOverlap(t *testing.T) {
	// Create a scenario where key remapping causes overlap
	// Original: {5: v1, 10: v2}
	// Remap: 5->3, 10->5
	// Expected final: {3: v1, 5: v2}
	// Bug: depending on iteration order, we could get {3: v1} (v2 lost!)

	strings := NewStringTable()
	strings.Add("") // index 0

	// Add strings so we can create the overlap scenario
	// We need old indices 5 and 10 to remap to 3 and 5 respectively
	for i := 1; i <= 10; i++ {
		strings.Add(fmt.Sprintf("str_%d", i)) // indices 1-10
	}

	// Create attributes at indices 5 and 10
	key5Str := strings.Get(5)   // "str_5"
	key10Str := strings.Get(10) // "str_10"
	t.Logf("key5Str=%q, key10Str=%q", key5Str, key10Str)

	attrs := map[uint32]*AnyValue{
		5:  {Value: &AnyValue_StringValueRef{StringValueRef: 1}}, // value "str_1"
		10: {Value: &AnyValue_StringValueRef{StringValueRef: 2}}, // value "str_2"
	}

	// Create a remap function: 5->3, 10->5
	remapFunc := func(oldRef uint32) uint32 {
		switch oldRef {
		case 5:
			return 3
		case 10:
			return 5
		default:
			return oldRef
		}
	}

	// Run remapAttributes multiple times to catch the race (since map iteration is random)
	failures := 0
	for i := 0; i < 100; i++ {
		testAttrs := make(map[uint32]*AnyValue)
		for k, v := range attrs {
			testAttrs[k] = v
		}

		remapAttributes(testAttrs, remapFunc)

		// Expected: {3: v1, 5: v2}
		if len(testAttrs) != 2 {
			failures++
			if failures == 1 {
				t.Logf("Iteration %d: Expected 2 attrs, got %d: %v", i, len(testAttrs), testAttrs)
			}
		}
	}

	if failures > 0 {
		t.Errorf("remapAttributes lost attributes in %d/100 iterations", failures)
	}
}

// TestDoubleCompactStrings_CorruptsSharedSpans demonstrates a bug where calling
// CompactStrings twice on payloads that share span objects corrupts the refs.
// This simulates what happens in TraceWriterV1 when splitting large payloads.
func TestDoubleCompactStrings_CorruptsSharedSpans(t *testing.T) {
	// Create a payload with multiple chunks where each chunk uses DIFFERENT strings.
	// This is crucial because when we split and re-compact, the different chunks
	// will have different mappings, causing corruption.
	strings := NewStringTable()

	// Chunk1 uses these strings
	service1Ref := strings.Add("service-chunk1")
	name1Ref := strings.Add("operation-chunk1")
	resource1Ref := strings.Add("resource-chunk1")
	attrKey1Ref := strings.Add("chunk1.attr.key")
	attrVal1Ref := strings.Add("chunk1.attr.value")

	// Chunk2 uses DIFFERENT strings
	service2Ref := strings.Add("service-chunk2")
	name2Ref := strings.Add("operation-chunk2")
	resource2Ref := strings.Add("resource-chunk2")
	attrKey2Ref := strings.Add("chunk2.attr.key")
	attrVal2Ref := strings.Add("chunk2.attr.value")

	// Add some unused strings so compaction actually changes indices
	for i := 0; i < 20; i++ {
		strings.Add("unused_" + string(rune('a'+i)))
	}

	// Create two chunks with distinct spans using distinct strings
	span1 := &Span{
		ServiceRef:  service1Ref,
		NameRef:     name1Ref,
		ResourceRef: resource1Ref,
		Attributes: map[uint32]*AnyValue{
			attrKey1Ref: {Value: &AnyValue_StringValueRef{StringValueRef: attrVal1Ref}},
		},
	}
	span2 := &Span{
		ServiceRef:  service2Ref,
		NameRef:     name2Ref,
		ResourceRef: resource2Ref,
		Attributes: map[uint32]*AnyValue{
			attrKey2Ref: {Value: &AnyValue_StringValueRef{StringValueRef: attrVal2Ref}},
		},
	}

	chunk1 := &TraceChunk{Spans: []*Span{span1}}
	chunk2 := &TraceChunk{Spans: []*Span{span2}}

	payload := &TracerPayload{
		Strings:         strings.strings,
		ContainerIDRef:  strings.Add("container-1"),
		LanguageNameRef: strings.Add("go"),
		Chunks:          []*TraceChunk{chunk1, chunk2},
	}

	// Step 1: First compaction (simulates pbTracerPayload.CompactStrings())
	payload.CompactStrings()

	// Verify both spans resolve correctly after first compaction
	assert.Equal(t, "service-chunk1", payload.Strings[span1.ServiceRef], "After first compact: span1 service should resolve")
	assert.Equal(t, "service-chunk2", payload.Strings[span2.ServiceRef], "After first compact: span2 service should resolve")

	// Record span2's refs after first compaction (these are now the "true" indices)
	span2ServiceRefAfterFirst := span2.ServiceRef
	span2ServiceValueAfterFirst := payload.Strings[span2ServiceRefAfterFirst]

	t.Logf("After first compact: span2.ServiceRef=%d -> %q", span2ServiceRefAfterFirst, span2ServiceValueAfterFirst)

	// Step 2: Create a split payload with just chunk1 (simulates NewStringsClone + Chunks slice)
	// This is what TraceWriterV1 does when splitting payloads
	splitPayload1 := payload.NewStringsClone()
	splitPayload1.Chunks = []*TraceChunk{chunk1} // Only include chunk1

	// Step 3: Compact the split payload
	// This will remap span1 to use indices valid for splitPayload1's string table.
	// BUT span1 was ALREADY remapped by payload.CompactStrings()!
	splitPayload1.CompactStrings()

	t.Logf("After splitPayload1.CompactStrings: splitPayload1 has %d strings", len(splitPayload1.Strings))

	// Check if span1 still resolves correctly
	if int(span1.ServiceRef) >= len(splitPayload1.Strings) {
		t.Errorf("CORRUPTION: span1.ServiceRef=%d >= len(splitPayload1.Strings)=%d",
			span1.ServiceRef, len(splitPayload1.Strings))
	} else {
		resolvedService := splitPayload1.Strings[span1.ServiceRef]
		if resolvedService != "service-chunk1" {
			t.Errorf("CORRUPTION: span1.ServiceRef resolved to %q, expected 'service-chunk1'", resolvedService)
		}
	}

	// Step 4: Create a second split payload with just chunk2
	splitPayload2 := payload.NewStringsClone()
	splitPayload2.Chunks = []*TraceChunk{chunk2} // Only include chunk2

	// Step 5: Compact the second split payload
	// This will remap span2 AGAIN (it was already remapped by payload.CompactStrings())
	splitPayload2.CompactStrings()

	t.Logf("After splitPayload2.CompactStrings: splitPayload2 has %d strings", len(splitPayload2.Strings))
	t.Logf("span2.ServiceRef is now: %d", span2.ServiceRef)

	// Check if span2's refs are still valid
	if int(span2.ServiceRef) >= len(splitPayload2.Strings) {
		t.Fatalf("CORRUPTION DETECTED: span2.ServiceRef=%d is out of bounds (strings len=%d)",
			span2.ServiceRef, len(splitPayload2.Strings))
	}
	resolvedService := splitPayload2.Strings[span2.ServiceRef]
	assert.Equal(t, "service-chunk2", resolvedService,
		"After double compact: span2 service should still resolve to 'service-chunk2', got %q", resolvedService)

	// Check if the attribute key/value still resolve correctly
	var foundAttr bool
	for keyRef, val := range span2.Attributes {
		if int(keyRef) >= len(splitPayload2.Strings) {
			t.Fatalf("CORRUPTION DETECTED: attribute key ref=%d is out of bounds (strings len=%d)",
				keyRef, len(splitPayload2.Strings))
		}
		keyStr := splitPayload2.Strings[keyRef]
		if keyStr == "chunk2.attr.key" {
			foundAttr = true
			if sv, ok := val.Value.(*AnyValue_StringValueRef); ok {
				if int(sv.StringValueRef) >= len(splitPayload2.Strings) {
					t.Fatalf("CORRUPTION DETECTED: attribute value ref=%d is out of bounds (strings len=%d)",
						sv.StringValueRef, len(splitPayload2.Strings))
				}
				valStr := splitPayload2.Strings[sv.StringValueRef]
				assert.Equal(t, "chunk2.attr.value", valStr,
					"After double compact: attribute value should resolve correctly")
			}
		}
	}
	assert.True(t, foundAttr, "After double compact: chunk2.attr.key attribute should still be findable")
}

// createBenchmarkPayload creates a payload with the specified characteristics
func createBenchmarkPayload(numChunks, spansPerChunk, attrsPerSpan int) *TracerPayload {
	strings := NewStringTable()

	// Add some unused strings to simulate real-world scenario
	for i := 0; i < 100; i++ {
		strings.Add("unused_string_" + string(rune('a'+i%26)) + string(rune('0'+i%10)))
	}

	chunks := make([]*TraceChunk, numChunks)
	for c := 0; c < numChunks; c++ {
		spans := make([]*Span, spansPerChunk)
		for s := 0; s < spansPerChunk; s++ {
			attrs := make(map[uint32]*AnyValue, attrsPerSpan)
			for a := 0; a < attrsPerSpan; a++ {
				keyRef := strings.Add("attr_key_" + string(rune('a'+a%26)))
				valRef := strings.Add("attr_val_" + string(rune('a'+a%26)))
				attrs[keyRef] = &AnyValue{Value: &AnyValue_StringValueRef{StringValueRef: valRef}}
			}
			spans[s] = &Span{
				ServiceRef:   strings.Add("service-name"),
				NameRef:      strings.Add("operation-name"),
				ResourceRef:  strings.Add("resource-name"),
				TypeRef:      strings.Add("web"),
				EnvRef:       strings.Add("production"),
				VersionRef:   strings.Add("1.0.0"),
				ComponentRef: strings.Add("http-client"),
				SpanID:       uint64(s),
				Start:        1000000,
				Duration:     5000,
				Attributes:   attrs,
			}
		}
		chunks[c] = &TraceChunk{
			Priority:  1,
			OriginRef: strings.Add("origin-" + string(rune('a'+c%26))),
			Spans:     spans,
		}
	}

	return &TracerPayload{
		Strings:            strings.strings,
		ContainerIDRef:     strings.Add("container-123"),
		LanguageNameRef:    strings.Add("go"),
		LanguageVersionRef: strings.Add("1.21"),
		TracerVersionRef:   strings.Add("v2.0.0"),
		RuntimeIDRef:       strings.Add("runtime-uuid"),
		EnvRef:             strings.Add("production"),
		HostnameRef:        strings.Add("host-abc"),
		AppVersionRef:      strings.Add("1.0.0"),
		Chunks:             chunks,
	}
}

func BenchmarkCompactStrings_Medium(b *testing.B) {
	for b.Loop() {
		payload := createBenchmarkPayload(10, 20, 5)
		payload.CompactStrings()
	}
}

func BenchmarkCompactStrings_Large(b *testing.B) {
	for b.Loop() {
		payload := createBenchmarkPayload(50, 50, 10)
		payload.CompactStrings()
	}
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
