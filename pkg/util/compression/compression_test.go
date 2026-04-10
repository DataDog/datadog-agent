// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression_test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"

	compression "github.com/DataDog/datadog-agent/pkg/util/compression"
	gzipimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-gzip"
	zlibimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
	zstdimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd"
	zstdnocgoimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zstd-nocgo"
)

// compressorCase pairs a name with a compressor for table-driven tests.
type compressorCase struct {
	name       string
	compressor compression.Compressor
}

// allCompressors returns the four compressor implementations that produce
// compressed frames (excludes noop, which is the identity function).
func allCompressors() []compressorCase {
	return []compressorCase{
		{"gzip", gzipimpl.New(gzipimpl.Requires{Level: gzip.DefaultCompression})},
		{"zlib", zlibimpl.New()},
		{"zstd-cgo", zstdimpl.New(zstdimpl.Requires{Level: 1})},
		{"zstd-nocgo", zstdnocgoimpl.New(zstdnocgoimpl.Requires{Level: 1})},
	}
}

// op represents a StreamCompressor operation.
const (
	opWrite = iota
	opFlush
	opClose
	opCount // sentinel for modular arithmetic
)

// FuzzStreamStateMachine exercises the StreamCompressor state machine with
// fuzz-generated operation sequences. For each compressor implementation:
//
//   - Interprets each byte of the fuzz input as an operation (Write, Flush, Close)
//   - Tracks whether the stream is closed
//   - Asserts that operations on a closed stream return errors
//   - After Close, asserts the output is a valid decompressible frame
//
// Derived from the StreamCompressor transition graph in compression.allium.
func FuzzStreamStateMachine(f *testing.F) {
	// Seeds: various operation sequences
	f.Add([]byte{0})             // single write
	f.Add([]byte{1})             // single flush
	f.Add([]byte{2})             // single close
	f.Add([]byte{0, 2})          // write, close
	f.Add([]byte{0, 1, 2})       // write, flush, close
	f.Add([]byte{0, 0, 1, 0, 2}) // write, write, flush, write, close
	f.Add([]byte{0, 1, 0, 1, 2}) // write, flush, write, flush, close
	f.Add([]byte{2, 0})          // close, then write (post-close)
	f.Add([]byte{2, 1})          // close, then flush (post-close)
	f.Add([]byte{2, 2})          // close, then close (post-close)
	f.Add([]byte{0, 2, 0, 1, 2}) // write, close, write, flush, close (post-close mix)

	compressors := allCompressors()

	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) == 0 {
			return
		}

		for _, cc := range compressors {
			t.Run(cc.name, func(t *testing.T) {
				var buf bytes.Buffer
				stream := cc.compressor.NewStreamCompressor(&buf)
				closed := false
				wroteData := false

				for i, b := range ops {
					op := b % byte(opCount)

					switch op {
					case opWrite:
						data := []byte(fmt.Sprintf("msg%d", i))
						_, err := stream.Write(data)
						if closed {
							if err == nil {
								t.Fatalf("op %d: Write after Close succeeded; expected error", i)
							}
						} else {
							if err != nil {
								t.Fatalf("op %d: Write failed: %v", i, err)
							}
							wroteData = true
						}

					case opFlush:
						err := stream.Flush()
						if closed {
							if err == nil {
								t.Fatalf("op %d: Flush after Close succeeded; expected error", i)
							}
						} else {
							if err != nil {
								t.Fatalf("op %d: Flush failed: %v", i, err)
							}
						}

					case opClose:
						err := stream.Close()
						if !closed {
							// Double close: behaviour varies by implementation.
							// We don't mandate error vs nil, just don't panic.
							//
							// If not closed, output must be a valid decompressible frame.
							if err != nil {
								t.Fatalf("op %d: Close failed: %v", i, err)
							}
							closed = true

							if buf.Len() == 0 && wroteData {
								t.Fatalf("op %d: Close produced empty output after writes", i)
							}

							decompressed, err := cc.compressor.Decompress(buf.Bytes())
							if err != nil {
								t.Fatalf("op %d: Decompress of stream output failed: %v", i, err)
							}

							if !wroteData && len(decompressed) != 0 {
								t.Fatalf("op %d: Empty stream decompressed to non-empty: %v", i, decompressed)
							}
						}
					}
				}

				// If the stream was never closed, close it now and verify.
				if !closed {
					if err := stream.Close(); err != nil {
						t.Fatalf("final Close failed: %v", err)
					}

					decompressed, err := cc.compressor.Decompress(buf.Bytes())
					if err != nil {
						t.Fatalf("Decompress of stream output failed: %v", err)
					}

					if !wroteData && len(decompressed) != 0 {
						t.Fatalf("Empty stream decompressed to non-empty: %v", decompressed)
					}
				}
			})
		}
	})
}

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
