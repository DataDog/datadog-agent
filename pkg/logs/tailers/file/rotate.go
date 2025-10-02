// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	logstypes "github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DidRotateViaFingerprint returns true if the file has been log-rotated via fingerprint.
//
// When a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotateViaFingerprint(fingerprinter *Fingerprinter) (bool, error) {
	// Step 1: Get the current fingerprint (either from buffer or from tailer)
	currentFingerprint := t.getCurrentFingerprintForComparison()
	if currentFingerprint == nil {
		// Not enough data for fingerprint comparison, fall back to filesystem check
		log.Debugf("No fingerprint available for %s, falling back to filesystem rotation check", t.file.Path)
		return t.DidRotate()
	}

	// Step 2: Compute the new fingerprint from the file on disk
	newFingerprint, err := fingerprinter.ComputeFingerprint(t.file)
	if err != nil {
		return false, err
	}

	// Step 3: Compare fingerprints
	if currentFingerprint.IsPartialFingerprint() || newFingerprint.IsPartialFingerprint() {
		return t.comparePartialFingerprints(currentFingerprint, newFingerprint)
	}
	return t.compareFullFingerprints(currentFingerprint, newFingerprint)
}

// getCurrentFingerprintForComparison returns the fingerprint to use for rotation detection
// If still buffering, returns a temporary partial fingerprint without clearing the buffer
func (t *Tailer) getCurrentFingerprintForComparison() *logstypes.Fingerprint {
	// If still accumulating data, get temporary partial fingerprint
	if t.isPartialFingerprintState != nil && t.isPartialFingerprintState.Load() {
		log.Debugf("Still accumulating fingerprint buffer for %s, getting temporary partial fingerprint", t.file.Path)
		partialFingerprint := t.getPartialFingerprintFromBuffer()
		if partialFingerprint != nil && partialFingerprint.IsValidFingerprint() {
			return partialFingerprint
		}
		return nil
	}
	return t.fingerprint
}

// comparePartialFingerprints handles rotation detection when at least one fingerprint is partial
func (t *Tailer) comparePartialFingerprints(current, new *logstypes.Fingerprint) (bool, error) {
	// Case 1: Same size - compare checksums
	if current.BytesUsed == new.BytesUsed {
		if current.Value == new.Value {
			// Same size and checksum - ambiguous, use filesystem check
			log.Debugf("Same-sized partial fingerprints match for %s (%d bytes)", t.file.Path, current.BytesUsed)
			return t.fallbackToFilesystemCheck()
		}
		// Same size, different checksum - rotation detected
		log.Debugf("Same-sized partial fingerprints differ for %s (%d bytes), assuming rotation", t.file.Path, current.BytesUsed)
		return true, nil
	}

	// Case 2: File shrunk - rotation detected
	if current.BytesUsed > new.BytesUsed {
		log.Debugf("Partial fingerprint size decreased for %s (old: %d bytes, new: %d bytes), assuming rotation",
			t.file.Path, current.BytesUsed, new.BytesUsed)
		return true, nil
	}

	// Case 3: File grew - ambiguous, use filesystem check
	log.Debugf("Partial fingerprint size increased for %s (old: %d bytes, new: %d bytes), checking filesystem",
		t.file.Path, current.BytesUsed, new.BytesUsed)
	return t.fallbackToFilesystemCheck()
}

// compareFullFingerprints handles rotation detection when both fingerprints are full
func (t *Tailer) compareFullFingerprints(current, new *logstypes.Fingerprint) (bool, error) {
	if current == nil {
		return false, nil
	}

	// Different fingerprints = rotation (includes truncation case where new fingerprint is invalid)
	rotated := !current.Equals(new)
	if rotated {
		log.Debugf("File rotation detected via fingerprint mismatch for %s (old: 0x%x, new: 0x%x)",
			t.file.Path, current.Value, new.Value)
	}
	return rotated, nil
}

// fallbackToFilesystemCheck performs filesystem-based rotation detection (inode/device check)
func (t *Tailer) fallbackToFilesystemCheck() (bool, error) {
	rotated, err := t.DidRotate()
	if err != nil {
		log.Debugf("Filesystem check failed for %s, assuming no rotation: %v", t.file.Path, err)
		return false, nil
	}
	return rotated, err
}
