// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gzipimpl

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// Asserts that b == decompress(compress(b)) for all b.
func FuzzGzipIdentity(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte(string(make([]byte, 1000))))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))
	f.Add(bytes.Repeat([]byte("abcd"), 250))

	c := New(Requires{Level: gzip.BestSpeed})
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
func FuzzGzipCompressBound(f *testing.F) {
	f.Add([]byte("hello world"), gzip.BestSpeed)
	f.Add([]byte(""), gzip.NoCompression)
	f.Add([]byte("a"), gzip.BestCompression)
	f.Add([]byte(string(make([]byte, 1000))), gzip.DefaultCompression)
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), gzip.HuffmanOnly)
	f.Add(bytes.Repeat([]byte("abcd"), 250), gzip.BestSpeed)

	f.Fuzz(func(t *testing.T, data []byte, level int) {
		// Clamp level to valid gzip range: HuffmanOnly (-2) to BestCompression (9)
		if level < gzip.HuffmanOnly {
			level = gzip.HuffmanOnly
		} else if level > gzip.BestCompression {
			level = gzip.BestCompression
		}

		c := New(Requires{Level: level})
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
