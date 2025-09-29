// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DidRotateViaFingerprint returns true if the file has been log-rotated via fingerprint.
//
// When a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotateViaFingerprint(fingerprinter *Fingerprinter) (bool, error) {
	newFingerprint, err := fingerprinter.ComputeFingerprint(t.file)

	// Handle partial fingerprint comparisons
	if t.fingerprint.IsPartialFingerprint() || newFingerprint.IsPartialFingerprint() {
		if t.fingerprint.BytesUsed == newFingerprint.BytesUsed {
			// Case 1: Same size partials - if different checksums, check if file was rotated, or just modified
			if t.fingerprint.Value != newFingerprint.Value {
				// TODO can we just assume this is a rotation?
				log.Debugf("Same-sized partial fingerprints differ for %s (%d bytes), checking filesystem", t.file.Path, t.fingerprint.BytesUsed)
				rotated, err := t.DidRotate()
				if err != nil {
					log.Debugf("Filesystem check failed for %s, assuming no rotation: %v", t.file.Path, err)
					return false, nil
				}
				return rotated, err
			}
			// Same size, same checksum - likely no rotation
			log.Debugf("Same-sized partial fingerprints match for %s (%d bytes), assuming no rotation", t.file.Path, t.fingerprint.BytesUsed)
			return false, nil
		}

		// Case 2: File shrunk - likely rotation
		if t.fingerprint.BytesUsed > newFingerprint.BytesUsed {
			log.Debugf("Partial fingerprint size decreased for %s (old: %d bytes, new: %d bytes), assuming rotation",
				t.file.Path, t.fingerprint.BytesUsed, newFingerprint.BytesUsed)
			return true, nil
		}

		// Case 3: File grew
		log.Debugf("Partial fingerprint size increased for %s (old: %d bytes, new: %d bytes), checking filesystem",
			t.file.Path, t.fingerprint.BytesUsed, newFingerprint.BytesUsed)
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
