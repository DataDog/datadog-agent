// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression_test

import (
	"compress/gzip"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	gzipimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-gzip"
	zlibimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
	zstdnocgoimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd-nocgo"
)

// FuzzCompressBoundZlib verifies that CompressBound provides sufficient buffer size
func FuzzCompressBoundZlib(f *testing.F) {
	addCommonSeeds(f)
	c := zlibimpl.New()

	f.Fuzz(func(t *testing.T, data []byte) {
		if c == nil {
			t.Fatal("zlib compressor is nil")
		}

		compressed, err := c.Compress(data)
		if err != nil {
			t.Fatalf("Compress failed: %v", err)
		}

		bound := c.CompressBound(len(data))
		if len(compressed) > bound {
			t.Errorf("CompressBound violated: compressed size %d > bound %d (input size %d)",
				len(compressed), bound, len(data))
		}
	})
}

// FuzzCompressBoundGzip verifies that CompressBound provides sufficient buffer size
func FuzzCompressBoundGzip(f *testing.F) {
	// Seed with various data and level combinations
	f.Add([]byte("hello world"), int8(gzip.BestSpeed))
	f.Add([]byte(""), int8(gzip.BestCompression))
	f.Add([]byte("a"), int8(gzip.DefaultCompression))
	f.Add([]byte(string(make([]byte, 1000))), int8(6))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), int8(9))

	f.Fuzz(func(t *testing.T, data []byte, level int8) {
		// Clamp level to valid range: -1 (DefaultCompression) to 9 (BestCompression)
		if level < -1 {
			level = -1
		}
		if level > 9 {
			level = 9
		}

		c := gzipimpl.New(gzipimpl.Requires{Level: int(level)})
		if c == nil {
			t.Fatal("gzip compressor is nil")
		}

		compressed, err := c.Compress(data)
		if err != nil {
			t.Fatalf("Compress failed with level %d: %v", level, err)
		}

		bound := c.CompressBound(len(data))
		if len(compressed) > bound {
			t.Errorf("CompressBound violated at level %d: compressed size %d > bound %d (input size %d)",
				level, len(compressed), bound, len(data))
		}
	})
}

// FuzzCompressBoundZstd verifies that CompressBound provides sufficient buffer size
func FuzzCompressBoundZstd(f *testing.F) {
	// Seed with various data and level combinations
	f.Add([]byte("hello world"), int8(1))
	f.Add([]byte(""), int8(3))
	f.Add([]byte("a"), int8(5))
	f.Add([]byte(string(make([]byte, 1000))), int8(9))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), int8(22))

	f.Fuzz(func(t *testing.T, data []byte, level int8) {
		// Clamp level to valid range: 1 to 22
		if level < 1 {
			level = 1
		}
		if level > 22 {
			level = 22
		}

		c := zstdimpl.New(zstdimpl.Requires{Level: compression.ZstdCompressionLevel(level)})
		if c == nil {
			t.Fatal("zstd compressor is nil")
		}

		compressed, err := c.Compress(data)
		if err != nil {
			t.Fatalf("Compress failed with level %d: %v", level, err)
		}

		bound := c.CompressBound(len(data))
		if len(compressed) > bound {
			t.Errorf("CompressBound violated at level %d: compressed size %d > bound %d (input size %d)",
				level, len(compressed), bound, len(data))
		}
	})
}

// FuzzCompressBoundZstdNoCgo verifies that CompressBound provides sufficient buffer size
func FuzzCompressBoundZstdNoCgo(f *testing.F) {
	// Seed with various data and level combinations
	f.Add([]byte("hello world"), int8(1))
	f.Add([]byte(""), int8(3))
	f.Add([]byte("a"), int8(5))
	f.Add([]byte(string(make([]byte, 1000))), int8(9))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), int8(22))

	f.Fuzz(func(t *testing.T, data []byte, level int8) {
		// Clamp level to valid range: 1 to 22
		if level < 1 {
			level = 1
		}
		if level > 22 {
			level = 22
		}

		c := zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: compression.ZstdCompressionLevel(level)})
		if c == nil {
			t.Fatal("zstd-nocgo compressor is nil")
		}

		compressed, err := c.Compress(data)
		if err != nil {
			t.Fatalf("Compress failed with level %d: %v", level, err)
		}

		bound := c.CompressBound(len(data))
		if len(compressed) > bound {
			t.Errorf("CompressBound violated at level %d: compressed size %d > bound %d (input size %d)",
				level, len(compressed), bound, len(data))
		}
	})
}
