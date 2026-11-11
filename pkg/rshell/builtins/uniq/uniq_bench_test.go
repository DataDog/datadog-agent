// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package uniq_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func createLargeFileUniq(tb testing.TB, dir, filename, line string, totalBytes int) string {
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

func cmdRunBUniq(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkUniq measures uniq on a 10MB file of identical lines (all deduplicated to one).
func BenchmarkUniq(b *testing.B) {
	dir := b.TempDir()
	createLargeFileUniq(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBUniq(b, "uniq input.txt", dir)
	}
}

// BenchmarkUniqCount measures uniq -c on a 10MB file.
func BenchmarkUniqCount(b *testing.B) {
	dir := b.TempDir()
	createLargeFileUniq(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBUniq(b, "uniq -c input.txt", dir)
	}
}

// TestUniqMemoryBounded asserts that uniq uses O(1) memory when processing
// large files. uniq is a streaming command: only the current and previous lines
// are kept in memory at any time (live heap is O(1)) and sc.Bytes() avoids
// per-line string allocations.
func TestUniqMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	createLargeFileUniq(t, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "uniq input.txt", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	const maxBytesPerOp = 4 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("uniq allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}
