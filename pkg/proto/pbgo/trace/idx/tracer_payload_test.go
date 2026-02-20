// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
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
