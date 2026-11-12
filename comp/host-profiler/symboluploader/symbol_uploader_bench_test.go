// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

// BenchmarkSymbolCompressionLevel measures encoder CPU and output size of the
// zstd levels considered for symbolCompressionLevel, on a real objcopy-extracted
// debug ELF (the payload streamRequestBody compresses), plus the previous
// transport for comparison: objcopy --compress-debug-sections=zstd applied to
// the same payload. Both are the delta each approach adds over a plain
// (uncompressed) extraction, so ns/op and output size are directly comparable
// across sub-benchmarks. Encoder options mirror streamRequestBody: one encoder
// per run (the upload path does not pool encoders), concurrency 1, window from
// symbolCompressionWindow. Use it to revisit symbolCompressionLevel.
//
// This benchmark target is self-contained (it can't reach the helpers compiled
// into the other test targets) and needs objcopy on PATH. Run it from a Linux
// dev container or workspace; add -v for input/ratio sizes:
//
//	go test -run=XXX -bench=BenchmarkSymbolCompressionLevel -benchmem -v \
//	  -benchtime=2s ./comp/host-profiler/symboluploader/
//
// The default hello world payload is small; point SYMBOL_BENCH_INPUT at a
// real binary for a realistically sized payload. The binary must contain
// DWARF: `dda inv host-profiler.build` strips it (-w) unless DELVE=1 is set.
// Use an absolute path; the test runs from the package directory:
//
//	DELVE=1 dda inv host-profiler.build
//	SYMBOL_BENCH_INPUT=$(pwd)/bin/host-profiler/host-profiler \
//	  go test -run=XXX -bench=BenchmarkSymbolCompressionLevel -benchmem -v \
//	  -benchtime=2s ./comp/host-profiler/symboluploader/
func BenchmarkSymbolCompressionLevel(b *testing.B) {
	data := buildBenchSymbolFile(b)

	for _, level := range []zstd.EncoderLevel{
		zstd.SpeedFastest,
		zstd.SpeedDefault,
		zstd.SpeedBetterCompression,
		zstd.SpeedBestCompression,
	} {
		b.Run(level.String(), func(b *testing.B) {
			var compressed bytes.Buffer
			enc, err := zstd.NewWriter(&compressed,
				zstd.WithEncoderLevel(level),
				zstd.WithEncoderConcurrency(1),
				zstd.WithWindowSize(symbolCompressionWindow(int64(len(data)))),
			)
			require.NoError(b, err)
			_, err = io.Copy(enc, bytes.NewReader(data))
			require.NoError(b, err)
			require.NoError(b, enc.Close())
			b.Logf("input=%d compressed=%d ratio=%.4f", len(data), compressed.Len(), float64(compressed.Len())/float64(len(data)))

			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				enc, err := zstd.NewWriter(io.Discard,
					zstd.WithEncoderLevel(level),
					zstd.WithEncoderConcurrency(1),
					zstd.WithWindowSize(symbolCompressionWindow(int64(len(data)))),
				)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := io.Copy(enc, bytes.NewReader(data)); err != nil {
					b.Fatal(err)
				}
				if err := enc.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	// The old transport: compress the debug sections in place with objcopy and
	// upload the file uncompressed. ns/op is objcopy's compression work on the
	// same input the encoders above compress; output size is the uploaded part.
	b.Run("objcopy-section-zstd", func(b *testing.B) {
		dir := b.TempDir()
		plainPath := filepath.Join(dir, "bench.dbg")
		require.NoError(b, os.WriteFile(plainPath, data, 0o644))
		sectPath := filepath.Join(dir, "bench-sect.dbg")

		out, err := exec.Command("objcopy", "--compress-debug-sections=zstd", plainPath, sectPath).CombinedOutput()
		require.NoError(b, err, "objcopy failed: %s", out)
		fi, err := os.Stat(sectPath)
		require.NoError(b, err)
		b.Logf("input=%d compressed=%d ratio=%.4f", len(data), fi.Size(), float64(fi.Size())/float64(len(data)))

		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for range b.N {
			out, err := exec.Command("objcopy", "--compress-debug-sections=zstd", plainPath, sectPath).CombinedOutput()
			if err != nil {
				b.Fatalf("objcopy failed: %s", out)
			}
		}
	})
}

// buildBenchSymbolFile returns a debug-only ELF to compress, mirroring what
// createSymbolFile feeds to streamRequestBody. With SYMBOL_BENCH_INPUT set to
// an executable path the payload is extracted from that binary; otherwise the
// test hello world is built (DWARF is included by default). The objcopy part
// is a slimmed-down copy of buildGo from symbol_uploader_test.go, kept
// self-contained because bazelle gives each test file its own target.
func buildBenchSymbolFile(t testing.TB) []byte {
	dir := t.TempDir()
	exe := os.Getenv("SYMBOL_BENCH_INPUT")
	if exe == "" {
		goBinary, err := exec.LookPath("go")
		require.NoError(t, err, "benchmark needs the Go toolchain on PATH, or SYMBOL_BENCH_INPUT set to a binary")

		exe = filepath.Join(dir, "helloworld")
		out, err := exec.Command(goBinary, "build", "-o", exe,
			"-ldflags=-buildid=bench_build_id", "./testdata/helloworld.go").CombinedOutput()
		require.NoError(t, err, "go build failed: %s", out)
	}

	symFile := filepath.Join(dir, "bench.dbg")
	out, err := exec.Command("objcopy", "--only-keep-debug", exe, symFile).CombinedOutput()
	require.NoError(t, err, "objcopy failed: %s", out)

	data, err := os.ReadFile(symFile)
	require.NoError(t, err)
	t.Logf("bench input %s: debug ELF %d bytes", exe, len(data))
	return data
}
