// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression_test

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	gzipimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-gzip"
	noopimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
	zlibimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
	zstdnocgoimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd-nocgo"
)

// Data sizes for benchmarking
const (
	sizeSmall  = 1 * 1024        // 1KB
	sizeMedium = 64 * 1024       // 64KB
	sizeLarge  = 1 * 1024 * 1024 // 1MB
)

// entropyLevel represents the compressibility of test data
type entropyLevel int

const (
	entropyLow    entropyLevel = iota // highly compressible (repeated patterns)
	entropyMedium                     // moderately compressible (text-like)
	entropyHigh                       // mostly incompressible (random)
)

func (e entropyLevel) String() string {
	switch e {
	case entropyLow:
		return "LowEntropy"
	case entropyMedium:
		return "MediumEntropy"
	case entropyHigh:
		return "HighEntropy"
	default:
		return "Unknown"
	}
}

// generateTestData creates test data with specified size and entropy level
func generateTestData(size int, entropy entropyLevel) []byte {
	data := make([]byte, size)

	switch entropy {
	case entropyLow:
		// Highly compressible: repeated 4-byte pattern
		pattern := []byte("AAAA")
		for i := 0; i < size; i++ {
			data[i] = pattern[i%len(pattern)]
		}

	case entropyMedium:
		// Moderately compressible: simulated log-like text with repetition
		// This mimics real-world data like JSON logs or metrics
		templates := []string{
			`{"timestamp":"2024-01-15T10:30:00Z","level":"info","service":"myapp","message":"Request processed successfully"}`,
			`{"timestamp":"2024-01-15T10:30:01Z","level":"debug","service":"myapp","message":"Cache hit for key user:12345"}`,
			`{"timestamp":"2024-01-15T10:30:02Z","level":"warn","service":"myapp","message":"Slow query detected: 250ms"}`,
			`{"timestamp":"2024-01-15T10:30:03Z","level":"error","service":"myapp","message":"Connection timeout to database"}`,
		}
		pos := 0
		for i := 0; pos < size; i++ {
			template := templates[i%len(templates)]
			copyLen := min(len(template), size-pos)
			copy(data[pos:pos+copyLen], template[:copyLen])
			pos += copyLen
			if pos < size {
				data[pos] = '\n'
				pos++
			}
		}

	case entropyHigh:
		// Mostly incompressible: random data
		_, _ = rand.Read(data)
	}

	return data
}

// compressorConfig holds configuration for creating a compressor
type compressorConfig struct {
	name   string
	create func() compression.Compressor
}

// getCompressors returns all compressor configurations to benchmark
func getCompressors() []compressorConfig {
	return []compressorConfig{
		{
			name:   "Noop",
			create: func() compression.Compressor { return noopimpl.New() },
		},
		{
			name:   "Zlib",
			create: func() compression.Compressor { return zlibimpl.New() },
		},
		{
			name:   "Gzip/Level1",
			create: func() compression.Compressor { return gzipimpl.New(gzipimpl.Requires{Level: gzip.BestSpeed}) },
		},
		{
			name:   "Gzip/Level6",
			create: func() compression.Compressor { return gzipimpl.New(gzipimpl.Requires{Level: gzip.DefaultCompression}) },
		},
		{
			name:   "Gzip/Level9",
			create: func() compression.Compressor { return gzipimpl.New(gzipimpl.Requires{Level: gzip.BestCompression}) },
		},
		{
			name:   "ZstdCGO/Level1",
			create: func() compression.Compressor { return zstdimpl.New(zstdimpl.Requires{Level: 1}) },
		},
		{
			name:   "ZstdCGO/Level3",
			create: func() compression.Compressor { return zstdimpl.New(zstdimpl.Requires{Level: 3}) },
		},
		{
			name:   "ZstdCGO/Level9",
			create: func() compression.Compressor { return zstdimpl.New(zstdimpl.Requires{Level: 9}) },
		},
		{
			name:   "ZstdCGO/Level19",
			create: func() compression.Compressor { return zstdimpl.New(zstdimpl.Requires{Level: 19}) },
		},
		{
			name:   "ZstdNoCGO/Level1",
			create: func() compression.Compressor { return zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 1}) },
		},
		{
			name:   "ZstdNoCGO/Level3",
			create: func() compression.Compressor { return zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 3}) },
		},
		{
			name:   "ZstdNoCGO/Level9",
			create: func() compression.Compressor { return zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 9}) },
		},
		{
			name:   "ZstdNoCGO/Level19",
			create: func() compression.Compressor { return zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 19}) },
		},
	}
}

// dataSizeConfig holds configuration for test data sizes
type dataSizeConfig struct {
	name string
	size int
}

func getDataSizes() []dataSizeConfig {
	return []dataSizeConfig{
		{"1KB", sizeSmall},
		{"64KB", sizeMedium},
		{"1MB", sizeLarge},
	}
}

func getEntropyLevels() []entropyLevel {
	return []entropyLevel{entropyLow, entropyMedium, entropyHigh}
}

// BenchmarkCompress benchmarks compression performance across all strategies,
// data sizes, and entropy levels.
func BenchmarkCompress(b *testing.B) {
	compressors := getCompressors()
	sizes := getDataSizes()
	entropies := getEntropyLevels()

	for _, cc := range compressors {
		cc := cc // capture for closure
		b.Run(cc.name, func(b *testing.B) {
			c := cc.create()
			if c == nil {
				b.Skip("compressor not available")
			}

			for _, sc := range sizes {
				sc := sc
				b.Run(sc.name, func(b *testing.B) {
					for _, entropy := range entropies {
						entropy := entropy
						b.Run(entropy.String(), func(b *testing.B) {
							data := generateTestData(sc.size, entropy)
							b.SetBytes(int64(len(data)))
							b.ResetTimer()

							for i := 0; i < b.N; i++ {
								_, err := c.Compress(data)
								if err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				})
			}
		})
	}
}

// BenchmarkDecompress benchmarks decompression performance across all strategies,
// data sizes, and entropy levels.
func BenchmarkDecompress(b *testing.B) {
	compressors := getCompressors()
	sizes := getDataSizes()
	entropies := getEntropyLevels()

	for _, cc := range compressors {
		cc := cc
		b.Run(cc.name, func(b *testing.B) {
			c := cc.create()
			if c == nil {
				b.Skip("compressor not available")
			}

			for _, sc := range sizes {
				sc := sc
				b.Run(sc.name, func(b *testing.B) {
					for _, entropy := range entropies {
						entropy := entropy
						b.Run(entropy.String(), func(b *testing.B) {
							data := generateTestData(sc.size, entropy)

							// Pre-compress data for decompression benchmark
							compressed, err := c.Compress(data)
							if err != nil {
								b.Fatalf("failed to prepare compressed data: %v", err)
							}

							b.SetBytes(int64(len(data))) // Report original size throughput
							b.ResetTimer()

							for i := 0; i < b.N; i++ {
								_, err := c.Decompress(compressed)
								if err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				})
			}
		})
	}
}

// BenchmarkStreamCompress benchmarks streaming compression performance. Stream
// compression is useful for large data that doesn't fit in memory or when data
// is being generated incrementally.
func BenchmarkStreamCompress(b *testing.B) {
	compressors := getCompressors()
	sizes := getDataSizes()
	entropies := getEntropyLevels()

	for _, cc := range compressors {
		cc := cc
		b.Run(cc.name, func(b *testing.B) {
			c := cc.create()
			if c == nil {
				b.Skip("compressor not available")
			}

			for _, sc := range sizes {
				sc := sc
				b.Run(sc.name, func(b *testing.B) {
					for _, entropy := range entropies {
						entropy := entropy
						b.Run(entropy.String(), func(b *testing.B) {
							data := generateTestData(sc.size, entropy)
							b.SetBytes(int64(len(data)))
							b.ResetTimer()

							for i := 0; i < b.N; i++ {
								buf := bytes.NewBuffer(nil)
								sc := c.NewStreamCompressor(buf)
								_, err := sc.Write(data)
								if err != nil {
									b.Fatal(err)
								}
								err = sc.Close()
								if err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				})
			}
		})
	}
}

// BenchmarkStreamCompressChunked benchmarks streaming compression with chunked
// writes, simulating real-world scenarios where data arrives in pieces.
func BenchmarkStreamCompressChunked(b *testing.B) {
	compressors := getCompressors()
	sizes := getDataSizes()
	chunkSize := 4096 // 4KB chunks

	for _, cc := range compressors {
		cc := cc
		b.Run(cc.name, func(b *testing.B) {
			c := cc.create()
			if c == nil {
				b.Skip("compressor not available")
			}

			for _, sc := range sizes {
				sc := sc
				b.Run(sc.name, func(b *testing.B) {
					// Only test medium entropy for chunked (representative case)
					data := generateTestData(sc.size, entropyMedium)
					b.SetBytes(int64(len(data)))
					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						buf := bytes.NewBuffer(nil)
						stream := c.NewStreamCompressor(buf)

						for offset := 0; offset < len(data); offset += chunkSize {
							end := offset + chunkSize
							if end > len(data) {
								end = len(data)
							}
							_, err := stream.Write(data[offset:end])
							if err != nil {
								b.Fatal(err)
							}
						}

						err := stream.Close()
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
