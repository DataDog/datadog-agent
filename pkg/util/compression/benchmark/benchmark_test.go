// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && zstd && !no_rust_compression

package benchmark

import (
	"bytes"
	"testing"

	gzipimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-gzip"
	rustimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-rust"
	zlibimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
)

var (
	// Small payload - typical small metric batch (~420 bytes)
	smallPayload = bytes.Repeat([]byte("metric.name:123|g|#tag1:value1,tag2:value2\n"), 10)
	// Medium payload - typical metric batch (~42KB)
	mediumPayload = bytes.Repeat([]byte("metric.name:123|g|#tag1:value1,tag2:value2\n"), 1000)
	// Large payload - large metric batch (~4.2MB)
	largePayload = bytes.Repeat([]byte("metric.name:123|g|#tag1:value1,tag2:value2\n"), 100000)
)

// ============================================================================
// ZSTD Compression Benchmarks
// ============================================================================

func BenchmarkZstdCompressInto_Go_Small(b *testing.B) {
	comp := zstdimpl.New(zstdimpl.Requires{Level: 3})
	dst := make([]byte, comp.CompressBound(len(smallPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(smallPayload, dst)
	}
}

func BenchmarkZstdCompressInto_Rust_Small(b *testing.B) {
	comp := rustimpl.NewZstd(3)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(smallPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(smallPayload, dst)
	}
}

func BenchmarkZstdCompressInto_RustStateless_Small(b *testing.B) {
	dst := make([]byte, rustimpl.ZstdCompressBound(len(smallPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rustimpl.ZstdCompressStateless(smallPayload, dst, 3)
	}
}

func BenchmarkZstdCompressInto_RustDirect_Small(b *testing.B) {
	comp := rustimpl.NewZstd(3).(*rustimpl.RustCompressor)
	defer comp.Close()
	dst := make([]byte, comp.CompressBound(len(smallPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.ZstdCompressDirect(smallPayload, dst)
	}
}

func BenchmarkZstdCompressInto_Go_Medium(b *testing.B) {
	comp := zstdimpl.New(zstdimpl.Requires{Level: 3})
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}

func BenchmarkZstdCompressInto_Rust_Medium(b *testing.B) {
	comp := rustimpl.NewZstd(3)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}

func BenchmarkZstdCompressInto_Go_Large(b *testing.B) {
	comp := zstdimpl.New(zstdimpl.Requires{Level: 3})
	dst := make([]byte, comp.CompressBound(len(largePayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(largePayload, dst)
	}
}

func BenchmarkZstdCompressInto_Rust_Large(b *testing.B) {
	comp := rustimpl.NewZstd(3)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(largePayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(largePayload, dst)
	}
}

// ============================================================================
// Gzip Compression Benchmarks
// ============================================================================

func BenchmarkGzipCompressInto_Go_Small(b *testing.B) {
	comp := gzipimpl.New(gzipimpl.Requires{Level: 6})
	dst := make([]byte, comp.CompressBound(len(smallPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(smallPayload, dst)
	}
}

func BenchmarkGzipCompressInto_Rust_Small(b *testing.B) {
	comp := rustimpl.NewGzip(6)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(smallPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(smallPayload, dst)
	}
}

func BenchmarkGzipCompressInto_Go_Medium(b *testing.B) {
	comp := gzipimpl.New(gzipimpl.Requires{Level: 6})
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}

func BenchmarkGzipCompressInto_Rust_Medium(b *testing.B) {
	comp := rustimpl.NewGzip(6)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}

func BenchmarkGzipCompressInto_Go_Large(b *testing.B) {
	comp := gzipimpl.New(gzipimpl.Requires{Level: 6})
	dst := make([]byte, comp.CompressBound(len(largePayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(largePayload, dst)
	}
}

func BenchmarkGzipCompressInto_Rust_Large(b *testing.B) {
	comp := rustimpl.NewGzip(6)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(largePayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(largePayload, dst)
	}
}

// ============================================================================
// Zlib Compression Benchmarks
// ============================================================================

func BenchmarkZlibCompressInto_Go_Small(b *testing.B) {
	comp := zlibimpl.New()
	dst := make([]byte, comp.CompressBound(len(smallPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(smallPayload, dst)
	}
}

func BenchmarkZlibCompressInto_Rust_Small(b *testing.B) {
	comp := rustimpl.NewZlib(6)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(smallPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(smallPayload, dst)
	}
}

func BenchmarkZlibCompressInto_Go_Medium(b *testing.B) {
	comp := zlibimpl.New()
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}

func BenchmarkZlibCompressInto_Rust_Medium(b *testing.B) {
	comp := rustimpl.NewZlib(6)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}

func BenchmarkZlibCompressInto_Go_Large(b *testing.B) {
	comp := zlibimpl.New()
	dst := make([]byte, comp.CompressBound(len(largePayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(largePayload, dst)
	}
}

func BenchmarkZlibCompressInto_Rust_Large(b *testing.B) {
	comp := rustimpl.NewZlib(6)
	defer comp.(*rustimpl.RustCompressor).Close()
	dst := make([]byte, comp.CompressBound(len(largePayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(largePayload, dst)
	}
}

// ============================================================================
// Stream Compression Benchmarks
// ============================================================================

func BenchmarkZstdStream_Go_Medium(b *testing.B) {
	comp := zstdimpl.New(zstdimpl.Requires{Level: 3})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		stream := comp.NewStreamCompressor(&buf)
		_, _ = stream.Write(mediumPayload)
		_ = stream.Close()
	}
}

func BenchmarkZstdStream_Rust_Medium(b *testing.B) {
	comp := rustimpl.NewZstd(3)
	defer comp.(*rustimpl.RustCompressor).Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		stream := comp.NewStreamCompressor(&buf)
		_, _ = stream.Write(mediumPayload)
		_ = stream.Close()
	}
}

func BenchmarkGzipStream_Go_Medium(b *testing.B) {
	comp := gzipimpl.New(gzipimpl.Requires{Level: 6})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		stream := comp.NewStreamCompressor(&buf)
		_, _ = stream.Write(mediumPayload)
		_ = stream.Close()
	}
}

func BenchmarkGzipStream_Rust_Medium(b *testing.B) {
	comp := rustimpl.NewGzip(6)
	defer comp.(*rustimpl.RustCompressor).Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		stream := comp.NewStreamCompressor(&buf)
		_, _ = stream.Write(mediumPayload)
		_ = stream.Close()
	}
}

func BenchmarkZlibStream_Go_Medium(b *testing.B) {
	comp := zlibimpl.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		stream := comp.NewStreamCompressor(&buf)
		_, _ = stream.Write(mediumPayload)
		_ = stream.Close()
	}
}

func BenchmarkZlibStream_Rust_Medium(b *testing.B) {
	comp := rustimpl.NewZlib(6)
	defer comp.(*rustimpl.RustCompressor).Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		stream := comp.NewStreamCompressor(&buf)
		_, _ = stream.Write(mediumPayload)
		_ = stream.Close()
	}
}

// ============================================================================
// Memory Allocation Benchmarks
// ============================================================================

func BenchmarkZstdCompressInto_Rust_Medium_Allocs(b *testing.B) {
	comp := rustimpl.NewZstd(3).(*rustimpl.RustCompressor)
	defer comp.Close()
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}

func BenchmarkGzipCompressInto_Rust_Medium_Allocs(b *testing.B) {
	comp := rustimpl.NewGzip(6).(*rustimpl.RustCompressor)
	defer comp.Close()
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}

func BenchmarkZlibCompressInto_Rust_Medium_Allocs(b *testing.B) {
	comp := rustimpl.NewZlib(6).(*rustimpl.RustCompressor)
	defer comp.Close()
	dst := make([]byte, comp.CompressBound(len(mediumPayload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressInto(mediumPayload, dst)
	}
}
