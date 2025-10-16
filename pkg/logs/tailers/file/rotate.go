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
	// compute the new fingerprint
	newFingerprint, err := fingerprinter.ComputeFingerprint(t.file)

	// If computing the fingerprint led to an error there was likely an IO issue, handle this appropriately below.
	if err != nil {
		return false, err
	}

	if t.fingerprint == nil {
		log.Debugf("No baseline fingerprint for %s; falling back to filesystem rotation check", t.file.Path)
		return t.DidRotate()
	}

	oldValid := t.fingerprint.ValidFingerprint()
	newValid := newFingerprint.ValidFingerprint()

	if !newValid {
		if oldValid {
			log.Debugf("File rotation detected for %s: new fingerprint invalid while previous fingerprint was valid", t.file.Path)
			return true, nil
		}
		log.Debugf("Falling back to filesystem rotation check for %s: both fingerprints invalid", t.file.Path)
		return t.DidRotate()
	}

	if !oldValid {
		log.Debugf("File rotation detected for %s: previous fingerprint invalid, new fingerprint set", t.file.Path)
		return true, nil
	}

	// If fingerprints are different, it means the file was rotated.
	rotated := !t.fingerprint.Equals(newFingerprint)
	if rotated {
		log.Debugf("File rotation detected via fingerprint mismatch for %s (old: 0x%x, new: 0x%x)",
			t.file.Path, t.fingerprint.Value, newFingerprint.Value)
	}
	return rotated, nil
}
