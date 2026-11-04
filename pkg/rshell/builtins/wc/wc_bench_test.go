// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package wc_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// createLargeFileWc writes totalBytes of repeating content to dir/filename.
func createLargeFileWc(tb testing.TB, dir, filename, line string, totalBytes int) string {
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

// cmdRunBWc runs a wc command with AllowedPaths set to dir (bench variant).
func cmdRunBWc(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkWcLines measures wc -l on a 10MB file.
func BenchmarkWcLines(b *testing.B) {
	dir := b.TempDir()
	createLargeFileWc(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBWc(b, "wc -l input.txt", dir)
	}
}

// BenchmarkWcAll measures wc (all counts) on a 10MB file.
func BenchmarkWcAll(b *testing.B) {
	dir := b.TempDir()
	createLargeFileWc(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBWc(b, "wc input.txt", dir)
	}
}

// TestWcMemoryBounded asserts that wc uses O(1) memory regardless of file size.
func TestWcMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	createLargeFileWc(t, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cmdRunBWc(b, "wc -l input.txt", dir)
		}
	})

	const maxBytesPerOp = 1 << 20 // 1MB ceiling for a streaming counter
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("wc -l allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}
