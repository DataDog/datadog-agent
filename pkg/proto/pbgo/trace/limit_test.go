// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Tests for VULN-80981 / VULN-963 regression: element-count bounds checks on
// msgpack decoders to prevent memory-exhaustion DoS via crafted array headers.

package trace

import (
	"bytes"
	"testing"

	"github.com/tinylib/msgp/msgp"
)

// maliciousArrayHeader32 returns a 5-byte msgpack array32 header declaring n elements.
func maliciousArrayHeader32(n uint32) []byte {
	return []byte{
		0xdd,
		byte(n >> 24),
		byte(n >> 16),
		byte(n >> 8),
		byte(n),
	}
}

// TestUnmarshalMsgLargeArrayCount verifies that Traces.UnmarshalMsg rejects a
// payload whose declared element count exceeds the configured limit, preventing
// the ~48 GB heap allocation a crafted 5-byte body would otherwise trigger.
func TestUnmarshalMsgLargeArrayCount(t *testing.T) {
	// array32 with 2^31-1 elements: 5 bytes total, within the 25 MB byte cap
	// but would allocate ~48 GB before the fix.
	payload := maliciousArrayHeader32(0x7FFFFFFF)

	var traces Traces
	_, err := traces.UnmarshalMsg(payload)
	if err != msgp.ErrLimitExceeded {
		t.Fatalf("expected msgp.ErrLimitExceeded, got %v", err)
	}
}

// TestUnmarshalMsgMaxUint32ArrayCount is the worst-case variant using the full
// uint32 maximum (4,294,967,295 elements).
func TestUnmarshalMsgMaxUint32ArrayCount(t *testing.T) {
	payload := maliciousArrayHeader32(0xFFFFFFFF)

	var traces Traces
	_, err := traces.UnmarshalMsg(payload)
	if err != msgp.ErrLimitExceeded {
		t.Fatalf("expected msgp.ErrLimitExceeded, got %v", err)
	}
}

// TestUnmarshalMsgWithinLimit confirms that payloads within the element-count
// limit are still accepted.
func TestUnmarshalMsgWithinLimit(t *testing.T) {
	// Build a legitimate small Traces payload.
	traces := Traces{Trace{}}
	bts, err := traces.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}

	var result Traces
	_, err = result.UnmarshalMsg(bts)
	if err != nil {
		t.Fatalf("unexpected error for payload within limit: %v", err)
	}
}

// TestDecodeStatsLargeArrayCount verifies that ClientStatsPayload.DecodeMsg
// rejects a payload whose declared Stats bucket count exceeds the configured
// limit.  A 12-byte body (well within the 25 MB byte cap) is sufficient to
// declare 4,294,967,295 buckets.
func TestDecodeStatsLargeArrayCount(t *testing.T) {
	// fixmap {1 entry}, key "Stats", array32 max count
	// fixmap with 1 entry: 0x81
	// fixstr "Stats" (5 bytes): 0xa5 0x53 0x74 0x61 0x74 0x73
	// array32 max: 0xdd 0xff 0xff 0xff 0xff
	payload := []byte{
		0x81,
		0xa5, 'S', 't', 'a', 't', 's',
		0xdd, 0xff, 0xff, 0xff, 0xff,
	}

	var p ClientStatsPayload
	reader := msgp.NewReader(bytes.NewReader(payload))
	err := p.DecodeMsg(reader)
	if err != msgp.ErrLimitExceeded {
		t.Fatalf("expected msgp.ErrLimitExceeded, got %v", err)
	}
}

// TestUnmarshalStatsLargeArrayCount covers the same attack via the binary
// UnmarshalMsg path (used when the body is already buffered).
func TestUnmarshalStatsLargeArrayCount(t *testing.T) {
	// Same 12-byte payload as TestDecodeStatsLargeArrayCount.
	payload := []byte{
		0x81,
		0xa5, 'S', 't', 'a', 't', 's',
		0xdd, 0xff, 0xff, 0xff, 0xff,
	}

	var p ClientStatsPayload
	_, err := p.UnmarshalMsg(payload)
	if err != msgp.ErrLimitExceeded {
		t.Fatalf("expected msgp.ErrLimitExceeded, got %v", err)
	}
}

// TestDecodeStatsWithinLimit confirms that valid stats payloads still decode
// correctly after the limit check was added.
func TestDecodeStatsWithinLimit(t *testing.T) {
	p := ClientStatsPayload{
		Env:     "prod",
		Service: "web",
		Stats:   []*ClientStatsBucket{},
	}
	bts, err := p.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}

	var result ClientStatsPayload
	reader := msgp.NewReader(bytes.NewReader(bts))
	if err := result.DecodeMsg(reader); err != nil {
		t.Fatalf("unexpected error for valid payload: %v", err)
	}
}
