// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package zlibimpl

import (
	"bytes"
	"testing"
)

// Asserts that b == decompress(compress(b)) for all b.
func FuzzZlibIdentity(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte(string(make([]byte, 1000))))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))
	f.Add(bytes.Repeat([]byte("abcd"), 250))

	c := New()
	f.Fuzz(func(t *testing.T, data []byte) {
		compressed, err := c.Compress(data)
		if err != nil {
			t.Fatalf("Compress failed: %v", err)
		}

		decompressed, err := c.Decompress(compressed)
		if err != nil {
			t.Fatalf("Decompress failed: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Errorf("Identity property violated: decompress(compress(b)) != b")
		}
	})
}

// Asserts that Compress(empty) produces a valid compressed frame (non-empty
// output that roundtrips back to empty). Derived from the ValidFrame invariant
// in compression.allium.
func TestZlibValidFrameEmpty(t *testing.T) {
	c := New()
	compressed, err := c.Compress([]byte{})
	if err != nil {
		t.Fatalf("Compress(empty) failed: %v", err)
	}

	if len(compressed) == 0 {
		t.Errorf("Compress(empty) produced empty output; expected a valid zlib frame")
	}

	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress of empty frame failed: %v", err)
	}

	if !bytes.Equal(decompressed, []byte{}) {
		t.Errorf("Roundtrip of empty input produced non-empty output: %v", decompressed)
	}
}

// Asserts that closing a StreamCompressor without writing produces a valid
// compressed frame (non-empty output that decompresses to empty). Derived from
// the EmptyStreamProducesValidFrame invariant in compression.allium.
func TestZlibEmptyStreamProducesValidFrame(t *testing.T) {
	c := New()
	var buf bytes.Buffer
	stream := c.NewStreamCompressor(&buf)

	if err := stream.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Errorf("Close without Write produced empty output; expected a valid zlib frame")
	}

	decompressed, err := c.Decompress(buf.Bytes())
	if err != nil {
		t.Fatalf("Decompress of empty stream frame failed: %v", err)
	}

	if len(decompressed) != 0 {
		t.Errorf("Empty stream frame decompressed to non-empty output: %v", decompressed)
	}
}

// Asserts that len(compress(b)) <= CompressBound(len(b)) for all b.
func FuzzZlibCompressBound(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte(string(make([]byte, 1000))))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))
	f.Add(bytes.Repeat([]byte("abcd"), 250))

	c := New()
	f.Fuzz(func(t *testing.T, data []byte) {
		compressed, err := c.Compress(data)
		if err != nil {
			t.Fatalf("Compress failed: %v", err)
		}

		bound := c.CompressBound(len(data))
		if len(compressed) > bound {
			t.Errorf("CompressBound violated: len(compress(b))=%d > CompressBound(len(b))=%d", len(compressed), bound)
		}
	})
}
