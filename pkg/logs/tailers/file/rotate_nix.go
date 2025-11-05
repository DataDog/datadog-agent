// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/opener"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DidRotate returns true if the file has been log-rotated.
//
// On *nix, when a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotate() (bool, error) {
	f, err := opener.OpenLogFile(t.fullpath)
	if err != nil {
		return false, fmt.Errorf("open %q: %w", t.fullpath, err)
	}
	defer f.Close()
	lastReadOffset := t.lastReadOffset.Load()

	fi1, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat %q: %w", f.Name(), err)
	}

	fi2, err := t.osFile.Stat()
	if err != nil {
		return true, nil
	}

	fileSize := fi1.Size()
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

	recreated := !os.SameFile(fi1, fi2)
	truncated := fileSize < lastReadOffset

	if recreated {
		log.Debugf("File rotation detected due to recreation, f1: %+v, f2: %+v", fi1, fi2)
		metrics.TlmRotationsNix.Inc("new_file")
	} else if truncated {
		log.Debugf("File rotation detected due to size change, lastReadOffset=%d, fileSize=%d", lastReadOffset, fileSize)
		metrics.TlmRotationsNix.Inc("truncated")
	}

	return recreated || truncated, nil
}
