// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"math"
	"testing"
	"unicode/utf8"

	"github.com/tinylib/msgp/msgp"
)

// FuzzRepairUTF8 fuzzes the repairUTF8 function which replaces invalid UTF-8
// sequences with the replacement character. This function is called for every
// non-UTF-8 string field in every decoded span, making it a critical path for
// adversarial input.
func FuzzRepairUTF8(f *testing.F) {
	// Valid UTF-8 strings
	f.Add("hello world")
	f.Add("")
	f.Add("日本語テスト")
	f.Add("emoji: 🎉🔥")

	// Strings with invalid UTF-8 sequences
	f.Add("invalid\x99\xbf")
	f.Add("\xff\xfe")
	f.Add(string([]byte{0x80, 0x81, 0x82}))
	f.Add(string([]byte{0xc0, 0xaf}))       // overlong encoding
	f.Add(string([]byte{0xed, 0xa0, 0x80})) // surrogate half

	// Edge cases
	f.Add(string([]byte{0x00})) // null byte

	f.Fuzz(func(t *testing.T, s string) {
		result := repairUTF8(s)

		// The output must always be valid UTF-8.
		if !utf8.ValidString(result) {
			t.Fatalf("repairUTF8 produced invalid UTF-8: %q", result)
		}

		// If the input was already valid UTF-8, output must equal input.
		if utf8.ValidString(s) && result != s {
			t.Fatalf("repairUTF8 modified valid UTF-8: input=%q output=%q", s, result)
		}

		// Idempotency: repairing the output should be a no-op.
		result2 := repairUTF8(result)
		if result2 != result {
			t.Fatalf("repairUTF8 is not idempotent: first=%q second=%q", result, result2)
		}
	})
}

// FuzzParseStringBytes fuzzes the msgpack string/binary decoder used for every
// string field in trace spans (service, name, resource, meta keys/values, etc.).
func FuzzParseStringBytes(f *testing.F) {
	// Valid msgpack-encoded strings
	f.Add(msgp.AppendString(nil, "hello"))
	f.Add(msgp.AppendString(nil, ""))
	f.Add(msgp.AppendString(nil, "日本語"))
	f.Add(msgp.AppendString(nil, string([]byte{0x80, 0x81}))) // invalid UTF-8 in msgpack string

	// Valid msgpack-encoded binary
	f.Add(msgp.AppendBytes(nil, []byte("binary data")))
	f.Add(msgp.AppendBytes(nil, []byte{}))

	// Nil
	f.Add(msgp.AppendNil(nil))

	// Wrong types to test error paths
	f.Add(msgp.AppendInt64(nil, 42))
	f.Add(msgp.AppendBool(nil, true))

	// Edge cases: truncated, empty
	f.Add([]byte{})
	f.Add([]byte{0xd9}) // str8 header with missing length

	f.Fuzz(func(t *testing.T, bts []byte) {
		s, remaining, err := parseStringBytes(bts)
		if err != nil {
			return // invalid msgpack is expected
		}

		// The returned string must always be valid UTF-8.
		if !utf8.ValidString(s) {
			t.Fatalf("parseStringBytes returned invalid UTF-8: %q", s)
		}

		// Byte accounting: consumed bytes + remaining must not exceed input.
		if len(remaining) > len(bts) {
			t.Fatalf("remaining bytes (%d) exceeds input length (%d)", len(remaining), len(bts))
		}
	})
}

// FuzzParseFloat64Bytes fuzzes the msgpack float64 decoder which accepts
// int64, uint64, and float64 encoded values. Used for span Metrics map values.
func FuzzParseFloat64Bytes(f *testing.F) {
	// Valid float64
	f.Add(msgp.AppendFloat64(nil, 3.14))
	f.Add(msgp.AppendFloat64(nil, 0))
	f.Add(msgp.AppendFloat64(nil, math.MaxFloat64))
	f.Add(msgp.AppendFloat64(nil, math.SmallestNonzeroFloat64))
	f.Add(msgp.AppendFloat64(nil, math.NaN()))
	f.Add(msgp.AppendFloat64(nil, math.Inf(1)))
	f.Add(msgp.AppendFloat64(nil, math.Inf(-1)))

	// Int64 encoded as int (the decoder must handle this)
	f.Add(msgp.AppendInt64(nil, 42))
	f.Add(msgp.AppendInt64(nil, -1))
	f.Add(msgp.AppendInt64(nil, math.MaxInt64))
	f.Add(msgp.AppendInt64(nil, math.MinInt64))

	// Uint64 encoded as uint
	f.Add(msgp.AppendUint64(nil, 0))
	f.Add(msgp.AppendUint64(nil, math.MaxUint64))

	// Nil
	f.Add(msgp.AppendNil(nil))

	// Wrong types
	f.Add(msgp.AppendString(nil, "not a number"))
	f.Add(msgp.AppendBool(nil, true))

	// Truncated/empty
	f.Add([]byte{})
	f.Add([]byte{0xcb}) // float64 header with missing payload

	f.Fuzz(func(t *testing.T, bts []byte) {
		f64, remaining, err := parseFloat64Bytes(bts)
		if err != nil {
			return // invalid msgpack is expected
		}

		// Result must be a valid float64 (not checking NaN equality since NaN != NaN).
		_ = f64

		// Byte accounting.
		if len(remaining) > len(bts) {
			t.Fatalf("remaining bytes (%d) exceeds input length (%d)", len(remaining), len(bts))
		}
	})
}

// FuzzParseInt64Bytes fuzzes the msgpack int64 decoder which also accepts
// uint64-encoded values with overflow detection. Used for span TraceID, SpanID,
// ParentID, Start, Duration fields.
func FuzzParseInt64Bytes(f *testing.F) {
	// Int64 values
	f.Add(msgp.AppendInt64(nil, 0))
	f.Add(msgp.AppendInt64(nil, 42))
	f.Add(msgp.AppendInt64(nil, -1))
	f.Add(msgp.AppendInt64(nil, math.MaxInt64))
	f.Add(msgp.AppendInt64(nil, math.MinInt64))

	// Uint64 values that fit in int64
	f.Add(msgp.AppendUint64(nil, 0))
	f.Add(msgp.AppendUint64(nil, uint64(math.MaxInt64)))

	// Uint64 values that overflow int64 (must return error)
	f.Add(msgp.AppendUint64(nil, math.MaxUint64))
	f.Add(msgp.AppendUint64(nil, uint64(math.MaxInt64)+1))

	// Nil
	f.Add(msgp.AppendNil(nil))

	// Wrong types
	f.Add(msgp.AppendString(nil, "not a number"))
	f.Add(msgp.AppendFloat64(nil, 3.14))

	// Truncated/empty
	f.Add([]byte{})
	f.Add([]byte{0xd3}) // int64 header with missing payload

	f.Fuzz(func(t *testing.T, bts []byte) {
		i64, remaining, err := parseInt64Bytes(bts)
		if err != nil {
			return // invalid msgpack or overflow is expected
		}

		_ = i64

		// Byte accounting.
		if len(remaining) > len(bts) {
			t.Fatalf("remaining bytes (%d) exceeds input length (%d)", len(remaining), len(bts))
		}

		// If the input was a uint64, validate that the result fits in int64.
		if len(bts) > 0 && msgp.NextType(bts) == msgp.UintType {
			if i64 < 0 {
				t.Fatalf("parseInt64Bytes returned negative value %d for uint input", i64)
			}
		}
	})
}

// FuzzParseUint64Bytes fuzzes the msgpack uint64 decoder which also accepts
// int64-encoded values (for Java/JRuby compatibility). Used for span TraceID
// and SpanID fields.
func FuzzParseUint64Bytes(f *testing.F) {
	// Uint64 values
	f.Add(msgp.AppendUint64(nil, 0))
	f.Add(msgp.AppendUint64(nil, 42))
	f.Add(msgp.AppendUint64(nil, math.MaxUint64))

	// Int64 values (Java compatibility path)
	f.Add(msgp.AppendInt64(nil, 0))
	f.Add(msgp.AppendInt64(nil, 42))
	f.Add(msgp.AppendInt64(nil, -1)) // negative int64 → uint64 wraps
	f.Add(msgp.AppendInt64(nil, math.MaxInt64))
	f.Add(msgp.AppendInt64(nil, math.MinInt64))

	// Nil
	f.Add(msgp.AppendNil(nil))

	// Wrong types
	f.Add(msgp.AppendString(nil, "not a number"))
	f.Add(msgp.AppendBool(nil, false))

	// Truncated/empty
	f.Add([]byte{})
	f.Add([]byte{0xcf}) // uint64 header with missing payload

	f.Fuzz(func(t *testing.T, bts []byte) {
		u64, remaining, err := parseUint64Bytes(bts)
		if err != nil {
			return // invalid msgpack is expected
		}

		_ = u64

		// Byte accounting.
		if len(remaining) > len(bts) {
			t.Fatalf("remaining bytes (%d) exceeds input length (%d)", len(remaining), len(bts))
		}
	})
}

// FuzzParseInt32Bytes fuzzes the msgpack int32 decoder which also accepts
// uint32-encoded values with overflow detection.
func FuzzParseInt32Bytes(f *testing.F) {
	// Int32 values
	f.Add(msgp.AppendInt32(nil, 0))
	f.Add(msgp.AppendInt32(nil, 42))
	f.Add(msgp.AppendInt32(nil, -1))
	f.Add(msgp.AppendInt32(nil, math.MaxInt32))
	f.Add(msgp.AppendInt32(nil, math.MinInt32))

	// Uint32 values that fit in int32
	f.Add(msgp.AppendUint32(nil, 0))
	f.Add(msgp.AppendUint32(nil, uint32(math.MaxInt32)))

	// Uint32 values that overflow int32 (must return error)
	f.Add(msgp.AppendUint32(nil, math.MaxUint32))
	f.Add(msgp.AppendUint32(nil, uint32(math.MaxInt32)+1))

	// Nil
	f.Add(msgp.AppendNil(nil))

	// Wrong types
	f.Add(msgp.AppendString(nil, "not a number"))

	// Truncated/empty
	f.Add([]byte{})
	f.Add([]byte{0xd2}) // int32 header with missing payload

	f.Fuzz(func(t *testing.T, bts []byte) {
		i32, remaining, err := parseInt32Bytes(bts)
		if err != nil {
			return // invalid msgpack or overflow is expected
		}

		_ = i32

		// Byte accounting.
		if len(remaining) > len(bts) {
			t.Fatalf("remaining bytes (%d) exceeds input length (%d)", len(remaining), len(bts))
		}

		// If the input was a uint32, validate that the result fits in int32.
		if len(bts) > 0 && msgp.NextType(bts) == msgp.UintType {
			if i32 < 0 {
				t.Fatalf("parseInt32Bytes returned negative value %d for uint input", i32)
			}
		}
	})
}
