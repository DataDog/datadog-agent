// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
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
			ContainerIDRef:     1,
			LanguageNameRef:    2,
			LanguageVersionRef: 3,
			TracerVersionRef:   4,
			RuntimeIDRef:       5,
			EnvRef:             6,
			HostnameRef:        7,
			AppVersionRef:      8,
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
			ContainerIDRef:     1,
			LanguageNameRef:    2,
			LanguageVersionRef: 3,
			TracerVersionRef:   4,
			RuntimeIDRef:       5,
			EnvRef:             6,
			HostnameRef:        7,
			AppVersionRef:      8,
			Attributes: map[uint32]*AnyValue{
				1: {Value: &AnyValue_IntValue{IntValue: 2}},
			},
		}
		tp.Strings.assertEqual(t, strings)
		assert.Equal(t, expectedTP, tp)
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
		bts = append(bts, []byte{0x07, 0xA2}...)                   // 7th key (decisionMaker), string of length 2
		bts = append(bts, []byte("-9")...)

		chunks, o, err := UnmarshalTraceChunkList(bts, strings)
		assert.NoError(t, err)
		assert.Len(t, chunks, 1)
		assert.Len(t, o, 0)

		expectedChunk := &InternalTraceChunk{
			Strings:   strings, // We will assert on this separately for improved readability here
			Priority:  2,
			OriginRef: 1,
			Attributes: map[uint32]*AnyValue{
				1: {Value: &AnyValue_IntValue{IntValue: 2}},
			},
			DroppedTrace:     true,
			TraceID:          []byte{0xAF},
			DecisionMakerRef: 2,
		}
		assert.Equal(t, expectedChunk, chunks[0])
		strings.assertEqual(t, []string{"", "lambda", "-9"})
	})
}
