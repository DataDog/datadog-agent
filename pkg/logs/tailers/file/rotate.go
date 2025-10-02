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
	// If we're still accumulating data for a fingerprint, get a temporary partial fingerprint for comparison
	// without clearing the buffer (we want to keep trying to reach full fingerprint size)
	var currentFingerprint *logstypes.Fingerprint
	if t.isPartialFingerprintState != nil && t.isPartialFingerprintState.Load() {
		log.Debugf("Still accumulating fingerprint buffer for %s, getting temporary partial fingerprint", t.file.Path)
		currentFingerprint = t.getPartialFingerprintFromBuffer()
		// If we don't have enough data yet, fall back to filesystem check
		if currentFingerprint == nil || !currentFingerprint.IsValidFingerprint() {
			log.Debugf("Not enough data for partial fingerprint for %s, falling back to filesystem rotation check", t.file.Path)
			return t.DidRotate()
		}
		// Use the temporary partial fingerprint for comparison (don't update t.fingerprint yet)
	} else {
		currentFingerprint = t.fingerprint
	}

	newFingerprint, err := fingerprinter.ComputeFingerprint(t.file)

	// Handle partial fingerprint comparisons
	if currentFingerprint != nil && (currentFingerprint.IsPartialFingerprint() || newFingerprint.IsPartialFingerprint()) {
		if currentFingerprint.BytesUsed == newFingerprint.BytesUsed {
			// Case 1: Same size partials
			if currentFingerprint.Value == newFingerprint.Value {
				// Same partial checksums, so check if the file was rotated
				log.Debugf("Same-sized partial fingerprints match for %s (%d bytes)", t.file.Path, currentFingerprint.BytesUsed)
				rotated, err := t.DidRotate()
				if err != nil {
					log.Debugf("Filesystem check failed for %s, assuming no rotation: %v", t.file.Path, err)
					return false, nil
				}
				return rotated, err
			}
			// Same size, different partial checksums - likely rotation
			log.Debugf("Same-sized partial fingerprints differ for %s (%d bytes), assuming rotation", t.file.Path, currentFingerprint.BytesUsed)
			return true, nil
		}

		// Case 2: File shrunk - likely rotation
		if currentFingerprint.BytesUsed > newFingerprint.BytesUsed {
			log.Debugf("Partial fingerprint size decreased for %s (old: %d bytes, new: %d bytes), assuming rotation",
				t.file.Path, currentFingerprint.BytesUsed, newFingerprint.BytesUsed)
			return true, nil
		}

		// Case 3: File grew
		log.Debugf("Partial fingerprint size increased for %s (old: %d bytes, new: %d bytes), checking filesystem",
			t.file.Path, currentFingerprint.BytesUsed, newFingerprint.BytesUsed)
		rotated, err := t.DidRotate()
		if err != nil {
			log.Debugf("Filesystem check failed for %s, assuming no rotation: %v", t.file.Path, err)
			return false, nil
		}
		return rotated, err
	}

	// If computing the fingerprint led to an error there was likely an IO issue, handle this appropriately below.
	if err != nil {
		return false, err
	}
	// If the original fingerprint is nil, we can't detect rotation
	if currentFingerprint == nil {
		return false, nil
	}

	// If fingerprints are different, it means the file was rotated.
	// This is also true if the new fingerprint is invalid (Value=0), which means the file was truncated.
	rotated := !currentFingerprint.Equals(newFingerprint)

	if rotated {
		log.Debugf("File rotation detected via fingerprint mismatch for %s (old: 0x%x, new: 0x%x)",
			t.file.Path, currentFingerprint.Value, newFingerprint.Value)
	}
	return rotated, nil
}
