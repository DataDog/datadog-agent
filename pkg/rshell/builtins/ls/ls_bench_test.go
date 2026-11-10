// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package ls_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// createFileDir creates a directory containing n empty files named
// file0000.txt … fileNNNN.txt and returns the directory path.
func createFileDir(tb testing.TB, n int) string {
	tb.Helper()
	dir := tb.TempDir()
	for i := range n {
		name := filepath.Join(dir, fmt.Sprintf("file%04d.txt", i))
		f, err := os.Create(name)
		if err != nil {
			tb.Fatal(err)
		}
		f.Close()
	}
	return dir
}

func cmdRunBLs(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkLs measures ls on a directory with 1000 entries.
func BenchmarkLs(b *testing.B) {
	dir := createFileDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBLs(b, "ls .", dir)
	}
}

// BenchmarkLsLong measures ls -l on a directory with 1000 entries.
func BenchmarkLsLong(b *testing.B) {
	dir := createFileDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBLs(b, "ls -l .", dir)
	}
}

// BenchmarkLsSmallDir measures ls on a small directory (10 entries).
func BenchmarkLsSmallDir(b *testing.B) {
	dir := createFileDir(b, 10)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBLs(b, "ls .", dir)
	}
}

// TestLsMemoryBounded asserts that ls allocation scales linearly with the
// number of directory entries rather than diverging to pathological levels.
// ls must load all directory entries into memory to sort them (O(n) live heap),
// but should not buffer additional data beyond what os.ReadDir returns.
//
// With 1000 entries of ~12-byte names the expected allocation is roughly
// 1000 × (name string + FileInfo struct) ≈ a few hundred KB. A 10MB ceiling
// catches regressions that accidentally buffer full file contents or loop
// without bound.
func TestLsMemoryBounded(t *testing.T) {
	dir := createFileDir(t, 1000)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cmdRunBLs(b, "ls .", dir)
		}
	})

	const maxBytesPerOp = 10 << 20 // 10MB ceiling for 1000-entry directory
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("ls allocated %d bytes/op on 1000-entry dir; want < %d", bpo, maxBytesPerOp)
	}
}
