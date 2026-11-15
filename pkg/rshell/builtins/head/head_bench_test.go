// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package head_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// createLargeFile writes totalBytes of repeating line content to dir/filename.
func createLargeFile(tb testing.TB, dir, filename, line string, totalBytes int) string {
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

// cmdRunB runs a head command with AllowedPaths set to dir (bench variant).
// Uses testutil.RunScript which accepts testing.TB.
func cmdRunB(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkHeadTenLines measures head -n 10 on a 10MB file of short lines.
func BenchmarkHeadTenLines(b *testing.B) {
	dir := b.TempDir()
	createLargeFile(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunB(b, "head -n 10 input.txt", dir)
	}
}

// BenchmarkHeadBytes measures head -c 1024 on a 10MB file.
func BenchmarkHeadBytes(b *testing.B) {
	dir := b.TempDir()
	createLargeFile(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunB(b, "head -c 1024 input.txt", dir)
	}
}

// BenchmarkHeadSingleLineNearCap measures head -n 1 on a file with one line
// just below MaxLineBytes (1MiB). Lines exceeding MaxLineBytes trigger an
// error path; this benchmark exercises the successful large-line path.
func BenchmarkHeadSingleLineNearCap(b *testing.B) {
	dir := b.TempDir()
	// 900KB line -- safely below MaxLineBytes (1MiB) so head succeeds.
	createLargeFile(b, dir, "input.txt", "x", 900<<10)
	// Append a newline to complete the line.
	f, err := os.OpenFile(filepath.Join(dir, "input.txt"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			b.Errorf("close input.txt: %v", err)
		}
	}()
	if _, err := f.WriteString("\n"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunB(b, "head -n 1 input.txt", dir)
	}
}

// TestHeadMemoryBoundedLines asserts that head -n 10 uses O(1) memory
// regardless of input file size.
func TestHeadMemoryBoundedLines(t *testing.T) {
	dir := t.TempDir()
	createLargeFile(t, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cmdRunB(b, "head -n 10 input.txt", dir)
		}
	})

	const maxBytesPerOp = 1 << 20 // 1MB ceiling
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("head -n 10 allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}

// TestHeadMemoryBoundedBytes asserts that head -c 1024 uses O(1) memory.
func TestHeadMemoryBoundedBytes(t *testing.T) {
	dir := t.TempDir()
	createLargeFile(t, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cmdRunB(b, "head -c 1024 input.txt", dir)
		}
	})

	const maxBytesPerOp = 1 << 20 // 1MB ceiling
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("head -c 1024 allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}
