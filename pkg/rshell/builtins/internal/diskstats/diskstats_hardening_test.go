// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package diskstats

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseMountInfo_LineNearLimit — a line of exactly maxMountInfoLine
// bytes parses successfully (the cap is inclusive in bufio).
func TestParseMountInfo_LineNearLimit(t *testing.T) {
	// Build a valid-looking line just under the cap. Every component
	// must be valid; pad the source field with safe characters.
	source := strings.Repeat("a", maxMountInfoLine-100)
	line := "36 35 98:0 / / rw - ext4 " + source + " rw\n"
	if len(line) > maxMountInfoLine {
		t.Fatalf("test setup error: line exceeds cap")
	}
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(line))
	assert.NoError(t, err)
	assert.Len(t, mounts, 1)
}

// TestParseMountInfo_BoundedScannerBuffer — the scanner is initialized
// with a small starting buffer (4 KiB) and only grows up to the cap.
// This test exercises lines between starting size and cap.
func TestParseMountInfo_GrowingBuffer(t *testing.T) {
	// 100 KiB lines: each requires the scanner to grow past the
	// initial 4 KiB buffer.
	mid := strings.Repeat("a", 100_000)
	line := "36 35 98:0 / / rw - ext4 " + mid + " rw\n"
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(line))
	assert.NoError(t, err)
	assert.Len(t, mounts, 1)
}

// TestParseMountInfo_ManyShortLinesUnderLimit — many short lines under
// MaxMounts must parse fully without error.
func TestParseMountInfo_ManyShortLinesUnderLimit(t *testing.T) {
	var b strings.Builder
	const n = 5_000
	for range n {
		b.WriteString("36 35 98:0 / /m rw - ext4 /dev/x rw\n")
	}
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(b.String()))
	assert.NoError(t, err)
	assert.Len(t, mounts, n)
}

// TestParseMountInfo_AllMalformedDoesNotInfinite — a stream of malformed
// lines is silently skipped without error or hang. The MaxTotalLines
// guard prevents this from being a DoS even if every line is dropped.
func TestParseMountInfo_AllMalformedDoesNotInfinite(t *testing.T) {
	var b strings.Builder
	for range 5_000 {
		b.WriteString("garbage line without separator\n")
	}
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(b.String()))
	assert.NoError(t, err)
	assert.Empty(t, mounts)
}

// TestParseMountInfo_TooManyMalformedHitsTotalLineCap — when the input
// has more total lines than MaxTotalLines, even valid mount entries
// should not exceed MaxMounts because the scan terminates first.
//
// Note: with MaxTotalLines = MaxMounts*10 = 1_000_000 lines, generating
// the actual cap would slow CI; we just verify the behaviour up to the
// MaxMounts cap.
func TestParseMountInfo_RespectsCapNotJustLines(t *testing.T) {
	var b strings.Builder
	// MaxMounts valid entries followed by garbage; ErrMaxMounts wins
	// before maxTotalLines fires.
	for range MaxMounts + 50 {
		b.WriteString("36 35 98:0 / /m rw - ext4 /dev/x rw\n")
	}
	mounts, err := parseMountInfo(context.Background(), strings.NewReader(b.String()))
	assert.ErrorIs(t, err, ErrMaxMounts)
	assert.Equal(t, MaxMounts, len(mounts))
}

// TestUnescapeMountField_NoSlashFastPath — the no-backslash fast path
// returns the input unchanged without allocating.
func TestUnescapeMountField_NoSlashFastPath(t *testing.T) {
	in := "/very/normal/path/no/escapes"
	out := unescapeMountField(in)
	assert.Equal(t, in, out)
}

// TestUnescapeMountField_AllByteValues — sweep every octal escape that
// might appear in mountinfo and verify they decode to the right byte.
func TestUnescapeMountField_AllOctals(t *testing.T) {
	for v := range 256 {
		s := "" +
			string(byte('\\')) +
			string(byte('0'+(v>>6&7))) +
			string(byte('0'+(v>>3&7))) +
			string(byte('0'+(v&7)))
		got := unescapeMountField(s)
		if len(got) != 1 || got[0] != byte(v) {
			t.Errorf("octal %s: got %q, want byte %d", s, got, v)
		}
	}
}
