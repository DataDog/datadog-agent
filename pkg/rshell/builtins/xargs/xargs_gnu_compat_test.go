// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package xargs_test contains GNU equivalence tests asserting byte-for-byte
// output parity between this builtin and GNU xargs (debian:bookworm-slim
// container) for the cases most sensitive to formatting. Reference output
// was captured once and embedded as string literals so the tests run
// without requiring GNU xargs at runtime.
//
// To re-capture: docker run --rm debian:bookworm-slim bash -c '<command>'
package xargs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGNUCompatDefaultEcho — `xargs` with no flags joins all items with spaces.
//
// GNU command: echo 'a b c' | xargs
// Expected: "a b c\n"
func TestGNUCompatDefaultEcho(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c' | xargs", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

// TestGNUCompatExplicitInitialArgs — initial args precede the input items.
//
// GNU command: echo 'a b c' | xargs echo HEAD
// Expected: "HEAD a b c\n"
func TestGNUCompatExplicitInitialArgs(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c' | xargs echo HEAD", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "HEAD a b c\n", stdout)
}

// TestGNUCompatMaxArgsBatching — -n N produces ⌈items/N⌉ invocations.
//
// GNU command: echo 'a b c d e' | xargs -n 2 echo
// Expected: "a b\nc d\ne\n"
func TestGNUCompatMaxArgsBatching(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c d e' | xargs -n 2 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\nc d\ne\n", stdout)
}

// TestGNUCompatMaxLinesBatching — -L N batches by input line.
//
// GNU command: printf 'a b\nc d\n' | xargs -L 1 echo
// Expected: "a b\nc d\n"
func TestGNUCompatMaxLinesBatching(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a b\\nc d\\n' | xargs -L 1 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\nc d\n", stdout)
}

// TestGNUCompatReplaceWholeLine — -I treats each line as a single item.
//
// GNU command: echo 'a b c' | xargs -I {} echo 'item: {}'
// Expected: "item: a b c\n"
func TestGNUCompatReplaceWholeLine(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c' | xargs -I {} echo 'item: {}'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "item: a b c\n", stdout)
}

// TestGNUCompatReplacePerLine — -I one invocation per non-blank line.
//
// GNU command: printf 'a\nb\nc\n' | xargs -I {} echo 'item: {}'
// Expected: "item: a\nitem: b\nitem: c\n"
func TestGNUCompatReplacePerLine(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\nb\\nc\\n' | xargs -I {} echo 'item: {}'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "item: a\nitem: b\nitem: c\n", stdout)
}

// TestGNUCompatNullSeparator — -0 splits on NUL only; whitespace stays literal.
//
// GNU command: printf 'a b\0c\0d\0' | xargs -0 echo
// Expected: "a b c d\n"
func TestGNUCompatNullSeparator(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a b\\0c\\0d\\0' | xargs -0 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c d\n", stdout)
}

// TestGNUCompatCustomDelimiter — -d C uses C as the only separator.
//
// GNU command: printf 'a,b,c' | xargs -d , echo
// Expected: "a b c\n"
func TestGNUCompatCustomDelimiter(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a,b,c' | xargs -d , echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

// TestGNUCompatEofMarker — -E STOP terminates input at the first lone STOP.
//
// GNU command: echo 'a b STOP c d' | xargs -E STOP echo
// Expected: "a b\n"
func TestGNUCompatEofMarker(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b STOP c d' | xargs -E STOP echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
}

// TestGNUCompatEmptyRunsOnce — with no -r, command runs once with no extra args.
//
// GNU command: printf ” | xargs echo done
// Expected: "done\n"
func TestGNUCompatEmptyRunsOnce(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf '' | xargs echo done", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "done\n", stdout)
}

// TestGNUCompatEmptyWithNoRunIfEmpty — -r suppresses the empty-input invocation.
//
// GNU command: printf ” | xargs -r echo done
// Expected: ""
func TestGNUCompatEmptyWithNoRunIfEmpty(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf '' | xargs -r echo done", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatVerbose — -t writes the resolved command line to stderr.
//
// GNU command: echo 'a b' | xargs -t echo
// Expected stdout: "a b\n"
// Expected stderr: "echo a b\n"
func TestGNUCompatVerbose(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "echo 'a b' | xargs -t echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
	assert.Equal(t, "echo a b\n", stderr)
}

// TestGNUCompatBackslashEscape — backslash escapes the next byte.
//
// GNU command: printf 'a\\ b c' | xargs echo
// Expected: "a b c\n"
//
// Without -0/-d, "\<space>" yields a single space inside the item, so the
// raw input `a\ b c` produces two items: "a b" and "c".
func TestGNUCompatBackslashEscape(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\\\ b c' | xargs echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

// TestGNUCompatSingleQuotedItem — single quotes group bytes into one item.
//
// GNU command: echo "'hello world' bye" | xargs echo
// Expected: "hello world bye\n"
func TestGNUCompatSingleQuotedItem(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `echo "'hello world' bye" | xargs echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world bye\n", stdout)
}

// TestGNUCompatRejectedFlag — unknown long flags exit 1 and write stderr.
//
// GNU command: xargs --not-a-flag (exit ≠ 0)
// Expected: exit 1, stderr non-empty.
func TestGNUCompatRejectedFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "xargs --not-a-flag", dir)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, stderr)
}
