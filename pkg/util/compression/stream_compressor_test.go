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

func getTestCompressors() []struct {
	name       string
	compressor compression.Compressor
} {
	return []struct {
		name       string
		compressor compression.Compressor
	}{
		{"zlib", zlibimpl.New()},
		{"gzip", gzipimpl.New(gzipimpl.Requires{Level: gzip.BestSpeed})},
		{"zstd", zstdimpl.New(zstdimpl.Requires{Level: 1})},
		{"zstd-nocgo", zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 1})},
	}
}

// TestStreamCompressorLifecycle tests the StreamCompressor API contract
func TestStreamCompressorLifecycle(t *testing.T) {
	for _, tc := range getTestCompressors() {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("The quick brown fox jumps over the lazy dog")
			var buf bytes.Buffer
			sc := tc.compressor.NewStreamCompressor(&buf)

			n, err := sc.Write(data)
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			if n != len(data) {
				t.Errorf("Write returned %d, expected %d", n, len(data))
			}

			if err := sc.Flush(); err != nil {
				t.Fatalf("Flush failed: %v", err)
			}
			if err := sc.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}

			decompressed, err := tc.compressor.Decompress(buf.Bytes())
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}
			if !bytes.Equal(data, decompressed) {
				t.Error("Decompressed data doesn't match original")
			}
		})
	}
}

// TestStreamCompressorMultipleWrites tests multiple writes with interleaved flushes
func TestStreamCompressorMultipleWrites(t *testing.T) {
	for _, tc := range getTestCompressors() {
		t.Run(tc.name, func(t *testing.T) {
			chunks := [][]byte{
				[]byte("First chunk. "),
				[]byte("Second chunk. "),
				[]byte("Third chunk."),
			}
			expected := bytes.Join(chunks, nil)

			var buf bytes.Buffer
			sc := tc.compressor.NewStreamCompressor(&buf)

			for _, chunk := range chunks {
				if _, err := sc.Write(chunk); err != nil {
					t.Fatalf("Write failed: %v", err)
				}
				if err := sc.Flush(); err != nil {
					t.Fatalf("Flush failed: %v", err)
				}
			}
			if err := sc.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}

			decompressed, err := tc.compressor.Decompress(buf.Bytes())
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}
			if !bytes.Equal(expected, decompressed) {
				t.Error("Decompressed data doesn't match")
			}
		})
	}
}

// TestStreamCompressorCloseIdempotency tests that Close can be called multiple times
func TestStreamCompressorCloseIdempotency(t *testing.T) {
	for _, tc := range getTestCompressors() {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			sc := tc.compressor.NewStreamCompressor(&buf)

			if _, err := sc.Write([]byte("test")); err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			for i := range 3 {
				if err := sc.Close(); err != nil {
					t.Errorf("Close call %d failed: %v", i+1, err)
				}
			}
		})
	}
}

// TestStreamCompressorWriteAfterClose tests that Write after Close returns an error
func TestStreamCompressorWriteAfterClose(t *testing.T) {
	for _, tc := range getTestCompressors() {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			sc := tc.compressor.NewStreamCompressor(&buf)

			if _, err := sc.Write([]byte("initial")); err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			if err := sc.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}

			if _, err := sc.Write([]byte("after close")); err == nil {
				t.Error("Write after Close should return error")
			}
		})
	}
}

// TestStreamCompressorEmptyWrite tests writing empty data
func TestStreamCompressorEmptyWrite(t *testing.T) {
	for _, tc := range getTestCompressors() {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			sc := tc.compressor.NewStreamCompressor(&buf)

			n, err := sc.Write([]byte{})
			if err != nil {
				t.Errorf("Write([]byte{}) failed: %v", err)
			}
			if n != 0 {
				t.Errorf("Write([]byte{}) returned %d, expected 0", n)
			}

			if err := sc.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}

			decompressed, err := tc.compressor.Decompress(buf.Bytes())
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}
			if len(decompressed) != 0 {
				t.Errorf("Expected empty output, got %d bytes", len(decompressed))
			}
		})
	}
}
