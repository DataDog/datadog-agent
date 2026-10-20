// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package tr_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func createLargeFileTr(tb testing.TB, dir, filename, line string, totalBytes int) string {
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

func cmdRunBTr(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkTrTranslateDiscard measures tr with stdout discarded to isolate
// tr's own allocations from output buffering. Used to calibrate the ceiling
// in TestTrMemoryBounded.
func BenchmarkTrTranslateDiscard(b *testing.B) {
	dir := b.TempDir()
	createLargeFileTr(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		testutil.RunScriptDiscard(b, "cat input.txt | tr 'a-z' 'A-Z'", dir, interp.AllowedPaths([]string{dir}))
	}
}

// BenchmarkTrTranslate measures tr 'a-z' 'A-Z' on a 1MB file piped through tr.
// tr reads input from stdin in fixed 32 KiB chunks and translates byte-by-byte
// using a pre-built 256-entry lookup table.
func BenchmarkTrTranslate(b *testing.B) {
	dir := b.TempDir()
	createLargeFileTr(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 1<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBTr(b, "cat input.txt | tr 'a-z' 'A-Z'", dir)
	}
}

// BenchmarkTrDelete measures tr -d on a 1MB file.
func BenchmarkTrDelete(b *testing.B) {
	dir := b.TempDir()
	createLargeFileTr(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 1<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBTr(b, "cat input.txt | tr -d ' '", dir)
	}
}

// BenchmarkTrSqueeze measures tr -s on a 1MB file.
func BenchmarkTrSqueeze(b *testing.B) {
	dir := b.TempDir()
	createLargeFileTr(b, dir, "input.txt", "the  quick  brown  fox  jumps  over  the  lazy  dog\n", 1<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBTr(b, "cat input.txt | tr -s ' '", dir)
	}
}

// TestTrMemoryBounded asserts that tr uses O(1) memory regardless of input
// size. tr operates on a 256-entry lookup table built once at startup. Input
// is read in fixed 32 KiB chunks and translated in-place; no allocation is
// proportional to input length.
func TestTrMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	createLargeFileTr(t, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "cat input.txt | tr 'a-z' 'A-Z'", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	const maxBytesPerOp = 4 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("tr allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}
