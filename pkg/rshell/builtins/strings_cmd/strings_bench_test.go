// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package strings_cmd_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func createLargeFileStrings(tb testing.TB, dir, filename, line string, totalBytes int) string {
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

func cmdRunBStrings(b *testing.B, script, dir string) (string, string, int) {
	b.Helper()
	return testutil.RunScript(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkStrings measures strings on a 1MB file containing many short
// printable sequences separated by null bytes. Each line is a 43-byte printable
// string followed by a null byte, producing ~24k strings.
func BenchmarkStrings(b *testing.B) {
	dir := b.TempDir()
	// Mix of printable chars + null byte so strings emits many short tokens.
	createLargeFileStrings(b, dir, "input.bin", "the quick brown fox jumps over lazy\x00", 1<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBStrings(b, "strings input.bin", dir)
	}
}

// BenchmarkStringsPrintableOnly measures strings on a 1MB fully-printable file.
// The entire file is one continuous printable run that exceeds maxStringLen
// (1 MiB cap), so only the first 1 MiB is emitted.
func BenchmarkStringsPrintableOnly(b *testing.B) {
	dir := b.TempDir()
	createLargeFileStrings(b, dir, "input.txt", "abcdefghij", 1<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		cmdRunBStrings(b, "strings input.txt", dir)
	}
}

// TestStringsMemoryBounded asserts that strings uses O(1) memory regardless
// of input size. strings reads in 32 KiB chunks and caps individual string
// accumulation at maxStringLen (1 MiB). With short printable sequences
// separated by non-printable bytes the current string buffer stays small.
func TestStringsMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	createLargeFileStrings(t, dir, "input.bin", "the quick brown fox jumps over lazy\x00", 10<<20)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			testutil.RunScriptDiscard(b, "strings input.bin", dir, interp.AllowedPaths([]string{dir}))
		}
	})

	const maxBytesPerOp = 4 << 20
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("strings allocated %d bytes/op on 10MB input; want < %d", bpo, maxBytesPerOp)
	}
}

func BenchmarkStringsDiscard(b *testing.B) {
	dir := b.TempDir()
	createLargeFileStrings(b, dir, "input.bin", "the quick brown fox jumps over lazy\x00", 10<<20)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		testutil.RunScriptDiscard(b, "strings input.bin", dir, interp.AllowedPaths([]string{dir}))
	}
}
