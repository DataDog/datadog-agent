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

	switch {
	case cacheIndicatesGrowth && !offsetIndicatesUnread:
		if t.rotationMismatchCacheActive.CompareAndSwap(false, true) {
			metrics.TlmRotationSizeMismatch.Inc("cache")
			log.Debugf("Rotation size mismatch: cache observed growth (old=%d, new=%d) but offset=%d >= fileSize=%d",
				cachedSize, fileSize, lastReadOffset, fileSize)
		}
		t.rotationMismatchOffsetActive.Store(false)
	case offsetIndicatesUnread && !cacheIndicatesGrowth && cachedSize > 0:
		if t.rotationMismatchOffsetActive.CompareAndSwap(false, true) {
			metrics.TlmRotationSizeMismatch.Inc("offset")
			log.Debugf("Rotation size mismatch: offset=%d < fileSize=%d but cache did not observe growth (old=%d, new=%d)",
				lastReadOffset, fileSize, cachedSize, fileSize)
		}
		t.rotationMismatchCacheActive.Store(false)
	default:
		t.rotationMismatchCacheActive.Store(false)
		t.rotationMismatchOffsetActive.Store(false)
	}

	// Track size differences when size-based rotation is detected
	if cachedSize > 0 && fileSize != cachedSize {
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

// DidRotateViaFingerprint returns true if the file has been log-rotated via fingerprint.
//
// On *nix, when a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotateViaFingerprint(fingerprinter Fingerprinter) (bool, error) {
	newFingerprint, err := fingerprinter.ComputeFingerprint(t.file)

	// If computing the fingerprint led to an error there was likely an IO issue, handle this appropriately below.
	if err != nil {
		return false, err
	}
	// If the original fingerprint is nil, we can't detect rotation
	if t.fingerprint == nil {
		return false, nil
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
