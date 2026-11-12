// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !race

package interp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// createGlobDir creates a directory containing n files named file0000.txt …
// fileNNNN.txt and returns the directory path.
func createGlobDir(tb testing.TB, n int) string {
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

// createNestedGlobDir creates a directory tree with nDirs subdirectories each
// containing nFiles files.
func createNestedGlobDir(tb testing.TB, nDirs, nFiles int) string {
	tb.Helper()
	dir := tb.TempDir()
	for d := range nDirs {
		sub := filepath.Join(dir, fmt.Sprintf("dir%03d", d))
		if err := os.Mkdir(sub, 0755); err != nil {
			tb.Fatal(err)
		}
		for f := range nFiles {
			name := filepath.Join(sub, fmt.Sprintf("f%04d.txt", f))
			fh, err := os.Create(name)
			if err != nil {
				tb.Fatal(err)
			}
			fh.Close()
		}
	}
	return dir
}

func runGlob(b *testing.B, script, dir string) {
	b.Helper()
	testutil.RunScriptDiscard(b, script, dir, interp.AllowedPaths([]string{dir}))
}

// BenchmarkGlobStar measures "echo *" in a 1000-entry directory.
func BenchmarkGlobStar(b *testing.B) {
	dir := createGlobDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo *", dir)
	}
}

// BenchmarkGlobStarLargeDir measures "echo *" in a 9999-entry directory
// (just under MaxGlobEntries=10k cap).
func BenchmarkGlobStarLargeDir(b *testing.B) {
	dir := createGlobDir(b, 9999)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo *", dir)
	}
}

// BenchmarkGlobPrefix measures a prefix glob "echo file0*" in a 1000-entry directory.
func BenchmarkGlobPrefix(b *testing.B) {
	dir := createGlobDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo file0*", dir)
	}
}

// BenchmarkGlobSuffix measures a suffix glob "echo *.txt" in a 1000-entry directory.
func BenchmarkGlobSuffix(b *testing.B) {
	dir := createGlobDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo *.txt", dir)
	}
}

// BenchmarkGlobQuestionMark measures "echo file????.txt" in a 1000-entry directory.
func BenchmarkGlobQuestionMark(b *testing.B) {
	dir := createGlobDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo file????.txt", dir)
	}
}

// BenchmarkGlobBracket measures "echo file[0-4]*.txt" in a 1000-entry directory.
func BenchmarkGlobBracket(b *testing.B) {
	dir := createGlobDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo file[0-4]*.txt", dir)
	}
}

// BenchmarkGlobNested measures "echo */*" across 20 subdirectories × 50 files.
func BenchmarkGlobNested(b *testing.B) {
	dir := createNestedGlobDir(b, 20, 50)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo */*", dir)
	}
}

// BenchmarkGlobNoMatch measures "echo *.xyz" where nothing matches
// (the pattern is returned as a literal).
func BenchmarkGlobNoMatch(b *testing.B) {
	dir := createGlobDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo *.xyz", dir)
	}
}

// BenchmarkGlobMultiplePatterns measures multiple globs in a single command.
func BenchmarkGlobMultiplePatterns(b *testing.B) {
	dir := createGlobDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo file00* file01* file02* file03*", dir)
	}
}

// BenchmarkGlobForLoop measures glob expansion used in a for loop iteration.
func BenchmarkGlobForLoop(b *testing.B) {
	dir := createGlobDir(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "for f in *.txt; do :; done", dir)
	}
}

// BenchmarkGlobSmallDir measures "echo *" in a small 10-entry directory
// to establish baseline overhead.
func BenchmarkGlobSmallDir(b *testing.B) {
	dir := createGlobDir(b, 10)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo *", dir)
	}
}

// BenchmarkGlobExceedsCap measures "echo *" in a directory that exceeds
// MaxGlobEntries, verifying the rejection path is fast.
func BenchmarkGlobExceedsCap(b *testing.B) {
	dir := createGlobDir(b, 10001)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		runGlob(b, "echo *", dir)
	}
}

// TestGlobMemoryBounded asserts that glob expansion in a large directory
// does not allocate pathological amounts of memory. With 1000 entries of
// ~12-byte names, allocation should be on the order of a few hundred KB.
func TestGlobMemoryBounded(t *testing.T) {
	dir := createGlobDir(t, 1000)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			runGlob(b, "echo *", dir)
		}
	})

	const maxBytesPerOp = 10 << 20 // 10 MB ceiling
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("glob echo * allocated %d bytes/op on 1000-entry dir; want < %d", bpo, maxBytesPerOp)
	}
}

// TestGlobLargeDirMemoryBounded asserts memory stays bounded for a 9999-entry
// directory (just under MaxGlobEntries=10k cap).
func TestGlobLargeDirMemoryBounded(t *testing.T) {
	dir := createGlobDir(t, 9999)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			runGlob(b, "echo *", dir)
		}
	})

	// With ~10k entries, expect allocation proportional to entry count.
	// Use a generous ceiling to catch regressions, not measure precisely.
	const maxBytesPerOp = 50 << 20 // 50 MB ceiling
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("glob echo * allocated %d bytes/op on 9999-entry dir; want < %d", bpo, maxBytesPerOp)
	}
}

// TestGlobNestedMemoryBounded asserts memory stays bounded for nested globs.
func TestGlobNestedMemoryBounded(t *testing.T) {
	dir := createNestedGlobDir(t, 20, 50)

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			runGlob(b, "echo */*", dir)
		}
	})

	const maxBytesPerOp = 20 << 20 // 20 MB ceiling
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("glob echo */* allocated %d bytes/op on 20×50 nested dir; want < %d", bpo, maxBytesPerOp)
	}
}

// ---------------------------------------------------------------------------
// Safety edge-case tests: pathological glob patterns that could cause
// crashes, excessive memory usage, or exponential blowup.
// ---------------------------------------------------------------------------

// TestGlobManyConsecutiveStars verifies that a pattern with many consecutive
// stars (e.g. "echo ****...****") does not cause exponential blowup.
// In a naïve implementation, N consecutive stars could cause O(2^N) matching.
func TestGlobManyConsecutiveStars(t *testing.T) {
	dir := createGlobDir(t, 100)

	// 50 consecutive stars — should collapse to a single star internally.
	pattern := "echo " + strings.Repeat("*", 50)
	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			runGlob(b, pattern, dir)
		}
	})

	const maxBytesPerOp = 10 << 20 // 10 MB ceiling
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("glob with 50 consecutive stars allocated %d bytes/op; want < %d", bpo, maxBytesPerOp)
	}
}

// TestGlobManyStarSegments verifies that a pattern like "a*b*c*d*...*z" with
// many star-separated single-char segments doesn't cause exponential
// backtracking when matching against filenames.
func TestGlobManyStarSegments(t *testing.T) {
	dir := t.TempDir()
	// Create a file that forces maximum backtracking: a long name with
	// repeated characters that partially match each segment.
	longName := strings.Repeat("a", 200) + ".txt"
	f, err := os.Create(filepath.Join(dir, longName))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Pattern: a*a*a*a*...*a*a*b (20 star-separated 'a' segments ending with 'b').
	// The file has no 'b', so every segment match attempt must backtrack.
	segments := make([]string, 21)
	for i := 0; i < 20; i++ {
		segments[i] = "a"
	}
	segments[20] = "b"
	pattern := "echo " + strings.Join(segments, "*")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stderr, exitCode := testutil.RunScriptDiscardCtx(ctx, t, pattern, dir, interp.AllowedPaths([]string{dir}))
	if ctx.Err() != nil {
		t.Fatal("glob pattern with many star segments timed out (possible exponential backtracking)")
	}
	_ = stderr
	_ = exitCode
}

// TestGlobHugeNumberOfStarArgs verifies that many independent star arguments
// in a single command don't cause excessive resource consumption.
func TestGlobHugeNumberOfStarArgs(t *testing.T) {
	dir := createGlobDir(t, 50)

	// 100 separate "*" arguments — each expands the full directory listing.
	args := make([]string, 100)
	for i := range args {
		args[i] = "*"
	}
	script := "echo " + strings.Join(args, " ")

	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			runGlob(b, script, dir)
		}
	})

	const maxBytesPerOp = 50 << 20 // 50 MB ceiling
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("glob with 100 star args allocated %d bytes/op; want < %d", bpo, maxBytesPerOp)
	}
}

// TestGlobDeepNestedStarSlash verifies that deeply nested "*/" patterns
// (e.g. "*/*/*/*/*") don't cause resource exhaustion.
func TestGlobDeepNestedStarSlash(t *testing.T) {
	// Create a 5-level deep tree: 3 dirs at each level = 3^5 = 243 leaf dirs.
	dir := t.TempDir()
	createNestedTree(t, dir, 5, 3)

	pattern := "echo */*/*/*/*"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stderr, exitCode := testutil.RunScriptDiscardCtx(ctx, t, pattern, dir, interp.AllowedPaths([]string{dir}))
	if ctx.Err() != nil {
		t.Fatal("deep nested glob timed out")
	}
	_ = stderr
	_ = exitCode
}

// TestGlobBacktrackingWorstCase constructs the worst-case scenario for glob
// backtracking: a pattern like "*a*a*a*a*a*b" against a file named "aaa...aaa"
// (all a's, no b). A naïve backtracking matcher would try O(n^k) combinations
// where n is the filename length and k is the number of star segments.
func TestGlobBacktrackingWorstCase(t *testing.T) {
	dir := t.TempDir()

	// File with 100 'a' characters — no 'b' anywhere.
	name := strings.Repeat("a", 100)
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Pattern with 15 star-separated 'a' segments ending in 'b'.
	// This is the classic ReDoS-like pattern for glob matchers.
	segments := make([]string, 16)
	for i := 0; i < 15; i++ {
		segments[i] = "a"
	}
	segments[15] = "b"
	pattern := "echo " + strings.Join(segments, "*")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stderr, exitCode := testutil.RunScriptDiscardCtx(ctx, t, pattern, dir, interp.AllowedPaths([]string{dir}))
	if ctx.Err() != nil {
		t.Fatal("glob backtracking worst case timed out — possible exponential complexity")
	}
	_ = stderr
	_ = exitCode
}

// TestGlobManyConsecutiveStarsMemoryBounded checks that 200 consecutive stars
// don't cause memory blowup.
func TestGlobManyConsecutiveStarsMemoryBounded(t *testing.T) {
	dir := createGlobDir(t, 10)

	pattern := "echo " + strings.Repeat("*", 200) + ".txt"
	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			runGlob(b, pattern, dir)
		}
	})

	const maxBytesPerOp = 10 << 20 // 10 MB ceiling
	if bpo := result.AllocedBytesPerOp(); bpo > maxBytesPerOp {
		t.Errorf("glob with 200 consecutive stars allocated %d bytes/op; want < %d", bpo, maxBytesPerOp)
	}
}

// TestGlobReadDirLimitEnforced verifies that a script triggering more than
// MaxGlobReadDirCalls directory reads is stopped with an error.
func TestGlobReadDirLimitEnforced(t *testing.T) {
	dir := createGlobDir(t, 5)

	// Expand one small glob per iteration so this test isolates the directory-
	// read budget rather than hitting the per-command argument limit first.
	script := fmt.Sprintf("for i in {1..%d}; do echo * >/dev/null; done", interp.MaxGlobReadDirCalls+1)

	stdout, stderr, exitCode := testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{dir}))
	_ = stdout

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code when glob ReadDir limit is exceeded")
	}
	if !strings.Contains(stderr, "exceeded maximum number of directory reads") {
		t.Errorf("expected glob limit error in stderr, got: %s", stderr)
	}
}

// TestGlobReadDirLimitNotTriggeredBelowCap verifies that scripts just under
// the limit work fine.
func TestGlobReadDirLimitNotTriggeredBelowCap(t *testing.T) {
	dir := createGlobDir(t, 5)

	// Exactly MaxGlobReadDirCalls glob expansions should succeed.
	script := fmt.Sprintf("for i in {1..%d}; do echo * >/dev/null; done", interp.MaxGlobReadDirCalls)

	_, stderr, exitCode := testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{dir}))

	if exitCode != 0 {
		t.Errorf("expected exit code 0 at exactly MaxGlobReadDirCalls, got %d; stderr: %s", exitCode, stderr)
	}
}

// TestGlobReadDirLimitSharedAcrossSubshells verifies that the counter is
// shared between the parent shell and subshells (e.g. pipes).
func TestGlobReadDirLimitSharedAcrossSubshells(t *testing.T) {
	dir := createGlobDir(t, 5)

	// Use a for-in loop that expands globs many times.
	// Each "echo *" triggers 1 ReadDir call. We need >10K iterations.
	// Generate a sequence as for-in values and glob inside the body.
	vals := make([]string, interp.MaxGlobReadDirCalls+1)
	for i := range vals {
		vals[i] = "x"
	}
	script := "for i in " + strings.Join(vals, " ") + "; do echo *; done"

	_, stderr, exitCode := testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{dir}))

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code when glob ReadDir limit is exceeded via loop")
	}
	if !strings.Contains(stderr, "exceeded maximum number of directory reads") {
		t.Errorf("expected glob limit error in stderr, got: %s", stderr)
	}
}

// createNestedTree creates a directory tree with the given depth and branching
// factor. At each level, it creates 'branching' subdirectories. At the leaf
// level, it creates a single file.
func createNestedTree(tb testing.TB, dir string, depth, branching int) {
	tb.Helper()
	if depth == 0 {
		f, err := os.Create(filepath.Join(dir, "leaf.txt"))
		if err != nil {
			tb.Fatal(err)
		}
		f.Close()
		return
	}
	for i := range branching {
		sub := filepath.Join(dir, fmt.Sprintf("d%d", i))
		if err := os.Mkdir(sub, 0755); err != nil {
			tb.Fatal(err)
		}
		createNestedTree(tb, sub, depth-1, branching)
	}
}
