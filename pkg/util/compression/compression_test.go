// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression_test

import (
	"bytes"
	"testing"

	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
	zstdnocgoimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd-nocgo"
)

// Asserts that data compressed by zstd CGO can be decompressed by zstd NoCGO
// and vice versa, for all b.
func FuzzZstdCrossCompatibility(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte(string(make([]byte, 1000))))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))
	f.Add(bytes.Repeat([]byte("abcd"), 250))

	cgo := zstdimpl.New(zstdimpl.Requires{Level: 1})
	nocgo := zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 1})

	f.Fuzz(func(t *testing.T, data []byte) {
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
			t.Errorf("CGO->NoCGO cross-compatibility failed")
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
			t.Errorf("NoCGO->CGO cross-compatibility failed")
		}
	})
}
