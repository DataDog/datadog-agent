// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression_test

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	gzipimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-gzip"
	zlibimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
	zstdnocgoimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd-nocgo"
)

// testIdentityProperty tests that b == decompress(compress(b))
func testIdentityProperty(t *testing.T, c compression.Compressor, data []byte) {
	if c == nil {
		t.Fatal("compressor is nil, initialization failed")
	}

	compressed, err := c.Compress(data)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(data, decompressed) {
		t.Errorf("Identity property violated. Original length: %d, Decompressed length: %d",
			len(data), len(decompressed))
		if len(data) < 100 && len(decompressed) < 100 {
			t.Errorf("Original: %v", data)
			t.Errorf("Decompressed: %v", decompressed)
		}
	}
}

func addCommonSeeds(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte(string(make([]byte, 1000))))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))
	f.Add(bytes.Repeat([]byte("abcd"), 250))
}

func FuzzZlib(f *testing.F) {
	addCommonSeeds(f)
	c := zlibimpl.New()
	f.Fuzz(func(t *testing.T, data []byte) {
		testIdentityProperty(t, c, data)
	})
}

func FuzzGzip(f *testing.F) {
	addCommonSeeds(f)
	c := gzipimpl.New(gzipimpl.Requires{Level: gzip.BestSpeed})
	f.Fuzz(func(t *testing.T, data []byte) {
		testIdentityProperty(t, c, data)
	})
}

func FuzzZstd(f *testing.F) {
	addCommonSeeds(f)
	c := zstdimpl.New(zstdimpl.Requires{Level: 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		testIdentityProperty(t, c, data)
	})
}

func FuzzZstdNoCgo(f *testing.F) {
	addCommonSeeds(f)
	c := zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		testIdentityProperty(t, c, data)
	})
}

// FuzzZstdCrossCompatibility tests that zstd CGO and NoCGO implementations
// are compatible: data compressed by one can be decompressed by the other
func FuzzZstdCrossCompatibility(f *testing.F) {
	addCommonSeeds(f)
	cgo := zstdimpl.New(zstdimpl.Requires{Level: 1})
	nocgo := zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 1})

	f.Fuzz(func(t *testing.T, data []byte) {
		if cgo == nil {
			t.Fatal("zstd cgo compressor is nil")
		}
		if nocgo == nil {
			t.Fatal("zstd nocgo compressor is nil")
		}

		// Test: CGO compress -> NoCGO decompress
		cgoCompressed, err := cgo.Compress(data)
		if err != nil {
			t.Fatalf("CGO Compress failed: %v", err)
		}

		nocgoDecompressed, err := nocgo.Decompress(cgoCompressed)
		if err != nil {
			t.Fatalf("NoCGO Decompress of CGO-compressed data failed: %v", err)
		}

		if !bytes.Equal(data, nocgoDecompressed) {
			t.Errorf("CGO->NoCGO cross-compatibility failed. Original length: %d, Decompressed length: %d",
				len(data), len(nocgoDecompressed))
		}

		// Test: NoCGO compress -> CGO decompress
		nocgoCompressed, err := nocgo.Compress(data)
		if err != nil {
			t.Fatalf("NoCGO Compress failed: %v", err)
		}

		cgoDecompressed, err := cgo.Decompress(nocgoCompressed)
		if err != nil {
			t.Fatalf("CGO Decompress of NoCGO-compressed data failed: %v", err)
		}

		if !bytes.Equal(data, cgoDecompressed) {
			t.Errorf("NoCGO->CGO cross-compatibility failed. Original length: %d, Decompressed length: %d",
				len(data), len(cgoDecompressed))
		}
	})
}
