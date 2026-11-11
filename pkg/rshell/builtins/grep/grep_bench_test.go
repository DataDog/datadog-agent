// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package grep_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func createLargeFileGrep(tb testing.TB, dir, filename, line string, totalBytes int) string {
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

func cmdRunBGrep(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkGrepMatch measures grep on a 10MB file where every line matches.
func BenchmarkGrepMatch(b *testing.B) {
	dir := b.TempDir()
	createLargeFileGrep(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBGrep(b, "grep fox input.txt", dir)
	}
}

// BenchmarkGrepNoMatch measures grep on a 10MB file where no lines match.
func BenchmarkGrepNoMatch(b *testing.B) {
	dir := b.TempDir()
	createLargeFileGrep(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBGrep(b, "grep NOMATCH input.txt", dir)
	}
}

// BenchmarkGrepFixedStrings measures grep -F on a 10MB file.
func BenchmarkGrepFixedStrings(b *testing.B) {
	dir := b.TempDir()
	createLargeFileGrep(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBGrep(b, "grep -F fox input.txt", dir)
	}
}

// BenchmarkGrepCount measures grep -c on a 10MB file.
func BenchmarkGrepCount(b *testing.B) {
	dir := b.TempDir()
	createLargeFileGrep(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBGrep(b, "grep -c fox input.txt", dir)
	}
}

// TestGrepMemoryBounded asserts that grep uses O(1) memory when processing
// large files. grep is a streaming command that reads one line at a time via
// sc.Bytes() (no per-line string allocation). Total allocations are dominated
// by the shell/runner overhead, not input size.
func TestGrepMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	createLargeFileGrep(t, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "grep fox input.txt", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	const maxBytesPerOp = 4 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("grep allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}

// TestGrepBeforeContextMemoryBounded asserts that grep -B N with large lines
// stays within the MaxContextBytes sliding-window cap. Lines are 8 KiB each;
// requesting -B 1000 would hold 1000 × 8 KiB ≈ 8 MiB live without the cap.
// With the cap the live window is bounded to MaxContextBytes (512 KiB).
//
// AllocedBytesPerOp captures total (not peak live) allocations: the before-
// context path allocates a copy of each line before deciding to evict, so
// total allocation tracks file size. The threshold here validates that
// allocations do not grow beyond the expected O(file_size) budget and that no
// additional unbounded accumulation occurs.
func TestGrepBeforeContextMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	// 8 KiB lines; 10 MiB file ≈ 1280 lines. Requesting -B 1000 means the
	// uncapped window would hold the entire file (1280 × 8 KiB ≈ 10 MiB live).
	// With the cap the window is bounded to MaxContextBytes (512 KiB).
	const lineSize = 8 * 1024
	createLargeFileGrep(t, dir, "input.txt", strings.Repeat("x", lineSize-1)+"\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "grep -B 1000 NOMATCH input.txt", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	// Total allocation budget: ~10 MiB of per-line copies + shell/runner overhead.
	// Capped at 24 MiB to catch any unexpected accumulation.
	const maxBytesPerOp = 24 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("grep -B 1000 allocated %d bytes/op on 10MB input with 8KiB lines; want < %d", bpo, maxBytesPerOp)
	}
}

func BenchmarkGrepMatchDiscard(b *testing.B) {
	dir := b.TempDir()
	createLargeFileGrep(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		testutil.RunScriptDiscard(b, "grep fox input.txt", dir, interp.AllowedPaths([]string{dir}))
	}
}
