// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && !no_rust_compression

package compression_test

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	rustimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-rust"
	zlibimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
	"github.com/DataDog/datadog-agent/pkg/util/compression/testutil"
)

// FuzzRoundTripZstdRust tests that Rust zstd compression round-trips correctly.
func FuzzRoundTripZstdRust(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte{0, 0, 0, 0})
	f.Add(bytes.Repeat([]byte("a"), 1000))
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc})

	compressor := rustimpl.NewZstd(3)
	defer compressor.(*rustimpl.RustCompressor).Close()

	f.Fuzz(func(t *testing.T, data []byte) {
		roundTrip(t, compressor, data)
	})
}

// FuzzRoundTripZstdGo tests that Go zstd compression round-trips correctly.
func FuzzRoundTripZstdGo(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte{0, 0, 0, 0})
	f.Add(bytes.Repeat([]byte("a"), 1000))
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc})

	compressor := zstdimpl.New(zstdimpl.Requires{Level: 3})

	f.Fuzz(func(t *testing.T, data []byte) {
		roundTrip(t, compressor, data)
	})
}

// FuzzRoundTripGzipRust tests that Rust gzip compression round-trips correctly.
func FuzzRoundTripGzipRust(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte{0, 0, 0, 0})
	f.Add(bytes.Repeat([]byte("a"), 1000))
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc})

	compressor := rustimpl.NewGzip(6)
	defer compressor.(*rustimpl.RustCompressor).Close()

	f.Fuzz(func(t *testing.T, data []byte) {
		roundTrip(t, compressor, data)
	})
}

// FuzzRoundTripZlibRust tests that Rust zlib compression round-trips correctly.
func FuzzRoundTripZlibRust(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte{0, 0, 0, 0})
	f.Add(bytes.Repeat([]byte("a"), 1000))
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc})

	compressor := rustimpl.NewZlib(6)
	defer compressor.(*rustimpl.RustCompressor).Close()

	f.Fuzz(func(t *testing.T, data []byte) {
		roundTrip(t, compressor, data)
	})
}

// FuzzRoundTripZlibGo tests that Go zlib compression round-trips correctly.
func FuzzRoundTripZlibGo(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte{0, 0, 0, 0})
	f.Add(bytes.Repeat([]byte("a"), 1000))
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc})

	compressor := zlibimpl.New()

	f.Fuzz(func(t *testing.T, data []byte) {
		roundTrip(t, compressor, data)
	})
}

// roundTrip compresses and decompresses data, verifying the result matches.
func roundTrip(t *testing.T, compressor compression.Compressor, data []byte) {
	t.Helper()

	if len(data) == 0 {
		return // Empty input is a no-op
	}

	// Compress
	dst := make([]byte, compressor.CompressBound(len(data)))
	n, err := compressor.CompressInto(data, dst)
	if err != nil {
		t.Fatalf("CompressInto failed: %v", err)
	}
	compressed := dst[:n]

	// Decompress
	decompressed, err := testutil.Decompress(compressed, compressor.ContentEncoding())
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	// Verify round-trip
	if !bytes.Equal(data, decompressed) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d bytes", len(decompressed), len(data))
	}
}

// FuzzCrossImplZstd tests that Go and Rust zstd implementations are compatible.
// Data compressed with one can be decompressed with the other.
func FuzzCrossImplZstd(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte{0, 0, 0, 0})
	f.Add(bytes.Repeat([]byte("a"), 1000))
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc})

	rustCompressor := rustimpl.NewZstd(3)
	defer rustCompressor.(*rustimpl.RustCompressor).Close()
	goCompressor := zstdimpl.New(zstdimpl.Requires{Level: 3})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}

		// Rust compress -> Go decompress (via testutil which uses Go's zstd)
		dst := make([]byte, rustCompressor.CompressBound(len(data)))
		n, err := rustCompressor.CompressInto(data, dst)
		if err != nil {
			t.Fatalf("Rust CompressInto failed: %v", err)
		}
		decompressed, err := testutil.DecompressZstd(dst[:n])
		if err != nil {
			t.Fatalf("Decompress Rust-compressed data failed: %v", err)
		}
		if !bytes.Equal(data, decompressed) {
			t.Fatalf("Rust->Go round-trip mismatch")
		}

		// Go compress -> decompress (both use same underlying zstd library)
		dst = make([]byte, goCompressor.CompressBound(len(data)))
		n, err = goCompressor.CompressInto(data, dst)
		if err != nil {
			t.Fatalf("Go CompressInto failed: %v", err)
		}
		decompressed, err = testutil.DecompressZstd(dst[:n])
		if err != nil {
			t.Fatalf("Decompress Go-compressed data failed: %v", err)
		}
		if !bytes.Equal(data, decompressed) {
			t.Fatalf("Go round-trip mismatch")
		}
	})
}

// FuzzCrossImplZlib tests that Go and Rust zlib implementations are compatible.
func FuzzCrossImplZlib(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte{0, 0, 0, 0})
	f.Add(bytes.Repeat([]byte("a"), 1000))
	f.Add([]byte{0xff, 0xfe, 0xfd, 0xfc})

	rustCompressor := rustimpl.NewZlib(6)
	defer rustCompressor.(*rustimpl.RustCompressor).Close()
	goCompressor := zlibimpl.New()

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}

		// Rust compress -> Go decompress
		dst := make([]byte, rustCompressor.CompressBound(len(data)))
		n, err := rustCompressor.CompressInto(data, dst)
		if err != nil {
			t.Fatalf("Rust CompressInto failed: %v", err)
		}
		decompressed, err := testutil.DecompressZlib(dst[:n])
		if err != nil {
			t.Fatalf("Decompress Rust-compressed data failed: %v", err)
		}
		if !bytes.Equal(data, decompressed) {
			t.Fatalf("Rust->Go round-trip mismatch")
		}

		// Go compress -> Go decompress
		dst = make([]byte, goCompressor.CompressBound(len(data)))
		n, err = goCompressor.CompressInto(data, dst)
		if err != nil {
			t.Fatalf("Go CompressInto failed: %v", err)
		}
		decompressed, err = testutil.DecompressZlib(dst[:n])
		if err != nil {
			t.Fatalf("Decompress Go-compressed data failed: %v", err)
		}
		if !bytes.Equal(data, decompressed) {
			t.Fatalf("Go round-trip mismatch")
		}
	})
}
