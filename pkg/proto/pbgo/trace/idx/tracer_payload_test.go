// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"bytes"
	"encoding/base64"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
)

func (s *StringTable) assertEqual(t *testing.T, expected []string) {
	assert.Equal(t, expected, s.strings)
	assert.Len(t, s.lookup, len(expected))
	for i, str := range expected {
		assert.Equal(t, uint32(i), s.lookup[str])
	}
}

func TestUnmarshalTracerPayload(t *testing.T) {
	t.Run("tracer payload no chunks", func(t *testing.T) {
		bts := []byte{0x89, 0x02} // map header 9 elements, 2 key (container ID)
		bts = msgp.AppendString(bts, "cidcid")
		bts = append(bts, []byte{0x03}...) // 3 key (Language Name), string of length 2
		bts = msgp.AppendString(bts, "go")
		bts = append(bts, []byte{0x04}...) // 4 key (Language Version), string of length 4
		bts = msgp.AppendString(bts, "1.24")
		bts = append(bts, []byte{0x05}...) // 5 key (Tracer Version), string of length 6
		bts = msgp.AppendString(bts, "v11.24")
		bts = append(bts, []byte{0x06}...) // 6 key (Runtime ID), string of length 10
		bts = msgp.AppendString(bts, "runtime-id")
		bts = append(bts, []byte{0x07}...) // 7 key (Env), string of length 3
		bts = msgp.AppendString(bts, "env")
		bts = append(bts, []byte{0x08}...) // 8 key (Hostname), string of length 6
		bts = msgp.AppendString(bts, "hostname")
		bts = append(bts, []byte{0x09}...) // 9 key (App Version), string of length 6
		bts = msgp.AppendString(bts, "appver")
		bts = append(bts, []byte{0x0A, 0x93, 0x01, 0x04, 0x02}...) // 10 key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 2

		var tp = &InternalTracerPayload{Strings: NewStringTable()}
		o, err := tp.UnmarshalMsg(bts)
		assert.NoError(t, err)
		assert.Len(t, o, 0)
		expectedStrings := []string{"", "cidcid", "go", "1.24", "v11.24", "runtime-id", "env", "hostname", "appver"}

		expectedTP := &InternalTracerPayload{
			Strings:            tp.Strings, // We will assert on this separately for improved readability here
			containerIDRef:     1,
			languageNameRef:    2,
			languageVersionRef: 3,
			tracerVersionRef:   4,
			runtimeIDRef:       5,
			envRef:             6,
			hostnameRef:        7,
			appVersionRef:      8,
			Attributes: map[uint32]*AnyValue{
				1: {Value: &AnyValue_IntValue{IntValue: 2}},
			},
		}
		tp.Strings.assertEqual(t, expectedStrings)
		assert.Equal(t, expectedTP, tp)
	})

	t.Run("strings up front", func(t *testing.T) {
		strings := []string{"", "cidcid", "go", "1.24", "v11.24", "runtime-id", "env", "hostname", "appver"}
		bts := []byte{0x8A, 0x01, 0x99} // map header 9 elements, 1 key (strings), array header 9 elements
		for _, v := range strings {
			bts = msgp.AppendString(bts, v)
		}
		bts = append(bts, []byte{0x02, 0x01}...)                   // 2 key (container ID), string index 1
		bts = append(bts, []byte{0x03, 0x02}...)                   // 3 key (Language Name), string index 2
		bts = append(bts, []byte{0x04, 0x03}...)                   // 4 key (Language Version), string index 3
		bts = append(bts, []byte{0x05, 0x04}...)                   // 5 key (Tracer Version), string index 4
		bts = append(bts, []byte{0x06, 0x05}...)                   // 6 key (Runtime ID), string index 5
		bts = append(bts, []byte{0x07, 0x06}...)                   // 7 key (Env), string index 6
		bts = append(bts, []byte{0x08, 0x07}...)                   // 8 key (Hostname), string index 7
		bts = append(bts, []byte{0x09, 0x08}...)                   // 9 key (App Version), string index 8
		bts = append(bts, []byte{0x0A, 0x93, 0x01, 0x04, 0x02}...) // 10 key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 2

		var tp = &InternalTracerPayload{Strings: NewStringTable()}
		o, err := tp.UnmarshalMsg(bts)
		assert.NoError(t, err)
		assert.Len(t, o, 0)

		expectedTP := &InternalTracerPayload{
			Strings:            tp.Strings, // We will assert on this separately for improved readability here
			containerIDRef:     1,
			languageNameRef:    2,
			languageVersionRef: 3,
			tracerVersionRef:   4,
			runtimeIDRef:       5,
			envRef:             6,
			hostnameRef:        7,
			appVersionRef:      8,
			Attributes: map[uint32]*AnyValue{
				1: {Value: &AnyValue_IntValue{IntValue: 2}},
			},
		}
		tp.Strings.assertEqual(t, strings)
		assert.Equal(t, expectedTP, tp)
	})

	t.Run("java example - unexpected streaming string", func(t *testing.T) {
		base64Payload := "gQuRhwEBAgIDkwMDy0A6AAAAAAAABJHeABABBAIFA612dWxuZXJhYmlsaXR5BM9bNBi+OuFgRwUABs8Yk4lX7E0/pQfOADI9AAjCCZOkdGVzdAPLQDoAAAAAAAAKrXZ1bG5lcmFiaWxpdHkLkAyQDQYOpHRvZG8PpHRvZG8QpHRvZG8FwgYHBwA="
		bts, err := base64.StdEncoding.DecodeString(base64Payload)
		assert.NoError(t, err)

		var tp = &InternalTracerPayload{Strings: NewStringTable()}
		_, err = tp.UnmarshalMsg(bts)
		assert.Error(t, err)
		t.Logf("Error: %v", err)
	})

	t.Run("identify nil error wrapping location", func(t *testing.T) {
		base64Payload := "gQuRhwEBAgIDkwMDy0A6AAAAAAAABJHeABABBAIFA612dWxuZXJhYmlsaXR5BM9bNBi+OuFgRwUABs8Yk4lX7E0/pQfOADI9AAjCCZOkdGVzdAPLQDoAAAAAAAAKrXZ1bG5lcmFiaWxpdHkLkAyQDQYOpHRvZG8PpHRvZG8QpHRvZG8FwgYHBwA="
		bts, err := base64.StdEncoding.DecodeString(base64Payload)
		assert.NoError(t, err)

		var tp = &InternalTracerPayload{Strings: NewStringTable()}
		_, err = tp.UnmarshalMsg(bts)
		assert.Error(t, err)

		// Check if the error contains nil pointer by trying to call Error()
		// This will help us identify which code path is causing the issue
		panicked := false
		var panicValue interface{}
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
					panicValue = r
				}
			}()
			_ = err.Error()
		}()

		if panicked {
			t.Logf("Panic occurred when calling err.Error(): %v", panicValue)
			t.Logf("Error type: %T", err)
			// Try to inspect the error structure
			if wrappedErr, ok := err.(interface{ Unwrap() error }); ok {
				unwrapped := wrappedErr.Unwrap()
				t.Logf("Unwrapped error: %v, type: %T, is nil: %v", unwrapped, unwrapped, unwrapped == nil)
			}
		} else {
			t.Logf("No panic occurred, error message: %s", err.Error())
		}
	})
}

func TestUnmarshalTraceChunk(t *testing.T) {
	t.Run("trace chunk no spans", func(t *testing.T) {
		strings := NewStringTable()
		bts := []byte{0x91, 0x86, 0x01, 0x02, 0x02, 0xA6}          // array header 1 element, map header 2 elements, 1 key (priority), 2 (int32), 2 key (origin), string of length 6
		bts = append(bts, []byte("lambda")...)                     // lambda bytes
		bts = append(bts, []byte{0x03, 0x93, 0x01, 0x04, 0x02}...) // 3rd key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 2
		bts = append(bts, []byte{0x05, mtrue}...)                  // 5th key (droppedTrace), bool true
		bts = append(bts, []byte{0x06, 0xc4, 0x01, 0xAF}...)       // 6th key (TraceID), bin header, 1 byte in length, 0xAF
		bts = append(bts, []byte{0x07, 0x04}...)                   // 7th key (samplingMechanism), uint32 4

		chunks, o, err := UnmarshalTraceChunkList(bts, strings)
		assert.NoError(t, err)
		assert.Len(t, chunks, 1)
		assert.Len(t, o, 0)

		expectedChunk := &InternalTraceChunk{
			Strings:   strings, // We will assert on this separately for improved readability here
			Priority:  2,
			originRef: 1,
			Attributes: map[uint32]*AnyValue{
				1: {Value: &AnyValue_IntValue{IntValue: 2}},
			},
			DroppedTrace:      true,
			TraceID:           []byte{0xAF},
			samplingMechanism: 4,
		}
		assert.Equal(t, expectedChunk, chunks[0])
		strings.assertEqual(t, []string{"", "lambda"})
	})
	t.Run("trace chunk with a span", func(t *testing.T) {
		strings := NewStringTable()
		bts := []byte{0x91, 0x87, 0x01, 0x02, 0x02, 0xA6}          // array header 1 element, map header 4 elements, 1 key (priority), 2 (int32), 2 key (origin), string of length 6
		bts = append(bts, []byte("lambda")...)                     // lambda bytes
		bts = append(bts, []byte{0x03, 0x93, 0x01, 0x04, 0x02}...) // 3rd key (attributes), array header 3 elements, fixint 1 (string index), 4 (int type), int 2
		bts = append(bts, []byte{0x05, mtrue}...)                  // 5th key (droppedTrace), bool true
		bts = append(bts, []byte{0x06, 0xc4, 0x01, 0xAF}...)       // 6th key (TraceID), bin header, 1 byte in length, 0xAF
		bts = append(bts, []byte{0x07, 0x04}...)                   // 7th key (samplingMechanism), uint32 4
		bts = append(bts, []byte{0x04, 0x91}...)                   // 4th key (spans), array header 1 element
		bts = append(bts, rawSpan()...)                            // span bytes

		chunks, o, err := UnmarshalTraceChunkList(bts, strings)
		assert.NoError(t, err)
		assert.Len(t, chunks, 1)
		assert.Len(t, chunks[0].Spans, 1)
		assert.Len(t, o, 0)

		expectedAttributes := map[uint32]*AnyValue{
			1: {Value: &AnyValue_IntValue{IntValue: 2}},
		}
		assert.Equal(t, int32(2), chunks[0].Priority)
		assert.Equal(t, uint32(1), chunks[0].originRef)
		assert.True(t, chunks[0].DroppedTrace)
		assert.Equal(t, []byte{0xAF}, chunks[0].TraceID)
		assert.Equal(t, uint32(4), chunks[0].samplingMechanism)
		assert.Len(t, chunks[0].Spans, 1)
		// Assert on the attributes map
		assert.Equal(t, len(expectedAttributes), len(chunks[0].Attributes))
		for k, v := range expectedAttributes {
			assert.Contains(t, chunks[0].Attributes, k)
			assert.Equal(t, v, chunks[0].Attributes[k])
		}
		strings.assertEqual(t, []string{"", "lambda", "my-service", "span-name", "GET /res", "foo", "bar", "foo2", "some-num"})
	})
}

// TestUnmarshalTracerPayloadUnknownField verifies InternalTracerPayload.UnmarshalMsg
// skips an unknown field interleaved between known fields AND harvests the new
// inline streaming string it carries, so a later known field that references
// that string by index resolves correctly (forward compatibility). Layout:
//
//	field 2  (containerID):  inline "cidcid"    -> string index 1
//	field 99 (unknown):      inline "harvested" -> string index 2 (must be harvested)
//	field 3  (languageName): ref index 2        -> "harvested"
func TestUnmarshalTracerPayloadUnknownField(t *testing.T) {
	var bts []byte
	bts = msgp.AppendMapHeader(bts, 3)
	bts = msgp.AppendUint32(bts, 2)
	bts = msgp.AppendString(bts, "cidcid")
	bts = msgp.AppendUint32(bts, 99)
	bts = msgp.AppendString(bts, "harvested")
	bts = msgp.AppendUint32(bts, 3)
	bts = msgp.AppendUint32(bts, 2) // languageName: streaming-string ref to index 2

	tp := &InternalTracerPayload{Strings: NewStringTable()}
	o, err := tp.UnmarshalMsg(bts)
	require.NoError(t, err)
	require.Empty(t, o)
	assert.Equal(t, "cidcid", tp.Strings.Get(tp.containerIDRef))
	assert.Equal(t, "harvested", tp.Strings.Get(2), "unknown field's inline string must be harvested into the table")
	assert.Equal(t, "harvested", tp.Strings.Get(tp.languageNameRef), "later field's index reference must resolve to the harvested string")
}

// TestUnmarshalTraceChunkUnknownField verifies InternalTraceChunk.UnmarshalMsg
// skips an unknown field and harvests its inline string so a later known
// field's index reference resolves. Layout:
//
//	field 1  (priority):     int32 2
//	field 99 (unknown):      inline "harvested" -> string index 1 (must be harvested)
//	field 2  (origin):       ref index 1        -> "harvested"
//	field 5  (droppedTrace): bool true
func TestUnmarshalTraceChunkUnknownField(t *testing.T) {
	strings := NewStringTable()
	var bts []byte
	bts = msgp.AppendMapHeader(bts, 4)
	bts = msgp.AppendUint32(bts, 1)
	bts = msgp.AppendInt32(bts, 2)
	bts = msgp.AppendUint32(bts, 99)
	bts = msgp.AppendString(bts, "harvested")
	bts = msgp.AppendUint32(bts, 2)
	bts = msgp.AppendUint32(bts, 1) // origin: streaming-string ref to index 1
	bts = msgp.AppendUint32(bts, 5)
	bts = msgp.AppendBool(bts, true)

	tc := &InternalTraceChunk{Strings: strings}
	o, err := tc.UnmarshalMsg(bts)
	require.NoError(t, err)
	require.Empty(t, o)
	assert.Equal(t, int32(2), tc.Priority)
	assert.Equal(t, "harvested", strings.Get(1))
	assert.Equal(t, "harvested", strings.Get(tc.originRef))
	assert.True(t, tc.DroppedTrace)
}

// FuzzUnmarshalTracerPayloadErrorHandling verifies that any error returned from
// UnmarshalMsg does not panic when Error() is called on it. This makes sure we don't ever return a wrapped error with an internal nil error.
func FuzzUnmarshalTracerPayloadErrorHandling(f *testing.F) {
	// Add the known problematic payload as a seed
	base64Payload := "gQuRhwEBAgIDkwMDy0A6AAAAAAAABJHeABABBAIFA612dWxuZXJhYmlsaXR5BM9bNBi+OuFgRwUABs8Yk4lX7E0/pQfOADI9AAjCCZOkdGVzdAPLQDoAAAAAAAAKrXZ1bG5lcmFiaWxpdHkLkAyQDQYOpHRvZG8PpHRvZG8QpHRvZG8FwgYHBwA="
	bts, err := base64.StdEncoding.DecodeString(base64Payload)
	if err == nil {
		f.Add(bts)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var tp = &InternalTracerPayload{Strings: NewStringTable()}
		_, err := tp.UnmarshalMsg(data)

		// If an error occurred, verify that calling Error() on it doesn't panic
		if err != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic when calling err.Error(): %v, error type: %T", r, err)
					}
				}()
				_ = err.Error()
			}()
		}
	})
}

// TestShortTraceIDSurvivesWireDecode confirms a short TraceID decoded from the
// wire (InternalTraceChunk.UnmarshalMsg) flows into the sampler's LegacyTraceID
// call without panicking.
func TestShortTraceIDSurvivesWireDecode(t *testing.T) {
	// Craft a minimal trace chunk msgpack: a 1-field map { field 6: TraceID }
	// where TraceID is only 4 bytes long.
	var b []byte
	b = msgp.AppendMapHeader(b, 1)
	b = msgp.AppendUint32(b, 6)                 // field 6 = TraceID
	b = msgp.AppendBytes(b, []byte{1, 2, 3, 4}) // 4-byte (short) TraceID

	c := &InternalTraceChunk{Strings: NewStringTable()}
	_, err := c.UnmarshalMsg(b)
	assert.NoError(t, err)

	// The sampler calls LegacyTraceID() on this chunk; it must not panic.
	assert.NotPanics(t, func() {
		assert.Equal(t, uint64(0x01020304), c.LegacyTraceID())
	})
}

// allocatedBytes returns the bytes allocated on the heap while running f.
func allocatedBytes(f func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestDecodersBoundAllocationsByInput checks that a few bytes of input declaring huge array/map counts
// can't make the decoders allocate memory sized from those counts before failing.
// See https://github.com/DataDog/datadog-agent/issues/59778
func TestDecodersBoundAllocationsByInput(t *testing.T) {
	const (
		huge     = 25_000_000 // the previous per-header limit
		maxAlloc = 1 << 20    // generous ceiling; each case allocated hundreds of MB before the fix
	)
	v1 := func(b []byte) error {
		tp := &InternalTracerPayload{}
		_, err := tp.UnmarshalMsg(b)
		return err
	}
	v04 := func(b []byte) error {
		tp := &InternalTracerPayload{}
		_, err := tp.UnmarshalMsgConverted(b)
		return err
	}
	v05 := func(b []byte) error {
		tp := &InternalTracerPayload{}
		return tp.UnmarshalMsgDictionary(b)
	}

	// v1: {11: chunks[{4: spans[{11: links[{3: attributes[key, type, value, ...]}]}]}]}
	// Nested cases declare one element at every parent level and an oversized count only at the
	// container under test, so decoding reaches that container's guard. Counts of maxCollectionSize
	// pass the per-header limit, so they are rejected only by the remaining-input check.
	v1Field := func(prefix []byte, field uint32) []byte {
		return msgp.AppendUint32(msgp.AppendMapHeader(bytes.Clone(prefix), 1), field)
	}
	v1Chunk := msgp.AppendArrayHeader(v1Field(nil, 11), 1)
	v1Span := msgp.AppendArrayHeader(v1Field(v1Chunk, 4), 1)
	v1Link := msgp.AppendArrayHeader(v1Field(v1Span, 11), 1)
	v1Chunks := msgp.AppendArrayHeader(v1Field(nil, 11), huge)
	v1ChunksAtLimit := msgp.AppendArrayHeader(v1Field(nil, 11), maxCollectionSize)
	v1Spans := msgp.AppendArrayHeader(v1Field(v1Chunk, 4), maxCollectionSize)
	v1Links := msgp.AppendArrayHeader(v1Field(v1Span, 11), maxCollectionSize)
	v1LinkAttrs := msgp.AppendArrayHeader(v1Field(v1Link, 3), maxCollectionSize-2) // a multiple of 3
	// One attribute (key 0, type 6 = array) whose array value declares maxCollectionSize elements.
	v1ArrayValue := msgp.AppendArrayHeader(msgp.AppendUint32(msgp.AppendUint32(msgp.AppendArrayHeader(v1Field(v1Link, 3), 3), 0), 6), maxCollectionSize)
	v1StringTable := msgp.AppendArrayHeader(v1Field(nil, 1), huge)

	// v0.4: [[span, ...], ...]
	v04Traces := msgp.AppendArrayHeader(nil, huge)
	v04TracesAtLimit := msgp.AppendArrayHeader(nil, maxCollectionSize)
	v04Spans := msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 1), maxCollectionSize)
	v04Meta := msgp.AppendMapHeader(msgp.AppendString(msgp.AppendMapHeader(msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 1), 1), 1), "meta"), maxCollectionSize)
	// One byte per declared entry passes the header check but fails checkSlabCount (two bytes per entry);
	// the attribute map must not be sized before that check runs.
	halfBacking := bytes.Repeat([]byte{0xc0}, maxCollectionSize)
	v04MetaHalfBacked := append(bytes.Clone(v04Meta), halfBacking...)
	v04SpanField := func(field string) []byte {
		return msgp.AppendString(msgp.AppendMapHeader(msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 1), 1), 1), field)
	}
	// [[{"span_events" / "span_links": [{"attributes": {...}}]}]]
	v04EventAttrsHalfBacked := msgp.AppendMapHeader(msgp.AppendString(msgp.AppendMapHeader(msgp.AppendArrayHeader(v04SpanField("span_events"), 1), 1), "attributes"), maxCollectionSize)
	v04EventAttrsHalfBacked = append(v04EventAttrsHalfBacked, halfBacking...)
	v04LinkAttrsHalfBacked := msgp.AppendMapHeader(msgp.AppendString(msgp.AppendMapHeader(msgp.AppendArrayHeader(v04SpanField("span_links"), 1), 1), "attributes"), maxCollectionSize)
	v04LinkAttrsHalfBacked = append(v04LinkAttrsHalfBacked, halfBacking...)
	v04Links := msgp.AppendArrayHeader(msgp.AppendString(msgp.AppendMapHeader(msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 1), 1), 1), "span_links"), maxCollectionSize)

	// v0.5: [dictionary, traces]
	v05Dict := msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 2), huge)
	v05Traces := msgp.AppendArrayHeader(msgp.AppendArrayHeader(msgp.AppendArrayHeader(nil, 2), 0), maxCollectionSize)

	for _, tc := range []struct {
		name   string
		decode func([]byte) error
		body   []byte
	}{
		{"v1 chunks", v1, v1Chunks},
		{"v1 chunks at limit", v1, v1ChunksAtLimit},
		{"v1 spans", v1, v1Spans},
		{"v1 span links", v1, v1Links},
		{"v1 link attributes", v1, v1LinkAttrs},
		{"v1 array value", v1, v1ArrayValue},
		{"v1 string table", v1, v1StringTable},
		{"v0.4 traces", v04, v04Traces},
		{"v0.4 traces at limit", v04, v04TracesAtLimit},
		{"v0.4 spans", v04, v04Spans},
		{"v0.4 meta", v04, v04Meta},
		{"v0.4 meta half backed", v04, v04MetaHalfBacked},
		{"v0.4 span event attributes half backed", v04, v04EventAttrsHalfBacked},
		{"v0.4 span link attributes half backed", v04, v04LinkAttrsHalfBacked},
		{"v0.4 span links", v04, v04Links},
		{"v0.5 dictionary", v05, v05Dict},
		{"v0.5 traces", v05, v05Traces},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			allocated := allocatedBytes(func() { err = tc.decode(tc.body) })
			assert.Error(t, err)
			assert.Less(t, allocated, uint64(maxAlloc), "decoding %d bytes allocated %d bytes", len(tc.body), allocated)
		})
	}
}

// TestDecodersRejectCountsAboveLimit checks the per-header count limit, which applies even when the
// input is long enough to hold that many elements.
func TestDecodersRejectCountsAboveLimit(t *testing.T) {
	nils := bytes.Repeat([]byte{0xc0}, maxCollectionSize+1) // msgpack nil
	t.Run("v1", func(t *testing.T) {
		b := msgp.AppendArrayHeader(msgp.AppendUint32(msgp.AppendMapHeader(nil, 1), 11), maxCollectionSize+1)
		tp := &InternalTracerPayload{}
		_, err := tp.UnmarshalMsg(append(b, nils...))
		assert.ErrorContains(t, err, "Array too large")
	})
	t.Run("v1 payload fields", func(t *testing.T) {
		b := msgp.AppendMapHeader(nil, maxCollectionSize+1)
		tp := &InternalTracerPayload{}
		_, err := tp.UnmarshalMsg(append(b, bytes.Repeat(nils, 2)...))
		assert.ErrorContains(t, err, "Map too large")
	})
	t.Run("v1 chunk fields", func(t *testing.T) {
		b := msgp.AppendMapHeader(nil, maxCollectionSize+1)
		tc := &InternalTraceChunk{Strings: NewStringTable()}
		_, err := tc.UnmarshalMsg(append(b, bytes.Repeat(nils, 2)...))
		assert.ErrorContains(t, err, "Map too large")
	})
	t.Run("v0.4", func(t *testing.T) {
		b := msgp.AppendArrayHeader(nil, maxCollectionSize+1)
		tp := &InternalTracerPayload{}
		_, err := tp.UnmarshalMsgConverted(append(b, nils...))
		assert.ErrorContains(t, err, "too long payload")
	})
}
