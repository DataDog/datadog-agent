// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package zstd

import (
	"bytes"
	"math/rand"
	"testing"
)

// TestCompressorReuse checks that a reused context emits independent frames: each
// output decompresses on its own to exactly its input, whatever came before.
func TestCompressorReuse(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	c := NewCompressor()
	var scratch []byte
	for i, size := range []int{0, 1, 100, 64 << 10, 300 << 10, 10, 1 << 20, 0, 5000} {
		src := make([]byte, size)
		for j := range src {
			// Compressible but not trivial.
			src[j] = byte('a' + rng.Intn(4))
		}
		bound := CompressBound(len(src))
		if cap(scratch) < bound {
			scratch = make([]byte, bound)
		}
		out, err := c.CompressLevel(scratch[:bound], src, DefaultCompression)
		if err != nil {
			t.Fatalf("input %d: compress: %v", i, err)
		}
		got, err := Decompress(nil, out)
		if err != nil {
			t.Fatalf("input %d: decompress: %v", i, err)
		}
		if !bytes.Equal(src, got) {
			t.Fatalf("input %d: round trip mismatch", i)
		}
	}
}

func TestCompressorMatchesCompressLevel(t *testing.T) {
	src := bytes.Repeat([]byte("connection payload "), 4096)
	c := NewCompressor()
	for range 3 {
		out, err := c.CompressLevel(nil, src, DefaultCompression)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := CompressLevel(nil, src, DefaultCompression)
		if err != nil {
			t.Fatal(err)
		}
		a, err := Decompress(nil, out)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Decompress(nil, ref)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) || !bytes.Equal(a, src) {
			t.Fatal("context output does not round trip like CompressLevel")
		}
	}
}
