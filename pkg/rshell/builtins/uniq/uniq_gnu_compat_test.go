// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// GNU compatibility tests for the uniq builtin.
//
// Expected outputs were captured from GNU coreutils uniq 9.6
// and are embedded as string literals so the tests run without any GNU
// tooling present on CI.

package uniq_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGNUCompatEmptyInput — empty input produces empty output.
//
// GNU command: printf ” | guniq
// Expected:    ""
func TestGNUCompatEmptyInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "uniq empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatAdjacentDuplicates — adjacent duplicates collapsed.
//
// GNU command: printf 'a\na\n' | guniq
// Expected:    "a\n"
func TestGNUCompatAdjacentDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

// TestGNUCompatNoTrailingNewline — last line without newline gets one added.
//
// GNU command: printf 'a\na' | guniq
// Expected:    "a\n"
func TestGNUCompatNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

// TestGNUCompatDifferentLines — different lines both preserved.
//
// GNU command: printf 'a\nb' | guniq
// Expected:    "a\nb\n"
func TestGNUCompatDifferentLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

// TestGNUCompatCountBasic — -c formats count with 7-char right-aligned field.
//
// GNU command: printf 'a\nb\n' | guniq -c
// Expected:    "      1 a\n      1 b\n"
func TestGNUCompatCountBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\n")
	stdout, _, code := cmdRun(t, "uniq -c in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "      1 a\n      1 b\n", stdout)
}

// TestGNUCompatCountDuplicates — -c with repeated lines.
//
// GNU command: printf 'a\na\n' | guniq -c
// Expected:    "      2 a\n"
func TestGNUCompatCountDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq -c in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "      2 a\n", stdout)
}

// TestGNUCompatIgnoreCase — -i folds case.
//
// GNU command: printf 'A\na\n' | guniq -i
// Expected:    "A\n"
func TestGNUCompatIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "A\na\n")
	stdout, _, code := cmdRun(t, "uniq -i in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

// TestGNUCompatAllRepeatedSeparate — --all-repeated=separate with two groups.
//
// GNU command: printf 'a\na\nb\nc\nc\n' | guniq --all-repeated=separate
// Expected:    "a\na\n\nc\nc\n"
func TestGNUCompatAllRepeatedSeparate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\nc\nc\n")
	stdout, _, code := cmdRun(t, "uniq --all-repeated=separate in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\nc\nc\n", stdout)
}

// TestGNUCompatAllRepeatedPrepend — --all-repeated=prepend prefixes first group.
//
// GNU command: printf 'a\na\nb\nc\nc\n' | guniq --all-repeated=prepend
// Expected:    "\na\na\n\nc\nc\n"
func TestGNUCompatAllRepeatedPrepend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\nc\nc\n")
	stdout, _, code := cmdRun(t, "uniq --all-repeated=prepend in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\na\n\nc\nc\n", stdout)
}

// TestGNUCompatGroupSeparate — --group=separate with two groups.
//
// GNU command: printf 'a\na\nb\n' | guniq --group=separate
// Expected:    "a\na\n\nb\n"
func TestGNUCompatGroupSeparate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group=separate in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\nb\n", stdout)
}

// TestGNUCompatGroupPrepend — --group=prepend with two groups.
//
// GNU command: printf 'a\na\nb\n' | guniq --group=prepend
// Expected:    "\na\na\n\nb\n"
func TestGNUCompatGroupPrepend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group=prepend in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\na\n\nb\n", stdout)
}

// TestGNUCompatGroupAppend — --group=append with two groups.
//
// GNU command: printf 'a\na\nb\n' | guniq --group=append
// Expected:    "a\na\n\nb\n\n"
func TestGNUCompatGroupAppend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group=append in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\nb\n\n", stdout)
}

// TestGNUCompatGroupBoth — --group=both with two groups.
//
// GNU command: printf 'a\na\nb\n' | guniq --group=both
// Expected:    "\na\na\n\nb\n\n"
func TestGNUCompatGroupBoth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group=both in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\na\n\nb\n\n", stdout)
}

// TestGNUCompatRepeatedOnly — -d only emits repeated lines.
//
// GNU command: printf 'a\na\nb\n' | guniq -d
// Expected:    "a\n"
func TestGNUCompatRepeatedOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq -d in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

// TestGNUCompatUniqueOnly — -u only emits unique lines.
//
// GNU command: printf 'a\nb\na\n' | guniq -u
// Expected:    "a\nb\na\n"
func TestGNUCompatUniqueOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\na\n")
	stdout, _, code := cmdRun(t, "uniq -u in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\na\n", stdout)
}

// TestGNUCompatRejectedFlag — unknown flag produces exit 1.
//
// GNU command: guniq --no-such-flag → exit 1
func TestGNUCompatRejectedFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\n")
	_, stderr, code := cmdRun(t, "uniq --no-such-flag in.txt", dir)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, stderr)
}

// TestGNUCompatSkipFields — -f 2 skips two fields.
//
// GNU command: printf 'a\ta a\na a a\n' | guniq -f 2
// Expected:    "a\ta a\n"
func TestGNUCompatSkipFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\ta a\na a a\n")
	stdout, _, code := cmdRun(t, "uniq -f 2 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\ta a\n", stdout)
}

// TestGNUCompatZeroTerminated — -z uses NUL delimiter.
//
// GNU command: printf 'a\0a\0b' | guniq -z
// Expected:    "a\0b\0"
func TestGNUCompatZeroTerminated(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\x00a\x00b")
	stdout, _, code := cmdRun(t, "uniq -z in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\x00b\x00", stdout)
}
