// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package cat_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// createLargeFileCat writes totalBytes of repeating content to dir/filename.
func createLargeFileCat(tb testing.TB, dir, filename, line string, totalBytes int) string {
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

// cmdRunBCat runs a cat command with AllowedPaths set to dir (bench variant).
func cmdRunBCat(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkCat measures cat on a 1MB file.
func BenchmarkCat(b *testing.B) {
	dir := b.TempDir()
	createLargeFileCat(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 1<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBCat(b, "cat input.txt", dir)
	}
}

// BenchmarkCatNumbered measures cat -n on a 1MB file.
func BenchmarkCatNumbered(b *testing.B) {
	dir := b.TempDir()
	createLargeFileCat(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 1<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBCat(b, "cat -n input.txt", dir)
	}
}

// TestCatMemoryBounded asserts that cat uses O(1) memory regardless of input
// size. cat streams input to output in fixed chunks with no per-line allocation.
func TestCatMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	createLargeFileCat(t, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "cat input.txt", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	const maxBytesPerOp = 4 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("cat allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}

func BenchmarkCatDiscard(b *testing.B) {
	dir := b.TempDir()
	createLargeFileCat(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		testutil.RunScriptDiscard(b, "cat input.txt", dir, interp.AllowedPaths([]string{dir}))
	}
}
