// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package truncate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTruncateHardeningRejectedFlags verifies that every flag outside the
// approved set is rejected with exit 1 and a non-empty error message.
func TestTruncateHardeningRejectedFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "data")
	for _, flag := range []string{
		"--reference=f.txt",
		"-r", "-r f.txt",
		"--io-blocks",
		"-o",
		"--no-such-flag",
		"-z",
	} {
		_, stderr, code := truncateRun(t, "truncate "+flag+" f.txt", dir)
		assert.Equal(t, 1, code, "flag should be rejected: %s", flag)
		assert.NotEmpty(t, stderr, "flag: %s", flag)
	}
}

// TestTruncateHardeningRelativeSizeForms verifies that all relative-size
// modifier prefixes are rejected with a user-facing hint.
// Note: < and > must be quoted because the shell parses them as redirects.
func TestTruncateHardeningRelativeSizeForms(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "data")
	// +, -, /, % can be passed unquoted; < and > must be quoted.
	for _, prefix := range []string{"+", "-", "/", "%"} {
		_, stderr, code := truncateRun(t, `truncate -s `+prefix+`5 f.txt`, dir)
		assert.Equal(t, 1, code, "prefix %q should be rejected", prefix)
		assert.Contains(t, stderr, "relative size operators not supported", "prefix: %s", prefix)
	}
	for _, prefix := range []string{"<", ">"} {
		_, stderr, code := truncateRun(t, `truncate -s '`+prefix+`5' f.txt`, dir)
		assert.Equal(t, 1, code, "prefix %q should be rejected", prefix)
		assert.Contains(t, stderr, "relative size operators not supported", "prefix: %s", prefix)
	}
}

// TestTruncateHardeningInvalidSuffixes verifies that unrecognised size
// suffixes (including the Z/Y/R/Q zetta+ forms) are rejected.
func TestTruncateHardeningInvalidSuffixes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "data")
	for _, bad := range []string{"1Z", "1Y", "1R", "1Q", "1p", "1e", "1x", "1b", "1w"} {
		_, stderr, code := truncateRun(t, "truncate -s "+bad+" f.txt", dir)
		assert.Equal(t, 1, code, "suffix should be rejected: %s", bad)
		assert.Contains(t, stderr, "invalid size", "suffix: %s", bad)
	}
}

// TestTruncateHardeningOverflow verifies that an astronomically large size
// (one that would overflow int64) is rejected before any syscall is issued.
func TestTruncateHardeningOverflow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "data")
	for _, big := range []string{
		"9999999999999999999", // > int64 max
		"9223372036854775808", // int64 max + 1
		"9E",                  // 9 * 2^60 > int64 max
	} {
		_, stderr, code := truncateRun(t, "truncate -s "+big+" f.txt", dir)
		assert.Equal(t, 1, code, "size should overflow: %s", big)
		assert.Contains(t, stderr, "invalid size", "size: %s", big)
	}
}

// TestTruncateHardeningEmptySize verifies that -s with an empty value is
// rejected cleanly.
func TestTruncateHardeningEmptySize(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "data")
	_, stderr, code := truncateRun(t, `truncate -s "" f.txt`, dir)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, stderr)
}
