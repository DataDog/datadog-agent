// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// FuzzIncrementalJSONValidator tests the incremental JSON validator against
// arbitrary byte input. The objCount counter can go negative on input like "}}}",
// and json.Decoder.Token() processes adversarial binary data.
func FuzzIncrementalJSONValidator(f *testing.F) {
	f.Add([]byte("{}"))
	f.Add([]byte(`{"key":"val"}`))
	f.Add([]byte("{"))
	f.Add([]byte("}}"))
	f.Add([]byte{0x00, 0xff, 0xfe})
	f.Add([]byte(`"string"`))
	f.Add([]byte("null"))
	f.Add([]byte("[]"))
	f.Add([]byte{})
	f.Add([]byte(`{"a":{"b":{"c":1}}}`))
	f.Add([]byte(`}}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		v := NewIncrementalJSONValidator()
		result := v.Write(data)
		if result != Incomplete && result != Complete && result != Invalid {
			t.Fatalf("unexpected JSONState result: %d", result)
		}
		// Verify clean state after Reset
		v.Reset()
		result2 := v.Write(data)
		if result2 != Incomplete && result2 != Complete && result2 != Invalid {
			t.Fatalf("unexpected JSONState result after Reset: %d", result2)
		}
	})
}

// FuzzJSONAggregator tests the JSON aggregator with two-message sequences.
// json.Compact is called on the concatenation of fuzzer-controlled contents,
// and the validator may declare Complete on non-JSON.
func FuzzJSONAggregator(f *testing.F) {
	f.Add([]byte(`{"key":`), []byte(`"val"}`))
	f.Add([]byte(`{`), []byte(`}`))
	f.Add([]byte("invalid"), []byte("json"))
	f.Add([]byte{}, []byte{})
	f.Add([]byte(`{"key": "value"}`), []byte(`{"key2": "value2"}`))
	f.Add([]byte{0x00, 0xff}, []byte{0xfe, 0x00})
	f.Add([]byte("{\n"), []byte("}\n"))

	f.Fuzz(func(t *testing.T, data1 []byte, data2 []byte) {
		agg := NewJSONAggregator(false, 256*1024)

		msg1 := message.NewMessage(data1, nil, "", 0)
		out1 := agg.Process(msg1)
		for _, m := range out1 {
			_ = m.GetContent()
		}

		msg2 := message.NewMessage(data2, nil, "", 0)
		out2 := agg.Process(msg2)
		for _, m := range out2 {
			_ = m.GetContent()
		}

		// Flush any remaining buffered messages
		flushed := agg.Flush()
		for _, m := range flushed {
			_ = m.GetContent()
		}
	})
}

// FuzzTokenizer tests the tokenizer against arbitrary byte input.
// The tokenizer uses unicode.IsLetter, unicode.IsDigit, and rune conversion
// on single bytes, which can produce unexpected behavior on high-bit bytes.
// The invariant len(tokens) <= len(data) must hold.
func FuzzTokenizer(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte("2024-01-01T00:00:00Z INFO message"))
	f.Add([]byte{0xff, 0xfe, 0x00})
	f.Add([]byte("aaaaaaaaaaaaaaaaaaaaaa")) // long run of same character
	f.Add([]byte{})
	f.Add([]byte("JAN FEB MAR UTC GMT"))
	f.Add([]byte("{}[]():;-_/\\.,'\"`~*+=!@#$%^&"))
	f.Add([]byte{0x80, 0x81, 0x82, 0x83, 0x84}) // high-bit bytes

	f.Fuzz(func(t *testing.T, data []byte) {
		tok := NewTokenizer(256)
		ctx := &messageContext{rawMessage: data}
		tok.ProcessAndContinue(ctx)

		if len(ctx.tokens) > len(data) {
			t.Fatalf("invariant violated: len(tokens)=%d > len(data)=%d", len(ctx.tokens), len(data))
		}
	})
}
