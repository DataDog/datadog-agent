// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// detectAndRecordRotationSizeMismatches checks for size mismatches that indicate potential
// missed rotations and records telemetry metrics. It updates the cached file size.
func (t *Tailer) detectAndRecordRotationSizeMismatches(fileSize, lastReadOffset int64) {
	cachedSize := t.cachedFileSize.Load()

	cacheIndicatesGrowth := cachedSize > 0 && fileSize > cachedSize
	offsetIndicatesUnread := lastReadOffset < fileSize

	recordCacheSizeDiff := false

	switch {
	// Case 1: Cache says file grew, but offset suggests we've read past the end
	// Potential missed rotation: the file likely rotated (was replaced), but we continued reading from the old position
	case cacheIndicatesGrowth && !offsetIndicatesUnread:
		// Calculate how much our read position advanced since last check
		offsetAdvance := lastReadOffset - cachedSize
		// Calculate how much the file actually grew
		cacheGrowth := fileSize - cachedSize

		// Advanced further than the file grew -- strange behavior (=> missed rotation)
		if offsetAdvance > cacheGrowth {
			// Only increment the metric once per detected mismatch (using atomic CAS)
			if t.rotationMismatchCacheActive.CompareAndSwap(false, true) {
				metrics.TlmRotationSizeMismatch.Inc("cache")
				log.Debugf("Rotation size mismatch: offset advanced %d bytes but file only grew %d bytes (cached=%d, current=%d, offset=%d)",
					offsetAdvance, cacheGrowth, cachedSize, fileSize, lastReadOffset)
				recordCacheSizeDiff = true
			}
		} else {
			t.rotationMismatchCacheActive.Store(false)
		}
		t.rotationMismatchOffsetActive.Store(false)
	// Case 2: Offset indicates unread data, but cache says file didn't grow
	// Potential false positive: the offset suggests unread data (and => rotation), but cache size doesn't show growth.
	case offsetIndicatesUnread && !cacheIndicatesGrowth && cachedSize > 0:
		// Only increment the metric once per detected mismatch
		if t.rotationMismatchOffsetActive.CompareAndSwap(false, true) {
			// Offset says "unread data" but cache says "no growth"
			metrics.TlmRotationSizeMismatch.Inc("offset")
			log.Debugf("Rotation size mismatch: offset=%d < fileSize=%d but cache did not observe growth (old=%d, new=%d)",
				lastReadOffset, fileSize, cachedSize, fileSize)
		}
		t.rotationMismatchCacheActive.Store(false)
	default:
		t.rotationMismatchCacheActive.Store(false)
		t.rotationMismatchOffsetActive.Store(false)
	}

	if recordCacheSizeDiff {
		sizeDiff := fileSize - cachedSize
		if sizeDiff < 0 {
			sizeDiff = -sizeDiff
		}
		metrics.TlmRotationSizeDifferences.Observe(float64(sizeDiff))
	}

	// Update cached file size for next check
	t.cachedFileSize.Store(fileSize)
}

// DidRotateViaFingerprint returns true if the file has been log-rotated via fingerprint.
//
// When a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotateViaFingerprint(fingerprinter Fingerprinter) (bool, error) {
	// compute the new fingerprint
	newFingerprint, err := fingerprinter.ComputeFingerprint(t.file)

	// If computing the fingerprint led to an error there was likely an IO issue, handle this appropriately below.
	if err != nil {
		return false, err
	}

	// New fingerprint is invalid: recreated/truncated file, so fall back to filesystem check
	if !newFingerprint.ValidFingerprint() {
		log.Debugf("Falling back to filesystem rotation check for %s. New fingerprint invalid", t.file.Path)
		return t.DidRotate()
	}

	// Old fingerprint invalid (not nil) but new one isn't: the file changed, assume rotation
	if !t.fingerprint.ValidFingerprint() {
		log.Debugf("File rotation detected for %s. Previous fingerprint invalid, new fingerprint valid; assuming rotation", t.file.Path)
		return true, nil
	}

	// If fingerprints are different, it means the file was rotated.
	// This is also true if the new fingerprint is invalid (Value=0), which means the file was truncated.
	rotated := !t.fingerprint.Equals(newFingerprint)
	if rotated {
		log.Debugf("File rotation detected via fingerprint mismatch for %s (old: 0x%x, new: 0x%x)",
			t.file.Path, t.fingerprint.Value, newFingerprint.Value)
	}
	return rotated, nil
}
