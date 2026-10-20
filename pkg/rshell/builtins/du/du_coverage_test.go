// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package du_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Hardlink dedup ---

// TestDuDedupsHardlinks confirms that two hardlinks to the same inode are
// counted only once when both appear in the same du invocation.
func TestDuDedupsHardlinks(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "primary.bin")
	require.NoError(t, os.WriteFile(primary, make([]byte, 4096), 0o644))
	require.NoError(t, os.Link(primary, filepath.Join(dir, "alias.bin")))

	stdout, _, code := cmdRun(t, "du -c -b primary.bin alias.bin", dir)
	assert.Equal(t, 0, code)
	// GNU du silently drops the second link from output and the grand
	// total when a hardlinked inode has already been counted in this
	// invocation. Confirmed against `du (GNU coreutils) 9.10`.
	assert.Equal(t, "4096\tprimary.bin\n4096\ttotal\n", stdout)
}

// TestDuDedupsSymlinkAliasesUnderL confirms that two symlinks pointing
// at the same regular file (nlink=1 on the target) are deduplicated
// under -L, matching GNU's default behaviour. Regression for the
// `nlink > 1` gate that previously prevented dedup of nlink=1 inodes.
func TestDuDedupsSymlinkAliasesUnderL(t *testing.T) {
	if !canSymlink() {
		t.Skip("symlinks unavailable")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), []byte("abc"), 0o644))
	require.NoError(t, os.Symlink("target", filepath.Join(dir, "l1")))
	require.NoError(t, os.Symlink("target", filepath.Join(dir, "l2")))

	stdout, _, code := cmdRun(t, "du -L -b l1 l2", dir)
	assert.Equal(t, 0, code)
	// l1 emitted once; l2 silently dropped because the target inode
	// has already been counted.
	assert.Equal(t, "3\tl1\n", stdout)
}

// TestDuDedupsRepeatedDirectoryOperands guards against the bug where
// the visited-inode map only tracked regular files, so passing the
// same directory twice (or two -L symlinks pointing at the same
// directory) emitted both with their own block contributions. GNU
// suppresses the second occurrence by default.
func TestDuDedupsRepeatedDirectoryOperands(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("abc"), 0o644))

	stdout, _, code := cmdRun(t, "du -b -c . .", dir)
	assert.Equal(t, 0, code)
	// Exactly two lines: one operand line and the total. The duplicate
	// operand must be silently dropped from both the per-operand emit
	// and the grand total.
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.Len(t, lines, 2, "expected dir dedup; got %q", stdout)
	// Assert structural invariants (which line is the total) before
	// numeric parsing so an unexpected ordering surfaces a clear message
	// rather than parseLeadingInt fataling on the literal "total".
	require.True(t, strings.HasSuffix(lines[1], "\ttotal"), "line 1 must be the grand total: %q", stdout)
	require.False(t, strings.HasSuffix(lines[0], "\ttotal"), "line 0 must be the operand, not the total: %q", stdout)
	operand := parseLeadingInt(t, lines[0])
	total := parseLeadingInt(t, lines[1])
	assert.Equal(t, operand, total, "grand total must equal single-operand value, not 2x")
}

// TestDuSummarizeRejectsNegativeDepth ensures `du -s -d -1` is
// rejected. The earlier validation order applied the -s/--max-depth=0
// equivalence first, which overwrote the negative value before the
// negative-depth check could run.
func TestDuSummarizeRejectsNegativeDepth(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "du -s -d -1 .", dir)
	assert.Equal(t, 1, code, "du -s -d -1 must exit 1")
	assert.Contains(t, stderr, "invalid maximum depth")
}

// TestDuSeparateDirsGrandTotalIncludesSubtrees regression-tests the
// case where `-S -c` was using the parent's printed value (which
// excludes subdirectory contributions) as the grand-total summand,
// underreporting by exactly the subdirectory subtree size.
func TestDuSeparateDirsGrandTotalIncludesSubtrees(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "p", "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p", "direct"), []byte("xyz"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p", "sub", "deep"), []byte("abc"), 0o644))

	stdout, _, code := cmdRun(t, "du -S -b -c p", dir)
	assert.Equal(t, 0, code)
	// Three lines: p/sub, p, total. Each uses bytes mode so file
	// contributions are exact (3 each). Directory inode bytes vary by
	// filesystem (APFS=0, ext4=4096), so assert structurally rather than
	// numerically: the total must equal p_subtree + sub_subtree.
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.Len(t, lines, 3, "expected 3 lines: %q", stdout)
	pSub := parseLeadingInt(t, lines[0])
	pSep := parseLeadingInt(t, lines[1])
	totalSep := parseLeadingInt(t, lines[2])
	assert.Equal(t, pSub+pSep, totalSep, "GNU -c sums all printed entries")
	assert.True(t, strings.HasSuffix(lines[2], "\ttotal"))
}

// --- Symlink-loop detection under -L ---

func TestDuDetectsSymlinkLoopWithL(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a"), 0o755))
	// b -> a creates a loop when followed.
	require.NoError(t, os.Symlink("..", filepath.Join(dir, "a", "loop")))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, stderr, code := cmdRunCtx(ctx, t, "du -L .", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "File system loop detected")
}

// TestDuFifoOperandDoesNotBlock ensures du treats FIFOs as metadata-only leaf
// entries. Opening a FIFO with no writer would block, so this guards the
// special-file invariant that du must use Stat/Lstat rather than OpenFile for
// non-directory operands.
func TestDuFifoOperandDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stdout, stderr, code := cmdRunCtx(ctx, t, "du -b pipe", dir)
	require.NoError(t, ctx.Err())
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "0\tpipe\n", stdout)
}

// --- humanSize edge values ---

// 1023 bytes is below the 1KiB threshold; -h prints raw.
func TestDuHumanSubKBytes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tiny.bin"), make([]byte, 700), 0o644))
	stdout, _, code := cmdRun(t, "du -h --apparent-size tiny.bin", dir)
	assert.Equal(t, 0, code)
	// 700 bytes < 1024 → "700".
	assert.Equal(t, "700\ttiny.bin\n", stdout)
}

// 9 GiB rendered as 9.0G (one decimal because <10).
func TestDuHumanGigabytes(t *testing.T) {
	// We cannot allocate 9 GiB of zero-filled bytes in the testing process,
	// so synthesise the file via Truncate (sparse).
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "big.bin"))
	require.NoError(t, err)
	require.NoError(t, f.Truncate(9*1024*1024*1024))
	require.NoError(t, f.Close())
	stdout, _, code := cmdRun(t, "du -h --apparent-size big.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "9.0G\tbig.bin\n", stdout)
}

// --- joinPath edge cases via emitted output ---

// When an operand ends with '/', the trailing slash is preserved in output
// because joinPath only adds a separator when the dir part doesn't already
// end with one.
func TestDuPreservesTrailingSlashInOperand(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "f"), []byte("x"), 0o644))

	stdout, _, code := cmdRun(t, "du -a -b sub/", dir)
	assert.Equal(t, 0, code)
	// "sub/f" — joinPath("sub/", "f") should produce "sub/f" not "sub//f".
	assert.Contains(t, stdout, "sub/f\n")
	assert.NotContains(t, stdout, "sub//f")
}

// --- Mega/SI rounding ---

// `--si` formats 1500 bytes as "1.5k" because 1500 / 1000 = 1.5 and < 9.95.
func TestDuSI1500Bytes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.bin"), make([]byte, 1500), 0o644))
	stdout, _, code := cmdRun(t, "du --apparent-size --si f.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1.5k\tf.bin\n", stdout)
}
