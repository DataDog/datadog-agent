// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package tail_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// createLargeFileTail writes totalBytes of repeating content to dir/filename.
func createLargeFileTail(tb testing.TB, dir, filename, line string, totalBytes int) string {
	tb.Helper()
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(testutil.NewRepeatReader(line), int64(totalBytes))); err != nil {
		tb.Fatal(err)
	}
	return path
}

// cmdRunBTail runs a tail command with AllowedPaths set to dir (bench variant).
func cmdRunBTail(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkTailTenLines measures tail -n 10 on a 10MB file.
func BenchmarkTailTenLines(b *testing.B) {
	dir := b.TempDir()
	createLargeFileTail(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBTail(b, "tail -n 10 input.txt", dir)
	}
}

// BenchmarkTailBytes measures tail -c 1024 on a 10MB file.
func BenchmarkTailBytes(b *testing.B) {
	dir := b.TempDir()
	createLargeFileTail(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBTail(b, "tail -c 1024 input.txt", dir)
	}
}

// TestTailMemoryBounded asserts that tail -n 10 uses O(1) memory regardless of
// input size. The ring buffer slots are reused via append(slot[:0], raw...),
// so no per-line allocation occurs after the first pass fills the ring.
func TestTailMemoryBounded(t *testing.T) {
	const line = "the quick brown fox jumps over the lazy dog\n" // 44 bytes
	const inputSize = 10 << 20                                   // 10 MB

	dir := t.TempDir()
	createLargeFileTail(t, dir, "input.txt", line, inputSize)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "tail -n 10 input.txt", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	const maxBytesPerOp = 4 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("tail -n 10 allocated %d bytes/op on %d-byte input; want < %d", bpo, inputSize, maxBytesPerOp)
	}
}
