// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

// newRotationTestTailer builds a Tailer with only the fields detectAndRecordRotationSizeMismatches
// touches, so the size-mismatch logic can be exercised without any file I/O.
func newRotationTestTailer(cachedSize int64) *Tailer {
	return &Tailer{
		cachedFileSize:               atomic.NewInt64(cachedSize),
		rotationMismatchCacheActive:  atomic.NewBool(false),
		rotationMismatchOffsetActive: atomic.NewBool(false),
	}
}

// TestDetectRotationSizeMismatches pins down which mismatch flag each (cachedSize, fileSize,
// lastReadOffset) combination sets. It covers both detection cases, their boundaries, and the
// guards that gate them (cache growth, offset-unread, cachedSize>0, offsetAdvance>cacheGrowth).
func TestDetectRotationSizeMismatches(t *testing.T) {
	tests := []struct {
		name           string
		cachedSize     int64
		fileSize       int64
		lastReadOffset int64
		wantCache      bool // rotationMismatchCacheActive after the call
		wantOffset     bool // rotationMismatchOffsetActive after the call
	}{
		{
			// Case 1: file grew 100->200, but we read to 250 (past EOF). offsetAdvance 150 >
			// cacheGrowth 100 => cache mismatch.
			name: "case1 cache mismatch", cachedSize: 100, fileSize: 200, lastReadOffset: 250,
			wantCache: true, wantOffset: false,
		},
		{
			// Case 1 boundary: offsetAdvance (100) == cacheGrowth (100). Not "advanced further", so
			// no mismatch. Guards the `offsetAdvance > cacheGrowth` boundary.
			name: "case1 advance equals growth", cachedSize: 100, fileSize: 200, lastReadOffset: 200,
			wantCache: false, wantOffset: false,
		},
		{
			// Case 2: offset (80) < fileSize (200) suggests unread data, but cache shows no growth
			// (200 -> 200). => offset mismatch.
			name: "case2 offset mismatch", cachedSize: 200, fileSize: 200, lastReadOffset: 80,
			wantCache: false, wantOffset: true,
		},
		{
			// cachedSize == 0 disables the cache-growth detector (it requires cachedSize > 0).
			// The offset is past EOF, so a `cachedSize > 0` -> `>= 0` boundary mutation would
			// wrongly enter the cache-mismatch branch; asserting no mismatch pins that boundary.
			name: "no cached size", cachedSize: 0, fileSize: 100, lastReadOffset: 150,
			wantCache: false, wantOffset: false,
		},
		{
			// Steady state: no growth, fully read. Neither flag set.
			name: "no mismatch", cachedSize: 100, fileSize: 100, lastReadOffset: 100,
			wantCache: false, wantOffset: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tl := newRotationTestTailer(tc.cachedSize)

			tl.detectAndRecordRotationSizeMismatches(tc.fileSize, tc.lastReadOffset)

			assert.Equal(t, tc.wantCache, tl.rotationMismatchCacheActive.Load(), "cache-mismatch flag")
			assert.Equal(t, tc.wantOffset, tl.rotationMismatchOffsetActive.Load(), "offset-mismatch flag")
			assert.Equal(t, tc.fileSize, tl.cachedFileSize.Load(), "cached file size is updated to the latest size")
		})
	}
}

// TestDetectRotationSizeMismatchActivatesOncePerEpisode asserts the cache-mismatch flag stays set
// across consecutive mismatching scans (CompareAndSwap latches it) and clears once the condition
// resolves.
func TestDetectRotationSizeMismatchFlagLifecycle(t *testing.T) {
	tl := newRotationTestTailer(100)

	// First mismatching scan latches the flag.
	tl.detectAndRecordRotationSizeMismatches(200, 250)
	assert.True(t, tl.rotationMismatchCacheActive.Load())

	// A subsequent scan with no mismatch clears it (cachedSize is now 200; steady read).
	tl.detectAndRecordRotationSizeMismatches(200, 200)
	assert.False(t, tl.rotationMismatchCacheActive.Load())
}
