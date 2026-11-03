// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package cut_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func createLargeFileCut(tb testing.TB, dir, filename, line string, totalBytes int) string {
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

func cmdRunBCut(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkCutBytes measures cut -b 1-10 on a 10MB file of short lines.
func BenchmarkCutBytes(b *testing.B) {
	dir := b.TempDir()
	createLargeFileCut(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBCut(b, "cut -b 1-10 input.txt", dir)
	}
}

// BenchmarkCutFields measures cut -f 1 -d ' ' on a 10MB file of short lines.
func BenchmarkCutFields(b *testing.B) {
	dir := b.TempDir()
	// Tab-delimited: "field1\tfield2\tfield3"
	createLargeFileCut(b, dir, "input.txt", "alpha\tbeta\tgamma\tdelta\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBCut(b, "cut -f 1 input.txt", dir)
	}
}

// BenchmarkCutFieldsMultiple measures cut selecting multiple fields on a 10MB file.
func BenchmarkCutFieldsMultiple(b *testing.B) {
	dir := b.TempDir()
	createLargeFileCut(b, dir, "input.txt", "alpha\tbeta\tgamma\tdelta\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBCut(b, "cut -f 1,3 input.txt", dir)
	}
}

// TestCutMemoryBounded asserts that cut -b uses O(1) memory regardless of
// input size. cut is a streaming command that writes selected byte ranges
// directly to Stdout with no per-line string allocation.
func TestCutMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	createLargeFileCut(t, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "cut -b 1-10 input.txt", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	const maxBytesPerOp = 4 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("cut -b 1-10 allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}

// TestCutFieldsMemoryBounded asserts that cut -f uses O(1) memory regardless
// of input size. Field mode scans raw bytes for the delimiter without
// converting to string or allocating a []string per line.
func TestCutFieldsMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	createLargeFileCut(t, dir, "input.txt", "alpha\tbeta\tgamma\tdelta\n", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "cut -f 1 input.txt", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	const maxBytesPerOp = 4 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("cut -f 1 allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}

func BenchmarkCutBytesDiscard(b *testing.B) {
	dir := b.TempDir()
	createLargeFileCut(b, dir, "input.txt", "the quick brown fox jumps over the lazy dog\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		testutil.RunScriptDiscard(b, "cut -b 1-10 input.txt", dir, interp.AllowedPaths([]string{dir}))
	}
}

func BenchmarkCutFieldsDiscard(b *testing.B) {
	dir := b.TempDir()
	createLargeFileCut(b, dir, "input.txt", "alpha\tbeta\tgamma\tdelta\n", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		testutil.RunScriptDiscard(b, "cut -f 1 input.txt", dir, interp.AllowedPaths([]string{dir}))
	}
}
