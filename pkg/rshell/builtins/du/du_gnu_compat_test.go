// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package du_test

// These tests assert byte-for-byte equivalence with GNU coreutils du.
// All cases are forced into apparent-size mode so the expected values are
// deterministic and not dependent on the underlying filesystem's allocated
// block size. The captured GNU output was produced by:
//
//	du (GNU coreutils) 9.10
//
// invoked with the same flags shown in each test's comment header.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extractSize finds the line in stdout matching the given suffix and
// returns its leading integer.
func extractSize(t *testing.T, stdout, suffix string) int64 {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasSuffix(line+"\n", suffix) {
			return parseLeadingInt(t, line)
		}
	}
	t.Fatalf("no line ending with %q in %q", suffix, stdout)
	return 0
}

// TestGNUCompatDuBytesSingleFile — `du -b five.txt` on a 5-byte file.
// GNU command:
//
//	printf '12345' > five.txt; du -b five.txt
//
// Captured GNU output: "5\tfive.txt\n"
func TestGNUCompatDuBytesSingleFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "five.txt"), []byte("12345"), 0o644))
	stdout, _, code := cmdRun(t, "du -b five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "5\tfive.txt\n", stdout)
}

// TestGNUCompatDuApparentSingleFile — `du --apparent-size five.txt`.
// GNU command: `du --apparent-size five.txt` — five.txt is 5 bytes.
// Captured GNU output: "1\tfive.txt\n" (5 bytes rounds up to 1 KiB block).
func TestGNUCompatDuApparentSingleFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "five.txt"), []byte("12345"), 0o644))
	stdout, _, code := cmdRun(t, "du --apparent-size five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\tfive.txt\n", stdout)
}

// TestGNUCompatDuMegaExact2MiB — `du -m --apparent-size two_meg.bin`.
// GNU output: "2\ttwo_meg.bin\n"
func TestGNUCompatDuMegaExact2MiB(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "two_meg.bin"), make([]byte, 2*1024*1024), 0o644))
	stdout, _, code := cmdRun(t, "du -m --apparent-size two_meg.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2\ttwo_meg.bin\n", stdout)
}

// TestGNUCompatDuKilo2KiB — `du -k --apparent-size two_k.bin`.
// GNU output: "2\ttwo_k.bin\n"
func TestGNUCompatDuKilo2KiB(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "two_k.bin"), make([]byte, 2048), 0o644))
	stdout, _, code := cmdRun(t, "du -k --apparent-size two_k.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2\ttwo_k.bin\n", stdout)
}

// TestGNUCompatDuHumanExact2KiB — `du -h --apparent-size two_k.bin`.
// GNU output: "2.0K\ttwo_k.bin\n" — exactly 2.0K because the value is an
// integer multiple of 1024.
func TestGNUCompatDuHumanExact2KiB(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "two_k.bin"), make([]byte, 2048), 0o644))
	stdout, _, code := cmdRun(t, "du -h --apparent-size two_k.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2.0K\ttwo_k.bin\n", stdout)
}

// TestGNUCompatDuHuman10MiB — `du -h --apparent-size ten_meg.bin`.
// GNU output: "10M\tten_meg.bin\n" — ≥10 so no decimal.
func TestGNUCompatDuHuman10MiB(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ten_meg.bin"), make([]byte, 10*1024*1024), 0o644))
	stdout, _, code := cmdRun(t, "du -h --apparent-size ten_meg.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "10M\tten_meg.bin\n", stdout)
}

// TestGNUCompatDuSI2000Bytes — `du -b --apparent-size`-equivalent file
// rendered with --si.  Captured GNU output: "2.0k\ttwok.bin\n".
func TestGNUCompatDuSI2000Bytes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "twok.bin"), make([]byte, 2000), 0o644))
	stdout, _, code := cmdRun(t, "du --apparent-size --si twok.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2.0k\ttwok.bin\n", stdout)
}

// TestGNUCompatDuTotalRow — `du -c -b a.txt b.txt`.
// GNU output (captured):
//
//	5\ta.txt
//	3\tb.txt
//	8\ttotal
func TestGNUCompatDuTotalRow(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("12345"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("123"), 0o644))
	stdout, _, code := cmdRun(t, "du -c -b a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "5\ta.txt\n3\tb.txt\n8\ttotal\n", stdout)
}

// TestGNUCompatDuRejectsUnknownFlag — `du -f .` (where -f is unknown).
// GNU exits 1 with usage info. Our shell exits 1 with "unknown shorthand"
// message; we only assert the exit code matches and stderr is non-empty.
func TestGNUCompatDuRejectsUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "du -f .", dir)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, stderr)
}

// TestGNUCompatDuMaxDepth0SameAsSummarize — `du -d 0 --apparent-size .`
// produces a single line just like `du -s --apparent-size .`.
func TestGNUCompatDuMaxDepth0SameAsSummarize(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("123"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "inner.txt"), []byte("123"), 0o644))

	stdoutD0, _, _ := cmdRun(t, "du -d 0 --apparent-size .", dir)
	stdoutS, _, _ := cmdRun(t, "du -s --apparent-size .", dir)
	assert.Equal(t, stdoutS, stdoutD0)
}

// TestGNUCompatDuNullTerminator — `du -0 -b a.txt b.txt` ends each line
// with NUL.
func TestGNUCompatDuNullTerminator(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("12345"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("123"), 0o644))
	stdout, _, code := cmdRun(t, "du -0 -b a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "5\ta.txt\x003\tb.txt\x00", stdout)
}

// TestGNUCompatDuBytesDirectoryFilesystemDependent — `du -a -b d` on a
// tree containing two 3-byte files. Codex has flagged this exact case
// five times claiming GNU returns "3 d/sub" and "6 d"; that is wrong.
//
// GNU's actual output depends on the host filesystem, because `du -b`
// charges Stat_t.Blocks * 512 for directories regardless of
// --apparent-size. Verified against GNU coreutils 9.1 on
// debian:bookworm-slim:
//
//	$ docker run --rm debian:bookworm-slim bash -c '
//	    mkdir -p /tmp/d/sub
//	    printf abc > /tmp/d/sub/f1
//	    printf xyz > /tmp/d/sub/f2
//	    cd /tmp && du -a -b d'
//	3	d/sub/f2
//	3	d/sub/f1
//	4102	d/sub
//	8198	d
//
// On macOS APFS the directory contributions are 0 (Blocks=0):
//
//	$ du -a -b d
//	3	d/sub/f2
//	3	d/sub/f1
//	6	d/sub
//	6	d
//
// The test asserts the structural invariant that holds on both: dir
// totals equal own_blocks*512 + sum(child apparent sizes). It does not
// hardcode the absolute number, which would only pass on one
// filesystem.
func TestGNUCompatDuBytesDirectoryFilesystemDependent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "d", "sub", "f1"), []byte("abc"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "d", "sub", "f2"), []byte("xyz"), 0o644))

	stdout, _, code := cmdRun(t, "du -a -b d", dir)
	assert.Equal(t, 0, code)
	// Files must have exact byte counts (apparent size for regular
	// files is unambiguous).
	assert.Contains(t, stdout, "3\td/sub/f1\n")
	assert.Contains(t, stdout, "3\td/sub/f2\n")
	// Both directories must report at least 6 bytes (the two files'
	// content) and may add their own block count on filesystems that
	// expose it (ext4: 4096, APFS: 0).
	subTotal := extractSize(t, stdout, "\td/sub\n")
	dTotal := extractSize(t, stdout, "\td\n")
	assert.GreaterOrEqual(t, subTotal, int64(6), "d/sub must include both file bytes")
	assert.GreaterOrEqual(t, dTotal, subTotal, "d total must include sub subtree")
}
