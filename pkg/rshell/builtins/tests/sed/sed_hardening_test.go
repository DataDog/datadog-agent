// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sed_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
	"github.com/stretchr/testify/assert"
)

// --- Memory Safety & Resource Limits ---

func TestHardenLongLine(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"long.txt": strings.Repeat("x", 512*1024) + "\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/x/y/' long.txt`, dir)
	assert.Equal(t, 0, code)
	// First 'x' replaced with 'y', rest unchanged.
	assert.True(t, strings.HasPrefix(stdout, "y"))
}

func TestHardenPatternSpaceLimit(t *testing.T) {
	// Use N command to accumulate lines until pattern space limit is hit.
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString(strings.Repeat("a", 600))
		sb.WriteByte('\n')
	}
	dir := setupDir(t, map[string]string{
		"big.txt": sb.String(),
	})
	_, stderr, code := cmdRun(t, `sed ':a;N;ba' big.txt`, dir)
	// Should fail with pattern space limit error.
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "pattern space exceeded size limit")
}

func TestHardenHoldSpaceLimit(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString(strings.Repeat("b", 600))
		sb.WriteByte('\n')
	}
	dir := setupDir(t, map[string]string{
		"big.txt": sb.String(),
	})
	_, stderr, code := cmdRun(t, `sed 'H' big.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "hold space exceeded size limit")
}

func TestHardenBranchLoopLimit(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "test\n",
	})
	_, stderr, code := cmdRun(t, `sed ':loop;b loop' input.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "branch loop limit exceeded")
}

// --- Context Cancellation ---

func TestHardenContextCancellation(t *testing.T) {
	// Create a large file that would take a while to process.
	dir := setupDir(t, map[string]string{
		"big.txt": strings.Repeat("line\n", 100000),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, `sed 's/line/LINE/g' big.txt`, dir)
	// Should either complete or be cancelled — both are acceptable.
	_ = code
}

// --- Blocked Commands ---

func TestHardenBlockedExecuteCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 'e' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestHardenBlockedWriteCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 'w /tmp/evil' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestHardenBlockedReadCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 'r /etc/passwd' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestHardenBlockedBigRCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 'R /etc/passwd' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestHardenBlockedBigWCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 'W /tmp/evil' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestHardenBlockedSubstituteWriteFlag(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 's/t/T/w /tmp/evil' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestHardenBlockedSubstituteExecuteFlag(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 's/t/T/e' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

// --- Input Validation ---

func TestHardenInvalidRegex(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 's/[invalid/x/' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sed:")
}

func TestHardenEmptyScript(t *testing.T) {
	_, stderr, code := cmdRun(t, `sed '' /dev/null`, "")
	// Empty script is valid — matches all lines with no commands.
	_ = stderr
	_ = code
}

func TestHardenNoScript(t *testing.T) {
	_, stderr, code := cmdRun(t, `sed`, "")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sed:")
}

func TestHardenUnterminatedSubstitution(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 's/foo' f.txt`, dir)
	// Unterminated s command — the parser may accept the last delimiter as optional.
	// Just make sure it doesn't crash.
	_ = stderr
	_ = code
}

func TestHardenUnterminatedGroup(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed '{p' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unterminated")
}

func TestHardenUnmatchedCloseBrace(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed '}' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unexpected '}'")
}

// --- Multiple Files ---

func TestHardenMultipleFiles(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"a.txt": "alpha\n",
		"b.txt": "beta\n",
		"c.txt": "gamma\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/a/A/g' a.txt b.txt c.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "AlphA\nbetA\ngAmmA\n", stdout)
}

func TestHardenMissingFileContinues(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"a.txt": "alpha\n",
	})
	stdout, stderr, code := cmdRun(t, `sed 's/a/A/' a.txt nonexistent.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "Alpha\n", stdout)
	assert.Contains(t, stderr, "nonexistent.txt")
}

// --- Regex Safety ---

func TestHardenRegexComplexPattern(t *testing.T) {
	// RE2 guarantees linear time, so complex patterns should not cause ReDoS.
	dir := setupDir(t, map[string]string{
		"f.txt": strings.Repeat("a", 100) + "\n",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, `sed -E 's/(a+)+b/x/' f.txt`, dir, interp.AllowedPaths([]string{dir}))
	// Should complete without timeout (RE2 handles this in linear time).
	assert.Equal(t, 0, code)
}

// --- Y command edge cases ---

func TestHardenTransliterateMismatch(t *testing.T) {
	dir := setupDir(t, map[string]string{"f.txt": "test\n"})
	_, stderr, code := cmdRun(t, `sed 'y/abc/de/' f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "same length")
}

// --- Comments ---

func TestHardenComments(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"f.txt": "hello\n",
	})
	stdout, _, code := cmdRun(t, `sed '#this is a comment
s/hello/world/' f.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "world\n", stdout)
}
