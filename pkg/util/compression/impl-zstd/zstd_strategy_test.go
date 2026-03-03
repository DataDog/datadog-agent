// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package zstdimpl

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Asserts that b == decompress(compress(b)) for all b.
func FuzzZstdIdentity(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte(string(make([]byte, 1000))))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))
	f.Add(bytes.Repeat([]byte("abcd"), 250))

	c := New(Requires{Level: compression.ZstdCompressionLevel(1)})
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

// Asserts that len(compress(b)) <= CompressBound(len(b)) for all b and levels.
func FuzzZstdCompressBound(f *testing.F) {
	f.Add([]byte("hello world"), 1)
	f.Add([]byte(""), 3)
	f.Add([]byte("a"), 5)
	f.Add([]byte(string(make([]byte, 1000))), 10)
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), 15)
	f.Add(bytes.Repeat([]byte("abcd"), 250), 22)

	f.Fuzz(func(t *testing.T, data []byte, level int) {
		// Clamp level to valid zstd range: 1 to 22
		if level < 1 {
			level = 1
		} else if level > 22 {
			level = 22
		}

		c := New(Requires{Level: compression.ZstdCompressionLevel(level)})
		compressed, err := c.Compress(data)
		if err != nil {
			t.Fatalf("Compress failed: %v", err)
		}

		bound := c.CompressBound(len(data))
		if len(compressed) > bound {
			t.Errorf("CompressBound violated at level %d: len(compress(b))=%d > CompressBound(len(b))=%d", level, len(compressed), bound)
		}
	})
}
