// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && zstd && !no_rust_compression

package benchmark

import (
	"bytes"
	"testing"

	rustimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-rust"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
)

var (
	// Small payload - typical small metric batch
	smallPayload = bytes.Repeat([]byte("metric.name:123|g|#tag1:value1,tag2:value2\n"), 10)
	// Medium payload - typical metric batch
	mediumPayload = bytes.Repeat([]byte("metric.name:123|g|#tag1:value1,tag2:value2\n"), 1000)
	// Large payload - large metric batch
	largePayload = bytes.Repeat([]byte("metric.name:123|g|#tag1:value1,tag2:value2\n"), 100000)
)

func BenchmarkZstdCompress_Go_Small(b *testing.B) {
	comp := zstdimpl.New(zstdimpl.Requires{Level: 3})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.Compress(smallPayload)
	}
}

func BenchmarkZstdCompress_Rust_Small(b *testing.B) {
	comp := rustimpl.NewZstd(3)
	defer comp.(*rustimpl.RustCompressor).Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.Compress(smallPayload)
	}
}

func BenchmarkZstdCompress_Go_Medium(b *testing.B) {
	comp := zstdimpl.New(zstdimpl.Requires{Level: 3})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.Compress(mediumPayload)
	}
}

func BenchmarkZstdCompress_Rust_Medium(b *testing.B) {
	comp := rustimpl.NewZstd(3)
	defer comp.(*rustimpl.RustCompressor).Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.Compress(mediumPayload)
	}
}

func BenchmarkZstdCompress_Go_Large(b *testing.B) {
	comp := zstdimpl.New(zstdimpl.Requires{Level: 3})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.Compress(largePayload)
	}
}

func BenchmarkZstdCompress_Rust_Large(b *testing.B) {
	comp := rustimpl.NewZstd(3)
	defer comp.(*rustimpl.RustCompressor).Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.Compress(largePayload)
	}
}

func BenchmarkZstdDecompress_Go_Medium(b *testing.B) {
	comp := zstdimpl.New(zstdimpl.Requires{Level: 3})
	compressed, _ := comp.Compress(mediumPayload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.Decompress(compressed)
	}
}

func BenchmarkZstdDecompress_Rust_Medium(b *testing.B) {
	comp := rustimpl.NewZstd(3)
	defer comp.(*rustimpl.RustCompressor).Close()
	compressed, _ := comp.Compress(mediumPayload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.Decompress(compressed)
	}
}

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
