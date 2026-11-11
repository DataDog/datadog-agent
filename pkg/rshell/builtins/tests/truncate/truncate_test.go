// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package truncate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// --- Happy-path: shrink and extend ---

func TestTruncateShrink(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "hello world")
	_, _, code := truncateRun(t, "truncate -s 5 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(5), fileSize(t, path))
}

func TestTruncateExtend(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "hi")
	_, _, code := truncateRun(t, "truncate -s 10 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(10), fileSize(t, path))
}

func TestTruncateZero(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "hello world")
	_, _, code := truncateRun(t, "truncate -s 0 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(0), fileSize(t, path))
}

func TestTruncateLongForm(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "hello world")
	_, _, code := truncateRun(t, "truncate --size=5 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(5), fileSize(t, path))
}

func TestTruncateCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, _, code := truncateRun(t, "truncate -s 100 newfile.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(100), fileSize(t, filepath.Join(dir, "newfile.txt")))
}

func TestTruncateMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFile(t, dir, "a.txt", "hello world")
	p2 := writeFile(t, dir, "b.txt", "goodbye world")
	_, _, code := truncateRun(t, "truncate -s 3 a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(3), fileSize(t, p1))
	assert.Equal(t, int64(3), fileSize(t, p2))
}

// --- Size suffixes ---

func TestTruncateSuffixK(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "")
	_, _, code := truncateRun(t, "truncate -s 1K file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(1024), fileSize(t, path))
}

func TestTruncateSuffixKB(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "")
	_, _, code := truncateRun(t, "truncate -s 1KB file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(1000), fileSize(t, path))
}

func TestTruncateSuffixKiB(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "")
	_, _, code := truncateRun(t, "truncate -s 2KiB file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(2048), fileSize(t, path))
}

func TestTruncateSuffixM(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "")
	_, _, code := truncateRun(t, "truncate -s 1M file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(1<<20), fileSize(t, path))
}

func TestTruncateSuffixLowerK(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "")
	_, _, code := truncateRun(t, "truncate -s 1k file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, int64(1024), fileSize(t, path))
}

// --- -c / --no-create ---

func TestTruncateNoCreate(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := truncateRun(t, "truncate -c -s 100 missing.txt", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	_, err := os.Stat(filepath.Join(dir, "missing.txt"))
	assert.True(t, os.IsNotExist(err), "file should not be created")
}

func TestTruncateNoCreateLongForm(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := truncateRun(t, "truncate --no-create -s 100 missing.txt", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
}

// --- --help ---

func TestTruncateHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := truncateRun(t, "truncate --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: truncate")
	assert.Contains(t, stdout, "--size")
	assert.Contains(t, stdout, "--no-create")
}

// --- Error cases ---

// TestTruncateMissingFile verifies that truncate creates a missing file by
// default (matching GNU truncate: -c is needed to suppress creation).
func TestTruncateMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := truncateRun(t, "truncate -s 5 missing.txt", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, int64(5), fileSize(t, filepath.Join(dir, "missing.txt")))
}

func TestTruncateMissingSize(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := truncateRun(t, "truncate file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--size")
}

func TestTruncateMissingFileOperand(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := truncateRun(t, "truncate -s 5", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing file operand")
}

func TestTruncateInvalidSize(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := truncateRun(t, "truncate -s abc file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid size")
}

func TestTruncateRelativeSizePlus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello")
	_, stderr, code := truncateRun(t, "truncate -s +10 file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "relative size operators not supported")
}

func TestTruncateRelativeSizeMinus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello")
	_, stderr, code := truncateRun(t, "truncate -s -10 file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "relative size operators not supported")
}

func TestTruncateDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))
	_, stderr, code := truncateRun(t, "truncate -s 0 subdir", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "subdir")
}

func TestTruncateSandboxBlocked(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	writeFile(t, other, "secret.txt", "secret data")
	// sandbox allows only dir, not other
	_, stderr, code := runScript(t, "truncate -s 0 "+filepath.Join(other, "secret.txt"), dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "secret.txt")
}

func TestTruncateReadOnlyMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello world")
	// No WithMode(ModeRemediation) — Truncate will be nil
	_, stderr, code := runScript(t, "truncate -s 5 file.txt", dir,
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "truncate")
}

// TestTruncatePartialFailure verifies that truncate processes all operands
// even after one fails, returning exit 1 at the end.
func TestTruncatePartialFailure(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))
	good := writeFile(t, dir, "good.txt", "hello world")
	stdout, stderr, code := truncateRun(t, "truncate -s 5 good.txt subdir", dir)
	assert.Equal(t, 1, code, "should fail because subdir is not a regular file")
	assert.Contains(t, stderr, "subdir")
	assert.Empty(t, stdout)
	assert.Equal(t, int64(5), fileSize(t, good), "good.txt should still be truncated")
}

func TestTruncateUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := truncateRun(t, "truncate --reference=other file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unrecognized option")
}
